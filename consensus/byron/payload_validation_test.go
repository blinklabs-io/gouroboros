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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// TestValidateBlockSignatureDoesNotMutateInput pins that proxy signature
// validation treats its ValidateHeaderInput as read-only. The type-0 branch
// has to fill in BlockSignature from BlockSig before it can verify
// anything, and used to write that back into the caller's struct.
func TestValidateBlockSignatureDoesNotMutateInput(t *testing.T) {
	validator := NewHeaderValidator(testByronConfig())
	publicKey := bytes.Repeat([]byte{0x11}, ed25519.PublicKeySize)
	signature := bytes.Repeat([]byte{0x22}, ed25519.SignatureSize)
	input := &ValidateHeaderInput{
		Slot:         100,
		BlockNumber:  10,
		IssuerPubKey: publicKey,
		BlockSig:     []any{uint64(byronSigTypeSimple), signature},
	}

	// The verification itself fails -- the signature is not real -- but
	// what matters here is what it leaves behind.
	require.Error(t, validator.validateBlockSignature(input))
	require.Empty(
		t,
		input.BlockSignature,
		"validation must not write the extracted signature back "+
			"into the caller's input",
	)
	require.Len(t, input.BlockSig, 2)
}

// TestValidateBlockSignatureInputReuse exercises the consequence of that
// mutation directly: a caller that reuses one ValidateHeaderInput across
// blocks must not have the first block's signature carried into the second.
func TestValidateBlockSignatureInputReuse(t *testing.T) {
	validator := NewHeaderValidator(testByronConfig())
	publicKey := bytes.Repeat([]byte{0x11}, ed25519.PublicKeySize)
	signature := bytes.Repeat([]byte{0x22}, ed25519.SignatureSize)
	input := &ValidateHeaderInput{
		Slot:         100,
		BlockNumber:  10,
		IssuerPubKey: publicKey,
		BlockSig:     []any{uint64(byronSigTypeSimple), signature},
	}
	require.Error(t, validator.validateBlockSignature(input))

	// Second header through the same struct, this time carrying no
	// signature at all. It must be rejected for having none, rather than
	// being checked against the signature left over from the first.
	input.Slot = 101
	input.BlockNumber = 11
	input.BlockSig = nil
	err := validator.validateBlockSignature(input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid block signature size: got 0")
}

// TestValidateBodyHashRealMainnetBlockWithPayloadValidation confirms the
// opt-in payload check does not reject a real mainnet block.
func TestValidateBodyHashRealMainnetBlockWithPayloadValidation(t *testing.T) {
	blockBytes, err := hex.DecodeString(testByronMainBlockHex)
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	require.NoError(t, block.ValidatePayloads())
	require.NoError(t, ValidateBodyHash(
		block,
		common.VerifyConfig{EnableByronPayloadValidation: true},
	))
	// The same block must also survive a decode that opts in.
	_, err = byron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{EnableByronPayloadValidation: true},
	)
	require.NoError(t, err)
}

// TestValidateBodyHashRejectsMalformedPayloadWhenEnabled pins that the
// opt-in check is what catches a payload the body proof cannot: dlg_proof
// binds the payload bytes to the header, so a block carrying a malformed
// certificate still passes the default validation.
func TestValidateBodyHashRejectsMalformedPayloadWhenEnabled(t *testing.T) {
	block := &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{ProtocolMagic: 764824073},
		Body: byron.ByronMainBlockBody{
			DlgPayload: []any{[]any{uint64(1), uint64(2)}},
		},
	}
	require.ErrorIs(
		t,
		block.ValidateDelegationPayload(),
		byron.ErrInvalidPayload,
	)
}

// TestValidateEBBBodyHashMalformedProof pins that an EBB whose body proof
// is not a hash at all is reported as malformed rather than as a mismatch
// against the zero hash that used to stand in for it.
func TestValidateEBBBodyHashMalformedProof(t *testing.T) {
	block := &byron.ByronEpochBoundaryBlock{
		BlockHeader: &byron.ByronEpochBoundaryBlockHeader{
			BodyProof: []any{uint64(0)},
		},
	}
	err := ValidateEBBBodyHash(block)
	require.Error(t, err)
	require.ErrorIs(t, err, byron.ErrMalformedBodyProof)

	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.NotNil(t, validationErr)
	require.Equal(
		t,
		common.ValidationErrorTypeBodyHash,
		validationErr.Type,
	)
	require.Contains(t, validationErr.Message, "malformed EBB body proof")
}
