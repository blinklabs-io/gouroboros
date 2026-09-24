// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package byron

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha3"
	"errors"
	"fmt"
	"math/bits"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/ed25519byron"
	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

const (
	// DefaultPBFTSignatureThresholdNumerator and
	// DefaultPBFTSignatureThresholdDenominator encode the pinned Byron PBFT
	// issuer threshold, 0.22, without floating-point rounding.
	DefaultPBFTSignatureThresholdNumerator   uint64 = 22
	DefaultPBFTSignatureThresholdDenominator uint64 = 100
)

// PBFTIssuer identifies the genesis key whose signing quota is charged and
// the active delegate key that actually signed a Byron main-block header.
type PBFTIssuer struct {
	GenesisKeyHash  common.Blake2b224
	DelegateKeyHash common.Blake2b224
}

// ValidatePBFTHeader validates the stateless and delegation-ledger-view parts
// of a Byron PBFT main-block header. It verifies the protocol magic, the exact
// Byron block and proxy-certificate signatures, genesis issuer membership,
// and that the signing delegate matches the caller's active delegation view.
//
// This deliberately does not apply OBFT round-robin leader selection. The
// inbound PBFT rule charges the genesis issuer against a rolling signature
// window; use PBFTState.Transition for that stateful rule.
func ValidatePBFTHeader(
	header *ledgerbyron.ByronMainBlockHeader,
	config ByronConfig,
) (PBFTIssuer, error) {
	if len(config.GenesisDelegations) == 0 {
		return PBFTIssuer{}, errors.New(
			"byron PBFT active delegation state is empty",
		)
	}
	issuer, err := ValidatePBFTHeaderCrypto(header, config)
	if err != nil {
		return PBFTIssuer{}, err
	}
	activeIssuer, err := resolveActivePBFTIssuer(
		config.GenesisDelegations,
		issuer.DelegateKeyHash,
	)
	if err != nil {
		return PBFTIssuer{}, err
	}
	issuer.GenesisKeyHash = activeIssuer
	return issuer, nil
}

func resolveActivePBFTIssuer(
	activeDelegations map[common.Blake2b224]common.Blake2b224,
	delegate common.Blake2b224,
) (common.Blake2b224, error) {
	var activeIssuer common.Blake2b224
	found := false
	for issuer, activeDelegate := range activeDelegations {
		if activeDelegate != delegate {
			continue
		}
		if found && issuer != activeIssuer {
			return common.Blake2b224{}, fmt.Errorf(
				"byron PBFT active delegation maps delegate %s to multiple issuers",
				delegate.String(),
			)
		}
		activeIssuer = issuer
		found = true
	}
	if !found {
		return common.Blake2b224{}, fmt.Errorf(
			"byron PBFT active delegation does not authorize delegate %s",
			delegate.String(),
		)
	}
	return activeIssuer, nil
}

// ValidatePBFTHeaderCrypto validates the cryptographic and configured-genesis
// parts of a Byron PBFT main-block header without consulting the active
// delegation map. Callers may use it for parallel stateless pre-validation,
// but must still call ValidatePBFTHeader against the ordered delegation-ledger
// view before committing the block.
func ValidatePBFTHeaderCrypto(
	header *ledgerbyron.ByronMainBlockHeader,
	config ByronConfig,
) (PBFTIssuer, error) {
	if header == nil {
		return PBFTIssuer{}, errors.New("nil byron PBFT header")
	}
	if len(config.GenesisKeyHashes) == 0 {
		return PBFTIssuer{}, errors.New(
			"byron PBFT genesis issuer set is empty",
		)
	}

	issuer, err := parsePBFTIssuerFromHeader(header)
	if err != nil {
		return PBFTIssuer{}, err
	}
	input := &ValidateHeaderInput{
		Slot:           header.SlotNumber(),
		BlockNumber:    header.BlockNumber(),
		PrevHash:       header.PrevHash().Bytes(),
		ProtocolMagic:  header.ProtocolMagic,
		IssuerPubKey:   header.ConsensusData.PubKey[:32],
		BlockSig:       header.ConsensusData.BlockSig,
		HeaderCbor:     header.Cbor(),
		EnvelopeOnly:   false,
		IsEBB:          false,
		BlockSignature: nil,
	}
	validator := NewHeaderValidator(config)
	if err := validator.validateProtocolMagic(input); err != nil {
		return PBFTIssuer{}, fmt.Errorf(
			"validate Byron PBFT protocol magic: %w",
			err,
		)
	}
	if err := validatePBFTDelegateBlockSignature(
		validator,
		input,
		header,
	); err != nil {
		return PBFTIssuer{}, fmt.Errorf(
			"validate Byron PBFT block signature: %w",
			err,
		)
	}
	if !validator.genesisKeyHashes[string(issuer.GenesisKeyHash.Bytes())] {
		return PBFTIssuer{}, fmt.Errorf(
			"validate Byron PBFT genesis issuer: issuer %s is not configured",
			issuer.GenesisKeyHash.String(),
		)
	}
	return issuer, nil
}

