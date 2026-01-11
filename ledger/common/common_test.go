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

package common

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/plutigo/data"
)

func TestBlake2b256_MarshalCBOR_ZeroHash(t *testing.T) {
	// Test that a zero Blake2b256 encodes as a proper 32-byte bytestring, not null
	var zeroHash Blake2b256

	encoded, err := cbor.Encode(zeroHash)
	if err != nil {
		t.Fatalf("Failed to encode zero hash: %v", err)
	}

	// Verify the encoding starts with the correct CBOR bytestring tag for 32 bytes (0x58 0x20)
	if len(encoded) < 2 || encoded[0] != 0x58 || encoded[1] != 0x20 {
		t.Errorf(
			"Zero hash not encoded as 32-byte bytestring. Expected prefix 0x5820, got: %x",
			encoded[:min(4, len(encoded))],
		)
	}

	// Verify total length is 34 bytes (2 bytes for CBOR tag + 32 bytes of data)
	expectedLength := 34
	if len(encoded) != expectedLength {
		t.Errorf(
			"Expected encoded length %d, got %d",
			expectedLength,
			len(encoded),
		)
	}

	// Verify the data portion is all zeros
	dataBytes := encoded[2:] // Skip the 2-byte CBOR header
	for i, b := range dataBytes {
		if b != 0x00 {
			t.Errorf("Expected zero at data position %d, got 0x%02x", i, b)
		}
	}

	// Verify round-trip decoding works
	var decoded Blake2b256
	_, err = cbor.Decode(encoded, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode zero hash: %v", err)
	}

	if !bytes.Equal(zeroHash[:], decoded[:]) {
		t.Errorf(
			"Round-trip failed: original %x, decoded %x",
			zeroHash,
			decoded,
		)
	}
}

func TestBlake2b256_MarshalCBOR_NonZeroHash(t *testing.T) {
	// Test that a non-zero Blake2b256 encodes correctly
	nonZeroHash := Blake2b256Hash([]byte("test"))

	encoded, err := cbor.Encode(nonZeroHash)
	if err != nil {
		t.Fatalf("Failed to encode non-zero hash: %v", err)
	}

	// Verify the encoding starts with the correct CBOR bytestring tag for 32 bytes
	if len(encoded) < 2 || encoded[0] != 0x58 || encoded[1] != 0x20 {
		t.Errorf(
			"Non-zero hash not encoded as 32-byte bytestring. Expected prefix 0x5820, got: %x",
			encoded[:min(4, len(encoded))],
		)
	}

	// Verify total length is 34 bytes
	expectedLength := 34
	if len(encoded) != expectedLength {
		t.Errorf(
			"Expected encoded length %d, got %d",
			expectedLength,
			len(encoded),
		)
	}

	// Verify round-trip decoding works
	var decoded Blake2b256
	_, err = cbor.Decode(encoded, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode non-zero hash: %v", err)
	}

	if !bytes.Equal(nonZeroHash[:], decoded[:]) {
		t.Errorf(
			"Round-trip failed: original %x, decoded %x",
			nonZeroHash,
			decoded,
		)
	}
}

func TestBlake2b256_InBlockHeaderStruct(t *testing.T) {
	// Test Blake2b256 in a struct similar to a block header to ensure no null values appear
	type MockBlockHeader struct {
		cbor.StructAsArray
		ProtocolMagic uint32
		PrevHash      Blake2b256
		BodyProof     []byte
		BlockNumber   uint64
	}

	var zeroHash Blake2b256
	nonZeroHash := Blake2b256Hash([]byte("test"))

	testCases := []struct {
		name   string
		header MockBlockHeader
	}{
		{
			name: "Genesis block with zero prev hash",
			header: MockBlockHeader{
				ProtocolMagic: 764824073,
				PrevHash:      zeroHash,
				BodyProof:     []byte{0x01, 0x02, 0x03},
				BlockNumber:   0,
			},
		},
		{
			name: "Normal block with non-zero prev hash",
			header: MockBlockHeader{
				ProtocolMagic: 764824073,
				PrevHash:      nonZeroHash,
				BodyProof:     []byte{0x04, 0x05, 0x06},
				BlockNumber:   1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := cbor.Encode(tc.header)
			if err != nil {
				t.Fatalf("Failed to encode header: %v", err)
			}

			// Verify round-trip decoding works (this ensures no semantic nulls)
			var decoded MockBlockHeader
			_, err = cbor.Decode(encoded, &decoded)
			if err != nil {
				t.Fatalf("Failed to decode header: %v", err)
			}

			// Verify all fields match and are properly typed (not null)
			if decoded.ProtocolMagic != tc.header.ProtocolMagic {
				t.Errorf(
					"ProtocolMagic mismatch: expected %d, got %d",
					tc.header.ProtocolMagic,
					decoded.ProtocolMagic,
				)
			}

			if !bytes.Equal(tc.header.PrevHash[:], decoded.PrevHash[:]) {
				t.Errorf(
					"PrevHash mismatch: expected %x, got %x",
					tc.header.PrevHash,
					decoded.PrevHash,
				)
			}

			// Specifically verify zero hash is properly decoded as zero bytes, not null
			if tc.name == "Genesis block with zero prev hash" {
				for i, b := range decoded.PrevHash {
					if b != 0 {
						t.Errorf(
							"Zero PrevHash not properly decoded: expected zero at position %d, got 0x%02x",
							i,
							b,
						)
					}
				}
			}

			if !bytes.Equal(tc.header.BodyProof, decoded.BodyProof) {
				t.Errorf(
					"BodyProof mismatch: expected %x, got %x",
					tc.header.BodyProof,
					decoded.BodyProof,
				)
			}

			if decoded.BlockNumber != tc.header.BlockNumber {
				t.Errorf(
					"BlockNumber mismatch: expected %d, got %d",
					tc.header.BlockNumber,
					decoded.BlockNumber,
				)
			}
		})
	}
}

