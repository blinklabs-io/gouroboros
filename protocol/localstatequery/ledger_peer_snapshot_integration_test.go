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

//go:build ledger_peer_snapshot_integration

// This file is excluded from the default Go build. To run it:
//
//   GOUROBOROS_NTC_SOCKET=/path/to/node.socket \
//   GOUROBOROS_NETWORK_MAGIC=764824073 \
//   go test -tags=ledger_peer_snapshot_integration \
//     -run TestGetLedgerPeerSnapshotIntegration -v \
//     ./protocol/localstatequery/...
//
// The test connects to a running cardano-node that has negotiated
// node-to-client protocol version 19 (cardano-node 10.7+) and exercises
// GetLedgerPeerSnapshot end-to-end against the real wire format. It is
// intentionally gated by a build tag so that CI without a live node does
// not attempt the call.

package localstatequery_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
)

func TestGetLedgerPeerSnapshotIntegration(t *testing.T) {
	socketPath := os.Getenv("GOUROBOROS_NTC_SOCKET")
	if socketPath == "" {
		t.Skip("GOUROBOROS_NTC_SOCKET not set; skipping integration test")
	}
	magicStr := os.Getenv("GOUROBOROS_NETWORK_MAGIC")
	if magicStr == "" {
		t.Fatal("GOUROBOROS_NETWORK_MAGIC must be set when GOUROBOROS_NTC_SOCKET is set")
	}
	magic, err := strconv.ParseUint(magicStr, 10, 32)
	if err != nil {
		t.Fatalf("invalid GOUROBOROS_NETWORK_MAGIC %q: %s", magicStr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		t.Fatalf("dial %s: %s", socketPath, err)
	}

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(uint32(magic)),
		ouroboros.WithNodeToNode(false),
		ouroboros.WithKeepAlive(false),
	)
	if err != nil {
		t.Fatalf("ouroboros handshake: %s", err)
	}
	defer func() {
		_ = oConn.Close()
	}()

	snap, err := oConn.LocalStateQuery().Client.GetLedgerPeerSnapshot(
		localstatequery.LedgerPeerKindAll,
	)
	if err != nil {
		t.Fatalf("GetLedgerPeerSnapshot: %s", err)
	}

	if snap.Version != 0 {
		t.Fatalf("snapshot version: got %d want 0 (V1)", snap.Version)
	}
	if len(snap.Pools) == 0 {
		t.Fatal("expected at least one pool in the snapshot")
	}
	t.Logf(
		"snapshot OK: slot=%+v, %d pools, first-pool %d relays",
		snap.Slot,
		len(snap.Pools),
		len(snap.Pools[0].Detail.Relays),
	)
	for i, pool := range snap.Pools {
		if pool.AccumulatedStake == nil || pool.Detail.PoolStake == nil {
			t.Fatalf("pool %d: missing stake", i)
		}
		if len(pool.Detail.Relays) == 0 {
			t.Fatalf("pool %d: empty relays list", i)
		}
		for j, r := range pool.Detail.Relays {
			switch r.Kind {
			case localstatequery.RelayKindIPv4:
				if r.IPv4 == nil || r.Port == nil {
					t.Fatalf("pool %d relay %d: IPv4 missing fields", i, j)
				}
				if r.IPv6 != nil || r.Domain != nil {
					t.Fatalf(
						"pool %d relay %d: IPv4 has incompatible fields set",
						i,
						j,
					)
				}
			case localstatequery.RelayKindIPv6:
				if r.IPv6 == nil || r.Port == nil {
					t.Fatalf("pool %d relay %d: IPv6 missing fields", i, j)
				}
				if r.IPv4 != nil || r.Domain != nil {
					t.Fatalf(
						"pool %d relay %d: IPv6 has incompatible fields set",
						i,
						j,
					)
				}
			case localstatequery.RelayKindDomain:
				if r.Domain == nil || r.Port == nil {
					t.Fatalf("pool %d relay %d: Domain missing fields", i, j)
				}
				if r.IPv4 != nil || r.IPv6 != nil {
					t.Fatalf(
						"pool %d relay %d: Domain has incompatible fields set",
						i,
						j,
					)
				}
			case localstatequery.RelayKindSRV:
				if r.Domain == nil {
					t.Fatalf("pool %d relay %d: SRV missing domain", i, j)
				}
				if r.Port != nil {
					t.Fatalf("pool %d relay %d: SRV must not carry port", i, j)
				}
				if r.IPv4 != nil || r.IPv6 != nil {
					t.Fatalf(
						"pool %d relay %d: SRV has incompatible fields set",
						i,
						j,
					)
				}
			default:
				t.Fatalf("pool %d relay %d: unknown kind %d", i, j, r.Kind)
			}
		}
	}
}