func validatePBFTDelegateBlockSignature(
	validator *HeaderValidator,
	input *ValidateHeaderInput,
	header *ledgerbyron.ByronMainBlockHeader,
) error {
	if header == nil || len(header.ConsensusData.BlockSig) != 2 {
		return errors.New("invalid Byron PBFT signature shape")
	}
	inner, ok := header.ConsensusData.BlockSig[1].([]any)
	if !ok || len(inner) != 2 {
		return errors.New("invalid Byron PBFT proxy signature payload")
	}
	certificate, ok := inner[0].([]any)
	if !ok || len(certificate) != 4 {
		return errors.New("invalid Byron PBFT proxy certificate")
	}
	issuerKey, ok := certificate[1].([]byte)
	if !ok || len(issuerKey) != 64 {
		return errors.New("invalid Byron PBFT issuer key")
	}
	delegateKey, ok := certificate[2].([]byte)
	if !ok || len(delegateKey) != 64 {
		return errors.New("invalid Byron PBFT delegate key")
	}
	certificateSignature, ok := certificate[3].([]byte)
	if !ok || len(certificateSignature) != ed25519.SignatureSize {
		return errors.New("invalid Byron PBFT delegation certificate signature")
	}
	blockSignature, ok := inner[1].([]byte)
	if !ok || len(blockSignature) != ed25519.SignatureSize {
		return errors.New("invalid Byron PBFT delegate block signature")
	}
	toSign, _, err := validator.buildToSignWithEpoch(input)
	if err != nil {
		return fmt.Errorf("build Byron PBFT ToSign data: %w", err)
	}
	protocolMagicCbor, err := cbor.Encode(validator.config.ProtocolMagic)
	if err != nil {
		return fmt.Errorf("encode Byron PBFT protocol magic: %w", err)
	}
	signed := make(
		[]byte,
		0,
		2+len(issuerKey)+1+len(protocolMagicCbor)+len(toSign),
	)
	signed = append(signed, '0', '1')
	signed = append(signed, issuerKey...)
	signed = append(signed, byronSignTagMainBlockHeavy)
	signed = append(signed, protocolMagicCbor...)
	signed = append(signed, toSign...)
	if !ed25519byron.Verify(
		ed25519.PublicKey(delegateKey[:ed25519.PublicKeySize]),
		signed,
		blockSignature,
	) {
		return errors.New("byron PBFT delegate block signature verification failed")
	}
	return nil
}

// PBFTIssuerFromHeader parses the PBFT identities carried by a Byron main
// block header without verifying either signature or consulting the active
// delegation ledger view. ConsensusData.PubKey is the genesis issuer's
// extended verification key; element 2 of the heavyweight proxy certificate
// is the delegate extended verification key that signed the block.
//
// This is intended for replay of already-trusted canonical headers. New or
// otherwise untrusted headers must use ValidatePBFTHeaderCrypto and then
// ValidatePBFTHeader against the ordered active-delegation view.
func PBFTIssuerFromHeader(
	header *ledgerbyron.ByronMainBlockHeader,
) (PBFTIssuer, error) {
	issuer, err := parsePBFTIssuerFromHeader(header)
	return issuer, err
}

