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

package localstatequery

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilteredShelleyQueriesDecode(t *testing.T) {
	const poolID = "1cb2c9bb54fb097fba58542c5adcf3a7d094db95e8b9c4e3d0bcbf1d"
	poolIDBytes, err := hex.DecodeString(poolID)
	require.NoError(t, err)
	wantPoolID := ledger.PoolId(ledger.NewBlake2b224(poolIDBytes))
	tests := []struct {
		name  string
		wire  string
		want  any
		check func(*testing.T, any)
	}{
		{
			name: "non-myopic member rewards",
			wire: "8202d901028182001903e8",
			want: &ShelleyNonMyopicMemberRewardsQuery{},
			check: func(t *testing.T, query any) {
				stakes := query.(*ShelleyNonMyopicMemberRewardsQuery).
					Stakes.Items()
				require.Len(t, stakes, 1)
				assert.Equal(t, "82001903e8", hex.EncodeToString(stakes[0]))
			},
		},
		{
			name: "pool state",
			wire: "82138181581c" + poolID,
			want: &ShelleyPoolStateQuery{},
			check: func(t *testing.T, query any) {
				filters := query.(*ShelleyPoolStateQuery).Pools
				require.Len(t, filters, 1)
				assert.Equal(t, []ledger.PoolId{wantPoolID}, filters[0].Items())
			},
		},
		{
			name: "pool distribution",
			wire: "82158181581c" + poolID,
			want: &ShelleyPoolDistrQuery{},
			check: func(t *testing.T, query any) {
				filters := query.(*ShelleyPoolDistrQuery).Pools
				require.Len(t, filters, 1)
				assert.Equal(t, []ledger.PoolId{wantPoolID}, filters[0].Items())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := hex.DecodeString(test.wire)
			require.NoError(t, err)

			query, err := decodeShelleyQuery(wire)
			require.NoError(t, err)
			require.IsType(t, test.want, query)
			test.check(t, query)

			roundTrip, err := cbor.Encode(query)
			require.NoError(t, err)
			assert.Equal(t, wire, roundTrip)
		})
	}
}

func TestFilteredPoolQueriesRejectInvalidMaybe(t *testing.T) {
	tests := []struct {
		name string
		wire string
	}{
		{name: "pool state", wire: "8213828080"},
		{name: "pool distribution", wire: "8215828080"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := hex.DecodeString(test.wire)
			require.NoError(t, err)

			_, err = decodeShelleyQuery(wire)
			require.ErrorContains(t, err, "expected at most one pool set")
		})
	}
}

func TestFilteredShelleyQueriesRejectOversizedSetsBeforeDecode(t *testing.T) {
	const oversizedCount = maxLocalStateQuerySetItems + 1
	definiteSet := append(
		[]byte{0xd9, 0x01, 0x02, 0x99, 0x27, 0x11},
		bytes.Repeat([]byte{0xf6}, oversizedCount)...,
	)
	indefiniteSet := append(
		[]byte{0xd9, 0x01, 0x02, 0x9f},
		bytes.Repeat([]byte{0xf6}, oversizedCount)...,
	)
	indefiniteSet = append(indefiniteSet, 0xff)

	tests := []struct {
		name      string
		queryType byte
		poolQuery bool
	}{
		{
			name:      "non-myopic member rewards",
			queryType: QueryTypeShelleyNonMyopicMemberRewards,
		},
		{
			name:      "pool state",
			queryType: QueryTypeShelleyPoolState,
			poolQuery: true,
		},
		{
			name:      "pool distribution",
			queryType: QueryTypeShelleyPoolDistr,
			poolQuery: true,
		},
	}
	for _, test := range tests {
		for _, set := range []struct {
			name string
			wire []byte
		}{
			{name: "definite", wire: definiteSet},
			{name: "indefinite", wire: indefiniteSet},
		} {
			t.Run(test.name+"/"+set.name, func(t *testing.T) {
				wire := []byte{0x82, test.queryType}
				if test.poolQuery {
					wire = append(wire, 0x81)
				}
				wire = append(wire, set.wire...)

				_, err := decodeShelleyQuery(wire)
				require.ErrorContains(
					t,
					err,
					"maximum is 10000",
				)
			})
		}
	}
}

func TestPoolFilterClientParamsMatchDecoder(t *testing.T) {
	const poolID = "1cb2c9bb54fb097fba58542c5adcf3a7d094db95e8b9c4e3d0bcbf1d"
	poolIDBytes, err := hex.DecodeString(poolID)
	require.NoError(t, err)
	pool := ledger.PoolId(ledger.NewBlake2b224(poolIDBytes))
	queries := []struct {
		name   string
		typeID int
	}{
		{name: "pool state", typeID: QueryTypeShelleyPoolState},
		{name: "pool distribution", typeID: QueryTypeShelleyPoolDistr},
	}
	for _, query := range queries {
		for _, tc := range []struct {
			name string
			ids  []any
		}{
			{name: "all pools"},
			{name: "specific pool", ids: []any{pool}},
		} {
			t.Run(query.name+"/"+tc.name, func(t *testing.T) {
				params, err := poolFilterQueryParam(tc.ids)
				require.NoError(t, err)
				wire, err := cbor.Encode(buildQuery(query.typeID, params))
				require.NoError(t, err)
				decoded, err := decodeShelleyQuery(wire)
				require.NoError(t, err)

				var pools []cbor.SetType[ledger.PoolId]
				switch q := decoded.(type) {
				case *ShelleyPoolStateQuery:
					pools = q.Pools
				case *ShelleyPoolDistrQuery:
					pools = q.Pools
				default:
					t.Fatalf("unexpected query type %T", decoded)
				}
				if len(tc.ids) == 0 {
					require.Empty(t, pools, "empty StrictMaybe must select all pools")
					return
				}
				require.Len(t, pools, 1)
				assert.Equal(t, []ledger.PoolId{pool}, pools[0].Items())
			})
		}
	}
}
