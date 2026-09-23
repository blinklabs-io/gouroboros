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

package ledger_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSlotsPerKesPeriod is mainnet's slotsPerKesPeriod. The real block and
// header fixtures below were signed under it, so their KES signatures only
// verify at this value.
const testSlotsPerKesPeriod = uint64(129600)

// skipAllValidationConfig returns a VerifyConfig that skips all validations.
// This is useful for tests that don't need full block validation.
func skipAllValidationConfig() common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
	}
}

// realConwayHeaderHex is the CBOR of mainnet Conway block header 10882991
// (slot 135747596). Its VRF proof, operational certificate cold signature
// and KES signature all verify against realConwayHeaderEta0Hex, the epoch
// nonce in force at that slot, so VerifyBlock accepts it unmodified. Every
// negative case below is one mutation away from that accepted state, which
// is what makes the rejection attributable to a single rule.
//
// ledger/verify_block_test.go carries its own copy of these two values.
// That file is in package ledger and internal/testdata imports ledger, so
// the two cannot share a fixture package without an import cycle.
const realConwayHeaderHex = "828a1a00a60faf1a0817580c58204eac1e7264c0e80436b04687e75d46d6a0d6b2338c2abb73a14fafbd689f69b2582012209e0b93f0128f670c9a02781c5466c4c4be003da3a51344b6a94f709ce51f58209c1a5fc5dec0a4b822d5a3b254ce9b168299479127aadcf97506ef257517fff682584023c2d70c24c44041644f5152f7e8a1bb580e516eb8e73c7df287116adb5f009c0c001feccfeebdf34c2275d1fce859c6c46182631b6306d5fd2724ac7ab1c6be58500dbe31ef7c00c34b6522e983d223e05075359cb170668d960b8cebfced178287ee6ca5cfc6e8e60aec97fd197aebfefc24aae695680631d575c6dacdfd9efc5687e46eb2a5c04a755c7f260af9ef830819c5ea5820d2b74b6333637801f2e9c7265792d5b8fc1647f9056d67c769dbac27f25f2fd08458200946347d22a3b6da29d79102424973c932b898808ff2436fa138df102484230a0a1904165840c75619c3ebad0758349eb1dedc154a8cd280d8189d6da973b4a147b0cdb0f60442d493feeba64167a05b5fc40bc695192bf1c08afad3c07ebd33cb5925f378018209015901c00a8442332bd3f33a4d78fe2736a75110b528a1e7501bc7887910d1475fc0e425f49a84f94e98f87047916cf622f3db1f61b60c5f06709769f98c4cc67de8f50c320c6772b647ac9916765b6985d4eafccb54e71064d01df41f8d0638ed5cd62b7b6e49ba15dd87cc687ab87d3fb22490d355e8fa9c5f7c24ed88b800fcc4cb1f1b54e65b5ba82c442f4643caadc86583072b8b6956f4f9a4530c29873f7231605efd7a7f961a863530512ef86b50f9b1004748c31fa07978f2ece7d8e76ffde67d713015824b28e19f05f0383c2def3cdeb67247f33f5eae329c38a375b2eb06a586dcc2e102a776a6deaad1741f2a7f5aa604074698e876afab4455278fd84a1db5768078e2848cc85e3c8a0b48630a2622832ecd2dbb3c505df2a70b93b49ce99616f601e5e2004a8ce8926319c23f2a26ac8550cb1c05c9d2d25fc5fcd122fc35b057a71d6e961250c99b19a7bfd9acdc60a8151d6c81ef2d7d69a62fd0f17d184dd753cce9a2e9c32b53baf317e31c6c5e3cf8ea8b203b413ae8b0253db53d0cbe19b0f0547a0e67d3591d1cade6ceb4a47779ba4a09e7526280acb62200f42c98f6185ea9da3daf47aa3d10ffe5307331fa3430af6c6361154943c39375"

const realConwayHeaderEta0Hex = "4ef95a10f639d0cf16bb963c3a580d4bf2a95b6ae7848702665884843e3c661d"

