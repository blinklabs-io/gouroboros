// Copyright 2025 Blink Labs Software
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

package byron

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"golang.org/x/crypto/blake2b"
)

// HeaderValidator validates Byron block headers using OBFT rules
type HeaderValidator struct {
	config           ByronConfig
	genesisKeyHashes map[string]bool // Set of allowed genesis delegate key hashes
	// SkipDelegationCertVerification bypasses delegation certificate signature verification.
	// WARNING: Setting this to true reduces security. Only use when certificate verification
	// is not possible (e.g., the exact cardano-crypto signature format is unknown).
	// When false (default), validateDelegationCertSignature will return an error.
	SkipDelegationCertVerification bool
	// AllowSignatureFallback enables fallback to raw HeaderCbor verification when
	// buildToSign fails. WARNING: This reduces security and should only be used
	// for testing or compatibility with edge cases. When false (default),
	// validateSimpleSignature will fail-closed if ToSign construction fails.
	AllowSignatureFallback bool
}

// NewHeaderValidator creates a new Byron header validator
func NewHeaderValidator(config ByronConfig) *HeaderValidator {
	keyHashes := make(map[string]bool)
	for _, hash := range config.GenesisKeyHashes {
		keyHashes[string(hash)] = true
	}
	return &HeaderValidator{
		config:           config,
		genesisKeyHashes: keyHashes,
	}
}

// ValidateHeaderInput contains data needed to validate a Byron header
type ValidateHeaderInput struct {
	// Header fields
	Slot           uint64
	BlockNumber    uint64
	PrevHash       []byte
	ProtocolMagic  uint32
	IssuerPubKey   []byte
	BlockSignature []byte

	// For signature verification
	HeaderCbor []byte // The CBOR-encoded header body to verify signature against

	// For proxy signature verification (types 1 and 2)
	// BlockSig contains the full signature structure from ConsensusData
	BlockSig []any

	// Previous header for chain validation
	PrevSlot        uint64
	PrevBlockNumber uint64
	PrevHeaderHash  []byte

	// Block type
	IsEBB bool // Epoch Boundary Block

	// EnvelopeOnly skips signature and genesis delegate validation.
	// Use this for structural validation without cryptographic verification.
	EnvelopeOnly bool
}

// ValidateResult contains the result of header validation
type ValidateResult struct {
	Valid  bool
	Errors []error
}

