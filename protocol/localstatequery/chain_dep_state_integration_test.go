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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugChainDepStateIntegration(t *testing.T) {
	socketPath := os.Getenv("GOUROBOROS_NTC_SOCKET")
	if socketPath == "" {
		t.Skip("GOUROBOROS_NTC_SOCKET not set; skipping integration test")
	}
	magicStr := os.Getenv("GOUROBOROS_NETWORK_MAGIC")
	require.NotEmpty(
		t,
		magicStr,
		"GOUROBOROS_NETWORK_MAGIC must be set when GOUROBOROS_NTC_SOCKET is set",
	)
	magic, err := strconv.ParseUint(magicStr, 10, 32)
	require.NoError(t, err, "invalid GOUROBOROS_NETWORK_MAGIC %q", magicStr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	require.NoError(t, err, "dial %s", socketPath)

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(uint32(magic)),
		ouroboros.WithNodeToNode(false),
		ouroboros.WithKeepAlive(false),
	)
	require.NoError(t, err, "ouroboros handshake")
	defer func() {
		_ = oConn.Close()
	}()

	client := oConn.LocalStateQuery().Client

	state, err := client.DebugChainDepState()
	require.NoError(t, err, "DebugChainDepState")
	require.Contains(
		t,
		[]localstatequery.ChainDepStateProtocol{
			localstatequery.ChainDepStateProtocolPraos,
			localstatequery.ChainDepStateProtocolTPraos,
		},
		state.Protocol,
		"unexpected protocol tag",
	)
	require.NotEmpty(
		t,
		state.OpCertCounters,
		"expected at least one on-chain opcert counter on a synced node",
	)
	t.Logf(
		"chain-dep-state OK: protocol=%d slot=%+v counters=%d",
		state.Protocol,
		state.LastSlot,
		len(state.OpCertCounters),
	)

	// The convenience helper must agree with the full result.
	counters, err := client.GetOpCertCounters()
	require.NoError(t, err, "GetOpCertCounters")
	assert.Len(t, counters, len(state.OpCertCounters))
	for pool, want := range state.OpCertCounters {
		got, ok := counters[pool]
		assert.Truef(t, ok, "counter for %s missing", pool)
		assert.Equalf(t, want, got, "counter for %s", pool)
	}
}
