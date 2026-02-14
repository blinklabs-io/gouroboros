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

package bench

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/syn"
)

// Test key hashes for benchmarks (28 bytes each)
var (
	testKeyHash1 = common.Blake2b224{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c,
	}
	testKeyHash2 = common.Blake2b224{
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
		0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38,
		0x39, 0x3a, 0x3b, 0x3c,
	}
	testKeyHash3 = common.Blake2b224{
		0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48,
		0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50,
		0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58,
		0x59, 0x5a, 0x5b, 0x5c,
	}
)

// createPubkeyScript creates a native script requiring a specific key signature
func createPubkeyScript(keyHash common.Blake2b224) common.NativeScript {
	script := common.NativeScriptPubkey{
		Type: 0,
		Hash: keyHash[:],
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode pubkey script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal pubkey script: " + err.Error())
	}
	return ns
}

// createAllScript creates a native script requiring all nested scripts to pass
func createAllScript(scripts ...common.NativeScript) common.NativeScript {
	script := common.NativeScriptAll{
		Type:    1,
		Scripts: scripts,
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode all script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal all script: " + err.Error())
	}
	return ns
}

// createAnyScript creates a native script requiring at least one nested script
// to pass
func createAnyScript(scripts ...common.NativeScript) common.NativeScript {
	script := common.NativeScriptAny{
		Type:    2,
		Scripts: scripts,
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode any script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal any script: " + err.Error())
	}
	return ns
}

// createNofKScript creates a native script requiring N of K scripts to pass
func createNofKScript(
	n uint,
	scripts ...common.NativeScript,
) common.NativeScript {
	script := common.NativeScriptNofK{
		Type:    3,
		N:       n,
		Scripts: scripts,
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode nofk script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal nofk script: " + err.Error())
	}
	return ns
}

// createInvalidBeforeScript creates a timelock script valid at/after a slot
func createInvalidBeforeScript(slot uint64) common.NativeScript {
	script := common.NativeScriptInvalidBefore{
		Type: 4,
		Slot: slot,
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode invalid before script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal invalid before script: " + err.Error())
	}
	return ns
}

// createInvalidHereafterScript creates a timelock script valid before a slot
func createInvalidHereafterScript(slot uint64) common.NativeScript {
	script := common.NativeScriptInvalidHereafter{
		Type: 5,
		Slot: slot,
	}
	data, err := cbor.Encode(&script)
	if err != nil {
		panic("failed to encode invalid hereafter script: " + err.Error())
	}
	var ns common.NativeScript
	if err := ns.UnmarshalCBOR(data); err != nil {
		panic("failed to unmarshal invalid hereafter script: " + err.Error())
	}
	return ns
}

// createNestedScript creates a nested script structure at a given depth
func createNestedScript(depth int, scriptType string) common.NativeScript {
	if depth <= 1 {
		return createPubkeyScript(testKeyHash1)
	}

	// Create child scripts
	child1 := createNestedScript(depth-1, scriptType)
	child2 := createNestedScript(depth-1, scriptType)
	child3 := createNestedScript(depth-1, scriptType)

	switch scriptType {
	case "All":
		return createAllScript(child1, child2, child3)
	case "Any":
		return createAnyScript(child1, child2, child3)
	case "NofK":
		return createNofKScript(2, child1, child2, child3)
	default:
		return createAllScript(child1, child2, child3)
	}
}

