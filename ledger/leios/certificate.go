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
		"Leios certificate selects a keyless seat",
	)
	// ErrInsufficientQuorum reports signer stake below the configured threshold.
	ErrInsufficientQuorum = errors.New(
		"Leios certificate does not meet stake quorum",
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

// VerifyDijkstraCertificate validates committee state, signer stake, key
// possession proofs, and the aggregate signature in a Dijkstra certificate.
func VerifyDijkstraCertificate(
	signers, signature []byte,
	committeeSize uint16,
	quorumStakeThreshold *big.Rat,
	context common.DijkstraLeiosCertificateContext,
) error {
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
		return fmt.Errorf("%w: %v", ErrMalformedCommittee, err)
	}

	var aggregatePublicKey bls12381.G2Affine
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
		pub, err := decodePublicKey(seat.Key.PublicKey)
		if err != nil {
			return fmt.Errorf(
				"%w: seat %d public key: %v",
				ErrMalformedCommittee,
				i,
				err,
			)
		}
		if err := verifySignature(
			pub,
			seat.Key.PublicKey,
			seat.Key.PossessionProof,
			leiosProofDST,
		); err != nil {
			return fmt.Errorf("%w: seat %d: %v", ErrInvalidProof, i, err)
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
			aggregatePublicKey = *pub
		} else {
			aggregatePublicKey.Add(&aggregatePublicKey, pub)
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
	if aggregatePublicKey.IsInfinity() {
		return fmt.Errorf(
			"%w: aggregate public key is infinity",
			ErrMalformedCommittee,
		)
	}
	message := make([]byte, 2, 2+len(context.AnnouncingBlockHash))
	message[0], message[1] = 0x58, 0x20 // CBOR header for a 32-byte hash.
	message = append(message, context.AnnouncingBlockHash[:]...)
	if err := verifySignature(
		&aggregatePublicKey,
		message,
		signature,
		leiosSignatureDST,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	return nil
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
		return nil, errors.New("public key is infinity or outside the G2 subgroup")
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
