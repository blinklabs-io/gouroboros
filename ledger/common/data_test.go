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
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

func TestDatumHash(t *testing.T) {
	testDatumBytes, _ := hex.DecodeString(
		"d8799fd8799fd8799f581cb255e2283f9b495dd663b841090c42bc5a5103283fc2aef5c6cd2f5cffd8799fd8799fd8799f581c07d8b4b15e9609e76a38b25637900d60cdf13a6abce984757bbc1349ffffffffd8799f581cf5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4c582073e1518e92f367fd5820ac2da1d40ab24fbca1d6cb2c28121ad92f57aff8abceff1b0000000148f3f3579fd8799fd8799f4040ff1a094f78d8ffd8799fd8799f581cf13ac4d66b3ee19a6aa0f2a22298737bd907cc95121662fc971b527546535452494b45ff1af7c5c601ffffff",
	)
	expectedHash := "4dfec91f63f946d7c91af0041e5d92a45531790a4a104637dd8691f46fdce842"
	var tmpDatum common.Datum
	if _, err := cbor.Decode(testDatumBytes, &tmpDatum); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	datumHash := tmpDatum.Hash()
	if datumHash.String() != expectedHash {
		t.Fatalf(
			"did not get expected datum hash: got %s, wanted %s",
			datumHash.String(),
			expectedHash,
		)
	}
}

func TestDatumDecode(t *testing.T) {
	testDatumBytes, _ := hex.DecodeString(
		"d8799fd8799fd8799f581cb255e2283f9b495dd663b841090c42bc5a5103283fc2aef5c6cd2f5cffd8799fd8799fd8799f581c07d8b4b15e9609e76a38b25637900d60cdf13a6abce984757bbc1349ffffffffd8799f581cf5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4c582073e1518e92f367fd5820ac2da1d40ab24fbca1d6cb2c28121ad92f57aff8abceff1b0000000148f3f3579fd8799fd8799f4040ff1a094f78d8ffd8799fd8799f581cf13ac4d66b3ee19a6aa0f2a22298737bd907cc95121662fc971b527546535452494b45ff1af7c5c601ffffff",
	)
	expectedDatum := common.Datum{
		Data: data.NewConstrDefIndef(
			true,
			0,
			data.NewConstrDefIndef(
				true,
				0,
				data.NewConstrDefIndef(
					true,
					0,
					data.NewByteString(
						test.DecodeHexString(
							"b255e2283f9b495dd663b841090c42bc5a5103283fc2aef5c6cd2f5c",
						),
					),
				),
				data.NewConstrDefIndef(
					true,
					0,
					data.NewConstrDefIndef(
						true,
						0,
						data.NewConstrDefIndef(
							true,
							0,
							data.NewByteString(
								test.DecodeHexString(
									"07d8b4b15e9609e76a38b25637900d60cdf13a6abce984757bbc1349",
								),
							),
						),
					),
				),
			),
			data.NewConstrDefIndef(
				true,
				0,
				data.NewByteString(
					test.DecodeHexString(
						"f5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4c",
					),
				),
				data.NewByteString(
					test.DecodeHexString(
						"73e1518e92f367fd5820ac2da1d40ab24fbca1d6cb2c28121ad92f57aff8abce",
					),
				),
			),
			data.NewInteger(big.NewInt(5518914391)),
			data.NewListDefIndef(
				true,
				data.NewConstrDefIndef(
					true,
					0,
					data.NewConstrDefIndef(
						true,
						0,
						data.NewByteString(nil),
						data.NewByteString(nil),
					),
					data.NewInteger(big.NewInt(156203224)),
				),
				data.NewConstrDefIndef(
					true,
					0,
					data.NewConstrDefIndef(
						true,
						0,
						data.NewByteString(
							test.DecodeHexString(
								"f13ac4d66b3ee19a6aa0f2a22298737bd907cc95121662fc971b5275",
							),
						),
						data.NewByteString(
							test.DecodeHexString("535452494b45"),
						),
					),
					data.NewInteger(big.NewInt(4156933633)),
				),
			),
		),
	}
	var tmpDatum common.Datum
	if _, err := cbor.Decode(testDatumBytes, &tmpDatum); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !reflect.DeepEqual(tmpDatum.Data, expectedDatum.Data) {
		t.Fatalf(
			"did not get expected datum\n     got: %#v\n  wanted: %#v",
			tmpDatum.Data,
			expectedDatum.Data,
		)
	}
}

// TestDatumHashToBech32 tests CIP-0005 bech32 encoding for datum hashes.
func TestDatumHashToBech32(t *testing.T) {
	testCases := []struct {
		name       string
		hash       common.DatumHash
		wantPrefix string
	}{
		{
			name: "ZeroHash",
			hash: common.DatumHash{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			wantPrefix: "datum1",
		},
		{
			name: "SequentialHash",
			hash: common.DatumHash{
				0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
				16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
			},
			wantPrefix: "datum1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := common.DatumHashToBech32(tc.hash)
			if len(result) <= len(tc.wantPrefix) {
				t.Errorf("result too short: got %s", result)
			}
			if result[:len(tc.wantPrefix)] != tc.wantPrefix {
				t.Errorf("wrong prefix: got %s, want prefix %s", result, tc.wantPrefix)
			}
		})
	}
}

// TestDatumHashBech32Specific tests specific expected bech32 encoded values.
func TestDatumHashBech32Specific(t *testing.T) {
	// Known datum hash from above test
	hashBytes, _ := hex.DecodeString("4dfec91f63f946d7c91af0041e5d92a45531790a4a104637dd8691f46fdce842")
	var hash common.DatumHash
	copy(hash[:], hashBytes)

	bech32Str := common.DatumHashToBech32(hash)

	// Verify length is reasonable for 32 bytes + prefix
	if len(bech32Str) < 50 {
		t.Errorf("bech32 string too short: %s", bech32Str)
		return
	}

	// Verify prefix
	if bech32Str[:6] != "datum1" {
		t.Errorf("wrong prefix: got %s", bech32Str[:6])
	}
}

func TestDatumRoundtripAndHash(t *testing.T) {
	// Simple byte-string datum
	d := common.Datum{
		Data: data.NewByteString([]byte("hello-datum")),
	}
	// Marshal to CBOR
	cborBytes, err := d.MarshalCBOR()
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	// Unmarshal into a fresh Datum
	var other common.Datum
	if _, err := cbor.Decode(cborBytes, &other); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if !reflect.DeepEqual(d.Data, other.Data) {
		t.Fatalf(
			"datum mismatch after roundtrip: got %#v wanted %#v",
			other.Data,
			d.Data,
		)
	}

	// Hash should equal Blake2b256Hash of the CBOR
	expected := common.Blake2b256Hash(cborBytes)
	if other.Hash() != expected {
		t.Fatalf(
			"datum hash mismatch: got %s wanted %s",
			other.Hash().String(),
			expected.String(),
		)
	}
}
