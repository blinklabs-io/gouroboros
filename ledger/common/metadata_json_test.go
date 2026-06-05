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

package common_test

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ExampleParseCardanoCLIMetadataJSONNoSchema() {
	metadata, err := common.ParseCardanoCLIMetadataJSONNoSchema(
		[]byte(`{"0":"cardano"}`),
	)
	if err != nil {
		panic(err)
	}
	auxiliaryData := common.NewShelleyAuxiliaryData(metadata)
	auxiliaryCbor, err := cbor.Encode(auxiliaryData)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", auxiliaryCbor)
	// Output:
	// a1006763617264616e6f
}

func TestParseCardanoCLIMetadataJSONNoSchemaRoundTrip(t *testing.T) {
	input := []byte(`{
		"721": {
			"0xdeadbeef": {
				"Asset": {
					"name": "Test NFT",
					"image": "ipfs://example",
					"bytes": "0x001122",
					"large": 9223372036854775808,
					"nested": [{"-1": "negative key"}]
				}
			}
		}
	}`)

	metadata, err := common.ParseCardanoCLIMetadataJSONNoSchema(input)
	require.NoError(t, err)
	decoded := roundTripMetadata(t, metadata)

	root := requireMetaMap(t, decoded)
	label721 := requireMetaMap(t, metaMapValueByUintKey(t, root, 721))
	policy := requireMetaMap(t, label721.Pairs[0].Value)
	asset := requireMetaMap(t, metaMapValueByTextKey(t, policy, "Asset"))

	assert.IsType(t, common.MetaBytes{}, label721.Pairs[0].Key)
	assert.Equal(
		t,
		[]byte{0xde, 0xad, 0xbe, 0xef},
		label721.Pairs[0].Key.(common.MetaBytes).Value,
	)
	assert.Equal(
		t,
		"9223372036854775808",
		metaMapValueByTextKey(t, asset, "large").(common.MetaInt).Value.String(),
	)
	assert.Equal(
		t,
		[]byte{0x00, 0x11, 0x22},
		metaMapValueByTextKey(t, asset, "bytes").(common.MetaBytes).Value,
	)
}

func TestParseCardanoCLIMetadataJSONDetailedSchemaRoundTrip(t *testing.T) {
	input := []byte(`{
		"0": {"string": "cardano"},
		"1": {"int": 9223372036854775808},
		"2": {"bytes": "2512a00e9653fe49a44a5886202e24d77eeb998f"},
		"3": {"list": [{"string": "test"}, {"int": -1}]},
		"4": {"map": [
			{
				"k": {"list": [{"int": 1}, {"bytes": "ff"}]},
				"v": {"map": [
					{"k": {"string": "key"}, "v": {"string": "value"}}
				]}
			},
			{"k": {"int": 14}, "v": {"int": 42}}
		]}
	}`)

	metadata, err := common.ParseCardanoCLIMetadataJSONDetailedSchema(input)
	require.NoError(t, err)
	decoded := roundTripMetadata(t, metadata)

	root := requireMetaMap(t, decoded)
	assert.Equal(
		t,
		"9223372036854775808",
		metaMapValueByUintKey(t, root, 1).(common.MetaInt).Value.String(),
	)
	assert.Equal(
		t,
		"2512a00e9653fe49a44a5886202e24d77eeb998f",
		hex.EncodeToString(
			metaMapValueByUintKey(t, root, 2).(common.MetaBytes).Value,
		),
	)

	detailedMap := requireMetaMap(t, metaMapValueByUintKey(t, root, 4))
	var foundListKey bool
	for _, pair := range detailedMap.Pairs {
		if _, ok := pair.Key.(common.MetaList); ok {
			foundListKey = true
			assert.IsType(t, common.MetaMap{}, pair.Value)
		}
	}
	assert.True(t, foundListKey, "expected detailed map to keep list key")
}

func TestParseCardanoCLIMetadataJSONCanonicalMapOrder(t *testing.T) {
	metadata, err := common.ParseCardanoCLIMetadataJSONNoSchema(
		[]byte(`{"10":"b","1":"a"}`),
	)
	require.NoError(t, err)

	encoded, err := cbor.Encode(metadata)
	require.NoError(t, err)
	assert.Equal(t, "a20161610a6162", hex.EncodeToString(encoded))
}

