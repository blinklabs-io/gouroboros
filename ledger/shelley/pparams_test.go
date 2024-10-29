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

package shelley_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestShelleyProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    shelley.ShelleyProtocolParameters
		updateCbor     string
		expectedParams shelley.ShelleyProtocolParameters
	}{
		{
			startParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: new(big.Rat).SetInt64(1)},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: shelley.ShelleyProtocolParameters{
				ProtocolMajor: 2,
			},
			updateCbor: "a10e820300",
			expectedParams: shelley.ShelleyProtocolParameters{
				ProtocolMajor: 3,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate shelley.ShelleyProtocolParameterUpdate
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

func TestShelleyProtocolParamsUpdateFromGenesis(t *testing.T) {
	testDefs := []struct {
		startParams    shelley.ShelleyProtocolParameters
		genesisJson    string
		expectedParams shelley.ShelleyProtocolParameters
	}{
		{
			startParams: shelley.ShelleyProtocolParameters{},
			genesisJson: `{"protocolParams":{"decentralisationParam":0.9}}`,
			expectedParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
	}
	for _, testDef := range testDefs {
		tmpGenesis, err := shelley.NewShelleyGenesisFromReader(
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

func TestShelleyUtxorpc(t *testing.T) {
	inputParams := shelley.ShelleyProtocolParameters{
		MinFeeA:            500,
		MinFeeB:            2,
		MaxBlockBodySize:   65536,
		MaxTxSize:          16384,
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
	}

	expectedUtxorpc := &cardano.PParams{
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
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
	}

	result := inputParams.Utxorpc()

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Shelley:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}
