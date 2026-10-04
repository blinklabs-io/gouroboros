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
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/stretchr/testify/require"
)

const (
	// ingressTestBlocks blocks of ingressTestBlockPadding bytes each make a
	// range of about 31 MB: larger than block-fetch's reference ingress
	// limit (blockfetch.IngressLimit, about 23 MB), so the range only fits
	// if the client accounts for what it asked for.
	ingressTestBlocks       = 60
	ingressTestBlockPadding = 512 * 1024
	ingressTestTimeout      = 20 * time.Second
)

type ingressTestBlock struct {
	raw     []byte
	wrapped []byte
	point   pcommon.Point
}

// ingressTestChain builds a chain of Babbage blocks, each carrying padding in
// its header signature so a handful of blocks adds up to tens of megabytes.
// The client is configured with SkipBlockValidation, so the body hash and
// the signature are never checked; the header hash and the previous-hash
// links the client does check are real.
func ingressTestChain(t *testing.T, count int) []ingressTestBlock {
	t.Helper()
	blocks := make([]ingressTestBlock, 0, count)
	var prevHash ledger.Blake2b256
	for i := range count {
		slot := uint64(i + 1)
		blk := ledger.BabbageBlock{BlockHeader: &ledger.BabbageBlockHeader{}}
		blk.BlockHeader.Body.BlockNumber = slot
		blk.BlockHeader.Body.Slot = slot
		blk.BlockHeader.Body.PrevHash = prevHash
		blk.BlockHeader.Signature = make([]byte, ingressTestBlockPadding)
		blockCbor, err := cbor.Encode(blk)
		require.NoError(t, err)
		var decoded ledger.BabbageBlock
		_, err = cbor.Decode(blockCbor, &decoded)
		require.NoError(t, err)
		wrapped, err := cbor.Encode(blockfetch.WrappedBlock{
			Type:     ledger.BlockTypeBabbage,
			RawBlock: cbor.RawMessage(blockCbor),
		})
		require.NoError(t, err)
		prevHash = decoded.Hash()
		blocks = append(blocks, ingressTestBlock{
			raw:     blockCbor,
			wrapped: wrapped,
			point:   pcommon.NewPoint(slot, prevHash.Bytes()),
		})
	}
	return blocks
}

func ingressTestChainBytes(blocks []ingressTestBlock) uint64 {
	var total uint64
	for _, blk := range blocks {
		total += uint64(len(blk.wrapped))
	}
	return total
}

// ingressTestPeer is a real node-to-node server connection that answers
// every block-fetch range request by streaming serve() as fast as the
// connection accepts it.
type ingressTestPeer struct {
	conn   *ouroboros.Connection
	served chan struct{}
	errs   chan error
}

func startIngressTestPeer(
	t *testing.T,
	pipe net.Conn,
	serve []ingressTestBlock,
) *ingressTestPeer {
	t.Helper()
	peer := &ingressTestPeer{
		served: make(chan struct{}),
		errs:   make(chan error, 8),
	}
	var serveOnce sync.Once
	bfCfg, err := blockfetch.NewConfig(
		blockfetch.WithRequestRangeFunc(
			func(ctx blockfetch.CallbackContext, _, _ pcommon.Point) error {
				go func() {
					server := ctx.Server
					if err := server.StartBatch(); err != nil {
						peer.errs <- err
						return
					}
					for _, blk := range serve {
						if err := server.Block(
							ledger.BlockTypeBabbage,
							blk.raw,
						); err != nil {
							peer.errs <- err
							return
						}
						// Pace on the server's own send queue, which holds
						// at most one block's worth of pending bytes.
						for !server.ProtocolInstance().WaitSendQueueDrained(
							ingressTestTimeout,
						) {
							if server.ProtocolInstance().IsDone() {
								return
							}
						}
					}
					if err := server.BatchDone(); err != nil {
						peer.errs <- err
						return
					}
					serveOnce.Do(func() { close(peer.served) })
				}()
				return nil
			},
		),
	)
	require.NoError(t, err)
	conn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(pipe),
		ouroboros.WithServer(true),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithNetworkMagic(42),
		ouroboros.WithBlockFetchConfig(bfCfg),
		ouroboros.WithErrorChan(peer.errs),
	)
	require.NoError(t, err)
	peer.conn = conn
	return peer
}