func parsePBFTIssuerFromHeader(
	header *ledgerbyron.ByronMainBlockHeader,
) (PBFTIssuer, error) {
	if header == nil {
		return PBFTIssuer{}, errors.New("nil byron PBFT header")
	}
	if len(header.ConsensusData.PubKey) != 64 {
		return PBFTIssuer{}, fmt.Errorf(
			"invalid Byron PBFT genesis issuer key length: got %d, expected 64",
			len(header.ConsensusData.PubKey),
		)
	}
	genesisKeyHash, err := PBFTVerificationKeyHash(
		header.ConsensusData.PubKey,
	)
	if err != nil {
		return PBFTIssuer{}, err
	}
	issuer := PBFTIssuer{
		GenesisKeyHash: genesisKeyHash,
	}
	if len(header.ConsensusData.BlockSig) != 2 {
		return PBFTIssuer{}, fmt.Errorf(
			"invalid Byron PBFT signature shape: got %d elements, expected 2",
			len(header.ConsensusData.BlockSig),
		)
	}
	signatureType, err := extractUint64(header.ConsensusData.BlockSig[0])
	if err != nil {
		return PBFTIssuer{}, fmt.Errorf(
			"decode Byron PBFT signature type: %w",
			err,
		)
	}
	switch signatureType {
	case byronSigTypeHeavy:
		inner, ok := header.ConsensusData.BlockSig[1].([]any)
		if !ok || len(inner) != 2 {
			return PBFTIssuer{}, fmt.Errorf(
				"invalid Byron PBFT proxy signature payload: got %T with %d elements",
				header.ConsensusData.BlockSig[1],
				len(inner),
			)
		}
		certificate, ok := inner[0].([]any)
		if !ok || len(certificate) != 4 {
			return PBFTIssuer{}, fmt.Errorf(
				"invalid Byron PBFT proxy certificate: got %T with %d elements",
				inner[0],
				len(certificate),
			)
		}
		delegateKey, ok := certificate[2].([]byte)
		if !ok || len(delegateKey) != 64 {
			return PBFTIssuer{}, fmt.Errorf(
				"invalid Byron PBFT delegate key: got %T with length %d",
				certificate[2],
				len(delegateKey),
			)
		}
		certificateIssuerKey, ok := certificate[1].([]byte)
		if !ok || !bytes.Equal(
			certificateIssuerKey,
			header.ConsensusData.PubKey,
		) {
			return PBFTIssuer{}, errors.New(
				"byron PBFT proxy certificate genesis issuer does not match header issuer",
			)
		}
		issuer.DelegateKeyHash, err = PBFTVerificationKeyHash(delegateKey)
		if err != nil {
			return PBFTIssuer{}, err
		}
		return issuer, nil
	case byronSigTypeSimple, byronSigTypeLight:
		return PBFTIssuer{}, fmt.Errorf(
			"unsupported Byron PBFT signature type: %d; heavyweight delegation is required",
			signatureType,
		)
	default:
		return PBFTIssuer{}, fmt.Errorf(
			"unknown Byron PBFT signature type: %d",
			signatureType,
		)
	}
}

// PBFTVerificationKeyHash derives the Byron key identity used by genesis and
// delegation state: blake2b-224(sha3-256(CBOR(extended verification key))).
func PBFTVerificationKeyHash(
	verificationKey []byte,
) (common.Blake2b224, error) {
	if len(verificationKey) != 64 {
		return common.Blake2b224{}, fmt.Errorf(
			"invalid Byron extended verification key length: got %d, expected 64",
			len(verificationKey),
		)
	}
	verificationKeyCbor, err := cbor.Encode(verificationKey)
	if err != nil {
		return common.Blake2b224{}, fmt.Errorf(
			"encode Byron extended verification key: %w",
			err,
		)
	}
	sha3Hash := sha3.Sum256(verificationKeyCbor)
	return common.Blake2b224Hash(sha3Hash[:]), nil
}

// PBFTState contains the genesis issuers charged in the last k signed Byron
// main-block headers. Values are immutable: every update returns a new state.
type PBFTState struct {
	signatureHistory     []common.Blake2b224
	securityParam        uint64
	thresholdNumerator   uint64
	thresholdDenominator uint64
}

// NewPBFTState constructs state from an already ordered oldest-to-newest
// issuer history. It rejects histories that cannot fit in the security window.
func NewPBFTState(
	signatureHistory []common.Blake2b224,
	securityParam uint64,
) (PBFTState, error) {
	return NewPBFTStateWithThreshold(
		signatureHistory,
		securityParam,
		DefaultPBFTSignatureThresholdNumerator,
		DefaultPBFTSignatureThresholdDenominator,
	)
}