func TestBlake2b224_MarshalCBOR_ZeroHash(t *testing.T) {
	// Test that a zero Blake2b224 encodes as a proper 28-byte bytestring, not null
	var zeroHash Blake2b224

	encoded, err := cbor.Encode(zeroHash)
	if err != nil {
		t.Fatalf("Failed to encode zero Blake2b224 hash: %v", err)
	}

	// Verify the encoding starts with the correct CBOR bytestring tag for 28 bytes (0x58 0x1c)
	if len(encoded) < 2 || encoded[0] != 0x58 || encoded[1] != 0x1c {
		t.Errorf(
			"Zero Blake2b224 hash not encoded as 28-byte bytestring. Expected prefix 0x581c, got: %x",
			encoded[:min(4, len(encoded))],
		)
	}

	// Verify total length is 30 bytes (2 bytes for CBOR tag + 28 bytes of data)
	expectedLength := 30
	if len(encoded) != expectedLength {
		t.Errorf(
			"Expected encoded length %d, got %d",
			expectedLength,
			len(encoded),
		)
	}

	// Verify round-trip (ensures semantic correctness, not null)
	var decoded Blake2b224
	_, err = cbor.Decode(encoded, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode zero Blake2b224 hash: %v", err)
	}

	if !bytes.Equal(zeroHash[:], decoded[:]) {
		t.Errorf(
			"Round-trip failed: original %x, decoded %x",
			zeroHash,
			decoded,
		)
	}
}

func TestBlake2b160_MarshalCBOR_ZeroHash(t *testing.T) {
	// Test that a zero Blake2b160 encodes as a proper 20-byte bytestring, not null
	var zeroHash Blake2b160

	encoded, err := cbor.Encode(zeroHash)
	if err != nil {
		t.Fatalf("Failed to encode zero Blake2b160 hash: %v", err)
	}

	// Verify the encoding starts with the correct CBOR bytestring tag for 20 bytes (0x54)
	if len(encoded) < 1 || encoded[0] != 0x54 {
		t.Errorf(
			"Zero Blake2b160 hash not encoded as 20-byte bytestring. Expected prefix 0x54, got: %x",
			encoded[:min(2, len(encoded))],
		)
	}

	// Verify total length is 21 bytes (1 byte for CBOR tag + 20 bytes of data)
	expectedLength := 21
	if len(encoded) != expectedLength {
		t.Errorf(
			"Expected encoded length %d, got %d",
			expectedLength,
			len(encoded),
		)
	}

	// Verify round-trip (ensures semantic correctness, not null)
	var decoded Blake2b160
	_, err = cbor.Decode(encoded, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode zero Blake2b160 hash: %v", err)
	}

	if !bytes.Equal(zeroHash[:], decoded[:]) {
		t.Errorf(
			"Round-trip failed: original %x, decoded %x",
			zeroHash,
			decoded,
		)
	}
}

