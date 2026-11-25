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

package localtxmonitor

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// testMempool is a mempool whose snapshot the test changes, with an
// AwaitMempoolFunc that waits for a change.
type testMempool struct {
	mu      sync.Mutex
	slot    uint64
	txs     []TxAndEraId
	changed chan struct{}
	awaited chan []TxAndEraId
}

func newTestMempool(txs []TxAndEraId) *testMempool {
	return &testMempool{
		slot:    1,
		txs:     txs,
		changed: make(chan struct{}),
		awaited: make(chan []TxAndEraId, 1),
	}
}

func (m *testMempool) get(CallbackContext) (uint64, uint32, []TxAndEraId, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.slot, 1000, m.txs, nil
}

func (m *testMempool) await(
	ctx context.Context,
	_ CallbackContext,
	_ uint64,
	held []TxAndEraId,
) error {
	m.awaited <- held
	select {
	case <-m.changed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *testMempool) add(tx TxAndEraId) {
	m.mu.Lock()
	m.txs = append(append([]TxAndEraId{}, m.txs...), tx)
	m.slot++
	m.mu.Unlock()
	close(m.changed)
}

// startAwaitPair connects a client and a server whose config is cfg. The
// client has no acquire timeout: the wait for a change has no bound.
func startAwaitPair(t *testing.T, cfg Config) (*Client, *Server, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	clientMuxer := muxer.New(connA)
	serverMuxer := muxer.New(connB)
	errorChan := make(chan error, 10)
	opts := func(m *muxer.Muxer) protocol.ProtocolOptions {
		return protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{},
				RemoteAddr: &net.TCPAddr{},
			},
			Muxer:     m,
			ErrorChan: errorChan,
			Mode:      protocol.ProtocolModeNodeToClient,
		}
	}
	cfg.AcquireTimeout = 0
	client := NewClient(opts(clientMuxer), &cfg)
	server := NewServer(opts(serverMuxer), &cfg)
	client.Start()
	server.Start()
	clientMuxer.Start()
	serverMuxer.Start()
	t.Cleanup(func() {
		_ = client.Protocol.Stop(context.Background())
		_ = server.Stop(context.Background())
		_ = clientMuxer.Stop(context.Background())
		_ = serverMuxer.Stop(context.Background())
		connA.Close()
		connB.Close()
	})
	return client, server, errorChan
}

// TestServerAwaitAcquireWaitsForChange acquires a snapshot, then acquires
// again, which is MsgAwaitAcquire. The reference node answers only once the
// mempool differs from the held snapshot, so with AwaitMempoolFunc set the
// server must not answer while the mempool is unchanged, and must answer
// with the new snapshot once it changes.
func TestServerAwaitAcquireWaitsForChange(t *testing.T) {
	first := TxAndEraId{EraId: 6, Tx: make([]byte, 10)}
	mempool := newTestMempool([]TxAndEraId{first})
	cfg := NewConfig(
		WithGetMempoolFunc(mempool.get),
		WithAwaitMempoolFunc(mempool.await),
	)
	client, _, errorChan := startAwaitPair(t, cfg)
	require.NoError(t, client.Acquire())

	awaited := make(chan error, 1)
	go func() { awaited <- client.Acquire() }()
	select {
	case held := <-mempool.awaited:
		require.Equal(t, []TxAndEraId{first}, held)
	case err := <-awaited:
		t.Fatalf("MsgAwaitAcquire answered with an unchanged mempool: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server never waited for a mempool change")
	}
	select {
	case err := <-awaited:
		t.Fatalf("MsgAwaitAcquire answered with an unchanged mempool: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	mempool.add(TxAndEraId{EraId: 6, Tx: make([]byte, 25)})
	select {
	case err := <-awaited:
		require.NoError(t, err)
	case err := <-errorChan:
		t.Fatalf("protocol failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("MsgAwaitAcquire not answered after the mempool changed")
	}
	require.Equal(t, uint64(2), client.acquiredSlot)
	_, size, numTxs, err := client.GetSizes()
	require.NoError(t, err)
	require.Equal(t, uint32(35), size)
	require.Equal(t, uint32(2), numTxs)
}

// TestServerAwaitAcquireStopsWithServer stops a server that is waiting for
// a mempool change and requires the wait to end.
func TestServerAwaitAcquireStopsWithServer(t *testing.T) {
	mempool := newTestMempool(nil)
	ended := make(chan error, 1)
	cfg := NewConfig(
		WithGetMempoolFunc(mempool.get),
		WithAwaitMempoolFunc(func(
			ctx context.Context,
			cctx CallbackContext,
			slot uint64,
			txs []TxAndEraId,
		) error {
			err := mempool.await(ctx, cctx, slot, txs)
			ended <- err
			return err
		}),
	)
	client, server, _ := startAwaitPair(t, cfg)
	require.NoError(t, client.Acquire())
	go func() { _ = client.Acquire() }()
	select {
	case <-mempool.awaited:
	case <-time.After(5 * time.Second):
		t.Fatal("server never waited for a mempool change")
	}
	_ = server.Stop(context.Background())
	select {
	case err := <-ended:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("stopping the server did not end the wait")
	}
}

// TestServerAwaitAcquireWithoutFuncAnswersAtOnce keeps today's behaviour
// when no AwaitMempoolFunc is configured.
func TestServerAwaitAcquireWithoutFuncAnswersAtOnce(t *testing.T) {
	mempool := newTestMempool(nil)
	client, _, _ := startAwaitPair(t, NewConfig(WithGetMempoolFunc(mempool.get)))
	require.NoError(t, client.Acquire())
	require.NoError(t, client.Acquire())
	require.Equal(t, uint64(1), client.acquiredSlot)
}
