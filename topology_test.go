// Copyright 2023 Blink Labs Software
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

package ouroboros_test

import (
	"reflect"
	"strings"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
)

type topologyTestDefinition struct {
	jsonData       string
	expectedObject *ouroboros.TopologyConfig
}

var topologyTests = []topologyTestDefinition{
	{
		jsonData: `
{
  "Producers": [
    {
      "addr": "backbone.cardano.iog.io",
      "port": 3001,
      "valency": 2
    }
  ]
}
`,
		expectedObject: &ouroboros.TopologyConfig{
			Producers: []ouroboros.TopologyConfigLegacyProducer{
				{
					Address: "backbone.cardano.iog.io",
					Port:    3001,
					Valency: 2,
				},
			},
		},
	},
	{
		jsonData: `
{
  "localRoots": [
    {
      "accessPoints": [],
      "advertise": false,
      "valency": 1
    }
  ],
  "publicRoots": [
    {
      "accessPoints": [
        {
          "address": "backbone.cardano.iog.io",
          "port": 3001
        }
      ],
      "advertise": false
    },
    {
      "accessPoints": [
        {
          "address": "backbone.mainnet.emurgornd.com",
          "port": 3001
        }
      ],
      "advertise": false
    }
  ],
  "useLedgerAfterSlot": 99532743
}
`,
		expectedObject: &ouroboros.TopologyConfig{
			LocalRoots: []ouroboros.TopologyConfigP2PLocalRoot{
				{
					AccessPoints: []ouroboros.TopologyConfigP2PAccessPoint{},
					Advertise:    false,
					Valency:      1,
				},
			},
			PublicRoots: []ouroboros.TopologyConfigP2PPublicRoot{
				{
					AccessPoints: []ouroboros.TopologyConfigP2PAccessPoint{
						{
							Address: "backbone.cardano.iog.io",
							Port:    3001,
						},
					},
					Advertise: false,
				},
				{
					AccessPoints: []ouroboros.TopologyConfigP2PAccessPoint{
						{
							Address: "backbone.mainnet.emurgornd.com",
							Port:    3001,
						},
					},
					Advertise: false,
				},
			},
			UseLedgerAfterSlot: 99532743,
		},
	},
}

func TestParseTopologyConfig(t *testing.T) {
	for _, test := range topologyTests {
		topology, err := ouroboros.NewTopologyConfigFromReader(
			strings.NewReader(test.jsonData),
		)
		if err != nil {
			t.Fatalf("failed to load TopologyConfig from JSON data: %s", err)
		}
		if !reflect.DeepEqual(topology, test.expectedObject) {
			t.Fatalf(
				"did not get expected object\n  got:\n    %#v\n  wanted:\n    %#v",
				topology,
				test.expectedObject,
			)
		}
	}
}
