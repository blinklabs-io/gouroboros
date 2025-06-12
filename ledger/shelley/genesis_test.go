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
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

const shelleyGenesisConfig = `
{
  "activeSlotsCoeff": 0.05,
  "protocolParams": {
    "protocolVersion": {
      "minor": 0,
      "major": 2
    },
    "decentralisationParam": 1,
    "eMax": 18,
    "extraEntropy": {
      "tag": "NeutralNonce"
    },
    "maxTxSize": 16384,
    "maxBlockBodySize": 65536,
    "maxBlockHeaderSize": 1100,
    "minFeeA": 44,
    "minFeeB": 155381,
    "minUTxOValue": 1000000,
    "poolDeposit": 500000000,
    "minPoolCost": 340000000,
    "keyDeposit": 2000000,
    "nOpt": 150,
    "rho": 0.003,
    "tau": 0.20,
    "a0": 0.3
  },
  "genDelegs": {
    "ad5463153dc3d24b9ff133e46136028bdc1edbb897f5a7cf1b37950c": {
      "delegate": "d9e5c76ad5ee778960804094a389f0b546b5c2b140a62f8ec43ea54d",
      "vrf": "64fa87e8b29a5b7bfbd6795677e3e878c505bc4a3649485d366b50abadec92d7"
    },
    "b9547b8a57656539a8d9bc42c008e38d9c8bd9c8adbb1e73ad529497": {
      "delegate": "855d6fc1e54274e331e34478eeac8d060b0b90c1f9e8a2b01167c048",
      "vrf": "66d5167a1f426bd1adcc8bbf4b88c280d38c148d135cb41e3f5a39f948ad7fcc"
    },
    "60baee25cbc90047e83fd01e1e57dc0b06d3d0cb150d0ab40bbfead1": {
      "delegate": "7f72a1826ae3b279782ab2bc582d0d2958de65bd86b2c4f82d8ba956",
      "vrf": "c0546d9aa5740afd569d3c2d9c412595cd60822bb6d9a4e8ce6c43d12bd0f674"
    },
    "f7b341c14cd58fca4195a9b278cce1ef402dc0e06deb77e543cd1757": {
      "delegate": "69ae12f9e45c0c9122356c8e624b1fbbed6c22a2e3b4358cf0cb5011",
      "vrf": "6394a632af51a32768a6f12dac3485d9c0712d0b54e3f389f355385762a478f2"
    },
    "162f94554ac8c225383a2248c245659eda870eaa82d0ef25fc7dcd82": {
      "delegate": "4485708022839a7b9b8b639a939c85ec0ed6999b5b6dc651b03c43f6",
      "vrf": "aba81e764b71006c515986bf7b37a72fbb5554f78e6775f08e384dbd572a4b32"
    },
    "2075a095b3c844a29c24317a94a643ab8e22d54a3a3a72a420260af6": {
      "delegate": "6535db26347283990a252313a7903a45e3526ec25ddba381c071b25b",
      "vrf": "fcaca997b8105bd860876348fc2c6e68b13607f9bbd23515cd2193b555d267af"
    },
    "268cfc0b89e910ead22e0ade91493d8212f53f3e2164b2e4bef0819b": {
      "delegate": "1d4f2e1fda43070d71bb22a5522f86943c7c18aeb4fa47a362c27e23",
      "vrf": "63ef48bc5355f3e7973100c371d6a095251c80ceb40559f4750aa7014a6fb6db"
    }
  },
  "updateQuorum": 5,
  "networkId": "Mainnet",
  "initialFunds": {},
  "maxLovelaceSupply": 45000000000000000,
  "networkMagic": 764824073,
  "epochLength": 432000,
  "systemStart": "2017-09-23T21:44:51Z",
  "slotsPerKESPeriod": 129600,
  "slotLength": 1,
  "maxKESEvolutions": 62,
  "securityParam": 2160
}
`

