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

// Test Shelley auxiliary data (just metadata)
func TestShelleyAuxiliaryData(t *testing.T) {
	// CBOR map with metadata: {0: "test"}
	cborHex := "a100647465737421"
	cborData, err := hex.DecodeString(cborHex)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	auxData, err := common.DecodeAuxiliaryData(cborData)
	if err != nil {
		t.Fatalf("failed to decode Shelley auxiliary data: %v", err)
	}

	// Verify it's Shelley format
	_, ok := auxData.(*common.ShelleyAuxiliaryData)
	if !ok {
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

	// Verify no scripts
	nativeScripts, err := auxData.NativeScripts()
	if err != nil {
		t.Fatalf("failed to get native scripts: %v", err)
	}
	if len(nativeScripts) != 0 {
		t.Fatalf("expected no native scripts, got %d", len(nativeScripts))
	}

	plutusV1Scripts, err := auxData.PlutusV1Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v1 scripts: %v", err)
	}
	if len(plutusV1Scripts) != 0 {
		t.Fatalf("expected no plutus v1 scripts, got %d", len(plutusV1Scripts))
	}
}

// Test Shelley-MA auxiliary data (metadata + native scripts)
func TestShelleyMaAuxiliaryData(t *testing.T) {
	// CBOR array: [metadata, [native_script]]
	// Simplified: [{"test": 42}, []]
	cborHex := "82a164746573741828f680"
	cborData, err := hex.DecodeString(cborHex)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	auxData, err := common.DecodeAuxiliaryData(cborData)
	if err != nil {
		t.Fatalf("failed to decode Shelley-MA auxiliary data: %v", err)
	}

	// Verify it's Shelley-MA format
	_, ok := auxData.(*common.ShelleyMaAuxiliaryData)
	if !ok {
		t.Fatalf("expected ShelleyMaAuxiliaryData, got %T", auxData)
	}

	// Verify metadata
	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Verify native scripts (empty array in this case)
	nativeScripts, err := auxData.NativeScripts()
	if err != nil {
		t.Fatalf("failed to get native scripts: %v", err)
	}
	// Empty array is fine, just check it's not erroring
	_ = nativeScripts
}

// Test Alonzo auxiliary data (tag 259 with all script types)
func TestAlonzoAuxiliaryData(t *testing.T) {
	// CBOR tag 259 with map: #6.259({0: metadata})
	// Simplified: #6.259({0: {"key": "value"}})
	cborHex := "d90103a100a1636b6579656576616c7565"
	cborData, err := hex.DecodeString(cborHex)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	auxData, err := common.DecodeAuxiliaryData(cborData)
	if err != nil {
		t.Fatalf("failed to decode Alonzo auxiliary data: %v", err)
	}

	// Verify it's Alonzo format
	_, ok := auxData.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData, got %T", auxData)
	}

	// Verify metadata
	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}
}

// Test Alonzo auxiliary data with multiple script types
func TestAlonzoAuxiliaryDataWithScripts(t *testing.T) {
	// Encode metadata
	metadataMap := make(map[string]string)
	metadataMap["test"] = "value"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Encode empty script arrays
	emptyNativeScripts, err := cbor.Encode([]common.NativeScript{})
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

	// Create raw message map
	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor
	auxMap[1] = emptyNativeScripts
	auxMap[2] = emptyPlutusV1
	auxMap[3] = emptyPlutusV2

	// Encode the map
	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode map: %v", err)
	}

	// Wrap in tag 259 using RawTag
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor

	taggedCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode tag: %v", err)
	}

	auxData, err := common.DecodeAuxiliaryData(taggedCbor)
	if err != nil {
		t.Fatalf("failed to decode Alonzo auxiliary data: %v", err)
	}

	// Verify it's Alonzo format
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
	// Empty array is fine
	_ = nativeScripts

	plutusV1Scripts, err := alonzoAux.PlutusV1Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v1 scripts: %v", err)
	}
	_ = plutusV1Scripts

	plutusV2Scripts, err := alonzoAux.PlutusV2Scripts()
	if err != nil {
		t.Fatalf("failed to get plutus v2 scripts: %v", err)
	}
	_ = plutusV2Scripts
}

