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

package blockfetch

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// heldRangeHarness connects a pipelining client to a real server that holds
// its answer to a MsgRequestRange until releaseAnswer is called, and then
// answers MsgNoBlocks. Nothing closes the muxers on a protocol error, so a
// test sees what the client itself does after one.
type heldRangeHarness struct {
	client        *Client
	clientMuxer   *muxer.Muxer
	clientErrors  chan error
	releaseAnswer func()
}

func newHeldRangeHarness(t *testing.T, clientErrors chan error) *heldRangeHarness {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	requested := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAnswer := func() { releaseOnce.Do(func() { close(release) }) }
	serverCfg, err := NewConfig(WithRequestRangeFunc(func(
		ctx CallbackContext, _ pcommon.Point, _ pcommon.Point,
	) error {
		select {
		case requested <- struct{}{}:
		default:
		}
		go func() {
			<-release
			_ = ctx.Server.NoBlocks()
		}()
		return nil
	}))
	require.NoError(t, err)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: make(chan error, 4),
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &serverCfg)

	clientCfg, err := NewConfig(
		WithRequestPipelining(true),
		WithRangeDoneFunc(func(CallbackContext, error) error { return nil }),
	)
	require.NoError(t, err)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &clientCfg)

	server.Start()
	t.Cleanup(server.Stop)
	client.Start()
	t.Cleanup(func() { _ = client.Stop() })
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(releaseAnswer)

	_, err = client.RequestRange(context.Background(), RangeRequest{
		Start: testPoint(1),
		End:   testPoint(2),
	})
	require.NoError(t, err)
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received MsgRequestRange")
	}
	return &heldRangeHarness{
		client:        client,
		clientMuxer:   clientMuxer,
		clientErrors:  clientErrors,
		releaseAnswer: releaseAnswer,
	}
}

// testPoint returns a point with a well-formed 32-byte hash.
func testPoint(slot uint64) pcommon.Point {
	hash := make([]byte, 32)
	hash[0] = byte(slot)
	return pcommon.NewPoint(slot, hash)
}

// stopWithoutDone calls Stop while the server holds its answer, so
// MsgClientDone cannot be sent, and requires ErrStoppedWithoutDone on the
// error channel.
func (h *heldRangeHarness) stopWithoutDone(t *testing.T) {
	t.Helper()
	require.Error(t, h.client.Stop(),
		"Stop delivered MsgClientDone with a range outstanding")
	select {
	case err := <-h.clientErrors:
		require.ErrorIs(t, err, ErrStoppedWithoutDone)
	case <-time.After(time.Second):
		t.Fatal("Stop left the server owing an answer without reporting an error")
	}
}

// TestStopWithRangeOutstandingReportsError stops a client while the server
// still owes the answer to a MsgRequestRange. MsgClientDone is legal only in
// StIdle, so Stop gives up without it and the server is left mid-range. The
// client must report ErrStoppedWithoutDone so its connection is closed.
func TestStopWithRangeOutstandingReportsError(t *testing.T) {
	t.Parallel()
	newHeldRangeHarness(t, make(chan error, 4)).stopWithoutDone(t)
}

// TestStartAfterStopWithoutDoneStaysStopped calls Start straight after a Stop
// that could not deliver MsgClientDone, before anything has closed the
// connection. A new protocol instance would receive the server's answer to
// the abandoned range in StIdle, so Start must leave the client stopped.
func TestStartAfterStopWithoutDoneStaysStopped(t *testing.T) {
	t.Parallel()
	h := newHeldRangeHarness(t, make(chan error, 4))
	h.stopWithoutDone(t)

	h.client.Start()
	h.releaseAnswer()
	_, err := h.client.RequestRange(context.Background(), RangeRequest{
		Start: testPoint(3),
		End:   testPoint(4),
	})
	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown,
		"client restarted on a connection where the server still owed an answer")
	select {
	case err := <-h.clientErrors:
		t.Fatalf("client received the abandoned range's answer: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestStopWithoutDoneStopsMuxerWhenErrorDropped stops a client with a range
// outstanding whose error channel nobody reads, so SendError drops
// ErrStoppedWithoutDone. The muxer must still be stopped, or the server's
// late answer could reach a new protocol instance on the same connection.
func TestStopWithoutDoneStopsMuxerWhenErrorDropped(t *testing.T) {
	t.Parallel()
	h := newHeldRangeHarness(t, make(chan error))
	require.Error(t, h.client.Stop())
	select {
	case <-h.clientMuxer.DoneChan():
	case <-time.After(time.Second):
		t.Fatal("muxer still running after Stop abandoned a range")
	}
}

// TestSlowDoneWithNothingOwedIsNotAbandoned stops an idle client whose
// MsgClientDone cannot be written within Stop's bound, because nothing reads
// the connection. MsgClientDone was sendable, so it may still reach the
// peer, which then owes nothing: Stop must not report ErrStoppedWithoutDone,
// stop the muxer, or refuse a later Start.
func TestSlowDoneWithNothingOwedIsNotAbandoned(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	t.Cleanup(clientMuxer.Stop)
	clientErrors := make(chan error, 4)
	clientCfg, err := NewConfig()
	require.NoError(t, err)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &clientCfg)
	client.Start()
	clientMuxer.Start()

	stopped := make(chan error, 1)
	go func() { stopped <- client.Stop() }()
	select {
	case err := <-stopped:
		require.Error(t, err, "MsgClientDone was written although nothing reads")
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
	select {
	case err := <-clientErrors:
		require.NotErrorIs(t, err, ErrStoppedWithoutDone)
	default:
	}
	client.lifecycleMutex.Lock()
	latched := client.stoppedWithoutDone
	client.lifecycleMutex.Unlock()
	require.False(t, latched, "Start refused although nothing was owed")
	select {
	case <-clientMuxer.DoneChan():
		t.Fatal("muxer stopped although nothing was owed")
	default:
	}
}
