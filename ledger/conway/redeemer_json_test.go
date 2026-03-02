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

package conway

import (
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConwayRedeemersMarshalJSON(t *testing.T) {
	redeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: 0}: {
				ExUnits: common.ExUnits{Memory: 100, Steps: 200},
			},
			{Tag: common.RedeemerTagMint, Index: 1}: {
				ExUnits: common.ExUnits{Memory: 300, Steps: 400},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	// Should be a flat array
	var arr []json.RawMessage
	err = json.Unmarshal(data, &arr)
	require.NoError(t, err)
	assert.Len(t, arr, 2)
}

func TestConwayRedeemersMarshalJSONSorted(t *testing.T) {
	redeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagMint, Index: 1}: {
				ExUnits: common.ExUnits{Memory: 300, Steps: 400},
			},
			{Tag: common.RedeemerTagSpend, Index: 0}: {
				ExUnits: common.ExUnits{Memory: 100, Steps: 200},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	var entries []struct {
		Tag   string `json:"tag"`
		Index uint32 `json:"index"`
	}
	err = json.Unmarshal(data, &entries)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	// Spend (tag 0) should come before Mint (tag 1)
	assert.Equal(t, "spend", entries[0].Tag)
	assert.Equal(t, "mint", entries[1].Tag)
}

func TestConwayRedeemersEmptyMarshalJSON(t *testing.T) {
	redeemers := ConwayRedeemers{}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data))
}

func TestConwayRedeemersRoundTrip(t *testing.T) {
	redeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: 0}: {
				ExUnits: common.ExUnits{Memory: 100, Steps: 200},
			},
			{Tag: common.RedeemerTagVoting, Index: 3}: {
				ExUnits: common.ExUnits{Memory: 500, Steps: 600},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	var decoded ConwayRedeemers
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Len(t, decoded.Redeemers, 2)

	spendKey := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 0}
	votingKey := common.RedeemerKey{Tag: common.RedeemerTagVoting, Index: 3}

	spendVal, ok := decoded.Redeemers[spendKey]
	require.True(t, ok)
	assert.Equal(t, common.Datum{}, spendVal.Data)
	assert.Equal(t, int64(100), spendVal.ExUnits.Memory)
	assert.Equal(t, int64(200), spendVal.ExUnits.Steps)

	votingVal, ok := decoded.Redeemers[votingKey]
	require.True(t, ok)
	assert.Equal(t, common.Datum{}, votingVal.Data)
	assert.Equal(t, int64(500), votingVal.ExUnits.Memory)
	assert.Equal(t, int64(600), votingVal.ExUnits.Steps)
}

func TestConwayRedeemersUnmarshalJSONDuplicateKey(t *testing.T) {
	jsonData := `[
		{"tag":"spend","index":0,"data":null,"exUnits":{"memory":100,"steps":200}},
		{"tag":"spend","index":0,"data":null,"exUnits":{"memory":300,"steps":400}}
	]`
	var r ConwayRedeemers
	err := json.Unmarshal([]byte(jsonData), &r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate redeemer key")
}

func TestConwayRedeemersLegacyMarshalJSON(t *testing.T) {
	redeemers := ConwayRedeemers{
		legacy: true,
		legacyRedeemers: alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagSpend,
					Index:   0,
					ExUnits: common.ExUnits{Memory: 100, Steps: 200},
				},
			},
		},
	}
	data, err := json.Marshal(redeemers)
	require.NoError(t, err)

	var arr []json.RawMessage
	err = json.Unmarshal(data, &arr)
	require.NoError(t, err)
	assert.Len(t, arr, 1)
}

// NOTE: These assertions use Go struct field names (e.g. "WsRedeemers")
// since the structs have no json tags. They will break if fields are renamed
// or json tags are added.
func TestConwayTransactionWitnessSetMarshalJSON(t *testing.T) {
	ws := ConwayTransactionWitnessSet{
		VkeyWitnesses: cbor.NewSetType(
			[]common.VkeyWitness{
				{Vkey: []byte{0x01, 0x02}, Signature: []byte{0x03, 0x04}},
			},
			false,
		),
		WsRedeemers: ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
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
	assert.Contains(t, parsed, "VkeyWitnesses")
}

func TestConwayTransactionMarshalJSON(t *testing.T) {
	tx := ConwayTransaction{
		TxIsValid: true,
		WitnessSet: ConwayTransactionWitnessSet{
			WsRedeemers: ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagSpend, Index: 0}: {
						ExUnits: common.ExUnits{Memory: 100, Steps: 200},
					},
				},
			},
		},
	}
	data, err := json.Marshal(tx)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "WitnessSet")
	assert.Contains(t, parsed, "TxIsValid")
}
