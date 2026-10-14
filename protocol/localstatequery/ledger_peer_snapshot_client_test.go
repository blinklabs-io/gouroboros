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

package localstatequery_test

import (
	"math/big"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetLedgerPeerSnapshotEraWrappedReply runs GetLedgerPeerSnapshot against
// a node that answers inside the single-element era result wrapper, for each
// snapshot version.
func TestGetLedgerPeerSnapshotEraWrappedReply(t *testing.T) {
	const relay = " 9f 83 00 19 0b b9 4d" +
		" 72 65 6c 61 79 2e 65 78 61 6d 70 6c 65 ff"
	cases := []struct {
		name     string
		version  uint16
		peerKind localstatequery.LedgerPeerKind
		wire     string
		want     uint64
		hasAcc   bool
	}{
		{
			name:     "v1_bare_request",
			version:  22,
			peerKind: localstatequery.LedgerPeerKindBig,
			wire: "81 82 01 82 82 01 19 30 39" +
				" 9f 82 82 01 0a 82 82 01 0a" + relay + " ff",
			want:   1,
			hasAcc: true,
		},
		{
			name:     "v2_big_peers",
			version:  23,
			peerKind: localstatequery.LedgerPeerKindBig,
			wire: "81 82 02 83 81 00 1a 2d 96 4a 09" +
				" 9f 83 82 01 0a 82 01 0a" + relay + " ff",
			want:   2,
			hasAcc: true,
		},
		{
			name:     "v3_all_peers",
			version:  23,
			peerKind: localstatequery.LedgerPeerKindAll,
			wire: "81 82 03 83 81 00 1a 2d 96 4a 09" +
				" 9f 82 82 01 0a" + relay + " ff",
			want: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runTest(
				t,
				conversationEraQueryAt(
					tc.version,
					6,
					poolDistr2Hex(t, tc.wire),
				),
				func(t *testing.T, oConn *ouroboros.Connection) {
					snap, err := oConn.LocalStateQuery().Client.
						GetLedgerPeerSnapshot(tc.peerKind)
					require.NoError(t, err)
					require.NotNil(t, snap)
					assert.Equal(t, tc.want, snap.Version)
					require.Len(t, snap.Pools, 1)
					pool := snap.Pools[0]
					require.NotNil(t, pool.Detail.PoolStake)
					assert.Zero(
						t,
						pool.Detail.PoolStake.Cmp(big.NewRat(1, 10)),
					)
					assert.Equal(t, tc.hasAcc, pool.AccumulatedStake != nil)
					require.Len(t, pool.Detail.Relays, 1)
					r := pool.Detail.Relays[0]
					require.NotNil(t, r.Domain)
					require.NotNil(t, r.Port)
					assert.Equal(t, "relay.example", *r.Domain)
					assert.Equal(t, uint16(3001), *r.Port)
				},
			)
		})
	}
}
