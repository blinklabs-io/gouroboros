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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func srvSnapshotFixture() LedgerPeerSnapshotResult {
	srv := RelayAccessPoint{Kind: RelayKindSRV, Domain: ptrStr("_r._tcp.x")}
	ipv4 := RelayAccessPoint{
		Kind: RelayKindIPv4,
		IPv4: ptrIP(net.IPv4(192, 0, 2, 1)),
		Port: ptrUint16(3001),
	}
	return LedgerPeerSnapshotResult{
		Version: 1,
		Slot:    WithOriginSlot{HasSlot: true, Slot: 7},
		Pools: []PoolLedgerPeers{
			{
				AccumulatedStake: newRat(1, 4),
				Detail: PoolLedgerPeersDetail{
					PoolStake: newRat(1, 4),
					Relays:    []RelayAccessPoint{srv, ipv4},
				},
			},
			{
				AccumulatedStake: newRat(1, 2),
				Detail: PoolLedgerPeersDetail{
					PoolStake: newRat(1, 4),
					Relays:    []RelayAccessPoint{srv},
				},
			},
		},
	}
}

// querySnapshotOverPipe answers GetLedgerPeerSnapshot with snap from a server
// at the given node-to-client version and returns what the client decodes.
func querySnapshotOverPipe(
	t *testing.T,
	version uint16,
	snap LedgerPeerSnapshotResult,
) *LedgerPeerSnapshotResult {
	t.Helper()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	serverErrors := make(chan error, 8)
	clientErrors := make(chan error, 8)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	server := NewServer(protocol.ProtocolOptions{
		Muxer:        serverMuxer,
		ConnectionId: connID,
		ErrorChan:    serverErrors,
		Version:      version,
	}, &Config{
		AcquireFunc: func(CallbackContext, AcquireTarget, bool) error {
			return nil
		},
		QueryFunc: func(_ CallbackContext, q QueryWrapper) (any, error) {
			if ledgerPeerSnapshotQuery(q) != nil {
				return []any{snap}, nil
			}
			return 6, nil
		},
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer:        clientMuxer,
		ConnectionId: connID,
		ErrorChan:    clientErrors,
		Version:      version,
	}, &Config{
		AcquireTimeout: 2 * time.Second,
		QueryTimeout:   2 * time.Second,
	})
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
		server.Stop()
		client.Stop()
		serverMuxer.Stop()
		clientMuxer.Stop()
	})
	require.NoError(t, local.SetDeadline(time.Now().Add(10*time.Second)))
	require.NoError(t, remote.SetDeadline(time.Now().Add(10*time.Second)))
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	got, err := client.GetLedgerPeerSnapshot(LedgerPeerKindBig)
	require.NoError(t, err)
	require.NotNil(t, got)
	return got
}

// TestServerDropsSRVRelaysBeforeVersion22 checks a version 1 snapshot sent
// below node-to-client version 22 carries no SRV relay, and no pool left
// without a relay, while version 22 keeps both.
func TestServerDropsSRVRelaysBeforeVersion22(t *testing.T) {
	snap := srvSnapshotFixture()
	got := querySnapshotOverPipe(t, ntc(21), snap)
	require.Len(t, got.Pools, 1)
	relays := got.Pools[0].Detail.Relays
	require.Len(t, relays, 1)
	assert.Equal(t, RelayKindIPv4, relays[0].Kind)
	require.NotNil(t, got.Pools[0].AccumulatedStake)
	assert.Zero(t, got.Pools[0].AccumulatedStake.Cmp(newRat(1, 4).Rat))
	assert.Len(t, snap.Pools[0].Detail.Relays, 2, "callback snapshot changed")

	got = querySnapshotOverPipe(t, ntc(22), snap)
	require.Len(t, got.Pools, 2)
	assert.Len(t, got.Pools[0].Detail.Relays, 2)
	assert.Equal(t, RelayKindSRV, got.Pools[1].Detail.Relays[0].Kind)
}

// TestLedgerPeerSnapshotRejectsBlockHashSize checks a version 2 or 3 snapshot
// refuses to encode a block hash that is not a 32-byte header hash.
func TestLedgerPeerSnapshotRejectsBlockHashSize(t *testing.T) {
	for _, version := range []uint64{2, 3} {
		for _, size := range []int{0, 31, 33} {
			_, err := LedgerPeerSnapshotResult{
				Version:   version,
				Slot:      WithOriginSlot{HasSlot: true, Slot: 1},
				BlockHash: make([]byte, size),
			}.MarshalCBOR()
			assert.Error(t, err, "version %d, %d-byte hash", version, size)
		}
		_, err := LedgerPeerSnapshotResult{
			Version:   version,
			Slot:      WithOriginSlot{HasSlot: true, Slot: 1},
			BlockHash: make([]byte, 32),
		}.MarshalCBOR()
		assert.NoError(t, err, "version %d, 32-byte hash", version)
	}
}