// TestOriginalIssue_ZeroHashNotNull specifically tests the original issue:
// empty prev hash should be encoded as a zero-filled bytestring, not null
func TestOriginalIssue_ZeroHashNotNull(t *testing.T) {
	// Simulate the original problem scenario from the user's example
	type OriginalBlockData struct {
		cbor.StructAsArray
		Field1   int
		Field2   int
		PrevHash Blake2b256 // This was encoding as null when zero
		Hash1    Blake2b256
		Hash2    Blake2b256
	}

	var zeroHash Blake2b256
	hash1Bytes, _ := hex.DecodeString(
		"ae29c552228a08f3c563450b10a9f548f4602d598e4b78dd4d70edaf12ba548d",
	)
	hash2Bytes, _ := hex.DecodeString(
		"7f69d32c32041f11992dff6d6b9445ba3e0997a074e4c700b504b359f1ef55b4",
	)

	var hash1, hash2 Blake2b256
	copy(hash1[:], hash1Bytes)
	copy(hash2[:], hash2Bytes)

	blockData := OriginalBlockData{
		Field1:   0,
		Field2:   0,
		PrevHash: zeroHash, // This should NOT encode as null
		Hash1:    hash1,
		Hash2:    hash2,
	}

	encoded, err := cbor.Encode(blockData)
	if err != nil {
		t.Fatalf("Failed to encode block data: %v", err)
	}

	// The key test: verify zero PrevHash is semantically correct through decoding
	// (rather than scanning raw bytes which could false-positive on valid 0xF6 in bytestrings)

	// Verify round-trip
	var decoded OriginalBlockData
	_, err = cbor.Decode(encoded, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode block data: %v", err)
	}

	// Verify the zero hash decoded correctly (this ensures it wasn't encoded as null)
	if !bytes.Equal(blockData.PrevHash[:], decoded.PrevHash[:]) {
		t.Errorf(
			"PrevHash round-trip failed: original %x, decoded %x",
			blockData.PrevHash,
			decoded.PrevHash,
		)
	}

	// Verify all zero bytes are preserved (semantic validation that it's not null)
	for i, b := range decoded.PrevHash {
		if b != 0 {
			t.Errorf("Expected zero at PrevHash position %d, got 0x%02x", i, b)
		}
	}

	// Additional semantic check: verify the PrevHash field exists and has the expected type/size
	if len(decoded.PrevHash) != Blake2b256Size {
		t.Errorf(
			"PrevHash has wrong size: expected %d bytes, got %d",
			Blake2b256Size,
			len(decoded.PrevHash),
		)
	}

	t.Logf(
		"SUCCESS: Zero PrevHash correctly encoded as zero-filled bytestring, not null",
	)
}

func TestAssetFingerprint(t *testing.T) {
	testDefs := []struct {
		policyIdHex         string
		assetNameHex        string
		expectedFingerprint string
	}{
		// CIP-0014 test vectors
		{
			policyIdHex:         "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc373",
			assetNameHex:        "",
			expectedFingerprint: "asset1rjklcrnsdzqp65wjgrg55sy9723kw09mlgvlc3",
		},
		{
			policyIdHex:         "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc37e",
			assetNameHex:        "",
			expectedFingerprint: "asset1nl0puwxmhas8fawxp8nx4e2q3wekg969n2auw3",
		},
		{
			policyIdHex:         "1e349c9bdea19fd6c147626a5260bc44b71635f398b67c59881df209",
			assetNameHex:        "",
			expectedFingerprint: "asset1uyuxku60yqe57nusqzjx38aan3f2wq6s93f6ea",
		},
		{
			policyIdHex:         "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc373",
			assetNameHex:        "504154415445",
			expectedFingerprint: "asset13n25uv0yaf5kus35fm2k86cqy60z58d9xmde92",
		},
		{
			policyIdHex:         "1e349c9bdea19fd6c147626a5260bc44b71635f398b67c59881df209",
			assetNameHex:        "504154415445",
			expectedFingerprint: "asset1hv4p5tv2a837mzqrst04d0dcptdjmluqvdx9k3",
		},
		{
			policyIdHex:         "1e349c9bdea19fd6c147626a5260bc44b71635f398b67c59881df209",
			assetNameHex:        "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc373",
			expectedFingerprint: "asset1aqrdypg669jgazruv5ah07nuyqe0wxjhe2el6f",
		},
		{
			policyIdHex:         "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc373",
			assetNameHex:        "1e349c9bdea19fd6c147626a5260bc44b71635f398b67c59881df209",
			expectedFingerprint: "asset17jd78wukhtrnmjh3fngzasxm8rck0l2r4hhyyt",
		},
		{
			policyIdHex:         "7eae28af2208be856f7a119668ae52a49b73725e326dc16579dcc373",
			assetNameHex:        "0000000000000000000000000000000000000000000000000000000000000000",
			expectedFingerprint: "asset1pkpwyknlvul7az0xx8czhl60pyel45rpje4z8w",
		},
		// NOTE: these test defs were created from a random sampling of recent assets on cexplorer.io
		{
			policyIdHex:         "29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61",
			assetNameHex:        "6675726e697368613239686e",
			expectedFingerprint: "asset1jdu2xcrwlqsjqqjger6kj2szddz8dcpvcg4ksz",
		},
		{
			policyIdHex:         "eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5",
			assetNameHex:        "426f7764757261436f6e63657074733638",
			expectedFingerprint: "asset1kp7hdhqc7chmyqvtqrsljfdrdt6jz8mg5culpe",
		},
		{
			policyIdHex:         "cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525",
			assetNameHex:        "363438346330393264363164373033656236333233346461",
			expectedFingerprint: "asset1rx3cnlsvh3udka56wyqyed3u695zd5q2jck2yd",
		},
	}
	for _, test := range testDefs {
		policyIdBytes, err := hex.DecodeString(test.policyIdHex)
		if err != nil {
			t.Fatalf("failed to decode policy ID hex: %s", err)
		}
		assetNameBytes, err := hex.DecodeString(test.assetNameHex)
		if err != nil {
			t.Fatalf("failed to decode asset name hex: %s", err)
		}
		fp := NewAssetFingerprint(policyIdBytes, assetNameBytes)
		if fp.String() != test.expectedFingerprint {
			t.Fatalf(
				"asset fingerprint did not match expected value, got: %s, wanted: %s",
				fp.String(),
				test.expectedFingerprint,
			)
		}
	}
}

