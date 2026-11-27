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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// tipServer is a node-to-node chain-sync server whose chain grows only when
// the test produces a block. A request it receives at its tip is answered
// with MsgAwaitReply and then with the next block produced.
type tipServer struct {
	mu       sync.Mutex
	cond     *sync.Cond
	produced uint64
	served   uint64
	closed   bool
	finds    chan struct{}
	consumed chan uint64
}

// tipWindowBlock returns a post-Byron block whose header body starts with
// blockNumber, the only field the client reads from it.
func tipWindowBlock(t *testing.T, blockNumber uint64) []byte {
	t.Helper()
	header := []any{[]any{blockNumber, blockNumber}, []byte{}}
	data, err := cbor.Encode([]any{header, []any{}, []any{}, map[uint]any{}})
	require.NoError(t, err)
	return data
}

func (s *tipServer) tip() Tip {
	return Tip{
		Point:       pcommon.NewPoint(s.produced, testPointHash(byte(s.produced))),
		BlockNumber: s.produced,
	}
}

func (s *tipServer) produce() {
	s.mu.Lock()
	s.produced++
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *tipServer) close() {
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()
}

func newTipWindowPair(
	t *testing.T,
	limit int,
	initialTip uint64,
) (*Client, *tipServer) {
	t.Helper()
	srv := &tipServer{
		produced: initialTip,
		finds:    make(chan struct{}, 8),
		consumed: make(chan uint64, 256),
	}
	srv.cond = sync.NewCond(&srv.mu)
	clientConn, serverConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	errs := make(chan error, 16)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:     serverMuxer,
			ErrorChan: errs,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&Config{
			FindIntersectFunc: func(
				_ CallbackContext,
				points []pcommon.Point,
			) (pcommon.Point, Tip, error) {
				srv.mu.Lock()
				defer srv.mu.Unlock()
				srv.finds <- struct{}{}
				srv.served = points[0].Slot
				return points[0], srv.tip(), nil
			},
			RequestNextFunc: func(ctx CallbackContext) error {
				srv.mu.Lock()
				if srv.served >= srv.produced {
					srv.mu.Unlock()
					if err := ctx.Server.AwaitReply(); err != nil {
						return err
					}
					srv.mu.Lock()
					for srv.served >= srv.produced && !srv.closed {
						srv.cond.Wait()
					}
					if srv.closed {
						srv.mu.Unlock()
						return nil
					}
				}
				srv.served++
				n, tip := srv.served, srv.tip()
				srv.mu.Unlock()
				return ctx.Server.RollForward(
					ledger.BlockTypeConway,
					tipWindowBlock(t, n),
					tip,
				)
			},
		},
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			ErrorChan:    errs,
			Mode:         protocol.ProtocolModeNodeToNode,
		},
		&Config{
			PipelineLimit: limit,
			RollForwardRawFunc: func(
				_ CallbackContext,
				_ uint,
				headerCbor []byte,
				_ Tip,
			) error {
				n, ok := headerBlockNumber(ledger.BlockTypeConway, headerCbor)
				require.True(t, ok)
				srv.consumed <- n
				return nil
			},
		},
	)
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = clientMuxer.Stop(context.Background())
		_ = serverMuxer.Stop(context.Background())
	})
	t.Cleanup(func() {
		srv.close()
		_ = client.Stop(context.Background())
	})
	return client, srv
}

// A client that has caught up keeps one request outstanding at the server's
// tip, not a full window, and Reintersect then needs only the next block to
// drain. With a full window each outstanding request needs its own new
// block, so Reintersect waits PipelineLimit blocks.
func TestClientWindowAtServerTip(t *testing.T) {
	t.Parallel()
	const limit = 10
	const initialTip = 5
	client, srv := newTipWindowPair(t, limit, initialTip)
	require.NoError(t, client.Sync([]pcommon.Point{
		pcommon.NewPoint(0, testPointHash(0)),
	}))
	recv(t, srv.finds, "initial FindIntersect")
	for want := uint64(1); want <= initialTip; want++ {
		require.Equal(t, want, recv(t, srv.consumed, "block"))
	}
	outstanding := func() int {
		client.lifecycleMutex.Lock()
		defer client.lifecycleMutex.Unlock()
		return client.outstandingRequestNext
	}
	require.Eventually(t, func() bool { return outstanding() == 1 },
		reintersectTestTimeout, time.Millisecond,
		"requests outstanding at the server's tip")

	done := make(chan error, 1)
	go func() {
		_, _, err := client.Reintersect([]pcommon.Point{
			pcommon.NewPoint(initialTip, testPointHash(initialTip)),
		})
		done <- err
	}()
	srv.produce()
	require.Equal(t, uint64(initialTip+1), recv(t, srv.consumed, "next block"))
	recv(t, srv.finds, "FindIntersect after one new block")
	require.NoError(t, recv(t, done, "Reintersect"))
}

// While behind the server's tip the client still pipelines a full window.
func TestClientWindowFillsWhileBehind(t *testing.T) {
	t.Parallel()
	const limit = 4
	client, srv := newTipWindowPair(t, limit, 200)
	maxOut := 0
	client.testSyncLoopAfterRequestNext = func() {
		client.lifecycleMutex.Lock()
		maxOut = max(maxOut, client.outstandingRequestNext)
		client.lifecycleMutex.Unlock()
	}
	require.NoError(t, client.Sync([]pcommon.Point{
		pcommon.NewPoint(0, testPointHash(0)),
	}))
	recv(t, srv.finds, "initial FindIntersect")
	for want := uint64(1); want <= 20; want++ {
		require.Equal(t, want, recv(t, srv.consumed, "block"))
	}
	client.lifecycleMutex.Lock()
	got := maxOut
	client.lifecycleMutex.Unlock()
	require.Equal(t, limit, got)
}
