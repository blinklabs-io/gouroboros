// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package leios

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

const (
	leiosSignatureDST = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
	leiosProofDST     = "BLS_POP_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
)

var (
	// ErrMalformedCommittee reports invalid committee or signer data.
	ErrMalformedCommittee = errors.New("malformed Leios committee")
	// ErrInvalidProof reports a committee key with an invalid possession proof.
	ErrInvalidProof = errors.New("invalid Leios proof of possession")
	// ErrKeylessSigner reports a certificate selecting a seat without a key.
	ErrKeylessSigner = errors.New(
		"leios certificate selects a keyless seat",
	)
	// ErrInsufficientQuorum reports signer stake below the configured threshold.
	ErrInsufficientQuorum = errors.New(
		"leios certificate does not meet stake quorum",
	)
	// ErrInvalidSignature reports a bad certificate aggregate signature.
	ErrInvalidSignature = errors.New("invalid Leios aggregate signature")
)

var negG2Generator = func() bls12381.G2Affine {
	_, _, _, generator := bls12381.Generators()
	var negated bls12381.G2Affine
	negated.Neg(&generator)
	return negated
}()

// VerifyDijkstraCertificate validates a certificate whose committee keys were
// registered under the IETF proof-of-possession domain. See
// VerifyDijkstraCertificateForDomain.
func VerifyDijkstraCertificate(
	signers, signature []byte,
	committeeSize uint16,
	quorumStakeThreshold *big.Rat,
	context common.DijkstraLeiosCertificateContext,
) error {
	return VerifyDijkstraCertificateForDomain(
		signers,
		signature,
		committeeSize,
		quorumStakeThreshold,
		context,
		common.DijkstraBlsProofDomainPop,
	)
}

// VerifyDijkstraCertificateForDomain validates committee state, signer stake,
// key possession proofs, and the aggregate signature in a Dijkstra
// certificate. Committee seat keys are pool keys, so their proofs are checked
// under the same domain the network's POOL rule used at registration.
func VerifyDijkstraCertificateForDomain(
	signers, signature []byte,
	committeeSize uint16,
	quorumStakeThreshold *big.Rat,
	context common.DijkstraLeiosCertificateContext,
	domain common.DijkstraBlsProofDomain,
) error {
	return VerifyDijkstraCertificateForMessage(
		signers,
		signature,
		committeeSize,
		quorumStakeThreshold,
		context,
		domain,
		common.DijkstraLeiosCertificateMessageRawHash,
	)
}

// VerifyDijkstraCertificateForMessage is VerifyDijkstraCertificateForDomain
// with an explicit choice of signed message. The aggregate signature is
// checked over exactly the chosen encoding of the announcing block hash; the
// other encoding is never tried as a fallback.
func VerifyDijkstraCertificateForMessage(
	signers, signature []byte,
	committeeSize uint16,
	quorumStakeThreshold *big.Rat,
	context common.DijkstraLeiosCertificateContext,
	domain common.DijkstraBlsProofDomain,
	signedMessage common.DijkstraLeiosCertificateMessage,
) error {
	proofDST, err := proofDomainDST(domain)
	if err != nil {
		return err
	}
	message, err := certificateMessage(
		signedMessage,
		context.AnnouncingBlockHash,
	)
	if err != nil {
		return err
	}
	if uint64(len(context.Committee)) != uint64(committeeSize) {
		return fmt.Errorf(
			"%w: got %d seats, want %d",
			ErrMalformedCommittee,
			len(context.Committee),
			committeeSize,
		)
	}
	if context.TotalActiveStake == 0 {
		return fmt.Errorf(
			"%w: total active stake is zero",
			ErrMalformedCommittee,
		)
	}
	threshold := quorumStakeThreshold
	if threshold == nil || threshold.Sign() < 0 ||
		threshold.Cmp(big.NewRat(1, 1)) > 0 {
		return fmt.Errorf(
			"%w: invalid quorum stake threshold",
			ErrMalformedCommittee,
		)
	}
	if err := common.ValidateLeiosSignerBitfield(
		signers,
		uint64(committeeSize),
	); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedCommittee, err)
	}

	// Summed in Jacobian coordinates: an affine addition inverts a field
	// element, which for a large committee costs more than the pairing.
	var aggregateJac bls12381.G2Jac
	var signerStake uint64
	var committeeStake uint64
	var signerCount int
	for i, seat := range context.Committee {
		if committeeStake > context.TotalActiveStake ||
			seat.Stake > context.TotalActiveStake-committeeStake {
			return fmt.Errorf(
				"%w: committee stake exceeds total active stake",
				ErrMalformedCommittee,
			)
		}
		committeeStake += seat.Stake
		if !common.LeiosSignerBit(signers, uint64(i)) {
			continue
		}
		if seat.Key == nil {
			return fmt.Errorf("%w: seat %d", ErrKeylessSigner, i)
		}
		pub, stage, err := verifiedPossessionKey(seat.Key, proofDST)
		if err != nil {
			if stage == keyCheckDecode {
				return fmt.Errorf(
					"%w: seat %d public key: %w",
					ErrMalformedCommittee,
					i,
					err,
				)
			}
			return fmt.Errorf("%w: seat %d: %w", ErrInvalidProof, i, err)
		}
		if signerStake > context.TotalActiveStake-seat.Stake {
			return fmt.Errorf(
				"%w: signer stake exceeds total active stake",
				ErrMalformedCommittee,
			)
		}
		signerStake += seat.Stake
		signerCount++
		if signerCount == 1 {
			aggregateJac.FromAffine(pub)
		} else {
			aggregateJac.AddMixed(pub)
		}
	}
	if signerCount == 0 {
		return fmt.Errorf("%w: no signers", ErrInsufficientQuorum)
	}
	left := new(big.Int).Mul(
		new(big.Int).SetUint64(signerStake),
		threshold.Denom(),
	)
	right := new(big.Int).Mul(
		new(big.Int).SetUint64(context.TotalActiveStake),
		threshold.Num(),
	)
	if left.Cmp(right) < 0 {
		return fmt.Errorf(
			"%w: signer stake %d, total active stake %d, threshold %s",
			ErrInsufficientQuorum,
			signerStake,
			context.TotalActiveStake,
			threshold.RatString(),
		)
	}
	var aggregatePublicKey bls12381.G2Affine
	aggregatePublicKey.FromJacobian(&aggregateJac)
	if aggregatePublicKey.IsInfinity() {
		return fmt.Errorf(
			"%w: aggregate public key is infinity",
			ErrMalformedCommittee,
		)
	}
	if err := verifySignature(
		&aggregatePublicKey,
		message,
		signature,
		leiosSignatureDST,
	); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}
	return nil
}

