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

package alonzo_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestAlonzoProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    alonzo.AlonzoProtocolParameters
		updateCbor     string
		expectedParams alonzo.AlonzoProtocolParameters
	}{
		{
			startParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							Decentralization: &cbor.Rat{
								Rat: new(big.Rat).SetInt64(1),
							},
						},
					},
				},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
						},
					},
				},
			},
		},
		{
			startParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							ProtocolMajor: 5,
						},
					},
				},
			},
			updateCbor: "a10e820600",
			expectedParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							ProtocolMajor: 6,
						},
					},
				},
			},
		},
		{
			startParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							MaxBlockBodySize: 1,
						},
					},
				},
				MaxTxExUnits: common.ExUnit{
					Mem:   1,
					Steps: 1,
				},
			},
			updateCbor: "a2021a0001200014821a00aba9501b00000002540be400",
			expectedParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							MaxBlockBodySize: 73728,
						},
					},
				},
				MaxTxExUnits: common.ExUnit{
					Mem:   11250000,
					Steps: 10000000000,
				},
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate alonzo.AlonzoProtocolParameterUpdate
		if _, err := cbor.Decode(cborBytes, &tmpUpdate); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		tmpParams.Update(&tmpUpdate)
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestAlonzoProtocolParamsUpdateFromGenesis(t *testing.T) {
	testDefs := []struct {
		startParams    alonzo.AlonzoProtocolParameters
		genesisJson    string
		expectedParams alonzo.AlonzoProtocolParameters
	}{
		{
			startParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							Decentralization: &cbor.Rat{
								Rat: new(big.Rat).SetInt64(1),
							},
						},
					},
				},
			},
			genesisJson: `{"lovelacePerUTxOWord": 34482}`,
			expectedParams: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: mary.MaryProtocolParameters{
					AllegraProtocolParameters: allegra.AllegraProtocolParameters{
						ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
							Decentralization: &cbor.Rat{
								Rat: new(big.Rat).SetInt64(1),
							},
						},
					},
				},
				AdaPerUtxoByte: 34482,
			},
		},
	}
	for _, testDef := range testDefs {
		tmpGenesis, err := alonzo.NewAlonzoGenesisFromReader(
			strings.NewReader(testDef.genesisJson),
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		tmpParams.UpdateFromGenesis(&tmpGenesis)
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestAlonzoUtxorpc(t *testing.T) {
	inputParams := alonzo.AlonzoProtocolParameters{
		MaryProtocolParameters: mary.MaryProtocolParameters{
			AllegraProtocolParameters: allegra.AllegraProtocolParameters{
				ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
					MaxTxSize:          16384,
					MinFeeA:            500,
					MinFeeB:            2,
					MaxBlockBodySize:   65536,
					MaxBlockHeaderSize: 1024,
					KeyDeposit:         2000,
					PoolDeposit:        500000,
					MaxEpoch:           2160,
					NOpt:               100,
					A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
					Rho:                &cbor.Rat{Rat: big.NewRat(3, 4)},
					Tau:                &cbor.Rat{Rat: big.NewRat(5, 6)},
					ProtocolMajor:      8,
					ProtocolMinor:      0,
				},
			},
		},
		AdaPerUtxoByte:       44,
		MinPoolCost:          340000000,
		MaxValueSize:         1024,
		CollateralPercentage: 150,
		MaxCollateralInputs:  5,
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
		MaxTxExUnits: common.ExUnit{
			Mem:   1000000,
			Steps: 200000,
		},
		MaxBlockExUnits: common.ExUnit{
			Mem:   5000000,
			Steps: 1000000,
		},
		CostModels: map[uint][]int64{
			1: {100, 200, 300},
			2: {400, 500, 600},
			3: {700, 800, 900},
		},
	}

	expectedUtxorpc := &cardano.PParams{
		CoinsPerUtxoByte:         44,
		MaxTxSize:                16384,
		MinFeeCoefficient:        500,
		MinFeeConstant:           2,
		MaxBlockBodySize:         65536,
		MaxBlockHeaderSize:       1024,
		StakeKeyDeposit:          2000,
		PoolDeposit:              500000,
		PoolRetirementEpochBound: 2160,
		DesiredNumberOfPools:     100,
		PoolInfluence: &cardano.RationalNumber{
			Numerator:   int32(1),
			Denominator: uint32(2),
		},
		MonetaryExpansion: &cardano.RationalNumber{
			Numerator:   int32(3),
			Denominator: uint32(4),
		},
		TreasuryExpansion: &cardano.RationalNumber{
			Numerator:   int32(5),
			Denominator: uint32(6),
		},
		MinPoolCost: 340000000,
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
		MaxValueSize:         1024,
		CollateralPercentage: 150,
		MaxCollateralInputs:  5,
		CostModels: &cardano.CostModels{
			PlutusV1: &cardano.CostModel{
				Values: []int64{100, 200, 300},
			},
			PlutusV2: &cardano.CostModel{
				Values: []int64{400, 500, 600},
			},
			PlutusV3: &cardano.CostModel{
				Values: []int64{700, 800, 900},
			},
		},
		Prices: &cardano.ExPrices{
			Memory: &cardano.RationalNumber{
				Numerator:   int32(1),
				Denominator: uint32(2),
			},
			Steps: &cardano.RationalNumber{
				Numerator:   int32(2),
				Denominator: uint32(3),
			},
		},
		MaxExecutionUnitsPerTransaction: &cardano.ExUnits{
			Memory: 1000000,
			Steps:  200000,
		},
		MaxExecutionUnitsPerBlock: &cardano.ExUnits{
			Memory: 5000000,
			Steps:  1000000,
		},
	}

	result := inputParams.Utxorpc()

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Alonzo:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}
