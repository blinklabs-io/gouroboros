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

package babbage_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestBabbageBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.BabbageBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Babbage block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into BabbageBlock struct
	var block babbage.BabbageBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into BabbageBlock: %v", err)
	}
	// Reset stored CBOR to nil
	block.SetCbor(nil)

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal BabbageBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from BabbageBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Babbage block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
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

func TestBabbageBlock_Utxorpc(t *testing.T) {
	// Decode the test block CBOR
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.BabbageBlockHex))
	if err != nil {
		t.Fatalf("failed to decode block hex: %v", err)
	}

	block, err := babbage.NewBabbageBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	if err != nil {
		t.Fatalf("failed to parse block: %v", err)
	}

	// Convert to utxorpc format
	utxoBlock, err := block.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert block to utxorpc: %v", err)
	}

	if utxoBlock.Header == nil {
		t.Fatal("block header is nil")
	}

	expectedHash := "db19fcfaba30607e363113b0a13616e6a9da5aa48b86ec2c033786f0a2e13f7d"
	hashBytes, err := hex.DecodeString(expectedHash)
	if err != nil {
		t.Fatalf("failed to decode expected hash: %v", err)
	}

	if !bytes.Equal(utxoBlock.Header.Hash, hashBytes) {
		t.Errorf(
			"unexpected block hash: got %x, want %x",
			utxoBlock.Header.Hash,
			hashBytes,
		)
	}

	// Verify block number matches what's in the header body
	if utxoBlock.Header.Height != block.BlockHeader.Body.BlockNumber {
		t.Errorf(
			"unexpected block height: got %d, want %d",
			utxoBlock.Header.Height,
			block.BlockHeader.Body.BlockNumber,
		)
	}

	// Verify slot number matches what's in the header body
	if utxoBlock.Header.Slot != block.BlockHeader.Body.Slot {
		t.Errorf(
			"unexpected block slot: got %d, want %d",
			utxoBlock.Header.Slot,
			block.BlockHeader.Body.Slot,
		)
	}

	// Verify transactions
	if len(utxoBlock.Body.Tx) != len(block.TransactionBodies) {
		t.Errorf(
			"unexpected transaction count: got %d, want %d",
			len(utxoBlock.Body.Tx),
			len(block.TransactionBodies),
		)
	}

	// Verify the first transaction as a sample
	if len(utxoBlock.Body.Tx) > 0 {
		tx := utxoBlock.Body.Tx[0]
		if tx == nil {
			t.Fatal("first transaction is nil")
		}

		if len(tx.Inputs) != len(block.TransactionBodies[0].TxInputs.Items()) {
			t.Errorf(
				"unexpected input count in first tx: got %d, want %d",
				len(tx.Inputs),
				len(block.TransactionBodies[0].TxInputs.Items()),
			)
		}

		if len(tx.Outputs) != len(block.TransactionBodies[0].TxOutputs) {
			t.Errorf(
				"unexpected output count in first tx: got %d, want %d",
				len(tx.Outputs),
				len(block.TransactionBodies[0].TxOutputs),
			)
		}
	}
}

func BenchmarkBabbageBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.BabbageBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block babbage.BabbageBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBabbageBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.BabbageBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block babbage.BabbageBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}