// ingressTestClient is a node-to-node client connection with keep-alive
// running on a short period and a block consumer the test holds.
type ingressTestClient struct {
	conn       *ouroboros.Connection
	errs       chan error
	pongs      chan struct{}
	blocks     chan struct{}
	rangeDone  chan error
	release    func()
	releaseCh  chan struct{}
	firstBlock chan struct{}
}

func newIngressTestClient(
	t *testing.T,
	pipe net.Conn,
	extra ...blockfetch.BlockFetchOptionFunc,
) *ingressTestClient {
	t.Helper()
	c := &ingressTestClient{
		errs:       make(chan error, 8),
		pongs:      make(chan struct{}, 1024),
		blocks:     make(chan struct{}, ingressTestBlocks*2),
		rangeDone:  make(chan error, 4),
		releaseCh:  make(chan struct{}),
		firstBlock: make(chan struct{}),
	}
	var releaseOnce, firstOnce sync.Once
	c.release = func() { releaseOnce.Do(func() { close(c.releaseCh) }) }
	opts := append([]blockfetch.BlockFetchOptionFunc{
		blockfetch.WithBlockRawFunc(
			func(blockfetch.CallbackContext, uint, []byte) error {
				firstOnce.Do(func() { close(c.firstBlock) })
				// The slow consumer: hold every block until released.
				<-c.releaseCh
				c.blocks <- struct{}{}
				return nil
			},
		),
		blockfetch.WithBatchDoneFunc(func(blockfetch.CallbackContext) error {
			c.rangeDone <- nil
			return nil
		}),
		blockfetch.WithRangeDoneFunc(
			func(_ blockfetch.CallbackContext, err error) error {
				c.rangeDone <- err
				return nil
			},
		),
	}, extra...)
	bfCfg, err := blockfetch.NewConfig(opts...)
	require.NoError(t, err)
	bfCfg.SkipBlockValidation = true
	kaCfg := keepalive.NewConfig(
		keepalive.WithPeriod(50*time.Millisecond),
		keepalive.WithTimeout(5*time.Second),
		keepalive.WithKeepAliveResponseFunc(
			func(keepalive.CallbackContext, uint16) error {
				select {
				case c.pongs <- struct{}{}:
				default:
				}
				return nil
			},
		),
	)
	conn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(pipe),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithNetworkMagic(42),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithKeepAliveConfig(kaCfg),
		ouroboros.WithBlockFetchConfig(bfCfg),
		ouroboros.WithErrorChan(c.errs),
	)
	require.NoError(t, err)
	c.conn = conn
	return c
}

