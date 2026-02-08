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

package alonzo_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

//

func TestAlonzoBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.AlonzoBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Alonzo block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into AlonzoBlock struct
	var block alonzo.AlonzoBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into AlonzoBlock: %v", err)
	}

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal AlonzoBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from AlonzoBlock is empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Alonzo block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
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

func TestAlonzoBlock_Utxorpc(t *testing.T) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AlonzoBlockHex))
	assert.NoError(t, err, "Failed to decode block hex")

	// First validate we can parse the block
	block, err := alonzo.NewAlonzoBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	assert.NoError(t, err, "Failed to parse block")
	assert.NotNil(t, block, "Parsed block is nil")

	assert.NotEmpty(t, block.Hash(), "Block hash should not be empty")
	assert.Greater(
		t,
		block.BlockNumber(),
		uint64(0),
		"Block number should be positive",
	)
	assert.Greater(
		t,
		block.SlotNumber(),
		uint64(0),
		"Slot number should be positive",
	)
	assert.Greater(
		t,
		len(block.Transactions()),
		0,
		"Block should contain transactions",
	)

	t.Run("UtxorpcConversion", func(t *testing.T) {
		pbBlock, err := block.Utxorpc()
		if err != nil {
			t.Fatalf("Utxorpc conversion failed: %v", err)
		}

		assert.NotNil(t, pbBlock, "Converted block should not be nil")
		assert.Equal(
			t,
			block.Hash().Bytes(),
			pbBlock.Header.Hash,
			"Block hash mismatch",
		)
		assert.Equal(
			t,
			block.BlockNumber(),
			pbBlock.Header.Height,
			"Block height mismatch",
		)
		assert.Equal(
			t,
			block.SlotNumber(),
			pbBlock.Header.Slot,
			"Block slot mismatch",
		)
		assert.Equal(
			t,
			len(block.Transactions()),
			len(pbBlock.Body.Tx),
			"Transaction count mismatch",
		)
	})
}

func BenchmarkAlonzoBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AlonzoBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block alonzo.AlonzoBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAlonzoBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AlonzoBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block alonzo.AlonzoBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}

func TestAlonzoBlock_Validation(t *testing.T) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.AlonzoBlockHex))
	assert.NoError(t, err, "Failed to decode block hex")

	// Test that validation works by default (should pass for valid block)
	block, err := alonzo.NewAlonzoBlockFromCbor(blockCbor)
	assert.NoError(t, err, "Failed to parse and validate block")
	assert.NotNil(t, block, "Parsed block is nil")
}
