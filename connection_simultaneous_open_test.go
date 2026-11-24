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

package ouroboros_test

import (
	"errors"
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type simOpenConnResult struct {
	conn *ouroboros.Connection
	err  error
}

// dialBothWays opens a node-to-node connection from each end of one pipe,
// as two nodes that dial each other at the same moment do: neither side
// runs the handshake server.
func dialBothWays(
	t *testing.T,
	leftMagic, rightMagic uint32,
) (simOpenConnResult, simOpenConnResult) {
	t.Helper()
	leftPipe, rightPipe := net.Pipe()
	t.Cleanup(func() {
		_ = leftPipe.Close()
		_ = rightPipe.Close()
	})
	dial := func(conn net.Conn, magic uint32) <-chan simOpenConnResult {
		result := make(chan simOpenConnResult, 1)
		go func() {
			c, err := ouroboros.NewConnection(
				ouroboros.WithConnection(conn),
				ouroboros.WithNodeToNode(true),
				ouroboros.WithNetworkMagic(magic),
				ouroboros.WithFullDuplex(true),
				ouroboros.WithDelayProtocolStart(true),
			)
			result <- simOpenConnResult{conn: c, err: err}
		}()
		return result
	}
	leftChan := dial(leftPipe, leftMagic)
	rightChan := dial(rightPipe, rightMagic)
	await := func(ch <-chan simOpenConnResult) simOpenConnResult {
		select {
		case r := <-ch:
			if r.conn != nil {
				t.Cleanup(func() { _ = r.conn.Close() })
			}
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("simultaneous open did not finish")
			return simOpenConnResult{}
		}
	}
	return await(leftChan), await(rightChan)
}

// TestSimultaneousOpenConnectionsAgree covers two connections that dial each
// other at once: both finish the handshake on the same version.
func TestSimultaneousOpenConnectionsAgree(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	left, right := dialBothWays(t, 42, 42)
	require.NoError(t, left.err)
	require.NoError(t, right.err)
	leftVersion, _ := left.conn.ProtocolVersion()
	rightVersion, _ := right.conn.ProtocolVersion()
	require.NotZero(t, leftVersion)
	require.Equal(t, leftVersion, rightVersion)
}

// TestSimultaneousOpenConnectionsRefuse covers two connections on different
// networks that dial each other at once. A refusing side closes the
// connection without sending a reason, so the side that refuses second may
// see the close before the peer's proposal; at least one side must report
// the refusal itself, and neither may finish.
func TestSimultaneousOpenConnectionsRefuse(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	left, right := dialBothWays(t, 42, 43)
	require.Error(t, left.err)
	require.Error(t, right.err)
	var refused *handshake.RefusedError
	require.True(
		t,
		errors.As(left.err, &refused) || errors.As(right.err, &refused),
		"neither side refused: %v; %v", left.err, right.err,
	)
}
