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

package shelley_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestShelleyBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.ShelleyBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Shelley block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into ShelleyBlock struct
	var block shelley.ShelleyBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into ShelleyBlock: %v", err)
	}
	// Reset stored CBOR to nil
	block.SetCbor(nil)

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal ShelleyBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from ShelleyBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Shelley block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
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

func TestShelleyBlockUtxorpc(t *testing.T) {
	// Decode the test block CBOR
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ShelleyBlockHex))
	if err != nil {
		t.Fatalf("failed to decode block hex: %v", err)
	}

	block, err := shelley.NewShelleyBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	if err != nil {
		t.Fatalf("failed to parse block: %v", err)
	}

	// Convert to utxorpc format
	utxoBlock, err := block.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert to utxorpc: %v", err)
	}

	if utxoBlock.Header == nil {
		t.Error("block header is nil")
	}
	if utxoBlock.Body == nil {
		t.Error("block body is nil")
	}

	expectedTxCount := len(block.TransactionBodies)
	if len(utxoBlock.Body.Tx) != expectedTxCount {
		t.Errorf(
			"expected %d transactions, got %d",
			expectedTxCount,
			len(utxoBlock.Body.Tx),
		)
	}

	if utxoBlock.Header.Height != block.BlockNumber() {
		t.Errorf(
			"height mismatch: expected %d, got %d",
			block.BlockNumber(),
			utxoBlock.Header.Height,
		)
	}
	if utxoBlock.Header.Slot != block.SlotNumber() {
		t.Errorf(
			"slot mismatch: expected %d, got %d",
			block.SlotNumber(),
			utxoBlock.Header.Slot,
		)
	}
}

func BenchmarkShelleyBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ShelleyBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block shelley.ShelleyBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShelleyBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ShelleyBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block shelley.ShelleyBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}
