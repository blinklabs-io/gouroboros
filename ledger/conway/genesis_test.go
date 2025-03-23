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

package conway_test

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

const conwayGenesisConfig = `
{
  "poolVotingThresholds": {
    "committeeNormal": 0.51,
    "committeeNoConfidence": 0.51,
    "hardForkInitiation": 0.51,
    "motionNoConfidence": 0.51,
    "ppSecurityGroup": 0.51
  },
  "dRepVotingThresholds": {
    "motionNoConfidence": 0.67,
    "committeeNormal": 0.67,
    "committeeNoConfidence": 0.6,
    "updateToConstitution": 0.75,
    "hardForkInitiation": 0.6,
    "ppNetworkGroup": 0.67,
    "ppEconomicGroup": 0.67,
    "ppTechnicalGroup": 0.67,
    "ppGovGroup": 0.75,
    "treasuryWithdrawal": 0.67
  },
  "committeeMinSize": 7,
  "committeeMaxTermLength": 146,
  "govActionLifetime": 6,
  "govActionDeposit": 100000000000,
  "dRepDeposit": 500000000,
  "dRepActivity": 20,
  "minFeeRefScriptCostPerByte": 15,
  "plutusV3CostModel": [
    100788,
    420,
    1,
    1,
    1000,
    173,
    0,
    1,
    1000,
    59957,
    4,
    1,
    11183,
    32,
    201305,
    8356,
    4,
    16000,
    100,
    16000,
    100,
    16000,
    100,
    16000,
    100,
    16000,
    100,
    16000,
    100,
    100,
    100,
    16000,
    100,
    94375,
    32,
    132994,
    32,
    61462,
    4,
    72010,
    178,
    0,
    1,
    22151,
    32,
    91189,
    769,
    4,
    2,
    85848,
    123203,
    7305,
    -900,
    1716,
    549,
    57,
    85848,
    0,
    1,
    1,
    1000,
    42921,
    4,
    2,
    24548,
    29498,
    38,
    1,
    898148,
    27279,
    1,
    51775,
    558,
    1,
    39184,
    1000,
    60594,
    1,
    141895,
    32,
    83150,
    32,
    15299,
    32,
    76049,
    1,
    13169,
    4,
    22100,
    10,
    28999,
    74,
    1,
    28999,
    74,
    1,
    43285,
    552,
    1,
    44749,
    541,
    1,
    33852,
    32,
    68246,
    32,
    72362,
    32,
    7243,
    32,
    7391,
    32,
    11546,
    32,
    85848,
    123203,
    7305,
    -900,
    1716,
    549,
    57,
    85848,
    0,
    1,
    90434,
    519,
    0,
    1,
    74433,
    32,
    85848,
    123203,
    7305,
    -900,
    1716,
    549,
    57,
    85848,
    0,
    1,
    1,
    85848,
    123203,
    7305,
    -900,
    1716,
    549,
    57,
    85848,
    0,
    1,
    955506,
    213312,
    0,
    2,
    270652,
    22588,
    4,
    1457325,
    64566,
    4,
    20467,
    1,
    4,
    0,
    141992,
    32,
    100788,
    420,
    1,
    1,
    81663,
    32,
    59498,
    32,
    20142,
    32,
    24588,
    32,
    20744,
    32,
    25933,
    32,
    24623,
    32,
    43053543,
    10,
    53384111,
    14333,
    10,
    43574283,
    26308,
    10,
    16000,
    100,
    16000,
    100,
    962335,
    18,
    2780678,
    6,
    442008,
    1,
    52538055,
    3756,
    18,
    267929,
    18,
    76433006,
    8868,
    18,
    52948122,
    18,
    1995836,
    36,
    3227919,
    12,
    901022,
    1,
    166917843,
    4307,
    36,
    284546,
    36,
    158221314,
    26549,
    36,
    74698472,
    36,
    333849714,
    1,
    254006273,
    72,
    2174038,
    72,
    2261318,
    64571,
    4,
    207616,
    8310,
    4,
    1293828,
    28716,
    63,
    0,
    1,
    1006041,
    43623,
    251,
    0,
    1
  ],
  "constitution": {
      "anchor": {
          "dataHash": "ca41a91f399259bcefe57f9858e91f6d00e1a38d6d9c63d4052914ea7bd70cb2",
          "url": "ipfs://bafkreifnwj6zpu3ixa4siz2lndqybyc5wnnt3jkwyutci4e2tmbnj3xrdm"
      },
      "script": "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64"
  },
  "committee": {
    "members": {
        "scriptHash-df0e83bde65416dade5b1f97e7f115cc1ff999550ad968850783fe50": 580,
        "scriptHash-b6012034ba0a7e4afbbf2c7a1432f8824aee5299a48e38e41a952686": 580,
        "scriptHash-ce8b37a72b178a37bbd3236daa7b2c158c9d3604e7aa667e6c6004b7": 580,
        "scriptHash-f0dc2c00d92a45521267be2d5de1c485f6f9d14466d7e16062897cf7": 580,
        "scriptHash-349e55f83e9af24813e6cb368df6a80d38951b2a334dfcdf26815558": 580,
        "scriptHash-84aebcfd3e00d0f87af918fc4b5e00135f407e379893df7e7d392c6a": 580,
        "scriptHash-e8165b3328027ee0d74b1f07298cb092fd99aa7697a1436f5997f625": 580
    },
    "threshold": {
      "numerator": 2,
      "denominator": 3
    }
  }
}
`

