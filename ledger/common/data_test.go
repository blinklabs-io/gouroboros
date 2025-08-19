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
		Data: data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewConstr(
					0,
					data.NewByteString(
						test.DecodeHexString(
							"b255e2283f9b495dd663b841090c42bc5a5103283fc2aef5c6cd2f5c",
						),
					),
				),
				data.NewConstr(
					0,
					data.NewConstr(
						0,
						data.NewConstr(
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
			data.NewConstr(
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
			data.NewList(
				data.NewConstr(
					0,
					data.NewConstr(
						0,
						data.NewByteString(nil),
						data.NewByteString(nil),
					),
					data.NewInteger(big.NewInt(156203224)),
				),
				data.NewConstr(
					0,
					data.NewConstr(
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
