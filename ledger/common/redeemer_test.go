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

package common

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedeemerTagMarshalJSON(t *testing.T) {
	tests := []struct {
		tag      RedeemerTag
		expected string
	}{
		{RedeemerTagSpend, `"spend"`},
		{RedeemerTagMint, `"mint"`},
		{RedeemerTagCert, `"cert"`},
		{RedeemerTagReward, `"reward"`},
		{RedeemerTagVoting, `"voting"`},
		{RedeemerTagProposing, `"proposing"`},
		{RedeemerTagGuarding, `"guarding"`},
		{RedeemerTagObserve, `"observe"`},
	}
	for _, tc := range tests {
		data, err := json.Marshal(tc.tag)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, string(data))
	}
}

func TestRedeemerTagUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected RedeemerTag
	}{
		{`"spend"`, RedeemerTagSpend},
		{`"mint"`, RedeemerTagMint},
		{`"cert"`, RedeemerTagCert},
		{`"reward"`, RedeemerTagReward},
		{`"voting"`, RedeemerTagVoting},
		{`"proposing"`, RedeemerTagProposing},
		{`"guarding"`, RedeemerTagGuarding},
		{`"observe"`, RedeemerTagObserve},
	}
	for _, tc := range tests {
		var tag RedeemerTag
		err := json.Unmarshal([]byte(tc.input), &tag)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, tag)
	}
}

func TestRedeemerTagMarshalJSONUnknown(t *testing.T) {
	tag := RedeemerTag(99)
	_, err := json.Marshal(tag)
	assert.Error(t, err)
}

func TestRedeemerTagUnmarshalJSONUnknown(t *testing.T) {
	var tag RedeemerTag
	err := json.Unmarshal([]byte(`"unknown"`), &tag)
	assert.Error(t, err)
}

func TestRedeemerTagRoundTrip(t *testing.T) {
	for tag := RedeemerTagSpend; tag <= RedeemerTagObserve; tag++ {
		data, err := json.Marshal(tag)
		require.NoError(t, err)

		var decoded RedeemerTag
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, tag, decoded)
	}
}

func TestExUnitsMarshalJSON(t *testing.T) {
	eu := ExUnits{Memory: 1000, Steps: 2000}
	data, err := json.Marshal(eu)
	require.NoError(t, err)
	assert.JSONEq(t, `{"memory":1000,"steps":2000}`, string(data))
}

func TestExUnitsRoundTrip(t *testing.T) {
	eu := ExUnits{Memory: 500000, Steps: 1000000}
	data, err := json.Marshal(eu)
	require.NoError(t, err)

	var decoded ExUnits
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, eu.Memory, decoded.Memory)
	assert.Equal(t, eu.Steps, decoded.Steps)
}

func TestRedeemerKeyMarshalJSON(t *testing.T) {
	key := RedeemerKey{Tag: RedeemerTagSpend, Index: 0}
	data, err := json.Marshal(key)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"spend","index":0}`, string(data))
}

func TestRedeemerKeyRoundTrip(t *testing.T) {
	key := RedeemerKey{Tag: RedeemerTagMint, Index: 5}
	data, err := json.Marshal(key)
	require.NoError(t, err)

	var decoded RedeemerKey
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, key.Tag, decoded.Tag)
	assert.Equal(t, key.Index, decoded.Index)
}

// TestRedeemerKeyCborRoundTripObserveTag pins CIP-0112's Observe tag through
// RedeemerKey's raw CBOR tuple ([tag, index]) encoding. cbor.Encode/Decode
// on RedeemerKey never validate the tag against redeemerTagNames -- that
// only happens in the JSON codec -- so an Observe-tagged key must round-trip
// exactly like any other tag.
func TestRedeemerKeyCborRoundTripObserveTag(t *testing.T) {
	key := RedeemerKey{Tag: RedeemerTagObserve, Index: 7}
	encoded, err := cbor.Encode(key)
	require.NoError(t, err)

	var decoded RedeemerKey
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	assert.Equal(t, key.Tag, decoded.Tag)
	assert.Equal(t, key.Index, decoded.Index)
}

// TestRedeemerValueCborRoundTripWithObserveKey pins that a RedeemerValue
// paired with an Observe-tagged RedeemerKey round-trips through CBOR
// unaffected by the key's tag -- the value's own encoding has no
// tag-dependent behavior.
func TestRedeemerValueCborRoundTripWithObserveKey(t *testing.T) {
	val := RedeemerValue{
		Data:    Datum{Data: data.NewInteger(big.NewInt(42))},
		ExUnits: ExUnits{Memory: 100, Steps: 200},
	}
	encoded, err := cbor.Encode(val)
	require.NoError(t, err)

	var decoded RedeemerValue
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	assert.Equal(t, val.ExUnits.Memory, decoded.ExUnits.Memory)
	assert.Equal(t, val.ExUnits.Steps, decoded.ExUnits.Steps)
}

func TestRedeemerValueMarshalJSON(t *testing.T) {
	val := RedeemerValue{
		ExUnits: ExUnits{Memory: 100, Steps: 200},
	}
	data, err := json.Marshal(val)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "data")
	assert.Contains(t, parsed, "exUnits")
}
