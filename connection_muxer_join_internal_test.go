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

package ouroboros

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// blockingMetrics holds the first IngressQueueDepth call until released.
type blockingMetrics struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blockingMetrics) IngressQueueDepth(uint16, muxer.ProtocolRole, int) {
	first := false
	b.once.Do(func() { first = true })
	if !first {
		return
	}
	close(b.entered)
	<-b.release
}

func (*blockingMetrics) IngressDeliveryBlocked(
	uint16, muxer.ProtocolRole, time.Duration,
) {
}

func (*blockingMetrics) IngressBackpressure(
	uint16, muxer.ProtocolRole, time.Duration,
) {
}

// TestCloseWaitsForMuxerMetricsCallback checks that Close returns only after
// a muxer Metrics callback already running has returned: the reference mux
// waits for its ingress and egress threads, as for every mini-protocol
// thread, before its run returns (network-mux Network/Mux.hs:280-287).
func TestCloseWaitsForMuxerMetricsCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	metrics := &blockingMetrics{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseMetrics := func() { releaseOnce.Do(func() { close(metrics.release) }) }
	t.Cleanup(releaseMetrics)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestOutput,
			ouroboros_mock.ConversationEntryHandshakeNtNResponseInput,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: keepalive.ProtocolId,
				Messages: []protocol.Message{
					keepalive.NewMsgKeepAlive(7),
				},
			},
		},
	)
	t.Cleanup(func() { _ = mockConn.Close() })
	conn, err := NewConnection(
		WithConnection(mockConn),
		WithNodeToNode(true),
		WithServer(true),
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		WithKeepAliveConfig(keepalive.NewConfig()),
		func(c *Connection) {
			// The muxer reads the keep-alive request only once it is
			// started, after this hook.
			c.beforeProtocolErrorForwarderHook = func() {
				c.muxer.SetMetrics(metrics)
			}
		},
	)
	require.NoError(t, err)
	select {
	case <-metrics.entered:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("metrics callback did not run")
	}
	closed := make(chan struct{})
	go func() {
		_ = conn.Close(context.Background())
		close(closed)
	}()
	// Close cannot return while the callback is held; this bounds how long
	// the test gives it to do so wrongly.
	select {
	case <-closed:
		t.Fatal("Close returned while a muxer metrics callback was running")
	case <-time.After(500 * time.Millisecond):
	}
	releaseMetrics()
	select {
	case <-closed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close did not return after the metrics callback returned")
	}
}

// closingMetrics closes its connection from the first IngressQueueDepth
// call, with the muxer's CallbackContext.
type closingMetrics struct {
	once     sync.Once
	conn     func() *Connection
	returned chan error
}

func (c *closingMetrics) IngressQueueDepth(uint16, muxer.ProtocolRole, int) {
	c.once.Do(func() {
		conn := c.conn()
		c.returned <- conn.Close(conn.muxer.CallbackContext())
	})
}

func (*closingMetrics) IngressDeliveryBlocked(
	uint16, muxer.ProtocolRole, time.Duration,
) {
}

func (*closingMetrics) IngressBackpressure(
	uint16, muxer.ProtocolRole, time.Duration,
) {
}

// TestMetricsCallbackClosesConnection checks that a muxer Metrics callback
// may close its own connection with the muxer's CallbackContext: Close
// returns without waiting for the shutdown, which waits for the callback,
// and the shutdown then finishes.
func TestMetricsCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	var conn *Connection
	var connMu sync.Mutex
	metrics := &closingMetrics{
		conn: func() *Connection {
			connMu.Lock()
			defer connMu.Unlock()
			return conn
		},
		returned: make(chan error, 1),
	}
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestOutput,
			ouroboros_mock.ConversationEntryHandshakeNtNResponseInput,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: keepalive.ProtocolId,
				Messages: []protocol.Message{
					keepalive.NewMsgKeepAlive(7),
				},
			},
		},
	)
	t.Cleanup(func() { _ = mockConn.Close() })
	newConn, err := NewConnection(
		WithConnection(mockConn),
		WithNodeToNode(true),
		WithServer(true),
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		WithKeepAliveConfig(keepalive.NewConfig()),
		func(c *Connection) {
			c.beforeProtocolErrorForwarderHook = func() {
				connMu.Lock()
				conn = c
				connMu.Unlock()
				c.muxer.SetMetrics(metrics)
			}
		},
	)
	require.NoError(t, err)
	select {
	case err := <-metrics.returned:
		require.NoError(t, err)
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close from a muxer Metrics callback did not return")
	}
	closed := make(chan struct{})
	go func() {
		_ = newConn.Close(context.Background())
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("the shutdown requested by the Metrics callback did not finish")
	}
}
