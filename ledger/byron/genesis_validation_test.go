// Copyright 2026 Blink Labs Software
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

package byron_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

func genesisJSONDocument(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(byronGenesisConfig), &doc))
	return doc
}

func genesisJSONObjectAtPath(
	t *testing.T,
	doc map[string]any,
	path string,
) map[string]any {
	t.Helper()
	object := doc
	for _, part := range strings.Split(path, ".") {
		object = object[part].(map[string]any)
	}
	return object
}

func TestByronGenesisJSONAcceptsUnknownFields(t *testing.T) {
	data := strings.Replace(
		byronGenesisConfig,
		`"avvmDistr":`,
		`"futureExtension": true, "avvmDistr":`,
		1,
	)
	data = strings.Replace(
		data,
		`"heavyDelThd":`,
		`"futureNestedExtension": true, "heavyDelThd":`,
		1,
	)
	_, err := byron.NewByronGenesisFromReader(strings.NewReader(data))
	require.NoError(t, err)
}

func TestNewByronGenesisFromReaderConsumesOneValue(t *testing.T) {
	reader := strings.NewReader(byronGenesisConfig + `
{}`)
	_, err := byron.NewByronGenesisFromReader(reader)
	require.NoError(t, err)
}

func TestByronGenesisJSONRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		path   []string
		fields []string
	}{
		{
			name: "top-level",
			fields: []string{
				"avvmDistr", "blockVersionData", "protocolConsts", "startTime",
				"bootStakeholders", "heavyDelegation", "nonAvvmBalances",
			},
		},
		{
			name: "blockVersionData",
			path: []string{"blockVersionData"},
			fields: []string{
				"heavyDelThd", "maxBlockSize", "maxHeaderSize", "maxProposalSize",
				"maxTxSize", "mpcThd", "scriptVersion", "slotDuration",
				"softforkRule", "txFeePolicy", "unlockStakeEpoch", "updateImplicit",
				"updateProposalThd", "updateVoteThd",
			},
		},
		{
			name:   "softforkRule",
			path:   []string{"blockVersionData", "softforkRule"},
			fields: []string{"initThd", "minThd", "thdDecrement"},
		},
		{
			name:   "txFeePolicy",
			path:   []string{"blockVersionData", "txFeePolicy"},
			fields: []string{"multiplier", "summand"},
		},
		{
			name:   "protocolConsts",
			path:   []string{"protocolConsts"},
			fields: []string{"k", "protocolMagic", "vssMinTTL", "vssMaxTTL"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, field := range test.fields {
				t.Run(field, func(t *testing.T) {
					doc := genesisJSONDocument(t)
					object := doc
					if len(test.path) > 0 {
						object = genesisJSONObjectAtPath(t, doc, strings.Join(test.path, "."))
					}
					delete(object, field)
					data, err := json.Marshal(doc)
					require.NoError(t, err)
					_, err = byron.NewByronGenesisFromReader(strings.NewReader(string(data)))
					require.ErrorContains(t, err, "missing required field")
				})
			}
		})
	}
}

func TestByronGenesisParameterDomains(t *testing.T) {
	check := func(path, value, want string) {
		t.Helper()
		parts := strings.Split(path, ".")
		name := parts[len(parts)-1] + "_" + value
		t.Run(name, func(t *testing.T) {
			doc := genesisJSONDocument(t)
			object := doc
			if len(parts) > 1 {
				parent := strings.Join(parts[:len(parts)-1], ".")
				object = genesisJSONObjectAtPath(t, doc, parent)
			}
			object[parts[len(parts)-1]] = value
			data, err := json.Marshal(doc)
			require.NoError(t, err)
			_, err = byron.NewByronGenesisFromReader(
				strings.NewReader(string(data)),
			)
			if want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, want)
			}
		})
	}
	portionFields := []string{
		"blockVersionData.mpcThd",
		"blockVersionData.heavyDelThd",
		"blockVersionData.updateVoteThd",
		"blockVersionData.updateProposalThd",
		"blockVersionData.softforkRule.initThd",
		"blockVersionData.softforkRule.minThd",
		"blockVersionData.softforkRule.thdDecrement",
	}
	for _, path := range portionFields {
		for _, value := range []string{"0", "1", "1000000000000000"} {
			check(path, value, "")
		}
		check(path, "-1", "between 0")
		check(path, "1000000000000001", "between 0")
	}
	unsignedFields := []string{
		"blockVersionData.slotDuration",
		"blockVersionData.maxBlockSize",
		"blockVersionData.maxHeaderSize",
		"blockVersionData.maxTxSize",
		"blockVersionData.maxProposalSize",
		"blockVersionData.updateImplicit",
	}
	for _, path := range unsignedFields {
		check(path, "0", "")
		check(path, "-1", "non-negative")
	}
}
