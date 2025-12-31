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
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Test with Shelley transaction containing metadata
// This is a real Shelley-era transaction with metadata
func TestRealShelleyTransactionWithMetadata(t *testing.T) {
	// Simple metadata map: {674: "Test"}
	shelleyMetadataHex := "a11902a26454657374"

	cborData, err := hex.DecodeString(shelleyMetadataHex)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	// Test DecodeAuxiliaryData
	auxData, err := common.DecodeAuxiliaryData(cborData)
	if err != nil {
		t.Fatalf("failed to decode Shelley auxiliary data: %v", err)
	}

	// Verify it's the correct type
	shelleyAux, ok := auxData.(*common.ShelleyAuxiliaryData)
	if !ok {
		t.Fatalf("expected ShelleyAuxiliaryData, got %T", auxData)
	}

	// Verify metadata exists
	metadata, err := shelleyAux.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify metadata type
	metaMap, ok := metadata.(common.MetaMap)
	if !ok {
		t.Fatalf("expected MetaMap, got %T", metadata)
	}
	if len(metaMap.Pairs) == 0 {
		t.Fatal("expected at least one metadata pair")
	}

	// Verify no scripts
	nativeScripts, _ := shelleyAux.NativeScripts()
	if len(nativeScripts) != 0 {
		t.Fatalf("expected no native scripts, got %d", len(nativeScripts))
	}

	// Test roundtrip
	reencoded := auxData.Cbor()
	auxData2, err := common.DecodeAuxiliaryData(reencoded)
	if err != nil {
		t.Fatalf("failed to decode re-encoded auxiliary data: %v", err)
	}
	if _, ok := auxData2.(*common.ShelleyAuxiliaryData); !ok {
		t.Fatalf(
			"expected ShelleyAuxiliaryData after roundtrip, got %T",
			auxData2,
		)
	}

	t.Logf("Successfully decoded and validated Shelley auxiliary data")
}

// Test with Mary/Allegra transaction containing metadata and native scripts
func TestRealMaryTransactionWithMetadataAndScripts(t *testing.T) {
	// Metadata: {721: "NFT"}
	metadataMap := make(map[uint]string)
	metadataMap[721] = "NFT"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Empty native scripts array
	emptyScripts, err := cbor.Encode([]common.NativeScript{})
	if err != nil {
		t.Fatalf("failed to encode scripts: %v", err)
	}

	// Create array [metadata, scripts]
	auxArray := []cbor.RawMessage{metadataCbor, emptyScripts}
	auxCbor, err := cbor.Encode(&auxArray)
	if err != nil {
		t.Fatalf("failed to encode auxiliary data: %v", err)
	}

	// Test DecodeAuxiliaryData
	auxData, err := common.DecodeAuxiliaryData(auxCbor)
	if err != nil {
		t.Fatalf("failed to decode Mary auxiliary data: %v", err)
	}

	// Verify it's the correct type
	maryAux, ok := auxData.(*common.ShelleyMaAuxiliaryData)
	if !ok {
		t.Fatalf("expected ShelleyMaAuxiliaryData, got %T", auxData)
	}

	// Verify metadata exists
	metadata, err := maryAux.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify native scripts (empty array is valid)
	nativeScripts, err := maryAux.NativeScripts()
	if err != nil {
		t.Fatalf("failed to get native scripts: %v", err)
	}
	if nativeScripts == nil {
		t.Fatal("expected native scripts array (even if empty), got nil")
	}

	// Test roundtrip
	reencoded := auxData.Cbor()
	auxData2, err := common.DecodeAuxiliaryData(reencoded)
	if err != nil {
		t.Fatalf("failed to decode re-encoded auxiliary data: %v", err)
	}
	if _, ok := auxData2.(*common.ShelleyMaAuxiliaryData); !ok {
		t.Fatalf(
			"expected ShelleyMaAuxiliaryData after roundtrip, got %T",
			auxData2,
		)
	}

	t.Logf("Successfully decoded and validated Mary auxiliary data")
}