// BenchmarkNativeScriptEvaluate benchmarks native script evaluation
func BenchmarkNativeScriptEvaluate(b *testing.B) {
	// Common benchmark parameters
	slot := uint64(1000000)
	validityStart := uint64(900000)
	validityEnd := uint64(1100000)

	// Key hashes map containing test keys
	keyHashes := map[common.Blake2b224]bool{
		testKeyHash1: true,
		testKeyHash2: true,
		testKeyHash3: true,
	}

	// Benchmark Pubkey script (single key check)
	b.Run("ScriptType_Pubkey", func(b *testing.B) {
		script := createPubkeyScript(testKeyHash1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// Benchmark All script (conjunction) with 3 pubkey scripts
	b.Run("ScriptType_All", func(b *testing.B) {
		script := createAllScript(
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// Benchmark Any script (disjunction) with 3 pubkey scripts
	b.Run("ScriptType_Any", func(b *testing.B) {
		script := createAnyScript(
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// Benchmark NofK script (threshold) with 2-of-3
	b.Run("ScriptType_NofK", func(b *testing.B) {
		script := createNofKScript(2,
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// Benchmark Timelock scripts (validity interval)
	b.Run("ScriptType_Timelock/InvalidBefore", func(b *testing.B) {
		script := createInvalidBeforeScript(validityStart)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	b.Run("ScriptType_Timelock/InvalidHereafter", func(b *testing.B) {
		script := createInvalidHereafterScript(validityEnd + 1000)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// Benchmark combined timelock script
	b.Run("ScriptType_Timelock/Combined", func(b *testing.B) {
		script := createAllScript(
			createInvalidBeforeScript(validityStart),
			createInvalidHereafterScript(validityEnd+1000),
			createPubkeyScript(testKeyHash1),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})
}

// BenchmarkNativeScriptEvaluateDepth benchmarks native script evaluation at
// various depths
func BenchmarkNativeScriptEvaluateDepth(b *testing.B) {
	slot := uint64(1000000)
	validityStart := uint64(900000)
	validityEnd := uint64(1100000)
	keyHashes := map[common.Blake2b224]bool{
		testKeyHash1: true,
		testKeyHash2: true,
		testKeyHash3: true,
	}

	depths := []int{1, 3, 5}
	scriptTypes := []string{"All", "Any", "NofK"}

	for _, scriptType := range scriptTypes {
		for _, depth := range depths {
			name := fmt.Sprintf("%s/Depth%d", scriptType, depth)
			b.Run(name, func(b *testing.B) {
				script := createNestedScript(depth, scriptType)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = script.Evaluate(
						slot,
						validityStart,
						validityEnd,
						keyHashes,
					)
				}
			})
		}
	}
}

// BenchmarkNativeScriptEvaluateWorstCase benchmarks worst-case native script
// evaluation
// where all branches must be checked
func BenchmarkNativeScriptEvaluateWorstCase(b *testing.B) {
	slot := uint64(1000000)
	validityStart := uint64(900000)
	validityEnd := uint64(1100000)

	// Only testKeyHash3 is present - forces checking all branches in Any
	keyHashes := map[common.Blake2b224]bool{
		testKeyHash3: true,
	}

	// Missing key hash for worst case
	missingKeyHash := common.Blake2b224{
		0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x99, 0x99, 0x99, 0x99,
	}

	// Any script where the matching script is last (worst case)
	b.Run("Any/MatchLast", func(b *testing.B) {
		script := createAnyScript(
			createPubkeyScript(missingKeyHash),
			createPubkeyScript(missingKeyHash),
			createPubkeyScript(testKeyHash3),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// All script where the failing script is last (worst case)
	b.Run("All/FailLast", func(b *testing.B) {
		script := createAllScript(
			createPubkeyScript(testKeyHash3),
			createPubkeyScript(testKeyHash3),
			createPubkeyScript(missingKeyHash),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	// NofK where we need to check all to find N passing
	b.Run("NofK/CheckAll", func(b *testing.B) {
		script := createNofKScript(2,
			createPubkeyScript(missingKeyHash),
			createPubkeyScript(testKeyHash3),
			createPubkeyScript(testKeyHash3),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})
}

// PlutusV3 script bytes from test vectors (a minimal "always succeeds" script)
// This is a real PlutusV3 script from the codebase tests
var testPlutusV3ScriptHex = "587f01010032323232323225333002323232323253" +
	"330073370e900118041baa0011323232533300a3370e900018059baa00513232" +
	"533300f301100214a22c6eb8c03c004c030dd50028b18069807001180600098" +
	"049baa00116300a300b0023009001300900230070013004375400229309b2b2" +
	"b9a5573aaae7955cfaba157441"

// PlutusV1 and V2 use the same test script as V3 for benchmarking purposes
// The script is CBOR-wrapped UPLC bytecode. The structure is:
// [CBOR bytestring tag] [length] [UPLC flat-encoded program]
var testPlutusV1ScriptHex = testPlutusV3ScriptHex
var testPlutusV2ScriptHex = testPlutusV3ScriptHex

// BenchmarkPlutusScriptDeserialize benchmarks Plutus script deserialization
// using plutigo
func BenchmarkPlutusScriptDeserialize(b *testing.B) {
	// Decode test script bytes
	plutusV3Bytes, err := hex.DecodeString(testPlutusV3ScriptHex)
	if err != nil {
		b.Fatalf("failed to decode PlutusV3 script hex: %v", err)
	}

	plutusV1Bytes, err := hex.DecodeString(testPlutusV1ScriptHex)
	if err != nil {
		b.Fatalf("failed to decode PlutusV1 script hex: %v", err)
	}

	plutusV2Bytes, err := hex.DecodeString(testPlutusV2ScriptHex)
	if err != nil {
		b.Fatalf("failed to decode PlutusV2 script hex: %v", err)
	}

	b.Run("PlutusV1", func(b *testing.B) {
		script := common.PlutusV1Script(plutusV1Bytes)

		// Get inner script bytes (CBOR unwrap)
		var innerScript []byte
		if _, err := cbor.Decode(script, &innerScript); err != nil {
			b.Fatalf("failed to decode CBOR wrapper: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := syn.Decode[syn.DeBruijn](innerScript)
			if err != nil {
				b.Fatalf("failed to deserialize PlutusV1 script: %v", err)
			}
		}
	})

	b.Run("PlutusV2", func(b *testing.B) {
		script := common.PlutusV2Script(plutusV2Bytes)

		// Get inner script bytes (CBOR unwrap)
		var innerScript []byte
		if _, err := cbor.Decode(script, &innerScript); err != nil {
			b.Fatalf("failed to decode CBOR wrapper: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := syn.Decode[syn.DeBruijn](innerScript)
			if err != nil {
				b.Fatalf("failed to deserialize PlutusV2 script: %v", err)
			}
		}
	})

	b.Run("PlutusV3", func(b *testing.B) {
		script := common.PlutusV3Script(plutusV3Bytes)

		// Get inner script bytes (CBOR unwrap)
		var innerScript []byte
		if _, err := cbor.Decode(script, &innerScript); err != nil {
			b.Fatalf("failed to decode CBOR wrapper: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := syn.Decode[syn.DeBruijn](innerScript)
			if err != nil {
				b.Fatalf("failed to deserialize PlutusV3 script: %v", err)
			}
		}
	})
}

// BenchmarkPlutusScriptHash benchmarks Plutus script hash computation
func BenchmarkPlutusScriptHash(b *testing.B) {
	plutusV1Bytes, err := hex.DecodeString(testPlutusV1ScriptHex)
	if err != nil {
		b.Fatalf("failed to decode PlutusV1 hex: %v", err)
	}
	plutusV3Bytes, err := hex.DecodeString(testPlutusV3ScriptHex)
	if err != nil {
		b.Fatalf("failed to decode PlutusV3 hex: %v", err)
	}

	b.Run("PlutusV1", func(b *testing.B) {
		script := common.PlutusV1Script(plutusV1Bytes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})

	b.Run("PlutusV3", func(b *testing.B) {
		script := common.PlutusV3Script(plutusV3Bytes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})
}

// BenchmarkNativeScriptHash benchmarks native script hash computation
func BenchmarkNativeScriptHash(b *testing.B) {
	b.Run("Pubkey", func(b *testing.B) {
		script := createPubkeyScript(testKeyHash1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})

	b.Run("All/Depth3", func(b *testing.B) {
		script := createNestedScript(3, "All")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})

	b.Run("Any/Depth3", func(b *testing.B) {
		script := createNestedScript(3, "Any")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})

	b.Run("NofK/Depth3", func(b *testing.B) {
		script := createNestedScript(3, "NofK")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = script.Hash()
		}
	})
}

// BenchmarkNativeScriptCBOR benchmarks native script CBOR
// serialization/deserialization
func BenchmarkNativeScriptCBOR(b *testing.B) {
	b.Run("Encode/Pubkey", func(b *testing.B) {
		script := createPubkeyScript(testKeyHash1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := cbor.Encode(&script)
			if err != nil {
				b.Fatalf("failed to encode: %v", err)
			}
		}
	})

	b.Run("Encode/All_Depth3", func(b *testing.B) {
		script := createNestedScript(3, "All")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := cbor.Encode(&script)
			if err != nil {
				b.Fatalf("failed to encode: %v", err)
			}
		}
	})

	b.Run("Decode/Pubkey", func(b *testing.B) {
		script := createPubkeyScript(testKeyHash1)
		data, err := cbor.Encode(&script)
		if err != nil {
			b.Fatalf("failed to encode script: %v", err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var ns common.NativeScript
			err := ns.UnmarshalCBOR(data)
			if err != nil {
				b.Fatalf("failed to decode: %v", err)
			}
		}
	})

	b.Run("Decode/All_Depth3", func(b *testing.B) {
		script := createNestedScript(3, "All")
		data, err := cbor.Encode(&script)
		if err != nil {
			b.Fatalf("failed to encode script: %v", err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var ns common.NativeScript
			err := ns.UnmarshalCBOR(data)
			if err != nil {
				b.Fatalf("failed to decode: %v", err)
			}
		}
	})
}