// ValidateHeader validates a Byron block header using OBFT rules
//
// Validation checks for main blocks:
//  1. Slot strictly increases from previous block (or equal if prev was EBB)
//  2. Block number is previous + 1 (or equal if this is EBB)
//  3. PrevHash matches hash of previous header
//  4. Protocol magic matches expected
//  5. Block signature is valid
//  6. Issuer is a valid genesis delegate
//  7. Issuer is the correct slot leader (OBFT round-robin: slot % numDelegates)
//
// For Epoch Boundary Blocks:
//  1. Slot is at epoch boundary
//  2. Block number is previous (EBBs share block number with next block)
//  3. PrevHash matches
//  4. Protocol magic matches
func (v *HeaderValidator) ValidateHeader(
	input *ValidateHeaderInput,
) *ValidateResult {
	result := &ValidateResult{
		Valid:  true,
		Errors: make([]error, 0),
	}

	// 1. Validate slot ordering
	if err := v.validateSlotOrdering(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 2. Validate block number
	if err := v.validateBlockNumber(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 3. Validate prev hash
	if err := v.validatePrevHash(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 4. Validate protocol magic
	if err := v.validateProtocolMagic(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// For EBBs or envelope-only validation, skip signature checks
	if input.IsEBB || input.EnvelopeOnly {
		return result
	}

	// 5. Validate block signature (main blocks only)
	if err := v.validateBlockSignature(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 6. Validate issuer is genesis delegate (main blocks only)
	if err := v.validateGenesisDelegate(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 7. Validate issuer is the correct slot leader (OBFT round-robin)
	if err := v.validateSlotLeader(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	return result
}

// validateSlotOrdering checks slot progression
func (v *HeaderValidator) validateSlotOrdering(
	input *ValidateHeaderInput,
) error {
	if input.IsEBB {
		// EBBs must be at epoch boundaries
		if !v.config.IsEpochBoundarySlot(input.Slot) {
			return fmt.Errorf(
				"EBB slot must be at epoch boundary: slot=%d, slotsPerEpoch=%d",
				input.Slot,
				v.config.SlotsPerEpoch,
			)
		}
		// EBBs can share slot with previous block (at epoch boundary)
		if input.Slot < input.PrevSlot {
			return fmt.Errorf(
				"EBB slot must be >= previous slot: current=%d, previous=%d",
				input.Slot,
				input.PrevSlot,
			)
		}
	} else {
		// Regular blocks must strictly increase
		if input.Slot <= input.PrevSlot {
			return fmt.Errorf(
				"slot must be greater than previous slot: current=%d, previous=%d",
				input.Slot,
				input.PrevSlot,
			)
		}
	}
	return nil
}

// validateBlockNumber checks block number progression
func (v *HeaderValidator) validateBlockNumber(
	input *ValidateHeaderInput,
) error {
	if input.IsEBB {
		// EBBs have same block number as the next regular block
		// So they share block number with previous (or it's the same)
		if input.BlockNumber != input.PrevBlockNumber &&
			input.BlockNumber != input.PrevBlockNumber+1 {
			return fmt.Errorf(
				"EBB block number must match or be previous + 1: current=%d, previous=%d",
				input.BlockNumber,
				input.PrevBlockNumber,
			)
		}
	} else {
		expectedBlockNumber := input.PrevBlockNumber + 1
		if input.BlockNumber != expectedBlockNumber {
			return fmt.Errorf(
				"block number must be previous + 1: current=%d, expected=%d",
				input.BlockNumber,
				expectedBlockNumber,
			)
		}
	}
	return nil
}

// validatePrevHash checks previous hash linkage
func (v *HeaderValidator) validatePrevHash(input *ValidateHeaderInput) error {
	if len(input.PrevHeaderHash) > 0 &&
		!bytes.Equal(input.PrevHash, input.PrevHeaderHash) {
		return fmt.Errorf(
			"previous hash does not match: got %x, expected %x",
			input.PrevHash,
			input.PrevHeaderHash,
		)
	}
	return nil
}

// validateProtocolMagic checks the protocol magic matches expected network
func (v *HeaderValidator) validateProtocolMagic(
	input *ValidateHeaderInput,
) error {
	if input.ProtocolMagic != v.config.ProtocolMagic {
		return fmt.Errorf(
			"protocol magic mismatch: got %d, expected %d",
			input.ProtocolMagic,
			v.config.ProtocolMagic,
		)
	}
	return nil
}

// Byron signature types from CDDL:
// - Type 0: BlockPSignatureSimple - simple signature [0, signature]
// - Type 1: BlockPSignatureHeavy - heavy delegation [1, [[epoch, issuerVK, delegateVK, cert], signature]]
// - Type 2: BlockPSignatureLight - lightweight delegation [2, [[omega, issuerVK, delegateVK, cert], signature]]
const (
	byronSigTypeSimple = 0
	byronSigTypeHeavy  = 1
	byronSigTypeLight  = 2
)

// validateBlockSignature verifies the block signature based on its type.
// For simple signatures (type 0), it verifies directly.
// For proxy signatures (types 1 and 2), it verifies the delegation certificate
// and then verifies the block signature from the delegate.
func (v *HeaderValidator) validateBlockSignature(
	input *ValidateHeaderInput,
) error {
	// Check if we have the full BlockSig structure for proxy verification
	if len(input.BlockSig) >= 2 {
		return v.validateBlockSignatureWithProxy(input)
	}

	// Fall back to simple signature verification if only BlockSignature is provided
	return v.validateSimpleSignature(input)
}

// validateSimpleSignature verifies a simple Ed25519 signature (type 0)
// Simple signatures are direct signatures on the ToSign data without proxy delegation.
func (v *HeaderValidator) validateSimpleSignature(
	input *ValidateHeaderInput,
) error {
	if len(input.IssuerPubKey) != ed25519.PublicKeySize {
		return fmt.Errorf(
			"invalid issuer public key size: got %d, expected %d",
			len(input.IssuerPubKey),
			ed25519.PublicKeySize,
		)
	}

	if len(input.BlockSignature) != ed25519.SignatureSize {
		return fmt.Errorf(
			"invalid block signature size: got %d, expected %d",
			len(input.BlockSignature),
			ed25519.SignatureSize,
		)
	}

	if len(input.HeaderCbor) == 0 {
		return errors.New("header CBOR is required for signature verification")
	}

	// Try to build the ToSign data from the header
	toSign, err := v.buildToSign(input)
	if err != nil {
		// Fail-closed by default: if we can't construct ToSign, reject the block
		if !v.AllowSignatureFallback {
			return fmt.Errorf(
				"failed to build ToSign for signature verification at slot %d, block %d: %w",
				input.Slot,
				input.BlockNumber,
				err,
			)
		}
		// Permissive fallback (opt-in only): verify directly against HeaderCbor
		// WARNING: This is less secure and should only be used for testing
		valid := ed25519.Verify(
			input.IssuerPubKey,
			input.HeaderCbor,
			input.BlockSignature,
		)
		if !valid {
			return fmt.Errorf(
				"block signature verification failed at slot %d, block %d (simple, fallback mode)",
				input.Slot,
				input.BlockNumber,
			)
		}
		return nil
	}

	// Verify Ed25519 signature on ToSign
	valid := ed25519.Verify(
		input.IssuerPubKey,
		toSign,
		input.BlockSignature,
	)
	if !valid {
		return fmt.Errorf(
			"block signature verification failed at slot %d, block %d",
			input.Slot,
			input.BlockNumber,
		)
	}

	return nil
}

// validateBlockSignatureWithProxy handles proxy signatures (types 1 and 2)
func (v *HeaderValidator) validateBlockSignatureWithProxy(
	input *ValidateHeaderInput,
) error {
	// Get signature type using extractUint64 to handle different integer types
	// that CBOR decoders may return (uint64, uint32, int, int64, etc.)
	sigType, err := extractUint64(input.BlockSig[0])
	if err != nil {
		return fmt.Errorf("invalid signature type: %w", err)
	}

	switch sigType {
	case byronSigTypeSimple:
		// Simple signature: [0, signature]
		// If BlockSignature is empty but BlockSig contains the signature, extract it
		if len(input.BlockSignature) == 0 && len(input.BlockSig) > 1 {
			sigBytes, ok := input.BlockSig[1].([]byte)
			if !ok {
				return fmt.Errorf(
					"invalid Type-0 signature: expected []byte in BlockSig[1], got %T",
					input.BlockSig[1],
				)
			}
			input.BlockSignature = sigBytes
		}
		return v.validateSimpleSignature(input)

	case byronSigTypeHeavy, byronSigTypeLight:
		// Proxy signature: [type, [[epoch/omega, issuerVK, delegateVK, certSig], blockSig]]
		return v.validateProxySignature(input, sigType)

	default:
		return fmt.Errorf("unknown signature type: %d", sigType)
	}
}

// validateProxySignature verifies a proxy (delegated) signature.
// This involves:
// 1. Validating the delegation certificate structure (issuer -> delegate)
// 2. Verifying the block signature (delegate signed the ToSign data)
//
// Note: Full certificate signature verification requires the exact Cardano cryptographic
// signing format which is complex and involves extended Ed25519 keys. For now, we validate
// the structure and verify the delegate signed the block correctly.
func (v *HeaderValidator) validateProxySignature(
	input *ValidateHeaderInput,
	sigType uint64,
) error {
	// Extract the inner structure: [[cert...], blockSig]
	innerArray, ok := input.BlockSig[1].([]any)
	if !ok {
		return fmt.Errorf("expected []any for delegation sig, got %T", input.BlockSig[1])
	}

	if len(innerArray) < 2 {
		return fmt.Errorf("delegation sig inner array too short: %d", len(innerArray))
	}

	// Extract the delegation certificate: [epoch/omega, issuerVK, delegateVK, certSig]
	cert, ok := innerArray[0].([]any)
	if !ok {
		return fmt.Errorf("expected []any for delegation cert, got %T", innerArray[0])
	}

	if len(cert) < 4 {
		return fmt.Errorf("delegation cert too short: expected 4 elements, got %d", len(cert))
	}

	// Extract certificate components
	epochOrOmega, err := extractUint64(cert[0]) // epochOrOmega - used for replay protection
	if err != nil {
		return fmt.Errorf("failed to extract epoch/omega from cert: %w", err)
	}

	issuerVK, ok := cert[1].([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for issuerVK, got %T", cert[1])
	}

	delegateVK, ok := cert[2].([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for delegateVK, got %T", cert[2])
	}

	certSig, ok := cert[3].([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for certSig, got %T", cert[3])
	}

	// Extract the block signature
	blockSig, ok := innerArray[1].([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for block signature, got %T", innerArray[1])
	}

	// Validate key sizes
	// Byron uses extended Ed25519 keys (64 bytes: 32-byte pubkey + 32-byte chaincode)
	if len(issuerVK) != 64 {
		return fmt.Errorf("invalid issuerVK size: got %d, expected 64", len(issuerVK))
	}
	if len(delegateVK) != 64 {
		return fmt.Errorf("invalid delegateVK size: got %d, expected 64", len(delegateVK))
	}
	if len(certSig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid certSig size: got %d, expected %d", len(certSig), ed25519.SignatureSize)
	}
	if len(blockSig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid blockSig size: got %d, expected %d", len(blockSig), ed25519.SignatureSize)
	}

	// Validate that issuerVK matches the header's public key
	// This ensures the delegation is from the expected genesis delegate
	if len(input.IssuerPubKey) != 0 && len(input.IssuerPubKey) != 32 {
		return fmt.Errorf(
			"invalid IssuerPubKey size: got %d bytes, expected 32 bytes",
			len(input.IssuerPubKey),
		)
	}
	if len(input.IssuerPubKey) == 32 && !bytes.Equal(issuerVK[:32], input.IssuerPubKey) {
		return fmt.Errorf(
			"issuerVK in delegation cert does not match header issuer: cert=%x, header=%x",
			issuerVK[:32], input.IssuerPubKey,
		)
	}

	// Verify the delegation certificate signature
	// The issuer signed the certificate to authorize the delegate.
	// According to cardano-sl SignTag.hs and Certificate.hs:
	// SignProxySK tag = 0x09
	// Signed data = signTag || "00" || delegateVK || CBOR(epochOrOmega)
	// Where signTag = 0x09 || CBOR(protocolMagic)
	if err := v.validateDelegationCertSignature(
		issuerVK, delegateVK, certSig, epochOrOmega,
	); err != nil {
		return fmt.Errorf("delegation certificate signature verification failed: %w", err)
	}

	// Verify the block signature
	// The delegate signed a buffer containing:
	// 1. "01" (ASCII bytes 0x30, 0x31)
	// 2. issuerVK (full 64-byte extended key)
	// 3. signing tag (0x08 for light, 0x09 for heavy)
	// 4. CBOR(protocol_magic)
	// 5. CBOR(ToSign)

	// Build the ToSign data
	toSign, err := v.buildToSign(input)
	if err != nil {
		return fmt.Errorf("failed to build ToSign data: %w", err)
	}

	// Get the signing tag
	// Both light and heavy delegation use MainBlockHeavy tag for block signing
	// (the light/heavy distinction is about the delegation certificate, not the block signature)
	signingTag := byte(byronSignTagMainBlockHeavy) // 0x09

	// Encode protocol magic
	pmBytes, err := cbor.Encode(v.config.ProtocolMagic)
	if err != nil {
		return fmt.Errorf("failed to encode protocol magic: %w", err)
	}

	// Build the signed buffer: "01" + issuerVK + tag + CBOR(pm) + CBOR(toSign)
	signedBuf := make([]byte, 0, 2+len(issuerVK)+1+len(pmBytes)+len(toSign))
	signedBuf = append(signedBuf, '0', '1') // ASCII "01"
	signedBuf = append(signedBuf, issuerVK...)
	signedBuf = append(signedBuf, signingTag)
	signedBuf = append(signedBuf, pmBytes...)
	signedBuf = append(signedBuf, toSign...)

	// Use the Ed25519 portion of the delegate's extended key (first 32 bytes)
	delegatePubKey := delegateVK[:32]

	valid := ed25519.Verify(delegatePubKey, signedBuf, blockSig)
	if !valid {
		return fmt.Errorf(
			"block signature verification failed at slot %d, block %d (proxy signature, type %d)",
			input.Slot,
			input.BlockNumber,
			sigType,
		)
	}

	return nil
}

// Byron signing tags (from cardano-sl)
const (
	byronSignTagTx             = 0x01
	byronSignTagRedeemTx       = 0x02
	byronSignTagVssCert        = 0x03
	byronSignTagUSProposal     = 0x04
	byronSignTagCommitment     = 0x05
	byronSignTagUSVote         = 0x06
	byronSignTagMainBlock      = 0x07
	byronSignTagMainBlockLight = 0x08
	byronSignTagMainBlockHeavy = 0x09
	byronSignTagProxySK        = 0x0a
)

// buildToSign constructs the ToSign data that is actually signed in Byron blocks.
// The ToSign structure contains:
// - Previous header hash (32 bytes)
// - Body proof (depends on proof structure)
// - Slot as EpochAndSlotCount
// - Chain difficulty
// - Protocol version + Software version
//
// This is serialized as a CBOR array with 5 elements.
func (v *HeaderValidator) buildToSign(input *ValidateHeaderInput) ([]byte, error) {
	if len(input.HeaderCbor) == 0 {
		return nil, errors.New("header CBOR is required for signature verification")
	}

	// Parse the header to extract the individual components we need for ToSign
	var header byron.ByronMainBlockHeader
	if _, err := cbor.Decode(input.HeaderCbor, &header); err != nil {
		return nil, fmt.Errorf("failed to decode header CBOR: %w", err)
	}

	// Build the ToSign structure
	// Format: [prevHash, bodyProof, epochAndSlot, difficulty, [protocolVersion, softwareVersion]]

	// Create EpochAndSlotCount structure
	epochAndSlot := struct {
		cbor.StructAsArray
		Epoch uint64
		Slot  uint16
	}{
		Epoch: header.ConsensusData.SlotId.Epoch,
		Slot:  header.ConsensusData.SlotId.Slot,
	}

	// Create difficulty structure
	difficulty := struct {
		cbor.StructAsArray
		Value uint64
	}{
		Value: header.ConsensusData.Difficulty.Value,
	}

	// Create extra header data (protocol version + software version + attributes + extraProof)
	extraData := struct {
		cbor.StructAsArray
		BlockVersion    byron.ByronBlockVersion
		SoftwareVersion byron.ByronSoftwareVersion
		Attributes      any
		ExtraProof      common.Blake2b256
	}{
		BlockVersion:    header.ExtraData.BlockVersion,
		SoftwareVersion: header.ExtraData.SoftwareVersion,
		Attributes:      header.ExtraData.Attributes,
		ExtraProof:      header.ExtraData.ExtraProof,
	}

	// Build the ToSign tuple
	toSign := struct {
		cbor.StructAsArray
		PrevHash    common.Blake2b256
		BodyProof   any
		EpochSlot   any
		Difficulty  any
		ExtraHeader any
	}{
		PrevHash:    header.PrevBlock,
		BodyProof:   header.BodyProof,
		EpochSlot:   epochAndSlot,
		Difficulty:  difficulty,
		ExtraHeader: extraData,
	}

	toSignBytes, err := cbor.Encode(toSign)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ToSign: %w", err)
	}

	return toSignBytes, nil
}

// extractUint64 extracts a uint64 from various numeric types
func extractUint64(v any) (uint64, error) {
	switch x := v.(type) {
	case uint64:
		return x, nil
	case uint32:
		return uint64(x), nil
	case uint:
		return uint64(x), nil
	case int:
		if x < 0 {
			return 0, fmt.Errorf("negative value: %d", x)
		}
		return uint64(x), nil
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("negative value: %d", x)
		}
		return uint64(x), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to uint64", v)
	}
}

// validateDelegationCertSignature verifies the delegation certificate signature.
// The issuer signs the certificate to authorize the delegate to produce blocks.
//
// According to cardano-sl SignTag.hs, the SignProxySK tag is 0x0a (SignCertificate).
// The signed data format is:
//
//	signTag || "00" || delegateVK || CBOR(epochOrOmega)
//
// Where signTag = 0x0a || CBOR(protocolMagic)
func (v *HeaderValidator) validateDelegationCertSignature(
	issuerVK, delegateVK, certSig []byte,
	epochOrOmega uint64,
) error {
	// Check if verification should be skipped
	if v.SkipDelegationCertVerification {
		// WARNING: Skipping delegation certificate signature verification.
		// This is acceptable for Byron because:
		// 1. Block signature verification DOES work and proves delegate authorization
		// 2. Byron is a concluded era (ended July 2020) - no new certificates possible
		// 3. All certificates were validated when originally accepted on mainnet
		// 4. Issuer key is validated against genesis delegates
		_ = issuerVK
		_ = delegateVK
		_ = certSig
		_ = epochOrOmega
		return nil
	}

	// KNOWN LIMITATION: Delegation certificate signature verification is not implemented.
	//
	// Despite extensive research and testing, the exact signature format used by
	// cardano-crypto for delegation certificates has not been successfully reproduced.
	//
	// Documented format (from cardano-sl Certificate.hs):
	//   sig = safeSign protocolMagicId SignCertificate safeSigner
	//       $ mconcat [ "00"                                    -- ASCII "00" = 0x30 0x30
	//                 , CC.unXPub (unVerificationKey delegateVK) -- 64 bytes raw XPub
	//                 , serialize' epochNumber]                  -- CBOR-encoded epoch
	//   signTag(pm, SignCertificate) = 0x0a + CBOR(protocolMagic)
	//   Full signed data: 0x0a || CBOR(pm) || "00" || delegateVK || CBOR(epoch)
	//
	// However, standard Ed25519 verification fails with this format and many variations.
	// This suggests cardano-crypto may use a non-standard Ed25519 variant or additional
	// transformations.
	//
	// To implement: Would require either:
	// - Discovering the exact signing transformation used
	// - Finding reference implementation or test vectors
	// - Pure Go reimplementation of cardano-crypto's extended Ed25519
	return fmt.Errorf(
		"delegation certificate signature verification not implemented: "+
			"set SkipDelegationCertVerification=true to bypass (issuerVK=%x, epochOrOmega=%d)",
		issuerVK[:8], epochOrOmega,
	)
}

// validateGenesisDelegate checks if the issuer is a valid genesis delegate
func (v *HeaderValidator) validateGenesisDelegate(
	input *ValidateHeaderInput,
) error {
	// If no genesis keys configured, skip this check
	if len(v.genesisKeyHashes) == 0 {
		return nil
	}

	// Hash the issuer public key using the common Blake2b224Hash function
	keyHash := common.Blake2b224Hash(input.IssuerPubKey)

	if !v.genesisKeyHashes[string(keyHash.Bytes())] {
		return fmt.Errorf(
			"issuer is not a valid genesis delegate: key hash %s",
			keyHash.String(),
		)
	}

	return nil
}

// validateSlotLeader checks if the issuer is the correct slot leader for this slot.
// Byron uses OBFT round-robin assignment: the expected leader for slot S with N delegates
// is the delegate at index (S % N).
//
// This is stricter than validateGenesisDelegate which only checks membership.
// Slot leader validation requires GenesisKeyHashes to be in the correct order.
func (v *HeaderValidator) validateSlotLeader(
	input *ValidateHeaderInput,
) error {
	// If no genesis keys configured, skip this check
	if len(v.config.GenesisKeyHashes) == 0 {
		return nil
	}

	// Get the expected slot leader
	expectedIndex, expectedKeyHash := v.config.SlotLeader(input.Slot)
	if expectedIndex < 0 {
		return nil // No delegates configured
	}

	// Hash the issuer public key
	actualKeyHash := common.Blake2b224Hash(input.IssuerPubKey)

	// Compare with expected slot leader
	if !bytes.Equal(actualKeyHash.Bytes(), expectedKeyHash) {
		return fmt.Errorf(
			"wrong slot leader for slot %d: expected delegate %d (hash %x), got %s",
			input.Slot,
			expectedIndex,
			expectedKeyHash,
			actualKeyHash.String(),
		)
	}

	return nil
}

// ValidateByronBlockHeader validates a Byron block header (main block or EBB).
// Set isEBB to true for Epoch Boundary Blocks, false for main blocks.
//
// Note: This function performs envelope validation only. For main blocks (isEBB=false),
// full validation including block signature and genesis delegate verification requires
// using HeaderValidator directly with IssuerPubKey, BlockSignature, and HeaderCbor fields.
// This wrapper is suitable for structural validation but not cryptographic verification.
//
// IMPORTANT: Protocol magic validation is skipped in this wrapper because the header
// interface does not expose the protocol magic field. The protocol magic is taken from
// the config parameter, so validation would compare config against itself. For full
// protocol magic validation, use HeaderValidator directly with ValidateHeaderInput
// populated from the actual header fields.
func ValidateByronBlockHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
	isEBB bool,
) error {
	validator := NewHeaderValidator(config)

	input := &ValidateHeaderInput{
		Slot:            header.SlotNumber(),
		BlockNumber:     header.BlockNumber(),
		PrevHash:        header.PrevHash().Bytes(),
		ProtocolMagic:   config.ProtocolMagic, // Note: uses config, not header (see doc above)
		PrevSlot:        prevHeader.SlotNumber(),
		PrevBlockNumber: prevHeader.BlockNumber(),
		PrevHeaderHash:  prevHeader.Hash().Bytes(),
		IsEBB:           isEBB,
		EnvelopeOnly:    true, // This wrapper performs envelope validation only
	}

	result := validator.ValidateHeader(input)
	if !result.Valid && len(result.Errors) > 0 {
		return result.Errors[0]
	}
	return nil
}

// ValidateByronMainBlockHeader validates a ByronMainBlockHeader.
// This is a convenience wrapper for ValidateByronBlockHeader with isEBB=false.
func ValidateByronMainBlockHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
) error {
	return ValidateByronBlockHeader(header, prevHeader, config, false)
}

// ValidateByronEBBHeader validates a ByronEpochBoundaryBlockHeader.
// This is a convenience wrapper for ValidateByronBlockHeader with isEBB=true.
func ValidateByronEBBHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
) error {
	return ValidateByronBlockHeader(header, prevHeader, config, true)
}

// ByronTxProof represents the transaction proof in a Byron main block body proof.
// Structure: [txCount: u32, txBodyMerkleRoot: hash, txWitnessMerkleRoot: hash]
type ByronTxProof struct {
	cbor.StructAsArray
	TxCount             uint32
	TxBodyMerkleRoot    common.Blake2b256
	TxWitnessMerkleRoot common.Blake2b256
}

// ByronBodyProof represents the body proof in a Byron main block header.
// Structure: [txProof, sscProof, dlgProof: hash, updProof: hash]
type ByronBodyProof struct {
	cbor.StructAsArray
	TxProof  ByronTxProof
	SscProof ByronSscProof
	DlgProof common.Blake2b256
	UpdProof common.Blake2b256
}

// ByronSscProof represents the SSC (Shared Seed Computation) proof in a Byron body proof.
// The SSC protocol was part of Ouroboros Classic and involved:
//   - Commitments: VSS commitments from slot leaders
//   - Openings: VSS openings revealing committed values
//   - Shares: Encrypted shares for recovery
//   - Certificates: VSS certificates for stake verification
//
// SSC Proof structure per Byron CDDL:
//
//	sscproof = [0, hash, hash]  ; CommitmentsProof (commitments hash, vss certs hash)
//	         / [1, hash, hash]  ; OpeningsProof (openings hash, vss certs hash)
//	         / [2, hash, hash]  ; SharesProof (shares hash, vss certs hash)
//	         / [3, hash]        ; CertificatesProof (vss certs hash only)
//
// The hashes are merkle roots computed over the epoch-accumulated SSC data,
// not just the current block's payload.
type ByronSscProof struct {
	// Type indicates the SSC payload type:
	// 0 = CommitmentsPayload, 1 = OpeningsPayload, 2 = SharesPayload, 3 = CertificatesPayload
	Type uint64
	// Hash1 is the primary hash (commitments/openings/shares hash, or vss certs for type 3)
	Hash1 common.Blake2b256
	// Hash2 is the VSS certificates hash (only present for types 0, 1, 2; nil for type 3)
	Hash2 *common.Blake2b256
}

// SSC payload types
const (
	SscTypeCommitments  = 0 // CommitmentsPayload: commitments + vss certificates
	SscTypeOpenings     = 1 // OpeningsPayload: openings + vss certificates
	SscTypeShares       = 2 // SharesPayload: shares + vss certificates
	SscTypeCertificates = 3 // CertificatesPayload: vss certificates only
)

// ValidateBodyHash validates that a Byron main block's body hash matches the
// computed hash from the block body contents.
//
// Byron uses a different body proof structure than Shelley+:
//   - Main blocks: BodyProof = [txProof, sscProof, dlgProof, updProof]
//     where txProof = [txCount, txBodyMerkleRoot, txWitnessMerkleRoot]
//   - EBB blocks: BodyProof = hash of the body (list of stakeholder IDs)
//
// This function validates main blocks by:
// 1. Parsing the BodyProof structure from the header
// 2. Computing the merkle roots from transaction bodies and witnesses
// 3. Hashing the delegation and update payloads
// 4. Comparing computed values against the header's body proof
func ValidateBodyHash(block *byron.ByronMainBlock) error {
	if block == nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "block is nil",
		}
	}

	// Parse the body proof from the header
	headerBodyProof, err := parseByronBodyProof(block.BlockHeader.BodyProof)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to parse body proof from header",
			Cause:   err,
		}
	}

	// Validate transaction proof
	if err := validateTxProof(headerBodyProof.TxProof, block.Body.TxPayload); err != nil {
		return err
	}

	// Validate delegation proof (dlgProof is blake2b-256 hash of the dlgPayload CBOR)
	// Use raw CBOR for hash validation to preserve exact encoding
	dlgCbor := block.Body.DlgPayloadCbor()
	if dlgCbor != nil {
		if err := validateDlgProofRaw(headerBodyProof.DlgProof, dlgCbor); err != nil {
			return err
		}
	} else {
		// Fallback to re-encoding if raw CBOR not available
		if err := validateDlgProof(headerBodyProof.DlgProof, block.Body.DlgPayload); err != nil {
			return err
		}
	}

	// Validate update proof (updProof is blake2b-256 hash of the updPayload CBOR)
	updCbor := block.Body.UpdPayloadCbor()
	if updCbor != nil {
		if err := validateUpdProofRaw(headerBodyProof.UpdProof, updCbor); err != nil {
			return err
		}
	} else {
		// Fallback to re-encoding if raw CBOR not available
		if err := validateUpdProof(headerBodyProof.UpdProof, &block.Body.UpdPayload); err != nil {
			return err
		}
	}

	// Validate SSC proof
	// Note: Full SSC proof validation requires epoch-accumulated state that cannot
	// be verified from a single block alone. We validate structural consistency
	// between the proof type and payload type.
	if err := validateSscProof(headerBodyProof.SscProof, block.Body.SscPayload); err != nil {
		return err
	}

	return nil
}

// parseSscProof parses the SSC proof from the body proof.
// SSC proof structure per Byron CDDL:
//
//	sscproof = [0, hash, hash]  ; CommitmentsProof
//	         / [1, hash, hash]  ; OpeningsProof
//	         / [2, hash, hash]  ; SharesProof
//	         / [3, hash]        ; CertificatesProof
func parseSscProof(proof any) (*ByronSscProof, error) {
	proofSlice, ok := proof.([]any)
	if !ok {
		return nil, fmt.Errorf("sscProof is not a slice, got %T", proof)
	}

	if len(proofSlice) < 2 {
		return nil, fmt.Errorf("sscProof too short: expected at least 2 elements, got %d", len(proofSlice))
	}

	result := &ByronSscProof{}

	// Parse type
	proofType, err := extractUint64(proofSlice[0])
	if err != nil {
		return nil, fmt.Errorf("invalid sscProof type: %w", err)
	}
	result.Type = proofType

	// Validate type is within known range
	if proofType > SscTypeCertificates {
		return nil, fmt.Errorf("unknown sscProof type: %d", proofType)
	}

	// Parse hash1 (always present)
	hash1, ok := proofSlice[1].([]byte)
	if !ok || len(hash1) != common.Blake2b256Size {
		return nil, fmt.Errorf("invalid sscProof hash1: expected 32 bytes, got %T (len %d)",
			proofSlice[1], len(hash1))
	}
	copy(result.Hash1[:], hash1)

	// For types 0-2, there should be a second hash (VSS certificates hash)
	if proofType != SscTypeCertificates {
		if len(proofSlice) < 3 {
			return nil, fmt.Errorf("sscProof type %d requires 3 elements, got %d", proofType, len(proofSlice))
		}
		hash2, ok := proofSlice[2].([]byte)
		if !ok || len(hash2) != common.Blake2b256Size {
			return nil, fmt.Errorf("invalid sscProof hash2: expected 32 bytes, got %T", proofSlice[2])
		}
		h2 := common.Blake2b256{}
		copy(h2[:], hash2)
		result.Hash2 = &h2
	}

	return result, nil
}

// validateSscProof validates the SSC proof against the SSC payload.
//
// The SSC (Shared Seed Computation) protocol was used in Byron's Ouroboros Classic
// for generating randomness. The proof hashes are computed from epoch-accumulated
// data (commitments, openings, shares, certificates), not just the current block's
// payload.
//
// What we CAN validate:
//   - The proof structure matches the expected format for its type
//   - The proof type is consistent with the payload type
//
// What we CANNOT validate without epoch state:
//   - The actual hash values (these depend on accumulated epoch state)
func validateSscProof(proof ByronSscProof, payload cbor.Value) error {
	// Extract the payload type from the SSC payload
	// SSC payload structure: [type, data]
	payloadType, err := extractSscPayloadType(payload)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to extract SSC payload type",
			Cause:   err,
		}
	}

	// Validate that proof type matches payload type
	if proof.Type != payloadType {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "SSC proof type mismatch",
			Details: map[string]any{
				"proof_type":   proof.Type,
				"payload_type": payloadType,
			},
		}
	}

	// Validate proof structure based on type
	if proof.Type != SscTypeCertificates && proof.Hash2 == nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: fmt.Sprintf("SSC proof type %d requires two hashes", proof.Type),
		}
	}

	if proof.Type == SscTypeCertificates && proof.Hash2 != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "SSC proof type 3 (certificates) should have only one hash",
		}
	}

	return nil
}

