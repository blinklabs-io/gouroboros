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

package chainsync

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

// tipHarness connects a client to a server at the chain tip: the server
// answers MsgRequestNext with MsgAwaitReply and sends its roll only when
// releaseRoll is called. Nothing closes the muxers on a protocol error, so a
// test sees what the client itself does after one.
type tipHarness struct {
	client       *Client
	clientMuxer  *muxer.Muxer
	clientErrors chan error
	releaseRoll  func()
}

func newTipHarness(t *testing.T, clientErrors chan error) *tipHarness {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(func() { _ = clientMuxer.Stop(context.Background()) })
	t.Cleanup(func() { _ = serverMuxer.Stop(context.Background()) })

	requested := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRoll := func() { releaseOnce.Do(func() { close(release) }) }
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			CallbackContext, []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			return pcommon.NewPointOrigin(), Tip{}, nil
		}),
		WithRequestNextFunc(func(ctx CallbackContext) error {
			if err := ctx.Server.AwaitReply(); err != nil {
				return err
			}
			select {
			case requested <- struct{}{}:
			default:
			}
			go func() {
				<-release
				_ = ctx.Server.RollBackward(pcommon.NewPointOrigin(), Tip{})
			}()
			return nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: make(chan error, 4),
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &serverCfg)

	clientCfg := NewConfig(
		WithPipelineLimit(1),
		WithRollBackwardFunc(func(CallbackContext, pcommon.Point, Tip) error {
			return nil
		}),
	)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &clientCfg)

	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(releaseRoll)

	require.NoError(t, client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received MsgRequestNext")
	}
	return &tipHarness{
		client:       client,
		clientMuxer:  clientMuxer,
		clientErrors: clientErrors,
		releaseRoll:  releaseRoll,
	}
}

// stopWithoutDone calls Stop while the server holds its roll, so MsgDone
// cannot be sent, and requires ErrStoppedWithoutDone on the error channel.
func (h *tipHarness) stopWithoutDone(t *testing.T) {
	t.Helper()
	require.Error(t, h.client.Stop(context.Background()),
		"Stop delivered MsgDone with a request unanswered")
	select {
	case err := <-h.clientErrors:
		require.ErrorIs(t, err, ErrStoppedWithoutDone)
	case <-time.After(time.Second):
		t.Fatal("Stop left the server owing a reply without reporting an error")
	}
}

// syncRefused requires Sync to fail with ErrProtocolShuttingDown at once. A
// restarted client would instead send MsgFindIntersect and wait for an answer
// the server cannot give from StMustReply, until the protocol is torn down,
// which also ends in ErrProtocolShuttingDown.
func syncRefused(t *testing.T, client *Client) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		result <- client.Sync([]pcommon.Point{pcommon.NewPointOrigin()})
	}()
	select {
	case err := <-result:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown,
			"client restarted on a connection where the server still owed a reply")
	case <-time.After(2 * time.Second):
		t.Fatal("Sync waited on a restarted protocol instead of being refused")
	}
}

// TestStopWithRequestUnansweredReportsError stops a client at the chain tip.
// MsgDone cannot be sent before the server's roll, so Stop gives up without
// it and the server is left owing a reply. The client must report
// ErrStoppedWithoutDone so its connection is closed.
func TestStopWithRequestUnansweredReportsError(t *testing.T) {
	t.Parallel()
	newTipHarness(t, make(chan error, 4)).stopWithoutDone(t)
}

// TestStartAfterStopWithoutDoneStaysStopped calls Start straight after a Stop
// that could not deliver MsgDone, before anything has closed the connection.
// A new protocol instance would receive the server's reply to the abandoned
// request in StIdle, so Start must leave the client stopped.
func TestStartAfterStopWithoutDoneStaysStopped(t *testing.T) {
	t.Parallel()
	h := newTipHarness(t, make(chan error, 4))
	h.stopWithoutDone(t)

	h.client.Start()
	h.releaseRoll()
	syncRefused(t, h.client)
	select {
	case err := <-h.clientErrors:
		t.Fatalf("client received the abandoned request's reply: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestStopWithoutDoneStopsMuxerWhenErrorDropped stops a client at the chain
// tip whose error channel nobody reads, so SendError drops
// ErrStoppedWithoutDone. The muxer must still be stopped, or the server's
// late reply could reach a new protocol instance on the same connection.
func TestStopWithoutDoneStopsMuxerWhenErrorDropped(t *testing.T) {
	t.Parallel()
	h := newTipHarness(t, make(chan error))
	require.Error(t, h.client.Stop(context.Background()))
	select {
	case <-h.clientMuxer.DoneChan():
	case <-time.After(time.Second):
		t.Fatal("muxer still running after Stop abandoned a request")
	}
}

// TestSlowDoneWithNothingOwedIsNotAbandoned stops an idle client whose
// MsgDone cannot be written within Stop's bound, because nothing reads the
// connection. MsgDone was sendable, so it may still reach the peer, which
// then owes nothing: Stop must not report ErrStoppedWithoutDone, stop the
// muxer, or refuse a later Start.
func TestSlowDoneWithNothingOwedIsNotAbandoned(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	t.Cleanup(func() { _ = clientMuxer.Stop(context.Background()) })
	clientErrors := make(chan error, 4)
	clientCfg := NewConfig()
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &clientCfg)
	client.Start()
	clientMuxer.Start()

	stopped := make(chan error, 1)
	go func() { stopped <- client.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		require.Error(t, err, "MsgDone was written although nothing reads")
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
