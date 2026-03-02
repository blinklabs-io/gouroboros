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

package allegra_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

func TestAllegraBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.AllegraBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Allegra block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into AllegraBlock struct
	var block allegra.AllegraBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into AllegraBlock: %v", err)
	}
	// Reset stored CBOR to nil
	block.SetCbor(nil)

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal AllegraBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from AllegraBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Allegra block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
			dataBytes,
			encoded,
		)

		// Check from which byte it differs
		diffIndex := -1
		for i := 0; i < len(dataBytes) && i < len(encoded); i++ {
			if dataBytes[i] != encoded[i] {
				diffIndex = i
				break
			}
		}
		if diffIndex != -1 {
			t.Logf("First mismatch at byte index: %d", diffIndex)
			t.Logf(
				"Original byte: 0x%02x, Re-encoded byte: 0x%02x",
				dataBytes[diffIndex],
				encoded[diffIndex],
			)
		} else {
			t.Logf("Length mismatch: original length = %d, re-encoded length = %d", len(dataBytes), len(encoded))
		}
	}
}

func TestAllegraUtxorpcBlock(t *testing.T) {
	// Decode the block hex to bytes
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AllegraBlockHex))
	assert.NoError(t, err, "Failed to decode block hex")

	// Parse the block
	block, err := allegra.NewAllegraBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	assert.NoError(t, err, "Failed to parse block")
	assert.NotNil(t, block, "Parsed block is nil")

	expectedHash := block.Hash().String()
	expectedHeight := block.BlockNumber()
	expectedSlot := block.SlotNumber()

	// Convert to UTxO RPC format
	utxorpcBlock, err := block.Utxorpc()
	assert.NoError(t, err, "Utxorpc() conversion failed")
	assert.NotNil(t, utxorpcBlock, "RPC block is nil")

	t.Run("BlockHeader", func(t *testing.T) {
		assert.Equal(
			t,
			expectedHash,
			hex.EncodeToString(utxorpcBlock.Header.Hash),
			"Block hash mismatch",
		)

		assert.Equal(t, expectedHeight, utxorpcBlock.Header.Height,
			"Block height mismatch")

		assert.Equal(t, expectedSlot, utxorpcBlock.Header.Slot,
			"Block slot mismatch")
	})

	t.Run("Transactions", func(t *testing.T) {
		originalTxs := block.Transactions()
		rpcTxs := utxorpcBlock.Body.Tx

		assert.Equal(t, len(originalTxs), len(rpcTxs),
			"Transaction count mismatch")

		if len(rpcTxs) > 0 {
			firstRpcTx := rpcTxs[0]
			assert.NotEmpty(
				t,
				firstRpcTx.Hash,
				"Transaction hash should not be empty",
			)
			assert.Greater(
				t,
				len(firstRpcTx.Inputs),
				0,
				"Transaction should have inputs",
			)
			assert.Greater(
				t,
				len(firstRpcTx.Outputs),
				0,
				"Transaction should have outputs",
			)
			fee := firstRpcTx.Fee
			if fee.GetInt() != 0 {
				assert.Greater(
					t,
					fee.GetInt(),
					int64(0),
					"Transaction fee should be positive",
				)
			} else {
				feeBigInt := new(big.Int).SetBytes(fee.GetBigUInt())
				assert.Greater(
					t,
					feeBigInt.Sign(),
					0,
					"Transaction fee should be positive",
				)
			}
		}
	})
}

func BenchmarkAllegraBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AllegraBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block allegra.AllegraBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAllegraBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AllegraBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block allegra.AllegraBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}

func TestAllegraBlock_Validation(t *testing.T) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AllegraBlockHex))
	assert.NoError(t, err, "Failed to decode block hex")

	// Test that validation works by default (should pass for valid block)
	block, err := allegra.NewAllegraBlockFromCbor(blockCbor)
	assert.NoError(t, err, "Failed to parse and validate block")
	assert.NotNil(t, block, "Parsed block is nil")
}
