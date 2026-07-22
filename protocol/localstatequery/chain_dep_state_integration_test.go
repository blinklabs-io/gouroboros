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

//go:build chain_dep_state_integration

// This file is excluded from the default Go build. To run it:
//
//   GOUROBOROS_NTC_SOCKET=/path/to/node.socket \
//   GOUROBOROS_NETWORK_MAGIC=764824073 \
//   go test -tags=chain_dep_state_integration \
//     -run TestDebugChainDepStateIntegration -v \
//     ./protocol/localstatequery/...
//
// The test connects to a running, synced cardano-node over its node-to-client
// socket and exercises DebugChainDepState / GetOpCertCounters end-to-end
// against the real wire format. It satisfies the "spot-check against a live
// node" acceptance criterion for the on-chain opcert counter (issue #1890) and
// is gated by a build tag so CI without a node does not attempt the call.

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

func TestDebugChainDepStateIntegration(t *testing.T) {
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

	client := oConn.LocalStateQuery().Client

	state, err := client.DebugChainDepState()
	if err != nil {
		t.Fatalf("DebugChainDepState: %s", err)
	}
	switch state.Protocol {
	case localstatequery.ChainDepStateProtocolPraos,
		localstatequery.ChainDepStateProtocolTPraos:
	default:
		t.Fatalf("unexpected protocol tag: %d", state.Protocol)
	}
	if len(state.OpCertCounters) == 0 {
		t.Fatal("expected at least one on-chain opcert counter on a synced node")
	}
	t.Logf(
		"chain-dep-state OK: protocol=%d slot=%+v counters=%d",
		state.Protocol,
		state.LastSlot,
		len(state.OpCertCounters),
	)

	// The convenience helper must agree with the full result.
	counters, err := client.GetOpCertCounters()
	if err != nil {
		t.Fatalf("GetOpCertCounters: %s", err)
	}
	if len(counters) != len(state.OpCertCounters) {
		t.Fatalf(
			"GetOpCertCounters size mismatch: got %d want %d",
			len(counters),
			len(state.OpCertCounters),
		)
	}
	for pool, want := range state.OpCertCounters {
		if got, ok := counters[pool]; !ok || got != want {
			t.Fatalf("counter for %s: got (%d,%v) want (%d,true)",
				pool, got, ok, want)
		}
	}
}
