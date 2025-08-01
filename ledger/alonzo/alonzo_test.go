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

package alonzo

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/plutigo/data"
)

func TestAlonzoTransactionOutputToPlutusDataCoinOnly(t *testing.T) {
	testAddr := "addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6"
	var testAmount uint64 = 123_456_789
	testTxOut := AlonzoTransactionOutput{
		OutputAddress: func() common.Address { foo, _ := common.NewAddress(testAddr); return foo }(),
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: testAmount,
		},
	}
	expectedData := data.NewConstr(
		0,
		// Address
		data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewByteString(
					[]byte{
						0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
					},
				),
			),
			data.NewConstr(
				1,
			),
		),
		// Value
		data.NewMap(
			[][2]data.PlutusData{
				{
					data.NewByteString(nil),
					data.NewMap(
						[][2]data.PlutusData{
							{
								data.NewByteString(nil),
								data.NewInteger(big.NewInt(int64(testAmount))),
							},
						},
					),
				},
			},
		),
		// TODO: empty datum option
		data.NewConstr(0),
		// TODO: empty script ref
		data.NewConstr(1),
	)
	tmpData := testTxOut.ToPlutusData()
	if !reflect.DeepEqual(tmpData, expectedData) {
		t.Fatalf("did not get expected PlutusData\n     got: %#v\n  wanted: %#v", tmpData, expectedData)
	}
}

func TestAlonzoTransactionOutputToPlutusDataCoinAssets(t *testing.T) {
	testAddr := "addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6"
	var testAmount uint64 = 123_456_789
	testAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
			common.NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
				cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): 123456,
			},
			common.NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
				cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e63657074733638")): 234567,
			},
		},
	)
	testTxOut := AlonzoTransactionOutput{
		OutputAddress: func() common.Address { foo, _ := common.NewAddress(testAddr); return foo }(),
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: testAmount,
			Assets: &testAssets,
		},
	}
	expectedData := data.NewConstr(
		0,
		// Address
		data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewByteString(
					[]byte{
						0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
					},
				),
			),
			data.NewConstr(
				1,
			),
		),
		// Value
		data.NewMap(
			[][2]data.PlutusData{
				{
					data.NewByteString(nil),
					data.NewMap(
						[][2]data.PlutusData{
							{
								data.NewByteString(nil),
								data.NewInteger(big.NewInt(int64(testAmount))),
							},
						},
					),
				},
				{
					data.NewByteString(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")),
					data.NewMap(
						[][2]data.PlutusData{
							{
								data.NewByteString(test.DecodeHexString("6675726e697368613239686e")),
								data.NewInteger(big.NewInt(123456)),
							},
						},
					),
				},
				{
					data.NewByteString(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")),
					data.NewMap(
						[][2]data.PlutusData{
							{
								data.NewByteString(test.DecodeHexString("426f7764757261436f6e63657074733638")),
								data.NewInteger(big.NewInt(234567)),
							},
						},
					),
				},
			},
		),
		// Empty datum option
		data.NewConstr(0),
		// Empty script ref
		data.NewConstr(1),
	)
	tmpData := testTxOut.ToPlutusData()
	if !reflect.DeepEqual(tmpData, expectedData) {
		t.Fatalf("did not get expected PlutusData\n     got: %#v\n  wanted: %#v", tmpData, expectedData)
	}
}

func TestAlonzoRedeemersIter(t *testing.T) {
	testRedeemers := AlonzoRedeemers{
		{
			Tag:   common.RedeemerTagMint,
			Index: 2,
			ExUnits: common.ExUnits{
				Memory: 1111,
				Steps:  2222,
			},
		},
		{
			Tag:   common.RedeemerTagMint,
			Index: 0,
			ExUnits: common.ExUnits{
				Memory: 1111,
				Steps:  0,
			},
		},
		{
			Tag:   common.RedeemerTagSpend,
			Index: 4,
			ExUnits: common.ExUnits{
				Memory: 0,
				Steps:  4444,
			},
		},
	}
	expectedOrder := []struct {
		Key   common.RedeemerKey
		Value common.RedeemerValue
	}{
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
		},
	}
	iterIdx := 0
	for key, val := range testRedeemers.Iter() {
		expected := expectedOrder[iterIdx]
		if !reflect.DeepEqual(key, expected.Key) {
			t.Fatalf("did not get expected key: got %#v, wanted %#v", key, expected.Key)
		}
		if !reflect.DeepEqual(val, expected.Value) {
			t.Fatalf("did not get expected value: got %#v, wanted %#v", val, expected.Value)
		}
		iterIdx++
	}
}