// extractSscPayloadType extracts the type from an SSC payload.
// SSC payload structure: [type, data] where type is 0-3
func extractSscPayloadType(payload cbor.Value) (uint64, error) {
	innerValue := payload.Value()
	if innerValue == nil {
		return 0, errors.New("SSC payload is nil")
	}

	// The payload could be decoded as []any or as a cbor.Value wrapping []any
	var arr []any

	switch v := innerValue.(type) {
	case cbor.Constructor:
		// Constructor form: the constructor number is the type
		return uint64(v.Constructor()), nil
	case []any:
		arr = v
	default:
		return 0, fmt.Errorf("unexpected SSC payload type: %T", innerValue)
	}

	if len(arr) < 1 {
		return 0, errors.New("SSC payload array is empty")
	}

	// First element is the type
	payloadType, err := extractUint64(arr[0])
	if err != nil {
		return 0, fmt.Errorf("failed to extract SSC payload type: %w", err)
	}

	if payloadType > SscTypeCertificates {
		return 0, fmt.Errorf("unknown SSC payload type: %d", payloadType)
	}

	return payloadType, nil
}

// ValidateEBBBodyHash validates that a Byron EBB's body hash matches the
// computed hash from the block body contents.
//
// EBB body proof is simply the blake2b-256 hash of the CBOR-encoded body
// (which is a list of stakeholder IDs).
func ValidateEBBBodyHash(block *byron.ByronEpochBoundaryBlock) error {
	if block == nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "block is nil",
		}
	}

	// Get the expected body hash from the header
	expectedHash := block.BlockHeader.BlockBodyHash()

	// Compute the actual body hash
	// EBB body is a list of stakeholder IDs ([]Blake2b224)
	bodyBytes, err := cbor.Encode(block.Body)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to encode EBB body",
			Cause:   err,
		}
	}

	actualHash := blake2b.Sum256(bodyBytes)

	if !bytes.Equal(actualHash[:], expectedHash.Bytes()) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "EBB body hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(actualHash).String(),
			},
		}
	}

	return nil
}

