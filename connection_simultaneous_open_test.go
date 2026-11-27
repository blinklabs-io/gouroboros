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
	"context"
	"errors"
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type simOpenConnResult struct {
	conn *ouroboros.Connection
	err  error
}

// simOpenPeer is how one of two dialling connections is configured.
type simOpenPeer struct {
	magic      uint32
	fullDuplex bool
}

// dialBothWays opens a node-to-node connection from each end of one pipe,
// as two nodes that dial each other at the same moment do: neither side
// runs the handshake server.
func dialBothWays(
	t *testing.T,
	left, right simOpenPeer,
) (simOpenConnResult, simOpenConnResult) {
	t.Helper()
	leftPipe, rightPipe := net.Pipe()
	t.Cleanup(func() {
		_ = leftPipe.Close()
		_ = rightPipe.Close()
	})
	dial := func(conn net.Conn, peer simOpenPeer) <-chan simOpenConnResult {
		result := make(chan simOpenConnResult, 1)
		go func() {
			c, err := ouroboros.NewConnection(
				ouroboros.WithConnection(conn),
				ouroboros.WithNodeToNode(true),
				ouroboros.WithNetworkMagic(peer.magic),
				ouroboros.WithFullDuplex(peer.fullDuplex),
				ouroboros.WithDelayProtocolStart(true),
			)
			result <- simOpenConnResult{conn: c, err: err}
		}()
		return result
	}
	leftChan := dial(leftPipe, left)
	rightChan := dial(rightPipe, right)
	await := func(ch <-chan simOpenConnResult) simOpenConnResult {
		select {
		case r := <-ch:
			if r.conn != nil {
				t.Cleanup(func() { _ = r.conn.Close(context.Background()) })
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
	left, right := dialBothWays(
		t,
		simOpenPeer{magic: 42, fullDuplex: true},
		simOpenPeer{magic: 42, fullDuplex: true},
	)
	require.NoError(t, left.err)
	require.NoError(t, right.err)
	leftVersion, _ := left.conn.ProtocolVersion()
	rightVersion, _ := right.conn.ProtocolVersion()
	require.NotZero(t, leftVersion)
	require.Equal(t, leftVersion, rightVersion)
}

// TestSimultaneousOpenConnectionsMixedModes covers a full-duplex connection
// and an initiator-only one that dial each other at once: both end with the
// agreed, initiator-only data, as the reference client returns the result of
// acceptVersion (Handshake/Client.hs 213-215), so neither derives a
// full-duplex muxer the other does not run.
func TestSimultaneousOpenConnectionsMixedModes(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	left, right := dialBothWays(
		t,
		simOpenPeer{magic: 42, fullDuplex: true},
		simOpenPeer{magic: 42, fullDuplex: false},
	)
	require.NoError(t, left.err)
	require.NoError(t, right.err)
	_, leftData := left.conn.ProtocolVersion()
	_, rightData := right.conn.ProtocolVersion()
	require.NotNil(t, leftData)
	require.Equal(t, leftData, rightData)
	require.Equal(
		t,
		protocol.DiffusionModeInitiatorOnly,
		leftData.DiffusionMode(),
	)
}

// TestSimultaneousOpenConnectionsRefuse covers two connections on different
// networks that dial each other at once. A refusing side closes the
// connection without sending a reason, so the side that refuses second may
// see the close before the peer's proposal; at least one side must report
// the refusal itself, and neither may finish.
func TestSimultaneousOpenConnectionsRefuse(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	left, right := dialBothWays(
		t,
		simOpenPeer{magic: 42, fullDuplex: true},
		simOpenPeer{magic: 43, fullDuplex: true},
	)
	require.Error(t, left.err)
	require.Error(t, right.err)
	var refused *handshake.RefusedError
	require.True(
		t,
		errors.As(left.err, &refused) || errors.As(right.err, &refused),
		"neither side refused: %v; %v", left.err, right.err,
	)
}