// decodeRealConwayHeader returns a freshly decoded copy of
// realConwayHeaderHex. Each caller needs its own copy because the negative
// cases mutate header fields in place.
func decodeRealConwayHeader(t *testing.T) *conway.ConwayBlockHeader {
	t.Helper()
	raw, err := hex.DecodeString(realConwayHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeConway,
		raw,
	)
	require.NoError(t, err)
	conwayHeader, ok := header.(*conway.ConwayBlockHeader)
	require.True(t, ok, "fixture must decode as a Conway header")
	return conwayHeader
}

// newRealConwayBlock wraps the fixture header in an otherwise empty Conway
// block. Body hash validation must stay off for this block: the header's
// block_body_hash is the mainnet one and the body here is empty.
func newRealConwayBlock(t *testing.T) *conway.ConwayBlock {
	t.Helper()
	return &conway.ConwayBlock{
		BlockHeader:            decodeRealConwayHeader(t),
		TransactionBodies:      []conway.ConwayTransactionBody{},
		TransactionWitnessSets: []conway.ConwayTransactionWitnessSet{},
		TransactionMetadataSet: common.TransactionMetadataSet{},
		InvalidTransactions:    []uint{},
	}
}

// headerOnlyConfig disables every check that needs data a bare header does
// not carry, leaving VRF, operational certificate and KES enabled.
func headerOnlyConfig() common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
		SkipBlockLimitsValidation: true,
	}
}

func requireValidationErrorType(
	t *testing.T,
	err error,
	want common.ValidationErrorType,
) {
	t.Helper()
	var validationErr *common.ValidationError
	require.True(
		t,
		errors.As(err, &validationErr),
		"expected a *common.ValidationError, got %v",
		err,
	)
	require.NotNil(t, validationErr)
	require.Equal(t, want, validationErr.Type)
}

// mustDecodeMainnetConwayBlock decodes the mainnet Conway block fixture with
// body hash validation left on, which is the default.
func mustDecodeMainnetConwayBlock(t *testing.T) (*conway.ConwayBlock, []byte) {
	t.Helper()
	raw := testdata.MustDecodeHex(testdata.ConwayBlockHex)
	block, err := ledger.NewBlockFromCbor(ledger.BlockTypeConway, raw)
	require.NoError(t, err, "mainnet fixture must pass body hash validation")
	conwayBlock, ok := block.(*conway.ConwayBlock)
	require.True(t, ok)
	return conwayBlock, raw
}

// flipByteAt returns a copy of data with the low bit of data[off] flipped.
func flipByteAt(data []byte, off int) []byte {
	tampered := bytes.Clone(data)
	tampered[off] ^= 0x01
	return tampered
}