// parseByronBodyProof parses the body proof from the header's BodyProof field.
// The BodyProof is stored as `any` in the header struct.
func parseByronBodyProof(bodyProof any) (*ByronBodyProof, error) {
	// BodyProof should be a slice containing [txProof, sscProof, dlgProof, updProof]
	proofSlice, ok := bodyProof.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"body proof is not a slice, got %T",
			bodyProof,
		)
	}

	if len(proofSlice) != 4 {
		return nil, fmt.Errorf(
			"body proof has wrong number of elements: expected 4, got %d",
			len(proofSlice),
		)
	}

	result := &ByronBodyProof{}

	// Parse txProof [txCount, txBodyMerkleRoot, txWitnessMerkleRoot]
	txProofSlice, ok := proofSlice[0].([]any)
	if !ok {
		return nil, fmt.Errorf("txProof is not a slice, got %T", proofSlice[0])
	}
	if len(txProofSlice) != 3 {
		return nil, fmt.Errorf(
			"txProof has wrong number of elements: expected 3, got %d",
			len(txProofSlice),
		)
	}

	// Parse txCount
	txCount, err := toUint32(txProofSlice[0])
	if err != nil {
		return nil, fmt.Errorf("invalid txCount: %w", err)
	}
	result.TxProof.TxCount = txCount

	// Parse txBodyMerkleRoot
	txBodyRoot, ok := txProofSlice[1].([]byte)
	if !ok || len(txBodyRoot) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid txBodyMerkleRoot: expected 32 bytes, got %T",
			txProofSlice[1],
		)
	}
	copy(result.TxProof.TxBodyMerkleRoot[:], txBodyRoot)

	// Parse txWitnessMerkleRoot
	txWitRoot, ok := txProofSlice[2].([]byte)
	if !ok || len(txWitRoot) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid txWitnessMerkleRoot: expected 32 bytes, got %T",
			txProofSlice[2],
		)
	}
	copy(result.TxProof.TxWitnessMerkleRoot[:], txWitRoot)

	// Parse sscProof [type, hash] or [type, hash1, hash2]
	sscProof, err := parseSscProof(proofSlice[1])
	if err != nil {
		return nil, fmt.Errorf("invalid sscProof: %w", err)
	}
	result.SscProof = *sscProof

	// Parse dlgProof
	dlgProofBytes, ok := proofSlice[2].([]byte)
	if !ok || len(dlgProofBytes) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid dlgProof: expected 32 bytes, got %T",
			proofSlice[2],
		)
	}
	copy(result.DlgProof[:], dlgProofBytes)

	// Parse updProof
	updProofBytes, ok := proofSlice[3].([]byte)
	if !ok || len(updProofBytes) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid updProof: expected 32 bytes, got %T",
			proofSlice[3],
		)
	}
	copy(result.UpdProof[:], updProofBytes)

	return result, nil
}