// waitFor blocks until ch yields, failing on any connection error first.
func (c *ingressTestClient) waitFor(
	t *testing.T,
	peer *ingressTestPeer,
	ch <-chan struct{},
	what string,
) {
	t.Helper()
	select {
	case <-ch:
	case err := <-c.errs:
		t.Fatalf("client connection failed while waiting for %s: %v", what, err)
	case err := <-peer.errs:
		t.Fatalf("peer connection failed while waiting for %s: %v", what, err)
	case <-time.After(ingressTestTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func connectIngressTestPair(
	t *testing.T,
	serve []ingressTestBlock,
	extra ...blockfetch.BlockFetchOptionFunc,
) (*ingressTestClient, *ingressTestPeer) {
	t.Helper()
	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	peerCh := make(chan *ingressTestPeer, 1)
	go func() { peerCh <- startIngressTestPeer(t, serverPipe, serve) }()
	client := newIngressTestClient(t, clientPipe, extra...)
	var peer *ingressTestPeer
	select {
	case peer = <-peerCh:
	case <-time.After(ingressTestTimeout):
		t.Fatal("peer connection did not finish its handshake")
	}
	// Registered after the connections so it runs before their Close, and
	// releases a held consumer that would otherwise block shutdown.
	t.Cleanup(func() {
		client.release()
		_ = client.conn.Close()
		_ = peer.conn.Close()
	})
	return client, peer
}

// TestSlowBlockConsumerDoesNotStarveKeepAlive requests a range larger than
// block-fetch's reference ingress limit, with its size estimated, from a
// peer that serves it promptly, and holds the block consumer. Every byte of
// the range must still be read off the connection, keep-alive must keep
// completing while the consumer is held, the connection must survive, and
// the range must then complete once the consumer is released. The range is
// also larger than DefaultMaxInFlightBytes. Unestimated ranges are covered by
// TestUnestimatedRangeAppliesBackpressure.
func TestSlowBlockConsumerDoesNotStarveKeepAlive(t *testing.T) {
	t.Parallel()
	chain := ingressTestChain(t, ingressTestBlocks)
	start, end := chain[0].point, chain[len(chain)-1].point
	require.Greater(
		t,
		ingressTestChainBytes(chain),
		uint64(blockfetch.IngressLimit),
	)

	cases := []struct {
		name    string
		options []blockfetch.BlockFetchOptionFunc
		request func(*blockfetch.Client) error
	}{
		{
			name: "RequestRange with ExpectedBytes",
			options: []blockfetch.BlockFetchOptionFunc{
				blockfetch.WithRequestPipelining(true),
			},
			request: func(c *blockfetch.Client) error {
				_, err := c.RequestRange(
					context.Background(),
					blockfetch.RangeRequest{
						Start:         start,
						End:           end,
						ExpectedBytes: ingressTestChainBytes(chain),
					},
				)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, peer := connectIngressTestPair(t, chain, tc.options...)
			bf := client.conn.BlockFetch().Client
			require.NoError(t, tc.request(bf))
			client.waitFor(t, peer, client.firstBlock, "the first block")

			// The peer can only finish writing the range if the client keeps
			// reading its socket while the consumer is held.
			client.waitFor(
				t,
				peer,
				peer.served,
				"the peer to finish serving the range",
			)
			for i := range 3 {
				client.waitFor(
					t,
					peer,
					client.pongs,
					fmt.Sprintf(
						"keep-alive response %d with the consumer held",
						i+1,
					),
				)
			}
			m := client.conn.Muxer()
			require.Greater(
				t,
				m.IngressLimit(
					blockfetch.ProtocolId,
					muxer.ProtocolRoleInitiator,
				),
				int(ingressTestChainBytes(chain)),
				"ingress limit while the range is outstanding",
			)

			client.release()
			for range chain {
				client.waitFor(t, peer, client.blocks, "block delivery")
			}
			select {
			case err := <-client.rangeDone:
				require.NoError(t, err)
			case err := <-client.errs:
				t.Fatalf(
					"client connection failed before the range completed: %v",
					err,
				)
			case <-time.After(ingressTestTimeout):
				t.Fatal(
					"range did not complete after the consumer was released",
				)
			}
			// The allowance is returned once the range is consumed.
			require.Eventually(
				t,
				func() bool {
					return m.IngressLimit(
						blockfetch.ProtocolId,
						muxer.ProtocolRoleInitiator,
					) == blockfetch.IngressLimit
				},
				ingressTestTimeout,
				time.Millisecond,
				"ingress limit did not return to the base after the range completed",
			)
			select {
			case err := <-client.errs:
				t.Fatalf("client connection failed: %v", err)
			default:
			}
		})
	}
}

// TestEstimatedRangeAboveConnectionBudgetSurvivesSlowConsumer requests an
// estimated range larger than the connection's default ingress budget while
// the block consumer is held. The allowance block-fetch raises for the range
// extends the budget, so every byte of an honest reply is still read and the
// connection survives.
func TestEstimatedRangeAboveConnectionBudgetSurvivesSlowConsumer(
	t *testing.T,
) {
	t.Parallel()
	chain := ingressTestChain(
		t,
		muxer.DefaultIngressBudget/ingressTestBlockPadding+12,
	)
	total := ingressTestChainBytes(chain)
	require.Greater(t, total, uint64(muxer.DefaultIngressBudget))
	client, peer := connectIngressTestPair(
		t,
		chain,
		blockfetch.WithRequestPipelining(true),
	)
	_, err := client.conn.BlockFetch().Client.RequestRange(
		context.Background(),
		blockfetch.RangeRequest{
			Start:         chain[0].point,
			End:           chain[len(chain)-1].point,
			ExpectedBytes: total,
		},
	)
	require.NoError(t, err)
	client.waitFor(t, peer, client.firstBlock, "the first block")
	client.waitFor(
		t,
		peer,
		peer.served,
		"the peer to finish serving the range with the consumer held",
	)
	client.waitFor(t, peer, client.pongs, "a keep-alive response")
	client.release()
	for range chain {
		client.waitFor(t, peer, client.blocks, "block delivery")
	}
	select {
	case err := <-client.rangeDone:
		require.NoError(t, err)
	case err := <-client.errs:
		t.Fatalf("client connection failed before the range completed: %v", err)
	case <-time.After(ingressTestTimeout):
		t.Fatal("range did not complete after the consumer was released")
	}
}

// TestNodeToNodeIngressLimits checks that each mini-protocol registers with
// its own ingress limit rather than one muxer-wide default.
func TestNodeToNodeIngressLimits(t *testing.T) {
	t.Parallel()
	client, peer := connectIngressTestPair(t, nil)
	for _, tc := range []struct {
		name  string
		conn  *ouroboros.Connection
		id    uint16
		role  muxer.ProtocolRole
		limit int
	}{
		// chain-sync's 462,000-byte reference limit is below one batch of
		// maximum-size segments, the least any protocol is given.
		{
			"chain-sync client", client.conn, chainsync.ProtocolIdNtN,
			muxer.ProtocolRoleInitiator, 10 * muxer.SegmentMaxPayloadLength,
		},
		{
			"block-fetch client", client.conn, blockfetch.ProtocolId,
			muxer.ProtocolRoleInitiator, blockfetch.IngressLimit,
		},
		{
			"block-fetch server", peer.conn, blockfetch.ProtocolId,
			muxer.ProtocolRoleResponder,
			blockfetch.StreamingMaxPendingMessageBytes,
		},
		{
			"tx-submission client", client.conn, txsubmission.ProtocolId,
			muxer.ProtocolRoleInitiator, txsubmission.MaxPendingMessageBytes,
		},
		// keep-alive declares no per-state limit, so it gets the largest
		// message it would reassemble.
		{
			"keep-alive client", client.conn, keepalive.ProtocolId,
			muxer.ProtocolRoleInitiator, 16 * 1024 * 1024,
		},
	} {
		require.Equal(
			t,
			tc.limit,
			tc.conn.Muxer().IngressLimit(tc.id, tc.role),
			tc.name,
		)
	}
}

// TestExcessBlockFetchIngressIsAProtocolError has the peer answer a
// one-block request, with an accurate ExpectedBytes, by streaming far more
// than it was asked for while the consumer is held, so the client's range
// checks cannot run. The muxer must refuse the excess as an ingress overflow
// rather than buffering it without bound.
func TestExcessBlockFetchIngressIsAProtocolError(t *testing.T) {
	t.Parallel()
	chain := ingressTestChain(t, ingressTestBlocks)
	client, _ := connectIngressTestPair(
		t,
		chain,
		blockfetch.WithRequestPipelining(true),
	)
	_, err := client.conn.BlockFetch().Client.RequestRange(
		context.Background(),
		blockfetch.RangeRequest{
			Start:         chain[0].point,
			End:           chain[0].point,
			ExpectedBytes: uint64(len(chain[0].wrapped)),
		},
	)
	require.NoError(t, err)
	select {
	case err := <-client.errs:
		require.True(
			t,
			errors.Is(err, muxer.ErrIngressOverflow),
			"expected an ingress overflow, got: %v", err,
		)
	case <-time.After(ingressTestTimeout):
		t.Fatal("excess ingress from the peer was not refused")
	}
}

// blockFetchIngressRecorder records the depth of the block-fetch client's
// muxer ingress queue, and closes full once the queue holds so much that a
// maximum-size segment no longer fits under blockfetch.IngressLimit.
type blockFetchIngressRecorder struct {
	mu       sync.Mutex
	maxDepth int
	full     chan struct{}
	fullOnce sync.Once
}

func newBlockFetchIngressRecorder() *blockFetchIngressRecorder {
	return &blockFetchIngressRecorder{full: make(chan struct{})}
}

func (r *blockFetchIngressRecorder) IngressQueueDepth(
	protocolId uint16,
	protocolRole muxer.ProtocolRole,
	bytes int,
) {
	if protocolId != blockfetch.ProtocolId ||
		protocolRole != muxer.ProtocolRoleInitiator {
		return
	}
	r.mu.Lock()
	r.maxDepth = max(r.maxDepth, bytes)
	r.mu.Unlock()
	if bytes > blockfetch.IngressLimit-muxer.SegmentMaxPayloadLength {
		r.fullOnce.Do(func() { close(r.full) })
	}
}

func (r *blockFetchIngressRecorder) IngressDeliveryBlocked(
	uint16,
	muxer.ProtocolRole,
	time.Duration,
) {
}

func (r *blockFetchIngressRecorder) IngressBackpressure(
	uint16,
	muxer.ProtocolRole,
	time.Duration,
) {
}

func (r *blockFetchIngressRecorder) max() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxDepth
}

// requireHeldAtIngressLimit waits for the block-fetch ingress queue to fill
// while the consumer is held, then checks that it stays within
// blockfetch.IngressLimit and that the peer cannot finish sending: the
// connection is being held by backpressure, not buffered and not dropped.
// The window only has to be long enough for an unbounded queue to take the
// rest of the peer's data, which over net.Pipe takes milliseconds.
func requireHeldAtIngressLimit(
	t *testing.T,
	client *ingressTestClient,
	peer *ingressTestPeer,
	rec *blockFetchIngressRecorder,
) {
	t.Helper()
	client.waitFor(t, peer, rec.full, "the block-fetch ingress queue to fill")
	require.Never(
		t,
		func() bool { return rec.max() > blockfetch.IngressLimit },
		500*time.Millisecond,
		5*time.Millisecond,
		"block-fetch ingress queued past its limit with the consumer held",
	)
	require.LessOrEqual(t, rec.max(), blockfetch.IngressLimit)
	select {
	case <-peer.served:
		t.Fatal("the peer finished sending while the consumer was held")
	case err := <-client.errs:
		t.Fatalf("client connection failed with the consumer held: %v", err)
	default:
	}
}

// ingressBackpressureTestBlocks makes a chain of about 42 MB, well past
// blockfetch.IngressLimit plus what the protocol layer holds downstream of
// the muxer, so the muxer's queue for block-fetch has to fill.
const ingressBackpressureTestBlocks = 80

// TestUnestimatedRangeAppliesBackpressure requests a range without a size
// estimate from a peer that serves it promptly, and holds the block
// consumer. The client cannot know how much the range holds, so the muxer
// holds at most blockfetch.IngressLimit for block-fetch and stops reading the
// connection until the consumer makes room. The peer must not be dropped,
// and the range must complete once the consumer is released.
func TestUnestimatedRangeAppliesBackpressure(t *testing.T) {
	t.Parallel()
	chain := ingressTestChain(t, ingressBackpressureTestBlocks)
	start, end := chain[0].point, chain[len(chain)-1].point
	cases := []struct {
		name    string
		options []blockfetch.BlockFetchOptionFunc
		request func(*blockfetch.Client) error
	}{
		{
			name: "RequestRange",
			options: []blockfetch.BlockFetchOptionFunc{
				blockfetch.WithRequestPipelining(true),
			},
			request: func(c *blockfetch.Client) error {
				_, err := c.RequestRange(
					context.Background(),
					blockfetch.RangeRequest{Start: start, End: end},
				)
				return err
			},
		},
		{
			name: "GetBlockRange",
			request: func(c *blockfetch.Client) error {
				return c.GetBlockRange(start, end)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, peer := connectIngressTestPair(t, chain, tc.options...)
			rec := newBlockFetchIngressRecorder()
			m := client.conn.Muxer()
			m.SetMetrics(rec)
			require.NoError(t, tc.request(client.conn.BlockFetch().Client))
			client.waitFor(t, peer, client.firstBlock, "the first block")
			requireHeldAtIngressLimit(t, client, peer, rec)

			client.release()
			for range chain {
				client.waitFor(t, peer, client.blocks, "block delivery")
			}
			client.waitFor(t, peer, peer.served, "the peer to finish serving")
			select {
			case err := <-client.rangeDone:
				require.NoError(t, err)
			case err := <-client.errs:
				t.Fatalf(
					"client connection failed before the range completed: %v",
					err,
				)
			case <-time.After(ingressTestTimeout):
				t.Fatal(
					"range did not complete after the consumer was released",
				)
			}
			require.LessOrEqual(t, rec.max(), blockfetch.IngressLimit)
			// The connection is healthy afterwards: keep-alive completes
			// again and block-fetch is back to failing excess ingress.
			for len(client.pongs) > 0 {
				<-client.pongs
			}
			client.waitFor(
				t,
				peer,
				client.pongs,
				"keep-alive after the range completed",
			)
			require.Eventually(
				t,
				func() bool {
					return !m.IngressBackpressure(
						blockfetch.ProtocolId,
						muxer.ProtocolRoleInitiator,
					)
				},
				ingressTestTimeout,
				time.Millisecond,
				"block-fetch backpressure was not turned off after the range",
			)
			require.Equal(
				t,
				blockfetch.IngressLimit,
				m.IngressLimit(
					blockfetch.ProtocolId,
					muxer.ProtocolRoleInitiator,
				),
			)
		})
	}
}

// TestUnestimatedRangeFloodIsBoundedThenRefused has the peer answer a
// ten-block request without a size estimate by streaming its whole chain
// while the consumer is held. The client cannot tell the excess from the
// range until the consumer reaches it, so the muxer must hold no more than
// blockfetch.IngressLimit for it and hold the peer back rather than buffer
// the rest. Once the consumer is released the client's range checks reach
// the first block past the range, and the connection fails. Failing the
// request also ends its backpressure, so a read loop still holding excess
// can report the ingress overflow first; either error drops the peer.
func TestUnestimatedRangeFloodIsBoundedThenRefused(t *testing.T) {
	t.Parallel()
	chain := ingressTestChain(t, ingressBackpressureTestBlocks)
	client, peer := connectIngressTestPair(
		t,
		chain,
		blockfetch.WithRequestPipelining(true),
	)
	rec := newBlockFetchIngressRecorder()
	client.conn.Muxer().SetMetrics(rec)
	_, err := client.conn.BlockFetch().Client.RequestRange(
		context.Background(),
		blockfetch.RangeRequest{Start: chain[0].point, End: chain[9].point},
	)
	require.NoError(t, err)
	client.waitFor(t, peer, client.firstBlock, "the first block")
	requireHeldAtIngressLimit(t, client, peer, rec)

	client.release()
	select {
	case err := <-client.errs:
		if !errors.Is(err, muxer.ErrIngressOverflow) {
			require.ErrorContains(t, err, "outside requested range")
		}
	case <-time.After(ingressTestTimeout):
		t.Fatal("the block past the requested range was not refused")
	}
	require.LessOrEqual(t, rec.max(), blockfetch.IngressLimit)
}