// VerifyPossessionProof checks that a Leios BLS key's proof of possession is
// a valid minimal-signature-size signature, by the key itself, over the key's
// compressed encoding under the proof-of-possession DST.
func VerifyPossessionProof(key common.LeiosKey) error {
	return VerifyPossessionProofForDomain(
		key,
		common.DijkstraBlsProofDomainPop,
	)
}

// VerifyPossessionProofForDomain verifies a pool key under exactly the chosen
// proof domain. The legacy signature domain is for networks whose reference
// ledger requires it; it is never tried as a fallback for an invalid proof.
func VerifyPossessionProofForDomain(
	key common.LeiosKey,
	domain common.DijkstraBlsProofDomain,
) error {
	dst, err := proofDomainDST(domain)
	if err != nil {
		return err
	}
	_, stage, err := verifiedPossessionKey(&key, dst)
	if err != nil && stage == keyCheckDecode {
		return fmt.Errorf("%w: public key: %w", ErrInvalidProof, err)
	}
	if err != nil {
		if errors.Is(err, ErrInvalidSignature) {
			// The single-key pairing check failed; the aggregate-signature
			// sentinel's text would misdescribe it.
			return fmt.Errorf("%w: pairing check failed", ErrInvalidProof)
		}
		return fmt.Errorf("%w: %w", ErrInvalidProof, err)
	}
	return nil
}

func certificateMessage(
	signedMessage common.DijkstraLeiosCertificateMessage,
	hash common.Blake2b256,
) ([]byte, error) {
	switch signedMessage {
	case common.DijkstraLeiosCertificateMessageRawHash:
		return hash[:], nil
	case common.DijkstraLeiosCertificateMessageCborHash:
		// The prototype signs the hash's CBOR encoding: a 32-byte string
		// header (0x58 0x20) followed by the hash.
		message := make([]byte, 2, 2+len(hash))
		message[0], message[1] = 0x58, 0x20
		return append(message, hash[:]...), nil
	default:
		return nil, common.NewValidationError(
			common.ValidationErrorTypeConfiguration,
			"unsupported Dijkstra Leios certificate message",
			map[string]any{"message": signedMessage},
			nil,
		)
	}
}

func proofDomainDST(domain common.DijkstraBlsProofDomain) (string, error) {
	switch domain {
	case common.DijkstraBlsProofDomainPop:
		return leiosProofDST, nil
	case common.DijkstraBlsProofDomainLegacySig:
		return leiosSignatureDST, nil
	default:
		return "", common.NewValidationError(
			common.ValidationErrorTypeConfiguration,
			"unsupported Dijkstra BLS proof domain",
			map[string]any{"domain": domain},
			nil,
		)
	}
}

func decodePublicKey(encoded []byte) (*bls12381.G2Affine, error) {
	if len(encoded) != common.LeiosBlsPublicKeySize {
		return nil, fmt.Errorf(
			"public key length is %d, want %d",
			len(encoded),
			common.LeiosBlsPublicKeySize,
		)
	}
	var pub bls12381.G2Affine
	if _, err := pub.SetBytes(encoded); err != nil {
		return nil, err
	}
	if pub.IsInfinity() || !pub.IsInSubGroup() {
		return nil, errors.New(
			"public key is infinity or outside the G2 subgroup",
		)
	}
	return &pub, nil
}

func verifySignature(
	pub *bls12381.G2Affine,
	message, signature []byte,
	dst string,
) error {
	if len(signature) != common.LeiosBlsSignatureSize {
		return fmt.Errorf(
			"signature length is %d, want %d",
			len(signature),
			common.LeiosBlsSignatureSize,
		)
	}
	var sig bls12381.G1Affine
	if _, err := sig.SetBytes(signature); err != nil {
		return err
	}
	if sig.IsInfinity() || !sig.IsInSubGroup() {
		return errors.New("signature is infinity or outside the G1 subgroup")
	}
	hash, err := bls12381.HashToG1(message, []byte(dst))
	if err != nil {
		return fmt.Errorf("hash message to G1: %w", err)
	}
	ok, err := bls12381.PairingCheck(
		[]bls12381.G1Affine{sig, hash},
		[]bls12381.G2Affine{negG2Generator, *pub},
	)
	if err != nil {
		return fmt.Errorf("pairing check: %w", err)
	}
	if !ok {
		return ErrInvalidSignature
	}
	return nil
}