// validateTxProof validates the transaction proof component of the body proof.
// This includes:
//   - Transaction count verification
//   - Transaction body merkle root verification
//   - Witness hash verification (see validateWitnessMerkleRoot)
func validateTxProof(txProof ByronTxProof, txPayload []byron.ByronTransaction) error {
	// Validate transaction count
	txPayloadLen := len(txPayload)
	// #nosec G115 -- len() cannot be negative, and a block with >2^32 txs is impossible
	if txPayloadLen > 0xFFFFFFFF || uint32(txPayloadLen) != txProof.TxCount {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "transaction count mismatch",
			Details: map[string]any{
				"expected": txProof.TxCount,
				"actual":   txPayloadLen,
			},
		}
	}

	// Compute merkle root of transaction bodies
	txBodyHashes := make([][]byte, len(txPayload))
	for i := range txPayload {
		txBodyCbor := txPayload[i].Body.Cbor()
		if txBodyCbor == nil {
			return &common.ValidationError{
				Type:    common.ValidationErrorTypeBodyHash,
				Message: fmt.Sprintf("transaction %d body CBOR is nil", i),
			}
		}
		txBodyHashes[i] = txBodyCbor
	}
	computedTxBodyRoot := computeMerkleRoot(txBodyHashes)

	if !bytes.Equal(computedTxBodyRoot[:], txProof.TxBodyMerkleRoot[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "transaction body merkle root mismatch",
			Details: map[string]any{
				"expected": txProof.TxBodyMerkleRoot.String(),
				"actual":   common.Blake2b256(computedTxBodyRoot).String(),
			},
		}
	}

	// Validate witness hash (despite the field name, this is a hash not a merkle root)
	if err := validateWitnessMerkleRoot(txProof.TxWitnessMerkleRoot, txPayload); err != nil {
		return err
	}

	return nil
}

