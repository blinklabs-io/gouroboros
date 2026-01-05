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

package common_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestE2EShelleyTransactionAuxiliaryData(t *testing.T) {
	// Minimal body (just inputs and outputs)
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	// Empty witness set
	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	// Metadata
	metadataMap := make(map[uint]string)
	metadataMap[674] = "Shelley Test"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Create transaction array
	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, metadataCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	// Decode transaction
	var tx shelley.ShelleyTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	// Verify auxiliary data is accessible
	auxData := tx.AuxiliaryData()
	if auxData == nil {
		t.Fatal("expected auxiliary data, got nil")
	}

	// Verify it's Shelley format
	if _, ok := auxData.(*common.ShelleyAuxiliaryData); !ok {
		t.Fatalf("expected ShelleyAuxiliaryData, got %T", auxData)
	}

	// Verify metadata
	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify backward compatibility - old Metadata() method still works
	oldMetadata := tx.Metadata()
	if oldMetadata == nil {
		t.Fatal("backward compatibility broken: Metadata() returned nil")
	}

	t.Logf("Successfully decoded Shelley transaction with auxiliary data")
}

func TestE2EMaryTransactionAuxiliaryData(t *testing.T) {
	// Minimal body
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	// Empty witness set
	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	// Shelley-MA auxiliary data: [metadata, scripts]
	metadataMap := make(map[uint]string)
	metadataMap[721] = "NFT"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	emptyScripts, err := cbor.Encode([]common.NativeScript{})
	if err != nil {
		t.Fatalf("failed to encode scripts: %v", err)
	}

	auxArray := []cbor.RawMessage{metadataCbor, emptyScripts}
	auxCbor, err := cbor.Encode(&auxArray)
	if err != nil {
		t.Fatalf("failed to encode auxiliary data: %v", err)
	}

	// Create transaction array
	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, auxCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	// Decode transaction
	var tx mary.MaryTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	// Verify auxiliary data is accessible
	auxData := tx.AuxiliaryData()
	if auxData == nil {
		t.Fatal("expected auxiliary data, got nil")
	}

	// Verify it's Shelley-MA format
	if _, ok := auxData.(*common.ShelleyMaAuxiliaryData); !ok {
		t.Fatalf("expected ShelleyMaAuxiliaryData, got %T", auxData)
	}

	// Verify both metadata and scripts are accessible
	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	nativeScripts, err := auxData.NativeScripts()
	if err != nil {
		t.Fatalf("failed to get native scripts: %v", err)
	}
	if nativeScripts == nil {
		t.Fatal("expected native scripts array, got nil")
	}

	t.Logf("Successfully decoded Mary transaction with auxiliary data")
}

func TestE2EAlonzoTransactionAuxiliaryData(t *testing.T) {
	// Minimal body
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	// Empty witness set
	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	// Is valid flag
	isValidCbor, err := cbor.Encode(true)
	if err != nil {
		t.Fatalf("failed to encode is_valid: %v", err)
	}

	// Alonzo auxiliary data: #6.259({0: metadata, 1: native, 2: plutusV1, 3: plutusV2})
	metadataMap := make(map[string]string)
	metadataMap["contract"] = "dex"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	emptyNative, err := cbor.Encode([]common.NativeScript{})
	if err != nil {
		t.Fatalf("failed to encode native scripts: %v", err)
	}

	emptyPlutusV1, err := cbor.Encode([]common.PlutusV1Script{})
	if err != nil {
		t.Fatalf("failed to encode plutus v1 scripts: %v", err)
	}

	emptyPlutusV2, err := cbor.Encode([]common.PlutusV2Script{})
	if err != nil {
		t.Fatalf("failed to encode plutus v2 scripts: %v", err)
	}

	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor
	auxMap[1] = emptyNative
	auxMap[2] = emptyPlutusV1
	auxMap[3] = emptyPlutusV2

	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode aux map: %v", err)
	}

	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor
	auxCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode auxiliary data: %v", err)
	}

	// Create transaction array
	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, isValidCbor, auxCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	// Decode transaction
	var tx alonzo.AlonzoTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	// Verify auxiliary data is accessible
	auxData := tx.AuxiliaryData()
	if auxData == nil {
		t.Fatal("expected auxiliary data, got nil")
	}

	// Verify it's Alonzo format
	alonzoAux, ok := auxData.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData, got %T", auxData)
	}

	// Verify all script types are accessible
	metadata, _ := alonzoAux.Metadata()
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	nativeScripts, _ := alonzoAux.NativeScripts()
	if nativeScripts == nil {
		t.Fatal("expected native scripts, got nil")
	}

	plutusV1Scripts, _ := alonzoAux.PlutusV1Scripts()
	if plutusV1Scripts == nil {
		t.Fatal("expected plutus v1 scripts, got nil")
	}

	plutusV2Scripts, _ := alonzoAux.PlutusV2Scripts()
	if plutusV2Scripts == nil {
		t.Fatal("expected plutus v2 scripts, got nil")
	}

	t.Logf("Successfully decoded Alonzo transaction with auxiliary data")
}

