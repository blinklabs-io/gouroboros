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

package muxer

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newPipeMuxer(t *testing.T) (*Muxer, net.Conn) {
	t.Helper()
	local, remote := net.Pipe()
	m := New(local)
	t.Cleanup(func() {
		_ = remote.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Stop(ctx)
	})
	return m, remote
}

func requireClosedSoon(t *testing.T, ch <-chan *Segment, what string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatalf("%s was not closed", what)
		}
	}
}

// Stop returns only after the read loop has finished, including the sweep
// that stops every receiver. The test holds the lock that sweep takes, so the
// read loop cannot finish while it is held.
func TestStopWaitsForReadLoopSweep(t *testing.T) {
	m, _ := newPipeMuxer(t)
	m.protocolReceiversMutex.Lock()
	held := true
	defer func() {
		if held {
			m.protocolReceiversMutex.Unlock()
		}
	}()
	ctx, cancel := context.WithTimeout(
		context.Background(),
		100*time.Millisecond,
	)
	defer cancel()
	require.ErrorIs(t, m.Stop(ctx), context.DeadlineExceeded,
		"Stop returned while the read loop was still in its exit sweep")
	m.protocolReceiversMutex.Unlock()
	held = false
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	require.NoError(t, m.Stop(ctx2))
}

// A second RegisterProtocol of a role that is still registered replaces its
// receiver and sender.
// The replaced receiver's forwarder must be stopped, or Stop waits for it
// for ever, and a segment queued on the replaced send channel must not reach
// the wire.
func TestReregisterLiveRoleStopsReplacedReceiver(t *testing.T) {
	m, remote := newPipeMuxer(t)
	firstSend, first, _ := m.RegisterProtocol(1, ProtocolRoleInitiator)
	require.NotNil(t, first)
	secondSend, second, _ := m.RegisterProtocol(1, ProtocolRoleInitiator)
	require.NotNil(t, second)
	requireClosedSoon(t, first, "the replaced receiver channel")

	// One segment is header plus a one-byte payload; the payload names the
	// send channel it was queued on.
	headerSize := binary.Size(SegmentHeader{})
	wire := make([]byte, headerSize+1)
	firstSend <- NewSegment(1, []byte{1}, false)
	require.NoError(t, remote.SetReadDeadline(
		time.Now().Add(200*time.Millisecond),
	))
	_, err := io.ReadFull(remote, wire)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded,
		"a segment queued on the replaced send channel reached the wire")
	secondSend <- NewSegment(1, []byte{2}, false)
	require.NoError(t, remote.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = io.ReadFull(remote, wire)
	require.NoError(t, err)
	require.Equal(t, byte(2), wire[headerSize],
		"the segment on the wire is not the current sender's")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, m.Stop(ctx))
	requireClosedSoon(t, second, "the current receiver channel")
	_, open := <-m.ErrorChan()
	require.False(t, open, "ErrorChan is closed once Stop has joined")
}

// forward parks in cond.Wait while its queue is empty; each change that makes
// its predicate true (a segment queued, the receiver stopped) wakes it. Each
// row is one wake source.
func TestForwarderWakesOnEveryPredicateChange(t *testing.T) {
	tests := []struct {
		name string
		wake func(*Muxer, *segmentChannel)
		// delivered is whether a segment reaches the receive channel
		// before it closes.
		delivered bool
	}{
		{
			name: "segment queued",
			wake: func(_ *Muxer, sc *segmentChannel) {
				_ = sc.enqueue(newTestSegment(1))
			},
			delivered: true,
		},
		{
			name: "receiver unregistered",
			wake: func(m *Muxer, _ *segmentChannel) {
				m.UnregisterProtocol(1, ProtocolRoleResponder)
			},
		},
		{
			name: "muxer stopped",
			wake: func(m *Muxer, _ *segmentChannel) {
				_ = m.Stop(context.Background())
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newPipeMuxer(t)
			_, recv, _ := m.RegisterProtocol(1, ProtocolRoleResponder)
			require.NotNil(t, recv)
			m.protocolReceiversMutex.Lock()
			sc := m.protocolReceivers[1][ProtocolRoleResponder]
			m.protocolReceiversMutex.Unlock()
			tc.wake(m, sc)
			select {
			case seg, ok := <-recv:
				require.Equal(t, tc.delivered, ok && seg != nil)
			case <-time.After(5 * time.Second):
				t.Fatal("forwarder was not woken")
			}
		})
	}
}