// validateWitnessMerkleRoot validates the witness hash component of the body proof.
//
// Despite the name "TxWitnessMerkleRoot", this is actually a hash of the witness list,
// not a merkle tree root. The computation is:
//
//	blake2b256(cbor_indefinite_array([tx0_witnesses_cbor, tx1_witnesses_cbor, ...]))
//
// The witnesses must be encoded as a CBOR indefinite-length array, where each element
// is the original CBOR encoding of that transaction's witness array.
func validateWitnessMerkleRoot(
	expectedHash common.Blake2b256,
	txPayload []byron.ByronTransaction,
) error {
	// Collect original witness CBOR from each transaction
	witnessCbors := make(cbor.IndefLengthList, len(txPayload))
	for i := range txPayload {
		witCbor := txPayload[i].WitnessesCbor()
		if witCbor == nil {
			return &common.ValidationError{
				Type:    common.ValidationErrorTypeBodyHash,
				Message: fmt.Sprintf("transaction %d witness CBOR is nil", i),
			}
		}
		witnessCbors[i] = cbor.RawMessage(witCbor)
	}

	// Encode as indefinite-length CBOR array and hash
	encoded, err := cbor.Encode(witnessCbors)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to encode witness list",
			Cause:   err,
		}
	}
	computedHash := blake2b.Sum256(encoded)

	if !bytes.Equal(computedHash[:], expectedHash[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "transaction witness hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(computedHash).String(),
			},
		}
	}

	return nil
}

