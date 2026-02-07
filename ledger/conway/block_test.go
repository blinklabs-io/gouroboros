// Copyright 2024 Blink Labs Software
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

package conway_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

func TestConwayBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.ConwayBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Conway block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into ConwayBlock struct
	var block conway.ConwayBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into ConwayBlock: %v", err)
	}
	// Reset stored CBOR to nil
	block.SetCbor(nil)

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal ConwayBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from ConwayBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Conway block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
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

func TestConwayBlockUtxorpc(t *testing.T) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ConwayBlockHex))
	if err != nil {
		t.Fatalf("failed to decode test block hex: %v", err)
	}

	block, err := conway.NewConwayBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	if err != nil {
		t.Fatalf("failed to parse Conway block: %v", err)
	}

	utxoBlock, err := block.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert Conway block to utxorpc: %v", err)
	}

	if utxoBlock.Header == nil {
		t.Fatal("block header is nil")
	}

	expectedHash := block.Hash().Bytes()
	if !compareByteSlices(utxoBlock.Header.Hash, expectedHash) {
		t.Errorf(
			"block hash mismatch:\nexpected: %x\nactual: %x",
			expectedHash,
			utxoBlock.Header.Hash,
		)
	}

	if utxoBlock.Header.Height != block.BlockNumber() {
		t.Errorf(
			"block height mismatch: expected %d, got %d",
			block.BlockNumber(),
			utxoBlock.Header.Height,
		)
	}

	if utxoBlock.Header.Slot != block.SlotNumber() {
		t.Errorf(
			"block slot mismatch: expected %d, got %d",
			block.SlotNumber(),
			utxoBlock.Header.Slot,
		)
	}

	if utxoBlock.Body == nil {
		t.Fatal("block body is nil")
	}

	expectedTxCount := len(block.TransactionBodies)
	if len(utxoBlock.Body.Tx) != expectedTxCount {
		t.Errorf(
			"transaction count mismatch: expected %d, got %d",
			expectedTxCount,
			len(utxoBlock.Body.Tx),
		)
	}

	if expectedTxCount > 0 {
		firstTx := block.Transactions()[0]
		utxoFirstTx := utxoBlock.Body.Tx[0]

		expectedTxHash := firstTx.Hash().Bytes()
		if !compareByteSlices(utxoFirstTx.Hash, expectedTxHash) {
			t.Errorf(
				"first tx hash mismatch:\nexpected: %x\nactual: %x",
				expectedTxHash,
				utxoFirstTx.Hash,
			)
		}

		if len(utxoFirstTx.Inputs) != len(firstTx.Inputs()) {
			t.Errorf(
				"first tx input count mismatch: expected %d, got %d",
				len(firstTx.Inputs()),
				len(utxoFirstTx.Inputs),
			)
		}

		if len(utxoFirstTx.Outputs) != len(firstTx.Outputs()) {
			t.Errorf(
				"first tx output count mismatch: expected %d, got %d",
				len(firstTx.Outputs()),
				len(utxoFirstTx.Outputs),
			)
		}
	}
}

func compareByteSlices(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func BenchmarkConwayBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ConwayBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block conway.ConwayBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConwayBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ConwayBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block conway.ConwayBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}
