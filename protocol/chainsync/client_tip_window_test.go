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

// startWindowPair starts a node-to-node chain-sync client and server over a
// pipe. The client is stopped during cleanup.
func startWindowPair(t *testing.T, serverCfg, clientCfg *Config) *Client {
	t.Helper()
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
		serverCfg,
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			ErrorChan:    errs,
			Mode:         protocol.ProtocolModeNodeToNode,
		},
		clientCfg,
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
		_ = client.Stop(context.Background())
	})
	return client
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
	client := startWindowPair(t,
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
	// Cleanups run last first: the server releases its held requests
	// before the client stops.
	t.Cleanup(srv.close)
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
	require.Eventually(t, func() bool {
		return outstandingRequests(client) == 1
	},
		reintersectTestTimeout, time.Millisecond,
		"requests outstanding at the server's tip")

	quiesced := make(chan struct{})
	// The hook runs under lifecycleMutex just before the loop's stop
	// channel closes, so it only signals.
	client.testQuiesceBeforeLoopStopClose = func() { close(quiesced) }
	done := make(chan error, 1)
	go func() {
		_, _, err := client.Reintersect([]pcommon.Point{
			pcommon.NewPoint(initialTip, testPointHash(initialTip)),
		})
		done <- err
	}()
	// Block six must arrive after Reintersect has stopped the sync loop:
	// a loop still running would request block seven, which is never
	// produced, and Reintersect would wait for it.
	recv(t, quiesced, "sync loop stop")
	srv.produce()
	require.Equal(t, uint64(initialTip+1), recv(t, srv.consumed, "next block"))
	recv(t, srv.finds, "FindIntersect after one new block")
	require.NoError(t, recv(t, done, "Reintersect"))
}

// scriptedTip is far ahead of every block the scripted server serves.
const scriptedTip = 200

// newScriptedWindowPair starts a client whose server answers the client's
// n-th MsgRequestNext, counting from one, with reply(ctx, n, held). held is
// closed before the client stops, so a reply blocked on it can return without
// answering. The client reports the block number of each roll forward it
// delivers on forwarded, and each roll backward on rolledBack.
func newScriptedWindowPair(
	t *testing.T,
	limit int,
	reply func(ctx CallbackContext, n int, held <-chan struct{}) error,
) (client *Client, forwarded <-chan uint64, rolledBack <-chan struct{}) {
	t.Helper()
	var mu sync.Mutex
	requests := 0
	held := make(chan struct{})
	forwards := make(chan uint64, 256)
	rollbacks := make(chan struct{}, 64)
	client = startWindowPair(t,
		&Config{
			FindIntersectFunc: func(
				_ CallbackContext,
				points []pcommon.Point,
			) (pcommon.Point, Tip, error) {
				return points[0], scriptedServerTip(), nil
			},
			RequestNextFunc: func(ctx CallbackContext) error {
				mu.Lock()
				requests++
				n := requests
				mu.Unlock()
				return reply(ctx, n, held)
			},
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
				forwards <- n
				return nil
			},
			RollBackwardFunc: func(
				CallbackContext, pcommon.Point, Tip,
			) error {
				rollbacks <- struct{}{}
				return nil
			},
		},
	)
	t.Cleanup(func() { close(held) })
	require.NoError(t, client.Sync([]pcommon.Point{
		pcommon.NewPoint(0, testPointHash(0)),
	}))
	return client, forwards, rollbacks
}

func scriptedServerTip() Tip {
	return Tip{
		Point:       pcommon.NewPoint(scriptedTip, testPointHash(scriptedTip)),
		BlockNumber: scriptedTip,
	}
}

func scriptedRollForward(
	t *testing.T,
	ctx CallbackContext,
	blockNumber uint64,
) error {
	return ctx.Server.RollForward(
		ledger.BlockTypeConway,
		tipWindowBlock(t, blockNumber),
		scriptedServerTip(),
	)
}

func outstandingRequests(c *Client) int {
	c.lifecycleMutex.Lock()
	defer c.lifecycleMutex.Unlock()
	return c.outstandingRequestNext
}

// While behind the server's tip the client keeps a full window outstanding.
// The server holds every request after block twenty, so once that block is
// delivered no reply can retire a request.
func TestClientWindowFillsWhileBehind(t *testing.T) {
	t.Parallel()
	const limit = 4
	const served = 20
	client, forwarded, _ := newScriptedWindowPair(t, limit,
		func(ctx CallbackContext, n int, held <-chan struct{}) error {
			if n > served {
				<-held
				return nil
			}
			return scriptedRollForward(t, ctx, uint64(n))
		})
	for want := uint64(1); want <= served; want++ {
		require.Equal(t, want, recv(t, forwarded, "block"))
	}
	require.Eventually(t, func() bool {
		return outstandingRequests(client) == limit
	}, reintersectTestTimeout, time.Millisecond,
		"requests outstanding while behind")
	require.Never(t, func() bool {
		return outstandingRequests(client) != limit
	}, 50*time.Millisecond, time.Millisecond,
		"requests outstanding while behind")
}

// A MsgRollBackward carries no block number for the client's new position,
// so after one the client keeps a single request outstanding, even far
// behind the server's tip, until a roll forward places it again. Requests
// sent before the rollback are not retracted; once they are answered, one
// remains.
func TestClientWindowAfterRollBackward(t *testing.T) {
	t.Parallel()
	const limit = 4
	const forwards = 10
	release := make(chan struct{})
	holding := make(chan struct{}, 1)
	client, forwarded, rolledBack := newScriptedWindowPair(t, limit,
		func(ctx CallbackContext, n int, held <-chan struct{}) error {
			switch {
			case n <= forwards:
				return scriptedRollForward(t, ctx, uint64(n))
			case n <= forwards+limit:
				// Answer the full window sent before the first
				// rollback with rollbacks to block five.
				return ctx.Server.RollBackward(
					pcommon.NewPoint(5, testPointHash(5)),
					scriptedServerTip(),
				)
			case n == forwards+limit+1:
				holding <- struct{}{}
				select {
				case <-release:
				case <-held:
					return nil
				}
				return scriptedRollForward(t, ctx, 6)
			default:
				<-held
				return nil
			}
		})
	for want := uint64(1); want <= forwards; want++ {
		require.Equal(t, want, recv(t, forwarded, "block"))
	}
	for range limit {
		recv(t, rolledBack, "roll backward")
	}
	recv(t, holding, "request after the rollbacks")
	require.Eventually(t, func() bool {
		return outstandingRequests(client) == 1
	}, reintersectTestTimeout, time.Millisecond,
		"requests outstanding after roll backward")
	require.Never(t, func() bool {
		return outstandingRequests(client) != 1
	}, 50*time.Millisecond, time.Millisecond,
		"requests outstanding before a roll forward")
	close(release)
	require.Equal(t, uint64(6), recv(t, forwarded, "block after rollback"))
	require.Eventually(t, func() bool {
		return outstandingRequests(client) == limit
	}, reintersectTestTimeout, time.Millisecond,
		"requests outstanding after the next roll forward")
}