// Test encoding and decoding roundtrip for Shelley
func TestShelleyAuxiliaryDataRoundtrip(t *testing.T) {
	// Create metadata
	metadataMap := make(map[string]string)
	metadataMap["key"] = "value"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Decode as auxiliary data
	auxData, err := common.DecodeAuxiliaryData(metadataCbor)
	if err != nil {
		t.Fatalf("failed to decode auxiliary data: %v", err)
	}

	// Re-encode
	reencoded := auxData.Cbor()
	if len(reencoded) == 0 {
		t.Fatal("expected non-empty CBOR")
	}

	// Decode again and verify
	auxData2, err := common.DecodeAuxiliaryData(reencoded)
	if err != nil {
		t.Fatalf("failed to decode re-encoded auxiliary data: %v", err)
	}

	metadata1, _ := auxData.Metadata()
	metadata2, _ := auxData2.Metadata()
	if metadata1 == nil || metadata2 == nil {
		t.Fatal("expected metadata in both decodings")
	}
}

// Test encoding and decoding roundtrip for Alonzo
func TestAlonzoAuxiliaryDataRoundtrip(t *testing.T) {
	// Create auxiliary data with metadata
	metadataMap := make(map[string]string)
	metadataMap["test"] = "roundtrip"
	metadataCbor, err := cbor.Encode(metadataMap)
	if err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	// Create raw message map
	auxMap := make(map[uint]cbor.RawMessage)
	auxMap[0] = metadataCbor

	mapCbor, err := cbor.Encode(&auxMap)
	if err != nil {
		t.Fatalf("failed to encode map: %v", err)
	}

	// Wrap in tag 259 using RawTag
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapCbor

	taggedCbor, err := cbor.Encode(&tmpTag)
	if err != nil {
		t.Fatalf("failed to encode tag: %v", err)
	}

	// Decode
	auxData, err := common.DecodeAuxiliaryData(taggedCbor)
	if err != nil {
		t.Fatalf("failed to decode auxiliary data: %v", err)
	}

	// Re-encode
	reencoded := auxData.Cbor()
	if len(reencoded) == 0 {
		t.Fatal("expected non-empty CBOR")
	}

	// Decode again and verify
	auxData2, err := common.DecodeAuxiliaryData(reencoded)
	if err != nil {
		t.Fatalf("failed to decode re-encoded auxiliary data: %v", err)
	}

	_, ok := auxData2.(*common.AlonzoAuxiliaryData)
	if !ok {
		t.Fatalf("expected AlonzoAuxiliaryData after roundtrip, got %T", auxData2)
	}
}

// Test error handling for invalid auxiliary data
func TestInvalidAuxiliaryData(t *testing.T) {
	testCases := []struct {
		name    string
		cborHex string
	}{
		{
			name:    "empty data",
			cborHex: "",
		},
		{
			name:    "invalid CBOR",
			cborHex: "ff",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cborData []byte
			var err error
			if tc.cborHex != "" {
				cborData, err = hex.DecodeString(tc.cborHex)
				if err != nil {
					t.Fatalf("failed to decode hex: %v", err)
				}
			}

			_, err = common.DecodeAuxiliaryData(cborData)
			if err == nil {
				t.Fatal("expected error for invalid auxiliary data")
			}
		})
	}
}

// Test auxiliary data with null metadata
func TestAuxiliaryDataNullMetadata(t *testing.T) {
	// Shelley-MA format with null metadata: [null, []]
	cborHex := "82f680"
	cborData, err := hex.DecodeString(cborHex)
	if err != nil {
		t.Fatalf("failed to decode hex: %v", err)
	}

	auxData, err := common.DecodeAuxiliaryData(cborData)
	if err != nil {
		t.Fatalf("failed to decode auxiliary data: %v", err)
	}

	metadata, err := auxData.Metadata()
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if metadata != nil {
		t.Fatalf("expected nil metadata, got %v", metadata)
	}
}