func TestMultiAssetJson(t *testing.T) {
	testDefs := []struct {
		multiAssetObj any
		expectedJson  string
	}{
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
					},
				},
			},
			expectedJson: `[{"name":"furnisha29hn","nameHex":"6675726e697368613239686e","policyId":"29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61","fingerprint":"asset1jdu2xcrwlqsjqqjger6kj2szddz8dcpvcg4ksz","amount":"123456"}]`,
		},
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
						cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e63657074733638")): big.NewInt(
							234567,
						),
					},
				},
			},
			expectedJson: `[{"name":"BowduraConcepts68","nameHex":"426f7764757261436f6e63657074733638","policyId":"eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5","fingerprint":"asset1kp7hdhqc7chmyqvtqrsljfdrdt6jz8mg5culpe","amount":"234567"}]`,
		},
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525")): {
						cbor.NewByteString(test.DecodeHexString("363438346330393264363164373033656236333233346461")): big.NewInt(
							12345678,
						),
					},
				},
			},
			expectedJson: `[{"name":"6484c092d61d703eb63234da","nameHex":"363438346330393264363164373033656236333233346461","policyId":"cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525","fingerprint":"asset1rx3cnlsvh3udka56wyqyed3u695zd5q2jck2yd","amount":"12345678"}]`,
		},
	}
	for _, test := range testDefs {
		var err error
		var jsonData []byte
		switch v := test.multiAssetObj.(type) {
		case MultiAsset[int64]:
			jsonData, err = json.Marshal(&v)
		case MultiAsset[uint64]:
			jsonData, err = json.Marshal(&v)
		case MultiAsset[*big.Int]:
			jsonData, err = json.Marshal(&v)
		default:
			t.Fatalf("unexpected test object type: %T", test.multiAssetObj)
		}
		if err != nil {
			t.Fatalf("failed to marshal MultiAsset object into JSON: %s", err)
		}
		if string(jsonData) != test.expectedJson {
			t.Fatalf(
				"MultiAsset object did not marshal into expected JSON\n  got: %s\n  wanted: %s",
				jsonData,
				test.expectedJson,
			)
		}
	}
}

func TestMultiAssetToPlutusData(t *testing.T) {
	testDefs := []struct {
		multiAssetObj any
		expectedData  data.PlutusData
	}{
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
					},
				},
			},
			expectedData: data.NewMap(
				[][2]data.PlutusData{
					{
						data.NewByteString(
							test.DecodeHexString(
								"29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61",
							),
						),
						data.NewMap(
							[][2]data.PlutusData{
								{
									data.NewByteString(
										test.DecodeHexString(
											"6675726e697368613239686e",
										),
									),
									data.NewInteger(big.NewInt(123456)),
								},
							},
						),
					},
				},
			),
		},
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
					},
					NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
						cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e63657074733638")): big.NewInt(
							234567,
						),
					},
				},
			},
			expectedData: data.NewMap(
				[][2]data.PlutusData{
					{
						data.NewByteString(
							test.DecodeHexString(
								"29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61",
							),
						),
						data.NewMap(
							[][2]data.PlutusData{
								{
									data.NewByteString(
										test.DecodeHexString(
											"6675726e697368613239686e",
										),
									),
									data.NewInteger(big.NewInt(123456)),
								},
							},
						),
					},
					{
						data.NewByteString(
							test.DecodeHexString(
								"eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5",
							),
						),
						data.NewMap(
							[][2]data.PlutusData{
								{
									data.NewByteString(
										test.DecodeHexString(
											"426f7764757261436f6e63657074733638",
										),
									),
									data.NewInteger(big.NewInt(234567)),
								},
							},
						),
					},
				},
			),
		},
	}
	for _, testDef := range testDefs {
		var tmpData data.PlutusData
		switch v := testDef.multiAssetObj.(type) {
		case MultiAsset[int64]:
			tmpData = v.ToPlutusData()
		case MultiAsset[uint64]:
			tmpData = v.ToPlutusData()
		case MultiAsset[*big.Int]:
			tmpData = v.ToPlutusData()
		default:
			t.Fatalf("test def multi-asset object was not expected type: %T", v)
		}
		if !reflect.DeepEqual(tmpData, testDef.expectedData) {
			t.Fatalf(
				"did not get expected PlutusData\n     got: %#v\n  wanted: %#v",
				tmpData,
				testDef.expectedData,
			)
		}
	}
}