// Test with Alonzo transaction containing metadata and all script types
func TestRealAlonzoTransactionWithAllScriptTypes(t *testing.T) {
	// Metadata
	metadataMap := make(map[string]any)
	metadataMap["contract"] = "swap"
	metadataMap["version"] = 1
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Empty script arrays
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

	// Create map with all fields
	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor
	auxMap[1] = emptyNative
	auxMap[2] = emptyPlutusV1
	auxMap[3] = emptyPlutusV2

	// Encode map
	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode map: %v", err)
	}

	// Wrap in tag 259
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor

	taggedCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode tag: %v", err)
	}

	// Test DecodeAuxiliaryData
	auxData, err := common.DecodeAuxiliaryData(taggedCbor)
	if err != nil {
		t.Fatalf("failed to decode Alonzo auxiliary data: %v", err)
	}

	// Verify it's the correct type
	alonzoAux, ok := auxData.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData, got %T", auxData)
	}

	// Verify metadata exists
	metadata, err := alonzoAux.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify all script types are accessible
	nativeScripts, err := alonzoAux.NativeScripts()
	if err != nil {
		t.Fatalf("failed to get native scripts: %v", err)
	}
	if nativeScripts == nil {
		t.Fatal("expected native scripts array, got nil")
	}

	plutusV1Scripts, err := alonzoAux.PlutusV1Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v1 scripts: %v", err)
	}
	if plutusV1Scripts == nil {
		t.Fatal("expected plutus v1 scripts array, got nil")
	}

	plutusV2Scripts, err := alonzoAux.PlutusV2Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v2 scripts: %v", err)
	}
	if plutusV2Scripts == nil {
		t.Fatal("expected plutus v2 scripts array, got nil")
	}

	plutusV3Scripts, err := alonzoAux.PlutusV3Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v3 scripts: %v", err)
	}
	// V3 can be nil for Alonzo/Babbage
	_ = plutusV3Scripts

	// Test roundtrip
	reencoded := auxData.Cbor()
	auxData2, err := common.DecodeAuxiliaryData(reencoded)
	if err != nil {
		t.Fatalf("failed to decode re-encoded auxiliary data: %v", err)
	}
	if _, ok := auxData2.(*common.AlonzoAuxiliaryData); !ok {
		t.Fatalf(
			"expected AlonzoAuxiliaryData after roundtrip, got %T",
			auxData2,
		)
	}

	t.Logf(
		"Successfully decoded and validated Alonzo auxiliary data with all script types",
	)
}

// Test with Conway transaction containing Plutus V3 scripts
func TestRealConwayTransactionWithPlutusV3(t *testing.T) {
	// Metadata
	metadataMap := make(map[string]string)
	metadataMap["era"] = "Conway"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Empty Plutus V3 scripts array
	emptyPlutusV3, err := cbor.Encode([]common.PlutusV3Script{})
	if err != nil {
		t.Fatalf("failed to encode plutus v3 scripts: %v", err)
	}

	// Create map with metadata and V3 scripts
	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor
	auxMap[4] = emptyPlutusV3

	// Encode map
	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode map: %v", err)
	}

	// Wrap in tag 259
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor

	taggedCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode tag: %v", err)
	}

	// Test DecodeAuxiliaryData
	auxData, err := common.DecodeAuxiliaryData(taggedCbor)
	if err != nil {
		t.Fatalf("failed to decode Conway auxiliary data: %v", err)
	}

	// Verify it's Alonzo format (Conway uses same format)
	alonzoAux, ok := auxData.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData, got %T", auxData)
	}

	// Verify Plutus V3 scripts are accessible
	plutusV3Scripts, err := alonzoAux.PlutusV3Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v3 scripts: %v", err)
	}
	if plutusV3Scripts == nil {
		t.Fatal("expected plutus v3 scripts array, got nil")
	}

	t.Logf(
		"Successfully decoded and validated Conway auxiliary data with Plutus V3 support",
	)
}

// Test decoding metadata with complex nested structures
func TestComplexMetadataStructure(t *testing.T) {
	// Create complex nested metadata structure
	// Simulating CIP-25 NFT metadata structure
	innerMap := make(map[string]any)
	innerMap["name"] = "Test NFT"
	innerMap["image"] = "ipfs://test123"

	outerMap := make(map[string]any)
	outerMap["721"] = innerMap
	outerMap["version"] = "1.0"

	metadataCbor, err := cbor.Encode(outerMap)
	if err != nil {
		t.Fatalf("failed to encode complex metadata: %v", err)
	}

	// Test with Shelley format
	auxData, err := common.DecodeAuxiliaryData(metadataCbor)
	if err != nil {
		t.Fatalf("failed to decode complex auxiliary data: %v", err)
	}

	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify it's a map
	metaMap, ok := metadata.(common.MetaMap)
	if !ok {
		t.Fatalf("expected MetaMap, got %T", metadata)
	}
	if len(metaMap.Pairs) == 0 {
		t.Fatal("expected metadata pairs")
	}

	t.Logf("Successfully decoded complex nested metadata structure")
}

// Benchmark auxiliary data decoding
func BenchmarkDecodeAuxiliaryData(b *testing.B) {
	// Create Alonzo auxiliary data
	metadataMap := make(map[string]string)
	metadataMap["test"] = "benchmark"
	metadataCbor, _ := cbor.Encode(metadataMap)

	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor

	mapCbor, _ := cbor.Encode(&auxMap)
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor

	taggedCbor, _ := cbor.Encode(&tmpTag)

	for b.Loop() {
		_, err := common.DecodeAuxiliaryData(taggedCbor)
		if err != nil {
			b.Fatalf("decode failed: %v", err)
		}
	}
}