// TestVerifyBlock_BodyHashTampering exercises the block body hash rule on the
// mainnet Conway block fixture, both at its decode-time call site
// (NewBlockFromCbor) and at its VerifyBlock call site, which is reached only
// after VRF, operational certificate and KES verification have passed.
func TestVerifyBlock_BodyHashTampering(t *testing.T) {
	t.Run("unmodified mainnet block verifies", func(t *testing.T) {
		block, raw := mustDecodeMainnetConwayBlock(t)
		assert.Equal(t, raw, block.Cbor())
		assert.Len(t, block.TransactionBodies, 8)
	})

	t.Run("one flipped bit in any body element is rejected",
		func(t *testing.T) {
			block, raw := mustDecodeMainnetConwayBlock(t)
			type region struct {
				name string
				cbor []byte
			}
			regions := make([]region, 0, 16)
			for i := range block.TransactionBodies {
				regions = append(regions, region{
					name: fmt.Sprintf("transaction body %d", i),
					cbor: block.TransactionBodies[i].Cbor(),
				})
			}
			for i := range block.TransactionWitnessSets {
				regions = append(regions, region{
					name: fmt.Sprintf("witness set %d", i),
					cbor: block.TransactionWitnessSets[i].Cbor(),
				})
			}
			require.Len(t, regions, 16)
			for _, r := range regions {
				t.Run(r.name, func(t *testing.T) {
					require.NotEmpty(t, r.cbor)
					off := bytes.Index(raw, r.cbor)
					require.GreaterOrEqual(t, off, 0,
						"element CBOR must be a slice of the block CBOR")
					tampered := flipByteAt(raw, off+len(r.cbor)-1)

					_, err := ledger.NewBlockFromCbor(
						ledger.BlockTypeConway,
						tampered,
					)
					require.Error(t, err)
					requireValidationErrorType(
						t,
						err,
						common.ValidationErrorTypeBodyHash,
					)

					// The same bytes decode cleanly once the rule is
					// disabled, so the rejection above is the body hash
					// check and not a CBOR structural error.
					_, err = ledger.NewBlockFromCbor(
						ledger.BlockTypeConway,
						tampered,
						common.VerifyConfig{SkipBodyHashValidation: true},
					)
					require.NoError(t, err)
				})
			}
		})

	t.Run("body hash does not cover the header", func(t *testing.T) {
		// The body hash preimage is the block's body elements only. A header
		// byte is covered by the KES signature instead, so tampering there
		// must leave the body hash satisfied and fail KES. Asserting the
		// boundary in both directions keeps a future change that widened or
		// narrowed the preimage from passing silently.
		block, raw := mustDecodeMainnetConwayBlock(t)
		headerCbor := block.BlockHeader.Cbor()
		off := bytes.Index(raw, headerCbor)
		require.GreaterOrEqual(t, off, 0)
		tampered := flipByteAt(raw, off+len(headerCbor)-1)

		decoded, err := ledger.NewBlockFromCbor(
			ledger.BlockTypeConway,
			tampered,
		)
		require.NoError(t, err, "body hash must be blind to header bytes")
		tamperedBlock, ok := decoded.(*conway.ConwayBlock)
		require.True(t, ok)

		kesValid, err := ledger.VerifyKes(
			block.BlockHeader,
			testSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		assert.True(t, kesValid, "unmodified header must satisfy KES")

		kesValid, err = ledger.VerifyKes(
			tamperedBlock.BlockHeader,
			testSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		assert.False(t, kesValid, "tampered header must fail KES")
	})

	t.Run("VerifyBlock rejects a mismatched body", func(t *testing.T) {
		// The fixture header's block_body_hash is the mainnet one; this
		// block's body is empty, so the two cannot agree. VerifyBlock only
		// reaches its body hash branch after VRF, the operational
		// certificate cold signature and KES have all passed, so this also
		// pins the order of those checks.
		raw, err := hex.DecodeString(realConwayHeaderHex)
		require.NoError(t, err)
		emptyBodyBlockCbor := []byte{0x85}
		emptyBodyBlockCbor = append(emptyBodyBlockCbor, raw...)
		emptyBodyBlockCbor = append(
			emptyBodyBlockCbor,
			0x80, 0x80, 0xa0, 0x80,
		)

		block, err := ledger.NewBlockFromCbor(
			ledger.BlockTypeConway,
			emptyBodyBlockCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)

		config := headerOnlyConfig()
		config.SkipBodyHashValidation = false
		isValid, _, _, _, err := ledger.VerifyBlock(
			block,
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			config,
		)
		require.Error(t, err)
		require.ErrorContains(t, err, "body hash mismatch")
		assert.False(t, isValid)

		// Skipping the rule is the only difference, so the error above is
		// attributable to it.
		isValid, _, _, _, err = ledger.VerifyBlock(
			block,
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			headerOnlyConfig(),
		)
		require.NoError(t, err)
		assert.True(t, isValid)
	})
}

// TestVerifyBlock_VRFEdgeCases exercises the leader VRF check in VerifyBlock
// (ledger/verify_block.go, vrf.Verify) against the mainnet Conway header
// fixture. Each case mutates exactly one VRF input of an otherwise accepted
// header.
func TestVerifyBlock_VRFEdgeCases(t *testing.T) {
	t.Run("unmodified header verifies", func(t *testing.T) {
		block := newRealConwayBlock(t)
		wantVrfHex := hex.EncodeToString(
			block.BlockHeader.Body.VrfResult.Output,
		)
		isValid, vrfHex, blockNo, slot, err := ledger.VerifyBlock(
			block,
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			headerOnlyConfig(),
		)
		require.NoError(t, err)
		assert.True(t, isValid)
		assert.Equal(t, wantVrfHex, vrfHex)
		assert.Equal(t, uint64(10882991), blockNo)
		assert.Equal(t, uint64(135747596), slot)
	})

	tests := []struct {
		name    string
		eta0Hex string
		mutate  func(*conway.ConwayBlockHeader)
		wantErr string
	}{
		{
			name: "tampered VRF output",
			mutate: func(h *conway.ConwayBlockHeader) {
				h.Body.VrfResult.Output[0] ^= 0xFF
			},
			wantErr: "VRF output mismatch",
		},
		{
			name: "tampered VRF proof",
			mutate: func(h *conway.ConwayBlockHeader) {
				h.Body.VrfResult.Proof[0] ^= 0xFF
			},
			wantErr: "VRF verification failed",
		},
		{
			name: "truncated VRF proof",
			mutate: func(h *conway.ConwayBlockHeader) {
				h.Body.VrfResult.Proof = h.Body.VrfResult.Proof[:40]
			},
			wantErr: "unexpected length of pi (must be 80)",
		},
		{
			name: "empty VRF key",
			mutate: func(h *conway.ConwayBlockHeader) {
				h.Body.VrfKey = []byte{}
			},
			wantErr: "invalid point encoding length",
		},
		{
			name: "zeroed VRF key",
			mutate: func(h *conway.ConwayBlockHeader) {
				h.Body.VrfKey = make([]byte, 32)
			},
			wantErr: "small order point",
		},
		{
			// The proof is bound to blake2b256(slot || eta0), so a header
			// replayed under a different epoch nonce must not verify.
			name:    "wrong epoch nonce",
			eta0Hex: strings.Repeat("11", 32),
			wantErr: "VRF verification failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := newRealConwayBlock(t)
			if tt.mutate != nil {
				tt.mutate(block.BlockHeader)
			}
			eta0Hex := tt.eta0Hex
			if eta0Hex == "" {
				eta0Hex = realConwayHeaderEta0Hex
			}
			isValid, vrfHex, _, _, err := ledger.VerifyBlock(
				block,
				eta0Hex,
				testSlotsPerKesPeriod,
				headerOnlyConfig(),
			)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
			requireValidationErrorType(
				t,
				err,
				common.ValidationErrorTypeVRF,
			)
			assert.False(t, isValid)
			assert.Empty(t, vrfHex)
		})
	}

	t.Run("tampered VRF proof on the wire", func(t *testing.T) {
		// The struct-level cases above leave the header's stored CBOR
		// intact. This one tampers with the bytes and decodes them, which is
		// the shape an adversary actually produces.
		raw, err := hex.DecodeString(realConwayHeaderHex)
		require.NoError(t, err)
		proof := decodeRealConwayHeader(t).Body.VrfResult.Proof
		off := bytes.Index(raw, proof)
		require.GreaterOrEqual(t, off, 0)

		header, err := ledger.NewBlockHeaderFromCbor(
			ledger.BlockTypeConway,
			flipByteAt(raw, off),
		)
		require.NoError(t, err, "tampering must not break CBOR decoding")
		conwayHeader, ok := header.(*conway.ConwayBlockHeader)
		require.True(t, ok)

		isValid, _, _, _, err := ledger.VerifyBlock(
			&conway.ConwayBlock{BlockHeader: conwayHeader},
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			headerOnlyConfig(),
		)
		require.Error(t, err)
		requireValidationErrorType(t, err, common.ValidationErrorTypeVRF)
		assert.False(t, isValid)
	})
}

// TestVerifyBlock_KESEdgeCases exercises VerifyKes and VerifyKesComponents
// against the mainnet Conway header fixture, and the KES branch of
// VerifyBlock that calls them.
//
// Not covered, because VerifyBlock cannot reach it: the maxKESEvolutions
// bound and operational certificate counter monotonicity are chain-state
// rules. VerifyBlock receives neither the Shelley genesis maxKESEvolutions
// nor the pool's last-seen counter, and ledger/verify_block.go says so. A
// test for either needs a caller that supplies chain state, not a mutation
// of this fixture.
func TestVerifyBlock_KESEdgeCases(t *testing.T) {
	header := decodeRealConwayHeader(t)
	bodyCbor := header.Body.Cbor()
	require.NotEmpty(t, bodyCbor)
	signature := header.Signature
	hotVkey := header.Body.OpCert.HotVkey
	kesPeriod := header.Body.OpCert.KesPeriod
	slot := header.SlotNumber()

	t.Run("unmodified header verifies", func(t *testing.T) {
		kesValid, err := ledger.VerifyKes(header, testSlotsPerKesPeriod)
		require.NoError(t, err)
		assert.True(t, kesValid)

		kesValid, err = ledger.VerifyKesComponents(
			bodyCbor,
			signature,
			hotVkey,
			kesPeriod,
			slot,
			testSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		assert.True(t, kesValid)
	})

	t.Run("tampered KES signature", func(t *testing.T) {
		block := newRealConwayBlock(t)
		block.BlockHeader.Signature[0] ^= 0xFF
		isValid, _, _, _, err := ledger.VerifyBlock(
			block,
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			headerOnlyConfig(),
		)
		require.Error(t, err)
		require.ErrorContains(t, err, "KES signature invalid")
		requireValidationErrorType(t, err, common.ValidationErrorTypeKES)
		assert.False(t, isValid)
	})

	t.Run("tampered header body", func(t *testing.T) {
		// The signature is over the header body's original CBOR bytes, so a
		// change anywhere in that preimage must invalidate it.
		kesValid, err := ledger.VerifyKesComponents(
			flipByteAt(bodyCbor, len(bodyCbor)/2),
			signature,
			hotVkey,
			kesPeriod,
			slot,
			testSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		assert.False(t, kesValid)
	})

	t.Run("truncated KES signature", func(t *testing.T) {
		kesValid, err := ledger.VerifyKesComponents(
			bodyCbor,
			signature[:100],
			hotVkey,
			kesPeriod,
			slot,
			testSlotsPerKesPeriod,
		)
		require.Error(t, err)
		require.ErrorContains(
			t,
			err,
			"invalid KES signature length: expected 448 bytes, got 100",
		)
		assert.False(t, kesValid)

		block := newRealConwayBlock(t)
		block.BlockHeader.Signature = block.BlockHeader.Signature[:100]
		isValid, _, _, _, err := ledger.VerifyBlock(
			block,
			realConwayHeaderEta0Hex,
			testSlotsPerKesPeriod,
			headerOnlyConfig(),
		)
		require.Error(t, err)
		requireValidationErrorType(t, err, common.ValidationErrorTypeKES)
		assert.False(t, isValid)
	})

	t.Run("certificate period in the future", func(t *testing.T) {
		kesValid, err := ledger.VerifyKesComponents(
			bodyCbor,
			signature,
			hotVkey,
			slot/testSlotsPerKesPeriod+1,
			slot,
			testSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		assert.False(t, kesValid)
	})

	t.Run("zero slots per KES period", func(t *testing.T) {
		kesValid, err := ledger.VerifyKesComponents(
			bodyCbor,
			signature,
			hotVkey,
			kesPeriod,
			slot,
			0,
		)
		require.Error(t, err)
		require.ErrorContains(
			t,
			err,
			"slotsPerKesPeriod must be greater than 0",
		)
		assert.False(t, kesValid)
	})

	t.Run("evolution count is bound to the signature", func(t *testing.T) {
		// VerifyKesComponents derives the evolution index t from
		// slot/slotsPerKesPeriod - kesPeriod. The signature was produced at
		// exactly one t, so every other value must fail: a signature does
		// not survive being replayed at a later evolution of the same key.
		actualT := slot/testSlotsPerKesPeriod - kesPeriod
		require.Equal(t, uint64(1), actualT)
		for offset := uint64(0); offset <= 4; offset++ {
			kesValid, err := ledger.VerifyKesComponents(
				bodyCbor,
				signature,
				hotVkey,
				kesPeriod,
				(kesPeriod+offset)*testSlotsPerKesPeriod,
				testSlotsPerKesPeriod,
			)
			require.NoError(t, err)
			assert.Equal(
				t,
				offset == actualT,
				kesValid,
				"evolution %d",
				offset,
			)
		}
	})
}

// TestVerifyBlock_MalformedCBOR tests graceful handling of malformed CBOR data.
func TestVerifyBlock_MalformedCBOR(t *testing.T) {
	t.Run("truncated CBOR data", func(t *testing.T) {
		// Truncated CBOR should fail gracefully with decode error
		blockBytes := testdata.MustDecodeHex(testdata.ConwayBlockHex)[:25]

		_, err := ledger.NewBlockFromCbor(
			ledger.BlockTypeConway,
			blockBytes,
		)
		require.Error(t, err, "expected error for truncated CBOR")

		// Error should indicate CBOR decode issue
		errStr := strings.ToLower(err.Error())
		isExpectedError := strings.Contains(errStr, "decode") ||
			strings.Contains(errStr, "cbor") ||
			strings.Contains(errStr, "unexpected") ||
			strings.Contains(errStr, "invalid") ||
			strings.Contains(errStr, "eof")
		assert.True(
			t,
			isExpectedError,
			"expected CBOR decode error, got: %v",
			err,
		)
	})

	t.Run("empty CBOR data", func(t *testing.T) {
		emptyBytes := []byte{}

		_, err := ledger.NewBlockFromCbor(ledger.BlockTypeConway, emptyBytes)
		assert.Error(t, err, "expected error for empty CBOR")
	})

	t.Run("invalid CBOR structure", func(t *testing.T) {
		// Valid CBOR but wrong structure for a block
		// This is a simple CBOR map instead of expected block array structure
		invalidStructureHex := "a16568656c6c6f65776f726c64" // {"hello": "world"}

		blockBytes, err := hex.DecodeString(invalidStructureHex)
		require.NoError(t, err, "failed to decode invalid structure hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		require.Error(t, err, "expected error for invalid CBOR structure")

		// Should indicate structure mismatch
		errStr := strings.ToLower(err.Error())
		isExpectedError := strings.Contains(errStr, "decode") ||
			strings.Contains(errStr, "structure") ||
			strings.Contains(errStr, "invalid") ||
			strings.Contains(errStr, "type") ||
			strings.Contains(errStr, "unmarshal")
		assert.True(
			t,
			isExpectedError,
			"expected structure error, got: %v",
			err,
		)
	})

	t.Run("CBOR with wrong array length", func(t *testing.T) {
		// CBOR array but wrong number of elements
		// A single-element array when block expects more elements
		wrongLengthHex := "8100" // [0] - single element array

		blockBytes, err := hex.DecodeString(wrongLengthHex)
		require.NoError(t, err, "failed to decode wrong length hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		assert.Error(t, err, "expected error for wrong array length")
	})

	t.Run("random garbage bytes", func(t *testing.T) {
		// Completely random bytes that aren't valid CBOR
		garbageBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}

		_, err := ledger.NewBlockFromCbor(ledger.BlockTypeConway, garbageBytes)
		assert.Error(t, err, "expected error for garbage bytes")
	})

	t.Run("CBOR indefinite length without break", func(t *testing.T) {
		// Start indefinite array but don't close it properly
		// 0x9F starts indefinite array, needs 0xFF break to close
		incompleteHex := "9f01020304" // indefinite array [1,2,3,4] without break

		blockBytes, err := hex.DecodeString(incompleteHex)
		require.NoError(t, err, "failed to decode incomplete indefinite hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		assert.Error(t, err, "expected error for incomplete indefinite CBOR")
	})
}

// TestVerifyBlock_BlockTypeEdgeCases tests edge cases for block type handling.
func TestVerifyBlock_BlockTypeEdgeCases(t *testing.T) {
	t.Run("unknown block type", func(t *testing.T) {
		validCBOR := []byte{0x82, 0x00, 0x00} // minimal valid CBOR array

		// Use an invalid block type constant
		unknownBlockType := uint(99)

		_, err := ledger.NewBlockFromCbor(unknownBlockType, validCBOR)
		require.Error(t, err, "expected error for unknown block type")
		assert.Contains(
			t,
			err.Error(),
			"unknown",
			"expected 'unknown' in error",
		)
	})
}

// TestNewBlockFromCbor_RejectsNullHeaders verifies that the public block
// decoder rejects a null header even when body-hash validation is disabled.
// A null header otherwise leaves a decoded block that panics when callers use
// its header accessors.
func TestNewBlockFromCbor_RejectsNullHeaders(t *testing.T) {
	tests := []struct {
		name      string
		blockType uint
		data      []byte
		wantError string
	}{
		{
			// The EBB body must be an indefinite-length list and the extra
			// body data [attributes] (blinklabs-io/gouroboros#2347), so this
			// fixture uses 0x9f, 0xff and 0x81, 0xa0 to stay shape-valid
			// there and isolate the null-header check.
			"byron ebb",
			ledger.BlockTypeByronEbb,
			[]byte{0x83, 0xf6, 0x9f, 0xff, 0x81, 0xa0},
			"decode Byron EBB block error: byron EBB block missing header",
		},
		{
			// Keep this fixture's transaction payload and extra body data
			// shape-valid so it isolates the null-header check below.
			"byron main",
			ledger.BlockTypeByronMain,
			[]byte{
				0x83,
				0xf6,
				0x84,
				0x9f,
				0xff,
				0xf6,
				0x9f,
				0xff,
				0x82,
				0x80,
				0x9f,
				0xff,
				0x81,
				0xa0,
			},
			"decode Byron main block error: byron main block missing header",
		},
		{
			"shelley",
			ledger.BlockTypeShelley,
			[]byte{0x84, 0xf6, 0x80, 0x80, 0xa0},
			"block header is nil",
		},
		{
			"allegra",
			ledger.BlockTypeAllegra,
			[]byte{0x84, 0xf6, 0x80, 0x80, 0xa0},
			"block header is nil",
		},
		{
			"mary",
			ledger.BlockTypeMary,
			[]byte{0x84, 0xf6, 0x80, 0x80, 0xa0},
			"block header is nil",
		},
		{
			"alonzo",
			ledger.BlockTypeAlonzo,
			[]byte{0x85, 0xf6, 0x80, 0x80, 0xa0, 0x80},
			"block header is nil",
		},
		{
			"babbage",
			ledger.BlockTypeBabbage,
			[]byte{0x85, 0xf6, 0x80, 0x80, 0xa0, 0x80},
			"block header is nil",
		},
		{
			"conway",
			ledger.BlockTypeConway,
			[]byte{0x85, 0xf6, 0x80, 0x80, 0xa0, 0x80},
			"block header is nil",
		},
		{
			"dijkstra",
			ledger.BlockTypeDijkstra,
			[]byte{0x82, 0xf6, 0x84, 0xf6, 0x80, 0xf6, 0xf6},
			"decode Dijkstra block error: dijkstra block header is nil",
		},
	}
	config := skipAllValidationConfig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ledger.NewBlockFromCbor(tt.blockType, tt.data, config)
			require.Error(t, err)
			require.EqualError(t, err, tt.wantError)
		})
	}
}