func TestMultiAssetCompare(t *testing.T) {
	testDefs := []struct {
		asset1         *MultiAsset[MultiAssetTypeOutput]
		asset2         *MultiAsset[MultiAssetTypeOutput]
		expectedResult bool
	}{
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(123),
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(123),
					},
				},
			},
			expectedResult: true,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(123),
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(124),
					},
				},
			},
			expectedResult: false,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(0),
					},
				},
			},
			asset2:         nil,
			expectedResult: true,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(123),
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): big.NewInt(123),
						cbor.NewByteString([]byte("efgh")): big.NewInt(123),
					},
				},
			},
			expectedResult: false,
		},
	}
	for _, testDef := range testDefs {
		tmpResult := testDef.asset1.Compare(testDef.asset2)
		if tmpResult != testDef.expectedResult {
			t.Errorf(
				"did not get expected result: got %v, wanted %v",
				tmpResult,
				testDef.expectedResult,
			)
		}
	}
}

// Test CBOR round-trip encoding/decoding for MultiAsset
func TestMultiAssetCborRoundTrip(t *testing.T) {
	testDefs := []struct {
		name       string
		multiAsset MultiAsset[MultiAssetTypeOutput]
	}{
		{
			name: "single policy, single asset",
			multiAsset: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
					},
				},
			},
		},
		{
			name: "single policy, multiple assets",
			multiAsset: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
						cbor.NewByteString(test.DecodeHexString("746f6b656e32")): big.NewInt(
							789,
						),
					},
				},
			},
		},
		{
			name: "multiple policies",
			multiAsset: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							123456,
						),
					},
					NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
						cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e636574737638")): big.NewInt(
							234567,
						),
					},
				},
			},
		},
	}

	for _, test := range testDefs {
		t.Run(test.name, func(t *testing.T) {
			// Test regular Encode
			encoded, err := cbor.Encode(&test.multiAsset)
			if err != nil {
				t.Fatalf("Failed to encode MultiAsset: %v", err)
			}

			// Test EncodeGeneric
			encodedGeneric, err := cbor.EncodeGeneric(&test.multiAsset)
			if err != nil {
				t.Fatalf(
					"Failed to encode MultiAsset with EncodeGeneric: %v",
					err,
				)
			}

			// EncodeGeneric may differ from Encode due to custom MarshalCBOR;
			// verify that decoding EncodeGeneric also works
			var decodedGeneric MultiAsset[MultiAssetTypeOutput]
			_, err = cbor.Decode(encodedGeneric, &decodedGeneric)
			if err != nil {
				t.Fatalf("Failed to decode EncodeGeneric output: %v", err)
			}

			// Decode back
			var decoded MultiAsset[MultiAssetTypeOutput]
			_, err = cbor.Decode(encoded, &decoded)
			if err != nil {
				t.Fatalf("Failed to decode MultiAsset: %v", err)
			}

			// Re-encode
			reEncoded, err := cbor.Encode(&decoded)
			if err != nil {
				t.Fatalf("Failed to re-encode MultiAsset: %v", err)
			}

			// Check round-trip fidelity
			if !bytes.Equal(encoded, reEncoded) {
				t.Errorf(
					"CBOR round-trip failed: original and re-encoded don't match",
				)
				t.Logf("Original:   %x", encoded)
				t.Logf("Re-encoded: %x", reEncoded)
			}

			// Also check semantic equality
			if !test.multiAsset.Compare(&decoded) {
				t.Errorf("Semantic comparison failed after round-trip")
			}
		})
	}
}