var expectedGenesisObj = conway.ConwayGenesis{
	PoolVotingThresholds: conway.ConwayGenesisPoolVotingThresholds{
		CommitteeNoConfidence: &common.GenesisRat{Rat: big.NewRat(51, 100)},
		CommitteeNormal:       &common.GenesisRat{Rat: big.NewRat(51, 100)},
		HardForkInitiation:    &common.GenesisRat{Rat: big.NewRat(51, 100)},
		MotionNoConfidence:    &common.GenesisRat{Rat: big.NewRat(51, 100)},
		PpSecurityGroup:       &common.GenesisRat{Rat: big.NewRat(51, 100)},
	},
	DRepVotingThresholds: conway.ConwayGenesisDRepVotingThresholds{
		CommitteeNoConfidence: &common.GenesisRat{Rat: big.NewRat(60, 100)},
		CommitteeNormal:       &common.GenesisRat{Rat: big.NewRat(67, 100)},
		HardForkInitiation:    &common.GenesisRat{Rat: big.NewRat(60, 100)},
		MotionNoConfidence:    &common.GenesisRat{Rat: big.NewRat(67, 100)},
		PpEconomicGroup:       &common.GenesisRat{Rat: big.NewRat(67, 100)},
		PpGovGroup:            &common.GenesisRat{Rat: big.NewRat(75, 100)},
		PpNetworkGroup:        &common.GenesisRat{Rat: big.NewRat(67, 100)},
		PpTechnicalGroup:      &common.GenesisRat{Rat: big.NewRat(67, 100)},
		TreasuryWithdrawal:    &common.GenesisRat{Rat: big.NewRat(67, 100)},
		UpdateToConstitution:  &common.GenesisRat{Rat: big.NewRat(75, 100)},
	},
	MinCommitteeSize:        7,
	CommitteeTermLimit:      146,
	GovActionValidityPeriod: 6,
	GovActionDeposit:        100000000000,
	DRepDeposit:             500000000,
	DRepInactivityPeriod:    20,
	MinFeeRefScriptCostPerByte: &common.GenesisRat{
		Rat: new(big.Rat).SetUint64(15),
	},
	PlutusV3CostModel: []int64{
		100788, 420, 1, 1, 1000, 173, 0, 1, 1000, 59957, 4, 1, 11183, 32, 201305, 8356, 4, 16000, 100, 16000, 100, 16000, 100, 16000, 100, 16000, 100, 16000, 100, 100, 100, 16000, 100, 94375, 32, 132994, 32, 61462, 4, 72010, 178, 0, 1, 22151, 32, 91189, 769, 4, 2, 85848, 123203, 7305, -900, 1716, 549, 57, 85848, 0, 1, 1, 1000, 42921, 4, 2, 24548, 29498, 38, 1, 898148, 27279, 1, 51775, 558, 1, 39184, 1000, 60594, 1, 141895, 32, 83150, 32, 15299, 32, 76049, 1, 13169, 4, 22100, 10, 28999, 74, 1, 28999, 74, 1, 43285, 552, 1, 44749, 541, 1, 33852, 32, 68246, 32, 72362, 32, 7243, 32, 7391, 32, 11546, 32, 85848, 123203, 7305, -900, 1716, 549, 57, 85848, 0, 1, 90434, 519, 0, 1, 74433, 32, 85848, 123203, 7305, -900, 1716, 549, 57, 85848, 0, 1, 1, 85848, 123203, 7305, -900, 1716, 549, 57, 85848, 0, 1, 955506, 213312, 0, 2, 270652, 22588, 4, 1457325, 64566, 4, 20467, 1, 4, 0, 141992, 32, 100788, 420, 1, 1, 81663, 32, 59498, 32, 20142, 32, 24588, 32, 20744, 32, 25933, 32, 24623, 32, 43053543, 10, 53384111, 14333, 10, 43574283, 26308, 10, 16000, 100, 16000, 100, 962335, 18, 2780678, 6, 442008, 1, 52538055, 3756, 18, 267929, 18, 76433006, 8868, 18, 52948122, 18, 1995836, 36, 3227919, 12, 901022, 1, 166917843, 4307, 36, 284546, 36, 158221314, 26549, 36, 74698472, 36, 333849714, 1, 254006273, 72, 2174038, 72, 2261318, 64571, 4, 207616, 8310, 4, 1293828, 28716, 63, 0, 1, 1006041, 43623, 251, 0, 1,
	},
	Constitution: conway.ConwayGenesisConstitution{
		Anchor: conway.ConwayGenesisConstitutionAnchor{
			DataHash: "ca41a91f399259bcefe57f9858e91f6d00e1a38d6d9c63d4052914ea7bd70cb2",
			Url:      "ipfs://bafkreifnwj6zpu3ixa4siz2lndqybyc5wnnt3jkwyutci4e2tmbnj3xrdm",
		},
		Script: "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64",
	},
	Committee: conway.ConwayGenesisCommittee{
		Members: map[string]int{
			"scriptHash-349e55f83e9af24813e6cb368df6a80d38951b2a334dfcdf26815558": 580,
			"scriptHash-84aebcfd3e00d0f87af918fc4b5e00135f407e379893df7e7d392c6a": 580,
			"scriptHash-b6012034ba0a7e4afbbf2c7a1432f8824aee5299a48e38e41a952686": 580,
			"scriptHash-ce8b37a72b178a37bbd3236daa7b2c158c9d3604e7aa667e6c6004b7": 580,
			"scriptHash-df0e83bde65416dade5b1f97e7f115cc1ff999550ad968850783fe50": 580,
			"scriptHash-e8165b3328027ee0d74b1f07298cb092fd99aa7697a1436f5997f625": 580,
			"scriptHash-f0dc2c00d92a45521267be2d5de1c485f6f9d14466d7e16062897cf7": 580,
		},
		Threshold: map[string]int{
			"denominator": 3,
			"numerator":   2,
		},
	},
}

