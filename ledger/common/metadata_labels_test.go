// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestAuxiliaryDataMetadataLabelPolicy(t *testing.T) {
	tests := []struct {
		name      string
		encoded   string
		duplicate bool
		nested    bool
		labels    int
		last      int64
	}{
		{name: "distinct", encoded: "a200010102", labels: 2, last: 1},
		{
			name:      "identical",
			encoded:   "a200010001",
			duplicate: true,
			labels:    1,
			last:      1,
		},
		{
			name:      "conflicting",
			encoded:   "a200010002",
			duplicate: true,
			labels:    1,
			last:      2,
		},
		{
			name:      "different integer encoding",
			encoded:   "a20001180002",
			duplicate: true,
			labels:    1,
			last:      2,
		},
		{
			name:      "nonadjacent",
			encoded:   "a3000101090002",
			duplicate: true,
			labels:    2,
			last:      2,
		},
		{
			name:      "indefinite",
			encoded:   "bf00010002ff",
			duplicate: true,
			labels:    1,
			last:      2,
		},
		{
			name:    "nested pairs",
			encoded: "a100a201010102",
			nested:  true,
			labels:  1,
		},
	}
	forms := []struct{ name, prefix, suffix string }{
		{"direct", "", ""},
		{"legacy array", "82", "80"},
		{"tag259", "d90103a100", ""},
	}
	eras := []struct {
		name   string
		era    AuxiliaryDataEra
		reject bool
	}{
		{"Babbage", AuxiliaryDataEraBabbage, false},
		{"Conway", AuxiliaryDataEraConway, true},
		{"Dijkstra", AuxiliaryDataEraDijkstra, true},
	}
	for _, tc := range tests {
		for _, form := range forms {
			for _, era := range eras {
				t.Run(tc.name+"/"+form.name+"/"+era.name, func(t *testing.T) {
					raw, err := hex.DecodeString(
						form.prefix + tc.encoded + form.suffix,
					)
					require.NoError(t, err)
					aux, err := DecodeAuxiliaryDataForEra(raw, era.era)
					var set TransactionMetadataSet
					blockRaw := append([]byte{0xa1, 0x00}, raw...)
					require.NoError(t, set.UnmarshalCBOR(blockRaw))
					if tc.duplicate && era.reject {
						require.ErrorContains(
							t,
							err,
							"duplicate transaction metadata label 0",
						)
						require.ErrorContains(
							t,
							set.ValidateAuxiliaryDataForEra(era.era),
							"duplicate transaction metadata label 0",
						)
						return
					}
					require.NoError(t, err)
					require.NoError(t, set.ValidateAuxiliaryDataForEra(era.era))
					metadata, err := aux.Metadata()
					require.NoError(t, err)
					require.IsType(t, MetaMap{}, metadata)
					labels := metadata.(MetaMap)
					require.Len(t, labels.Pairs, tc.labels)
					require.NotEmpty(t, labels.Pairs)
					cached, found := set.GetMetadata(0)
					require.True(t, found)
					require.Equal(t, metadata, cached)
					if tc.nested {
						require.IsType(t, MetaMap{}, labels.Pairs[0].Value)
						nested := labels.Pairs[0].Value.(MetaMap)
						require.Len(t, nested.Pairs, 2)
						require.Equal(
							t,
							int64(1),
							nested.Pairs[0].Value.(MetaInt).Value.Int64(),
						)
						require.Equal(
							t,
							int64(2),
							nested.Pairs[1].Value.(MetaInt).Value.Int64(),
						)
					} else {
						require.Equal(t, tc.last, labels.Pairs[0].Value.(MetaInt).Value.Int64())
					}
					encoded, err := cbor.Encode(aux)
					require.NoError(t, err)
					require.Equal(t, raw, encoded)
					encoded, err = cbor.Encode(labels)
					require.NoError(t, err)
					originalMetadata, err := hex.DecodeString(tc.encoded)
					require.NoError(t, err)
					require.Equal(t, originalMetadata, encoded)
					require.Equal(t, blockRaw, set.Cbor())
				})
			}
		}
	}
}

func TestMetadataLabelsStillRequireUnsignedIntegers(t *testing.T) {
	for _, encoded := range []string{
		"a12001",                         // negative label
		"a1410001",                       // byte string label
		"a2c249010000000000000000010002", // overflow before valid label
		"a20001c24901000000000000000002", // overflow after valid label
	} {
		raw, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		for _, era := range []AuxiliaryDataEra{AuxiliaryDataEraBabbage, AuxiliaryDataEraConway} {
			_, err := DecodeAuxiliaryDataForEra(raw, era)
			require.Error(t, err)
		}
	}
}