// Test CBOR round-trip encoding/decoding for MultiAsset with mint (negative values)
func TestMultiAssetCborRoundTripMint(t *testing.T) {
	testDefs := []struct {
		name       string
		multiAsset MultiAsset[MultiAssetTypeMint]
	}{
		{
			name: "mint single policy, single asset (burn)",
			multiAsset: MultiAsset[MultiAssetTypeMint]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeMint{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							-50000,
						),
					},
				},
			},
		},
		{
			name: "mint single policy, mixed assets (mint and burn)",
			multiAsset: MultiAsset[MultiAssetTypeMint]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeMint{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							100000,
						),
						cbor.NewByteString(test.DecodeHexString("746f6b656e32")): big.NewInt(
							-25000,
						),
					},
				},
			},
		},
		{
			name: "mint multiple policies with negative values",
			multiAsset: MultiAsset[MultiAssetTypeMint]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeMint{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
							-75000,
						),
					},
					NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
						cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e636574737638")): big.NewInt(
							-100000,
						),
					},
				},
			},
		},
	}

	for _, test := range testDefs {
		t.Run(test.name, func(t *testing.T) {
			// Test regular Encode
			encoded, err := cbor.Encode(&test.multiAsset)
			if err != nil {
				t.Fatalf("Failed to encode MultiAsset: %v", err)
			}

			// Test EncodeGeneric
			encodedGeneric, err := cbor.EncodeGeneric(&test.multiAsset)
			if err != nil {
				t.Fatalf(
					"Failed to encode MultiAsset with EncodeGeneric: %v",
					err,
				)
			}

			// EncodeGeneric may differ from Encode due to custom MarshalCBOR;
			// verify that decoding EncodeGeneric also works
			var decodedGeneric MultiAsset[MultiAssetTypeMint]
			_, err = cbor.Decode(encodedGeneric, &decodedGeneric)
			if err != nil {
				t.Fatalf("Failed to decode EncodeGeneric output: %v", err)
			}

			// Decode back
			var decoded MultiAsset[MultiAssetTypeMint]
			_, err = cbor.Decode(encoded, &decoded)
			if err != nil {
				t.Fatalf("Failed to decode MultiAsset: %v", err)
			}

			// Re-encode
			reEncoded, err := cbor.Encode(&decoded)
			if err != nil {
				t.Fatalf("Failed to re-encode MultiAsset: %v", err)
			}

			// Check round-trip fidelity
			if !bytes.Equal(encoded, reEncoded) {
				t.Errorf(
					"CBOR round-trip failed: original and re-encoded don't match",
				)
				t.Logf("Original:   %x", encoded)
				t.Logf("Re-encoded: %x", reEncoded)
			}

			// Also check semantic equality
			if !test.multiAsset.Compare(&decoded) {
				t.Errorf("Semantic comparison failed after round-trip")
			}
		})
	}
}

// Test the MarshalJSON method for Blake2b224 to ensure it properly converts to JSON.
func TestBlake2b224_MarshalJSON(t *testing.T) {
	// Example data to represent Blake2b224 hash
	data := []byte("blinklabs")
	hash := NewBlake2b224(data)

	// Blake2b224 always produces 28 bytes (224 bits) as its output.
	// Expected JSON value: the hex-encoded string of "blinklabs" padded to fit 28 bytes.
	// JSON marshalling adds quotes around the string, so include quotes in expected value.
	expected := `"626c696e6b6c61627300000000000000000000000000000000000000"`

	// Marshal the Blake2b224 object to JSON
	jsonData, err := json.Marshal(hash)
	if err != nil {
		t.Fatalf("failed to marshal Blake2b224: %v", err)
	}

	// Compare the result with the expected output
	if string(jsonData) != expected {
		t.Errorf("expected %s but got %s", expected, string(jsonData))
	}
}

func TestBlake2b224_String(t *testing.T) {
	data := []byte("blinklabs") // Less than 28 bytes
	hash := NewBlake2b224(data)

	// Expected hex string for "blinklabs" padded/truncated to fit 28 bytes
	expected := "626c696e6b6c61627300000000000000000000000000000000000000"

	// Verify if String() gives the correct hex-encoded string
	if hash.String() != expected {
		t.Errorf("expected %s but got %s", expected, hash.String())
	}
}