func TestGenesisFromJson(t *testing.T) {
	tmpGenesis, err := conway.NewConwayGenesisFromReader(
		strings.NewReader(conwayGenesisConfig),
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

func TestNewConwayGenesisFromReader(t *testing.T) {
	jsonData := `{
    "poolVotingThresholds": {
      "committeeNormal": null,
      "committeeNoConfidence": null,
      "hardForkInitiation": null,
      "motionNoConfidence": null,
      "ppSecurityGroup": null
    },
    "dRepVotingThresholds": {
      "motionNoConfidence": null,
      "committeeNormal": null,
      "committeeNoConfidence": null,
      "updateToConstitution": null,
      "hardForkInitiation": null,
      "ppNetworkGroup": null,
      "ppEconomicGroup": null,
      "ppTechnicalGroup": null,
      "ppGovGroup": null,
      "treasuryWithdrawal": null
    },
    "committeeMinSize": 5,
    "committeeMaxTermLength": 365,
    "govActionLifetime": 90,
    "govActionDeposit": 1000,
    "dRepDeposit": 200,
    "dRepActivity": 60,
    "minFeeRefScriptCostPerByte": null,
    "plutusV3CostModel": [1, 2, 3],
    "constitution": {
      "anchor": {
        "dataHash": "abc123",
        "url": "https://example.com"
      },
      "script": "example-script"
    },
    "committee": {
      "members": { "key1": 1 },
      "threshold": { "key1": 2 }
    }
  }`

	expected := conway.ConwayGenesis{}
	err := json.Unmarshal([]byte(jsonData), &expected)
	if err != nil {
		t.Errorf("Failed to unmarshal expected JSON: %v", err)
	}

	reader := strings.NewReader(jsonData)
	actual, err := conway.NewConwayGenesisFromReader(reader)
	if err != nil {
		t.Errorf("Failed to decode JSON via NewConwayGenesisFromReader: %v", err)
	}

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Mismatch between expected and actual structs\nExpected: %#v\nActual:   %#v", expected, actual)
	} else {
		t.Logf("ConwayGenesis decoded correctly and matches expected structure")
	}
}
