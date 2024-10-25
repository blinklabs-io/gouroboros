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