func TestBlake2b224_ToPlutusData(t *testing.T) {
	testData := []byte("blinklabs")
	hash := Blake2b224Hash(testData)
	expectedHash, err := hex.DecodeString(
		"d33ef286551f50d455cfeb68b45b02622fb05ef21cfd1aabd0d7880c",
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedPd := data.NewByteString(expectedHash)
	tmpPd := hash.ToPlutusData()
	if !reflect.DeepEqual(tmpPd, expectedPd) {
		t.Fatalf(
			"did not get expected PlutusData:     got: %#v\n  wanted: %#v",
			tmpPd,
			expectedPd,
		)
	}
}

var certificateTypeTests = []struct {
	name     string
	cert     Certificate
	expected uint
}{
	{
		"StakeRegistration",
		&StakeRegistrationCertificate{
			CertType: uint(CertificateTypeStakeRegistration),
		},
		uint(CertificateTypeStakeRegistration),
	},
	{
		"StakeDeregistration",
		&StakeDeregistrationCertificate{
			CertType: uint(CertificateTypeStakeDeregistration),
		},
		uint(CertificateTypeStakeDeregistration),
	},
	{
		"StakeDelegation",
		&StakeDelegationCertificate{
			CertType: uint(CertificateTypeStakeDelegation),
		},
		uint(CertificateTypeStakeDelegation),
	},
	{
		"PoolRegistration",
		&PoolRegistrationCertificate{
			CertType: uint(CertificateTypePoolRegistration),
		},
		uint(CertificateTypePoolRegistration),
	},
	{
		"PoolRetirement",
		&PoolRetirementCertificate{
			CertType: uint(CertificateTypePoolRetirement),
		},
		uint(CertificateTypePoolRetirement),
	},
	{
		"GenesisKeyDelegation",
		&GenesisKeyDelegationCertificate{
			CertType: uint(CertificateTypeGenesisKeyDelegation),
		},
		uint(CertificateTypeGenesisKeyDelegation),
	},
	{
		"MoveInstantaneousRewards",
		&MoveInstantaneousRewardsCertificate{
			CertType: uint(CertificateTypeMoveInstantaneousRewards),
		},
		uint(CertificateTypeMoveInstantaneousRewards),
	},
	{
		"Registration",
		&RegistrationCertificate{
			CertType: uint(CertificateTypeRegistration),
		},
		uint(CertificateTypeRegistration),
	},
	{
		"Deregistration",
		&DeregistrationCertificate{
			CertType: uint(CertificateTypeDeregistration),
		},
		uint(CertificateTypeDeregistration),
	},
	{
		"VoteDelegation",
		&VoteDelegationCertificate{
			CertType: uint(CertificateTypeVoteDelegation),
		},
		uint(CertificateTypeVoteDelegation),
	},
	{
		"StakeVoteDelegation",
		&StakeVoteDelegationCertificate{
			CertType: uint(CertificateTypeStakeVoteDelegation),
		},
		uint(CertificateTypeStakeVoteDelegation),
	},
	{
		"StakeRegistrationDelegation",
		&StakeRegistrationDelegationCertificate{
			CertType: uint(CertificateTypeStakeRegistrationDelegation),
		},
		uint(CertificateTypeStakeRegistrationDelegation),
	},
	{
		"VoteRegistrationDelegation",
		&VoteRegistrationDelegationCertificate{
			CertType: uint(CertificateTypeVoteRegistrationDelegation),
		},
		uint(CertificateTypeVoteRegistrationDelegation),
	},
	{
		"StakeVoteRegistrationDelegation",
		&StakeVoteRegistrationDelegationCertificate{
			CertType: uint(CertificateTypeStakeVoteRegistrationDelegation),
		},
		uint(CertificateTypeStakeVoteRegistrationDelegation),
	},
	{
		"AuthCommitteeHot",
		&AuthCommitteeHotCertificate{
			CertType: uint(CertificateTypeAuthCommitteeHot),
		},
		uint(CertificateTypeAuthCommitteeHot),
	},
	{
		"ResignCommitteeCold",
		&ResignCommitteeColdCertificate{
			CertType: uint(CertificateTypeResignCommitteeCold),
		},
		uint(CertificateTypeResignCommitteeCold),
	},
	{
		"RegistrationDrep",
		&RegistrationDrepCertificate{
			CertType: uint(CertificateTypeRegistrationDrep),
		},
		uint(CertificateTypeRegistrationDrep),
	},
	{
		"DeregistrationDrep",
		&DeregistrationDrepCertificate{
			CertType: uint(CertificateTypeDeregistrationDrep),
		},
		uint(CertificateTypeDeregistrationDrep),
	},
	{
		"UpdateDrep",
		&UpdateDrepCertificate{CertType: uint(CertificateTypeUpdateDrep)},
		uint(CertificateTypeUpdateDrep),
	},
	{
		"LeiosEb",
		&LeiosEbCertificate{},
		uint(CertificateTypeLeiosEb),
	},
}

func TestCertificateTypeMethods(t *testing.T) {
	tests := certificateTypeTests

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cert.Type()
			if got != tt.expected {
				t.Errorf(
					"FAIL: %s -> Type() = %d, expected = %d",
					tt.name,
					got,
					tt.expected,
				)
			}
		})
	}
}

// TestCertificateTypeCoverage ensures all CertificateType constants are tested
func TestCertificateTypeCoverage(t *testing.T) {
	// Get all CertificateType constants via reflection
	certTypeType := reflect.TypeFor[CertificateType]()
	if certTypeType.Kind() != reflect.Uint {
		t.Fatalf("CertificateType is not uint, got %s", certTypeType.Kind())
	}

	// Collect all constants from the test table
	testedTypes := make(map[uint]bool)
	for _, tt := range certificateTypeTests {
		testedTypes[tt.expected] = true
	}

	// Check that we have at least one test for each known certificate type constant
	// This is a best-effort check - if new constants are added, this test will need updating
	expectedTypes := map[uint]string{
		uint(CertificateTypeStakeRegistration):               "CertificateTypeStakeRegistration",
		uint(CertificateTypeStakeDeregistration):             "CertificateTypeStakeDeregistration",
		uint(CertificateTypeStakeDelegation):                 "CertificateTypeStakeDelegation",
		uint(CertificateTypePoolRegistration):                "CertificateTypePoolRegistration",
		uint(CertificateTypePoolRetirement):                  "CertificateTypePoolRetirement",
		uint(CertificateTypeGenesisKeyDelegation):            "CertificateTypeGenesisKeyDelegation",
		uint(CertificateTypeMoveInstantaneousRewards):        "CertificateTypeMoveInstantaneousRewards",
		uint(CertificateTypeRegistration):                    "CertificateTypeRegistration",
		uint(CertificateTypeDeregistration):                  "CertificateTypeDeregistration",
		uint(CertificateTypeVoteDelegation):                  "CertificateTypeVoteDelegation",
		uint(CertificateTypeStakeVoteDelegation):             "CertificateTypeStakeVoteDelegation",
		uint(CertificateTypeStakeRegistrationDelegation):     "CertificateTypeStakeRegistrationDelegation",
		uint(CertificateTypeVoteRegistrationDelegation):      "CertificateTypeVoteRegistrationDelegation",
		uint(CertificateTypeStakeVoteRegistrationDelegation): "CertificateTypeStakeVoteRegistrationDelegation",
		uint(CertificateTypeAuthCommitteeHot):                "CertificateTypeAuthCommitteeHot",
		uint(CertificateTypeResignCommitteeCold):             "CertificateTypeResignCommitteeCold",
		uint(CertificateTypeRegistrationDrep):                "CertificateTypeRegistrationDrep",
		uint(CertificateTypeDeregistrationDrep):              "CertificateTypeDeregistrationDrep",
		uint(CertificateTypeUpdateDrep):                      "CertificateTypeUpdateDrep",
		uint(CertificateTypeLeiosEb):                         "CertificateTypeLeiosEb",
	}

	for certType, name := range expectedTypes {
		if !testedTypes[certType] {
			t.Errorf(
				"Certificate type %s (value %d) is not covered by TestCertificateTypeMethods",
				name,
				certType,
			)
		}
	}
}