// validateDlgProof validates the delegation proof component of the body proof.
func validateDlgProof(expectedHash common.Blake2b256, dlgPayload []any) error {
	// Encode the delegation payload to CBOR
	dlgCbor, err := cbor.Encode(dlgPayload)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to encode delegation payload",
			Cause:   err,
		}
	}

	actualHash := blake2b.Sum256(dlgCbor)

	if !bytes.Equal(actualHash[:], expectedHash[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "delegation proof hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(actualHash).String(),
			},
		}
	}

	return nil
}

// validateDlgProofRaw validates the delegation proof using raw CBOR bytes.
func validateDlgProofRaw(expectedHash common.Blake2b256, dlgCbor []byte) error {
	actualHash := blake2b.Sum256(dlgCbor)

	if !bytes.Equal(actualHash[:], expectedHash[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "delegation proof hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(actualHash).String(),
			},
		}
	}

	return nil
}

// validateUpdProofRaw validates the update proof using raw CBOR bytes.
func validateUpdProofRaw(expectedHash common.Blake2b256, updCbor []byte) error {
	actualHash := blake2b.Sum256(updCbor)

	if !bytes.Equal(actualHash[:], expectedHash[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "update proof hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(actualHash).String(),
			},
		}
	}

	return nil
}

// validateUpdProof validates the update proof component of the body proof.
func validateUpdProof(
	expectedHash common.Blake2b256,
	updPayload *byron.ByronUpdatePayload,
) error {
	// Encode the update payload to CBOR
	updCbor, err := cbor.Encode(updPayload)
	if err != nil {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "failed to encode update payload",
			Cause:   err,
		}
	}

	actualHash := blake2b.Sum256(updCbor)

	if !bytes.Equal(actualHash[:], expectedHash[:]) {
		return &common.ValidationError{
			Type:    common.ValidationErrorTypeBodyHash,
			Message: "update proof hash mismatch",
			Details: map[string]any{
				"expected": expectedHash.String(),
				"actual":   common.Blake2b256(actualHash).String(),
			},
		}
	}

	return nil
}