func TestParseCardanoCLIMetadataJSONCanonicalMapOrderUsesShortLex(
	t *testing.T,
) {
	metadata, err := common.ParseCardanoCLIMetadataJSONDetailedSchema(
		[]byte(`{
			"0": {"map": [
				{"k": {"int": 24}, "v": {"string": "int"}},
				{"k": {"string": ""}, "v": {"string": "empty"}}
			]}
		}`),
	)
	require.NoError(t, err)

	root := requireMetaMap(t, metadata)
	nested := requireMetaMap(t, metaMapValueByUintKey(t, root, 0))
	require.Len(t, nested.Pairs, 2)
	assert.IsType(t, common.MetaText{}, nested.Pairs[0].Key)
	assert.IsType(t, common.MetaInt{}, nested.Pairs[1].Key)

	encoded, err := cbor.Encode(metadata)
	require.NoError(t, err)
	assert.Equal(
		t,
		"a100a26065656d707479181863696e74",
		hex.EncodeToString(encoded),
	)
}

func TestParseCardanoCLIMetadataJSONRejectsInvalidLabels(t *testing.T) {
	tests := []string{
		`{"18446744073709551616":"too large"}`,
		`{"01":"leading zero"}`,
		`{"-1":"negative"}`,
		`{"not-a-label":"text"}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := common.ParseCardanoCLIMetadataJSONNoSchema([]byte(input))
			require.Error(t, err)
		})
	}
}

func TestParseCardanoCLIMetadataJSONRejectsDetailedInvalidHex(t *testing.T) {
	_, err := common.ParseCardanoCLIMetadataJSONDetailedSchema(
		[]byte(`{"0":{"bytes":"0xzz"}}`),
	)
	require.Error(t, err)
}

func TestParseCardanoCLIMetadataJSONRejectsDuplicateMapEntries(t *testing.T) {
	_, err := common.ParseCardanoCLIMetadataJSONDetailedSchema([]byte(`{
		"0": {"map": [
			{"k": {"int": 1}, "v": {"string": "a"}},
			{"k": {"int": 1}, "v": {"string": "b"}}
		]}
	}`))
	require.Error(t, err)

	_, err = common.ParseCardanoCLIMetadataJSONNoSchema(
		[]byte(`{"0":{"0":"a","-0":"b"}}`),
	)
	require.Error(t, err)
}

func TestParseCardanoCLIMetadataJSONRejectsMalformedDetailedMapEntries(
	t *testing.T,
) {
	tests := []string{
		`{"0":{"map":[{"k":{"int":1}}]}}`,
		`{"0":{"map":[{"k":{"int":1},"v":{"int":2},"x":{"int":3}}]}}`,
		`{"0":{"map":[{"k":{"int":1},"k":{"int":2},"v":{"int":3}}]}}`,
		`{"0":{"int":1,"string":"x"}}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := common.ParseCardanoCLIMetadataJSONDetailedSchema(
				[]byte(input),
			)
			require.Error(t, err)
		})
	}
}

func roundTripMetadata(
	t *testing.T,
	metadata common.TransactionMetadatum,
) common.TransactionMetadatum {
	t.Helper()

	encoded, err := cbor.Encode(metadata)
	require.NoError(t, err)

	decoded, err := common.DecodeMetadatumRaw(encoded)
	require.NoError(t, err)

	reencoded, err := cbor.Encode(decoded)
	require.NoError(t, err)
	assert.Equal(t, encoded, reencoded)

	return decoded
}

func requireMetaMap(
	t *testing.T,
	metadata common.TransactionMetadatum,
) common.MetaMap {
	t.Helper()

	ret, ok := metadata.(common.MetaMap)
	require.True(t, ok, "expected MetaMap, got %T", metadata)
	return ret
}

func metaMapValueByUintKey(
	t *testing.T,
	metadata common.MetaMap,
	key uint64,
) common.TransactionMetadatum {
	t.Helper()

	want := new(big.Int).SetUint64(key)
	for _, pair := range metadata.Pairs {
		key, ok := pair.Key.(common.MetaInt)
		if ok && key.Value != nil && key.Value.Cmp(want) == 0 {
			return pair.Value
		}
	}
	require.FailNow(t, fmt.Sprintf("missing metadata int key %d", key))
	return nil
}

func metaMapValueByTextKey(
	t *testing.T,
	metadata common.MetaMap,
	key string,
) common.TransactionMetadatum {
	t.Helper()

	for _, pair := range metadata.Pairs {
		textKey, ok := pair.Key.(common.MetaText)
		if ok && textKey.Value == key {
			return pair.Value
		}
	}
	require.FailNow(t, fmt.Sprintf("missing metadata text key %q", key))
	return nil
}
