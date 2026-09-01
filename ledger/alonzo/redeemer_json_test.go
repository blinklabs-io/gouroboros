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

package alonzo

import (
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlonzoRedeemerMarshalJSON(t *testing.T) {
	r := AlonzoRedeemer{
		Tag:     common.RedeemerTagSpend,
		Index:   0,
		ExUnits: common.ExUnits{Memory: 1000, Steps: 2000},
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"tag": "spend",
		"index": 0,
		"data": null,
		"exUnits": {"memory": 1000, "steps": 2000}
	}`, string(data))
}

func TestAlonzoRedeemerRoundTrip(t *testing.T) {
	r := AlonzoRedeemer{
		Tag:     common.RedeemerTagMint,
		Index:   3,
		ExUnits: common.ExUnits{Memory: 500000, Steps: 1000000},
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded AlonzoRedeemer
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, r.Tag, decoded.Tag)
	assert.Equal(t, r.Index, decoded.Index)
	assert.Equal(t, r.ExUnits.Memory, decoded.ExUnits.Memory)
	assert.Equal(t, r.ExUnits.Steps, decoded.ExUnits.Steps)
}

func TestAlonzoRedeemersMarshalJSON(t *testing.T) {
	redeemers := AlonzoRedeemers{
		Redeemers: []AlonzoRedeemer{
			{
				Tag:     common.RedeemerTagSpend,
				Index:   0,
				ExUnits: common.ExUnits{Memory: 100, Steps: 200},
			},
			{
				Tag:     common.RedeemerTagMint,
				Index:   1,
				ExUnits: common.ExUnits{Memory: 300, Steps: 400},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	// Should be a flat array, not {"Redeemers": [...]}
	var arr []json.RawMessage
	err = json.Unmarshal(data, &arr)
	require.NoError(t, err)
	assert.Len(t, arr, 2)
}

// Empty/nil AlonzoRedeemers serializes as "[]" for consistency with ConwayRedeemers.
func TestAlonzoRedeemersEmptyMarshalJSON(t *testing.T) {
	redeemers := AlonzoRedeemers{}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data))
}

func TestAlonzoRedeemersRoundTrip(t *testing.T) {
	redeemers := AlonzoRedeemers{
		Redeemers: []AlonzoRedeemer{
			{
				Tag:     common.RedeemerTagSpend,
				Index:   0,
				ExUnits: common.ExUnits{Memory: 100, Steps: 200},
			},
			{
				Tag:     common.RedeemerTagCert,
				Index:   2,
				ExUnits: common.ExUnits{Memory: 500, Steps: 600},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	var decoded AlonzoRedeemers
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Len(t, decoded.Redeemers, 2)
	assert.Equal(t, common.RedeemerTagSpend, decoded.Redeemers[0].Tag)
	assert.Equal(t, uint32(0), decoded.Redeemers[0].Index)
	assert.Equal(t, common.RedeemerTagCert, decoded.Redeemers[1].Tag)
	assert.Equal(t, uint32(2), decoded.Redeemers[1].Index)
}

func TestAlonzoTransactionWitnessSetMarshalJSON(t *testing.T) {
	ws := AlonzoTransactionWitnessSet{
		WsRedeemers: AlonzoRedeemers{
			Redeemers: []AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagSpend,
					Index:   0,
					ExUnits: common.ExUnits{Memory: 100, Steps: 200},
				},
			},
		},
	}
	data, err := json.Marshal(ws)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "WsRedeemers")
}