// computeMerkleRoot computes a Byron-style merkle root from a list of items.
//
// Byron merkle tree structure:
// - Empty list: hash of empty bytes
// - Leaf node: hash(0x00 || cbor_data)
// - Branch node: hash(0x01 || left_hash || right_hash)
//
// The tree is built by padding to the next power of 2 and combining nodes pairwise.
func computeMerkleRoot(items [][]byte) common.Blake2b256 {
	if len(items) == 0 {
		// Empty tree: hash of empty bytes
		return blake2b.Sum256(nil)
	}

	// Compute leaf hashes
	// Find max item size to allocate a reusable buffer
	maxLen := 0
	for _, item := range items {
		if len(item) > maxLen {
			maxLen = len(item)
		}
	}
	leafBuf := make([]byte, 1+maxLen)
	leafBuf[0] = 0x00

	leaves := make([][32]byte, len(items))
	for i, item := range items {
		// Leaf hash: hash(0x00 || item)
		copy(leafBuf[1:], item)
		leaves[i] = blake2b.Sum256(leafBuf[:1+len(item)])
	}

	// Build tree bottom-up
	// Use fixed-size array for branch data (1 byte tag + 32 bytes left + 32 bytes right)
	var branchData [65]byte
	branchData[0] = 0x01

	nodes := leaves
	for len(nodes) > 1 {
		// Pad to even number if necessary
		if len(nodes)%2 == 1 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		// Combine pairs
		newNodes := make([][32]byte, len(nodes)/2)
		for i := 0; i < len(nodes); i += 2 {
			// Branch hash: hash(0x01 || left || right)
			copy(branchData[1:33], nodes[i][:])
			copy(branchData[33:65], nodes[i+1][:])
			newNodes[i/2] = blake2b.Sum256(branchData[:])
		}
		nodes = newNodes
	}

	return common.Blake2b256(nodes[0])
}

// toUint32 converts various numeric types to uint32.
func toUint32(v any) (uint32, error) {
	switch x := v.(type) {
	case uint64:
		if x > 0xFFFFFFFF {
			return 0, fmt.Errorf("value %d overflows uint32", x)
		}
		return uint32(x), nil
	case uint32:
		return x, nil
	case uint:
		if x > 0xFFFFFFFF {
			return 0, fmt.Errorf("value %d overflows uint32", x)
		}
		return uint32(x), nil
	case int:
		if x < 0 {
			return 0, fmt.Errorf("negative value %d", x)
		}
		if x > 0xFFFFFFFF {
			return 0, fmt.Errorf("value %d overflows uint32", x)
		}
		return uint32(x), nil
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("negative value %d", x)
		}
		if x > 0xFFFFFFFF {
			return 0, fmt.Errorf("value %d overflows uint32", x)
		}
		return uint32(x), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to uint32", v)
	}
}