func TestE2EBabbageTransactionAuxiliaryData(t *testing.T) {
	// Minimal body
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	isValidCbor, err := cbor.Encode(true)
	if err != nil {
		t.Fatalf("failed to encode is_valid: %v", err)
	}

	// Simple metadata
	metadataMap := make(map[uint]string)
	metadataMap[100] = "Babbage"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor

	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode aux map: %v", err)
	}
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor
	auxCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode auxiliary data: %v", err)
	}

	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, isValidCbor, auxCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	var tx babbage.BabbageTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	auxData := tx.AuxiliaryData()
	if auxData == nil {
		t.Fatal("expected auxiliary data, got nil")
	}

	metadata, _ := auxData.Metadata()
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	t.Logf("Successfully decoded Babbage transaction with auxiliary data")
}

func TestE2EConwayTransactionAuxiliaryData(t *testing.T) {
	// Minimal body
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	isValidCbor, err := cbor.Encode(true)
	if err != nil {
		t.Fatalf("failed to encode is_valid: %v", err)
	}

	// Metadata with Plutus V3
	metadataMap := make(map[uint]string)
	metadataMap[200] = "Conway"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	emptyPlutusV3, err := cbor.Encode([]common.PlutusV3Script{})
	if err != nil {
		t.Fatalf("failed to encode plutus v3 scripts: %v", err)
	}

	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor
	auxMap[4] = emptyPlutusV3 // Key 4 is Plutus V3

	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode aux map: %v", err)
	}
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor
	auxCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode auxiliary data: %v", err)
	}

	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, isValidCbor, auxCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	var tx conway.ConwayTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	auxData := tx.AuxiliaryData()
	if auxData == nil {
		t.Fatal("expected auxiliary data, got nil")
	}

	alonzoAux, ok := auxData.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData, got %T", auxData)
	}

	// Verify Plutus V3 is accessible
	plutusV3Scripts, _ := alonzoAux.PlutusV3Scripts()
	if plutusV3Scripts == nil {
		t.Fatal("expected plutus v3 scripts, got nil")
	}

	t.Logf("Successfully decoded Conway transaction with Plutus V3 support")
}

// Test that transactions without auxiliary data work correctly
func TestE2ETransactionWithoutAuxiliaryData(t *testing.T) {
	// Minimal body
	bodyMap := make(map[uint]any)
	bodyMap[0] = []any{}   // inputs
	bodyMap[1] = []any{}   // outputs
	bodyMap[2] = uint64(0) // fee
	bodyCbor, err := cbor.Encode(bodyMap)
	if err != nil {
		t.Fatalf("failed to encode body: %v", err)
	}

	witnessMap := make(map[uint]any)
	witnessCbor, err := cbor.Encode(witnessMap)
	if err != nil {
		t.Fatalf("failed to encode witness: %v", err)
	}

	isValidCbor, err := cbor.Encode(true)
	if err != nil {
		t.Fatalf("failed to encode is_valid: %v", err)
	}

	// CBOR null for no auxiliary data
	nullCbor, err := hex.DecodeString("f6")
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	txArray := []cbor.RawMessage{bodyCbor, witnessCbor, isValidCbor, nullCbor}
	txCbor, err := cbor.Encode(&txArray)
	if err != nil {
		t.Fatalf("failed to encode transaction: %v", err)
	}

	var tx alonzo.AlonzoTransaction
	if _, err := cbor.Decode(txCbor, &tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}

	// Verify auxiliary data is nil
	auxData := tx.AuxiliaryData()
	if auxData != nil {
		t.Fatalf("expected nil auxiliary data, got %T", auxData)
	}

	// Verify old methods also return nil
	if tx.Metadata() != nil {
		t.Fatal("expected nil metadata")
	}

	t.Logf("✓ E2E: Successfully handled transaction without auxiliary data")
}