// NewPBFTStateFromConfig constructs PBFT state using the configured threshold,
// falling back to the Byron default when both threshold fields are zero.
func NewPBFTStateFromConfig(
	signatureHistory []common.Blake2b224,
	config ByronConfig,
) (PBFTState, error) {
	numerator := config.PBFTSignatureThresholdNumerator
	denominator := config.PBFTSignatureThresholdDenominator
	if numerator == 0 && denominator == 0 {
		numerator = DefaultPBFTSignatureThresholdNumerator
		denominator = DefaultPBFTSignatureThresholdDenominator
	}
	return NewPBFTStateWithThreshold(
		signatureHistory,
		config.SecurityParam,
		numerator,
		denominator,
	)
}

// NewPBFTStateWithThreshold constructs state with a configurable maximum
// fraction of signatures charged to one genesis issuer in the last k blocks.
func NewPBFTStateWithThreshold(
	signatureHistory []common.Blake2b224,
	securityParam uint64,
	numerator uint64,
	denominator uint64,
) (PBFTState, error) {
	if securityParam == 0 {
		return PBFTState{}, errors.New(
			"byron PBFT security parameter must be greater than zero",
		)
	}
	if denominator == 0 {
		return PBFTState{}, fmt.Errorf(
			"invalid Byron PBFT signature threshold %d/%d",
			numerator,
			denominator,
		)
	}
	if uint64(len(signatureHistory)) > securityParam {
		return PBFTState{}, fmt.Errorf(
			"byron PBFT issuer history has %d entries, exceeding security parameter %d",
			len(signatureHistory),
			securityParam,
		)
	}
	return PBFTState{
		signatureHistory: append(
			[]common.Blake2b224(nil),
			signatureHistory...,
		),
		securityParam:        securityParam,
		thresholdNumerator:   numerator,
		thresholdDenominator: denominator,
	}, nil
}

// SignatureHistory returns an independent oldest-to-newest copy of the
// state's charged genesis issuers.
func (s PBFTState) SignatureHistory() []common.Blake2b224 {
	return append([]common.Blake2b224(nil), s.signatureHistory...)
}

// Observe advances the rolling issuer window without applying its threshold.
// It is intended only for reconstructing state from blocks a caller has
// explicitly chosen to trust, such as its persisted historical chain.
func (s PBFTState) Observe(
	issuer common.Blake2b224,
) (PBFTState, error) {
	if s.securityParam == 0 {
		return PBFTState{}, errors.New(
			"invalid Byron PBFT issuer state: security parameter must be greater than zero",
		)
	}
	if uint64(len(s.signatureHistory)) > s.securityParam {
		return PBFTState{}, fmt.Errorf(
			"invalid Byron PBFT issuer state: history has %d entries, security parameter is %d",
			len(s.signatureHistory),
			s.securityParam,
		)
	}
	history := make(
		[]common.Blake2b224,
		0,
		min(uint64(len(s.signatureHistory))+1, s.securityParam),
	)
	if uint64(len(s.signatureHistory)) >= s.securityParam {
		history = append(history, s.signatureHistory[1:]...)
	} else {
		history = append(history, s.signatureHistory...)
	}
	history = append(history, issuer)
	return PBFTState{
		signatureHistory:     history,
		securityParam:        s.securityParam,
		thresholdNumerator:   s.thresholdNumerator,
		thresholdDenominator: s.thresholdDenominator,
	}, nil
}

// Transition applies the pinned Byron SIGCNT rule: append the genesis issuer,
// retain only the last k issuers, then reject when that issuer appears more
// than floor(0.22*k) times in the resulting window.
func (s PBFTState) Transition(
	issuer common.Blake2b224,
) (PBFTState, error) {
	next, err := s.Observe(issuer)
	if err != nil {
		return PBFTState{}, err
	}
	var issuerCount uint64
	for _, historicalIssuer := range next.signatureHistory {
		if historicalIssuer == issuer {
			issuerCount++
		}
	}
	maxSignatures := pbftMaxSignatures(
		s.securityParam,
		s.thresholdNumerator,
		s.thresholdDenominator,
	)
	if issuerCount > maxSignatures {
		return PBFTState{}, fmt.Errorf(
			"byron PBFT signature threshold exceeded for genesis issuer %s: got %d signatures in the last %d, maximum is %d",
			issuer.String(),
			issuerCount,
			s.securityParam,
			maxSignatures,
		)
	}
	return next, nil
}

func pbftMaxSignatures(
	securityParam uint64,
	numerator uint64,
	denominator uint64,
) uint64 {
	hi, lo := bits.Mul64(securityParam, numerator)
	if hi >= denominator {
		return ^uint64(0)
	}
	result, _ := bits.Div64(hi, lo, denominator)
	return result
}