// Test deterministic encoding of MultiAsset
func TestMultiAssetDeterministicEncoding(t *testing.T) {
	multiAsset := MultiAsset[MultiAssetTypeOutput]{
		data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
			NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
				cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): big.NewInt(
					123456,
				),
				cbor.NewByteString(test.DecodeHexString("746f6b656e32")): big.NewInt(
					789,
				),
			},
			NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
				cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e636574737638")): big.NewInt(
					234567,
				),
			},
		},
	}

	// Encode multiple times
	var encodings [][]byte
	for i := range 10 {
		encoded, err := cbor.Encode(&multiAsset)
		if err != nil {
			t.Fatalf("Failed to encode MultiAsset on attempt %d: %v", i, err)
		}
		encodings = append(encodings, encoded)
	}

	// Check all encodings are identical
	for i := 1; i < len(encodings); i++ {
		if !bytes.Equal(encodings[0], encodings[i]) {
			t.Errorf("Encoding not deterministic: attempt 0 != attempt %d", i)
			t.Logf("Attempt 0: %x", encodings[0])
			t.Logf("Attempt %d: %x", i, encodings[i])
		}
	}
}

// TestNewPoolIdFromBech32_Negative tests error cases for PoolId bech32 decoding
// CIP-0005: pool IDs use "pool" prefix with 28-byte payload
func TestNewPoolIdFromBech32_Negative(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "InvalidBech32",
			input: "not-valid-bech32!@#",
		},
		{
			name:  "EmptyString",
			input: "",
		},
		{
			name: "WrongLength",
			// Valid bech32 but only 20 bytes (address hash, not pool id)
			input: "pool1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqs5rx55",
		},
		{
			name: "InvalidChecksum",
			// Valid pool ID with corrupted checksum
			input: "pool1pu5jlj4q9w9jlxeu370a3c9myx47md5j5m2str0xxxxyzkxewzq",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPoolIdFromBech32(tc.input)
			if err == nil {
				t.Errorf(
					"NewPoolIdFromBech32(%q) expected error, got nil",
					tc.input,
				)
			}
		})
	}
}

// TestPoolIdString tests encoding pool IDs to bech32
// CIP-0005: pool IDs use "pool" prefix
func TestPoolIdString(t *testing.T) {
	testCases := []struct {
		name   string
		poolId PoolId
		want   string
	}{
		{
			name:   "ZeroHash",
			poolId: PoolId{},
			want:   "pool1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8a7a2d",
		},
		{
			name: "SequentialBytes",
			poolId: PoolId{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1a, 0x1b, 0x1c,
			},
			want: "pool1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5z5tpwxqergd3c6vr4kr",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.poolId.String()
			if got != tc.want {
				t.Errorf("PoolId.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPoolIdBech32RoundTrip tests that encoding and decoding a pool ID is lossless
func TestPoolIdBech32RoundTrip(t *testing.T) {
	testCases := []struct {
		name   string
		poolId PoolId
	}{
		{
			name:   "ZeroHash",
			poolId: PoolId{},
		},
		{
			name: "SequentialBytes",
			poolId: PoolId{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1a, 0x1b, 0x1c,
			},
		},
		{
			name: "MaxBytes",
			poolId: PoolId{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := tc.poolId.String()
			decoded, err := NewPoolIdFromBech32(encoded)
			if err != nil {
				t.Fatalf("NewPoolIdFromBech32() unexpected error: %v", err)
			}
			if decoded != tc.poolId {
				t.Errorf(
					"Round-trip failed: got %x, want %x",
					decoded,
					tc.poolId,
				)
			}
		})
	}
}