var expectedGenesisObj = shelley.ShelleyGenesis{
	SystemStart: time.Date(
		2017,
		time.September,
		23,
		21,
		44,
		51,
		0,
		time.UTC,
	),
	NetworkMagic: 764824073,
	NetworkId:    "Mainnet",
	ActiveSlotsCoeff: common.GenesisRat{
		Rat: big.NewRat(5, 100),
	},
	SecurityParam:     2160,
	EpochLength:       432000,
	SlotsPerKESPeriod: 129600,
	MaxKESEvolutions:  62,
	SlotLength:        1,
	UpdateQuorum:      5,
	MaxLovelaceSupply: 45000000000000000,
	ProtocolParameters: shelley.ShelleyGenesisProtocolParams{
		MinFeeA:            44,
		MinFeeB:            155381,
		MaxBlockBodySize:   65536,
		MaxTxSize:          16384,
		MaxBlockHeaderSize: 1100,
		KeyDeposit:         2000000,
		PoolDeposit:        500000000,
		MaxEpoch:           18,
		NOpt:               150,
		A0:                 &common.GenesisRat{Rat: big.NewRat(3, 10)},
		Rho:                &common.GenesisRat{Rat: big.NewRat(3, 1000)},
		Tau:                &common.GenesisRat{Rat: big.NewRat(2, 10)},
		Decentralization:   &common.GenesisRat{Rat: new(big.Rat).SetInt64(1)},
		ExtraEntropy: common.Nonce{
			Type: common.NonceTypeNeutral,
		},
		ProtocolVersion: struct {
			Major uint `json:"major"`
			Minor uint `json:"minor"`
		}{
			Major: 2,
			Minor: 0,
		},
		MinUtxoValue: 1000000,
		MinPoolCost:  340000000,
	},
	GenDelegs: map[string]map[string]string{
		"162f94554ac8c225383a2248c245659eda870eaa82d0ef25fc7dcd82": {
			"delegate": "4485708022839a7b9b8b639a939c85ec0ed6999b5b6dc651b03c43f6",
			"vrf":      "aba81e764b71006c515986bf7b37a72fbb5554f78e6775f08e384dbd572a4b32",
		},
		"2075a095b3c844a29c24317a94a643ab8e22d54a3a3a72a420260af6": {
			"delegate": "6535db26347283990a252313a7903a45e3526ec25ddba381c071b25b",
			"vrf":      "fcaca997b8105bd860876348fc2c6e68b13607f9bbd23515cd2193b555d267af",
		},
		"268cfc0b89e910ead22e0ade91493d8212f53f3e2164b2e4bef0819b": {
			"delegate": "1d4f2e1fda43070d71bb22a5522f86943c7c18aeb4fa47a362c27e23",
			"vrf":      "63ef48bc5355f3e7973100c371d6a095251c80ceb40559f4750aa7014a6fb6db",
		},
		"60baee25cbc90047e83fd01e1e57dc0b06d3d0cb150d0ab40bbfead1": {
			"delegate": "7f72a1826ae3b279782ab2bc582d0d2958de65bd86b2c4f82d8ba956",
			"vrf":      "c0546d9aa5740afd569d3c2d9c412595cd60822bb6d9a4e8ce6c43d12bd0f674",
		},
		"ad5463153dc3d24b9ff133e46136028bdc1edbb897f5a7cf1b37950c": {
			"delegate": "d9e5c76ad5ee778960804094a389f0b546b5c2b140a62f8ec43ea54d",
			"vrf":      "64fa87e8b29a5b7bfbd6795677e3e878c505bc4a3649485d366b50abadec92d7",
		},
		"b9547b8a57656539a8d9bc42c008e38d9c8bd9c8adbb1e73ad529497": {
			"delegate": "855d6fc1e54274e331e34478eeac8d060b0b90c1f9e8a2b01167c048",
			"vrf":      "66d5167a1f426bd1adcc8bbf4b88c280d38c148d135cb41e3f5a39f948ad7fcc",
		},
		"f7b341c14cd58fca4195a9b278cce1ef402dc0e06deb77e543cd1757": {
			"delegate": "69ae12f9e45c0c9122356c8e624b1fbbed6c22a2e3b4358cf0cb5011",
			"vrf":      "6394a632af51a32768a6f12dac3485d9c0712d0b54e3f389f355385762a478f2",
		},
	},
	InitialFunds: map[string]uint64{},
}

func TestGenesisFromJson(t *testing.T) {
	tmpGenesis, err := shelley.NewShelleyGenesisFromReader(
		strings.NewReader(shelleyGenesisConfig),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !reflect.DeepEqual(tmpGenesis, expectedGenesisObj) {
		t.Fatalf(
			"did not get expected object:\n     got: %#v\n  wanted: %#v",
			tmpGenesis,
			expectedGenesisObj,
		)
	}
}

func TestGenesisUtxos(t *testing.T) {
	testHexAddr := "000045183c1dcaeb0ca5cf583a68b9e31a6301bcbde487065bd35b955a98ba9d3061e1bd15749cc857e94b30583c120e3255adb93b44681bad"
	testAmount := uint64(120_000_000_000_000)
	expectedTxId := "23e41590bf49ad07dd6f28db73f9f16c804b9b5791b9dd669bfc58df8a9a1129"
	expectedAddr := "addr_test1qqqy2xpurh9wkr99eavr569euvdxxqduhhjgwpjm6dde2k5ch2wnqc0ph52hf8xg2l55kvzc8sfquvj44kunk3rgrwksfahlvw"
	// Generate genesis config JSON
	tmpGenesisData := map[string]any{
		"initialFunds": map[string]uint64{
			testHexAddr: testAmount,
		},
	}
	tmpGenesisJson, err := json.Marshal(tmpGenesisData)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Parse genesis config JSON
	tmpGenesis, err := shelley.NewShelleyGenesisFromReader(
		strings.NewReader(string(tmpGenesisJson)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tmpGenesisUtxos, err := tmpGenesis.GenesisUtxos()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(tmpGenesisUtxos) != 1 {
		t.Fatalf("did not get expected count of genesis UTxOs")
	}
	tmpUtxo := tmpGenesisUtxos[0]
	if tmpUtxo.Id.Id().String() != expectedTxId {
		t.Fatalf(
			"did not get expected TxID: got %s, wanted %s",
			tmpUtxo.Id.Id().String(),
			expectedTxId,
		)
	}
	if tmpUtxo.Output.Address().String() != expectedAddr {
		t.Fatalf(
			"did not get expected address: got %s, wanted %s",
			tmpUtxo.Output.Address().String(),
			expectedAddr,
		)
	}
	if tmpUtxo.Output.Amount() != testAmount {
		t.Fatalf(
			"did not get expected amount: got %d, wanted %d",
			tmpUtxo.Output.Amount(),
			testAmount,
		)
	}
}
