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

package blockfetch_test

import (
	"context"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// testTimeout bounds every channel wait in this file. Nothing here uses
// time.Sleep for synchronization.
const testTimeout = 5 * time.Second

type deliveredBlock struct {
	requestId uint64
	slot      uint64
}

type rangeResult struct {
	requestId uint64
	err       error
}

// testBlock builds a Babbage block for the given slot, along with the
// block-fetch wrapped CBOR the mock peer sends and the point identifying it.
func testBlock(t *testing.T, slot uint64) ([]byte, pcommon.Point) {
	t.Helper()
	blk := ledger.BabbageBlock{BlockHeader: &ledger.BabbageBlockHeader{}}
	blk.BlockHeader.Body.BlockNumber = slot
	blk.BlockHeader.Body.Slot = slot
	blockCbor, err := cbor.Encode(blk)
	require.NoError(t, err)
	_, err = cbor.Decode(blockCbor, &blk)
	require.NoError(t, err)
	wrapped, err := cbor.Encode(blockfetch.WrappedBlock{
		Type:     ledger.BlockTypeBabbage,
		RawBlock: cbor.RawMessage(blockCbor),
	})
	require.NoError(t, err)
	return wrapped, pcommon.NewPoint(slot, blk.Hash().Bytes())
}

func requestRangeInput() ouroboros_mock.ConversationEntry {
	return ouroboros_mock.ConversationEntryInput{
		ProtocolId:  blockfetch.ProtocolId,
		MessageType: blockfetch.MessageTypeRequestRange,
	}
}

func batchOutput(msgs ...protocol.Message) ouroboros_mock.ConversationEntry {
	return ouroboros_mock.ConversationEntryOutput{
		ProtocolId: blockfetch.ProtocolId,
		IsResponse: true,
		Messages:   msgs,
	}
}

type pipelineHarness struct {
	oConn  *ouroboros.Connection
	client *blockfetch.Client
	blocks chan deliveredBlock
	done   chan rangeResult
	// connErrs receives errors reported by the Ouroboros connection, which is
	// where a protocol violation by the mock peer surfaces.
	connErrs chan error
}

func newPipelineHarness(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	options ...blockfetch.BlockFetchOptionFunc,
) *pipelineHarness {
	t.Helper()
	h := &pipelineHarness{
		blocks:   make(chan deliveredBlock, 16),
		done:     make(chan rangeResult, 16),
		connErrs: make(chan error, 4),
	}
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	opts := append(
		[]blockfetch.BlockFetchOptionFunc{
			blockfetch.WithRequestPipelining(true),
			blockfetch.WithBlockFunc(
				func(ctx blockfetch.CallbackContext, _ uint, block ledger.Block) error {
					h.blocks <- deliveredBlock{
						requestId: ctx.RequestId,
						slot:      block.SlotNumber(),
					}
					return nil
				},
			),
			blockfetch.WithRangeDoneFunc(
				func(ctx blockfetch.CallbackContext, err error) error {
					h.done <- rangeResult{requestId: ctx.RequestId, err: err}
					return nil
				},
			),
		},
		options...,
	)
	cfg, err := blockfetch.NewConfig(opts...)
	require.NoError(t, err)
	cfg.SkipBlockValidation = true
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithBlockFetchConfig(cfg),
	)
	require.NoError(t, err)
	h.oConn = oConn
	h.client = oConn.BlockFetch().Client
	go func() {
		for err := range oConn.ErrorChan() {
			if err != nil {
				h.connErrs <- err
			}
		}
		close(h.connErrs)
	}()
	return h
}

func (h *pipelineHarness) close(t *testing.T) {
	t.Helper()
	_ = h.oConn.Close()
	// Drain remaining connection errors so the reader goroutine exits
	deadline := time.After(testTimeout)
	for {
		select {
		case _, ok := <-h.connErrs:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("connection did not shut down within timeout")
		}
	}
}

func (h *pipelineHarness) nextBlock(t *testing.T) deliveredBlock {
	t.Helper()
	select {
	case blk := <-h.blocks:
		return blk
	case err := <-h.connErrs:
		t.Fatalf("unexpected connection error while awaiting block: %s", err)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for a block")
	}
	return deliveredBlock{}
}

func (h *pipelineHarness) nextDone(t *testing.T) rangeResult {
	t.Helper()
	select {
	case res := <-h.done:
		return res
	case err := <-h.connErrs:
		t.Fatalf(
			"unexpected connection error while awaiting range done: %s",
			err,
		)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for a range request to complete")
	}
	return rangeResult{}
}

// TestRequestRangePipelinedOutstandingRequests covers request accounting with
// three MsgRequestRange outstanding at once: the responses are attributed to
// them in request order. The mock peer answers nothing until it has received
// all three requests, so a client that waits for each response before sending
// the next cannot get past the second conversation entry.
//
// It does not prove the pipelined send path ran. sendLoop batches messages
// that are already queued into a single segment and defers their state
// transitions, so which path carries the second and third request depends on
// scheduling. TestRequestRangeInFlightByteBound covers the pipelined send
// path deterministically.
func TestRequestRangePipelinedOutstandingRequests(t *testing.T) {
	defer goleak.VerifyNone(t)
	wrapped1, point1 := testBlock(t, 100)
	wrapped2, point2 := testBlock(t, 200)
	wrapped3, point3 := testBlock(t, 300)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		requestRangeInput(),
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped1),
			blockfetch.NewMsgBatchDone(),
		),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped2),
			blockfetch.NewMsgBatchDone(),
		),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped3),
			blockfetch.NewMsgBatchDone(),
		),
	}
	h := newPipelineHarness(t, conversation)
	defer h.close(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	ids := make([]uint64, 0, 3)
	for _, point := range []pcommon.Point{point1, point2, point3} {
		id, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
			Start:         point,
			End:           point,
			ExpectedBytes: 1024,
		})
		require.NoError(t, err)
		ids = append(ids, id)
	}
	require.Equal(t, []uint64{1, 2, 3}, ids)

	for i, slot := range []uint64{100, 200, 300} {
		blk := h.nextBlock(t)
		require.Equal(
			t,
			deliveredBlock{requestId: ids[i], slot: slot},
			blk,
			"block for request %d attributed incorrectly",
			ids[i],
		)
		res := h.nextDone(t)
		require.Equal(t, ids[i], res.requestId)
		require.NoError(t, res.err)
	}
}

// TestRequestRangeRequiresOptIn verifies a client that has not opted into
// request pipelining rejects RequestRange instead of silently changing its
// wire behavior.
func TestRequestRangeRequiresOptIn(t *testing.T) {
	defer goleak.VerifyNone(t)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
	}
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
	)
	require.NoError(t, err)
	errDrained := make(chan struct{})
	go func() {
		for range oConn.ErrorChan() {
		}
		close(errDrained)
	}()
	_, err = oConn.BlockFetch().Client.RequestRange(
		context.Background(),
		blockfetch.RangeRequest{},
	)
	require.ErrorIs(t, err, blockfetch.ErrRequestPipeliningDisabled)
	require.NoError(t, oConn.Close())
	select {
	case <-errDrained:
	case <-time.After(testTimeout):
		t.Fatal("connection did not shut down within timeout")
	}
}

// TestRequestRangeRequiresRangeDoneFunc verifies a pipelining client refuses
// to queue a request it cannot report the completion of.
func TestRequestRangeRequiresRangeDoneFunc(t *testing.T) {
	defer goleak.VerifyNone(t)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
	}
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			RequestPipelining:   true,
		}),
	)
	require.NoError(t, err)
	errDrained := make(chan struct{})
	go func() {
		for range oConn.ErrorChan() {
		}
		close(errDrained)
	}()
	_, err = oConn.BlockFetch().Client.RequestRange(
		context.Background(),
		blockfetch.RangeRequest{},
	)
	require.ErrorContains(t, err, "requires a RangeDoneFunc")
	require.NoError(t, oConn.Close())
	select {
	case <-errDrained:
	case <-time.After(testTimeout):
		t.Fatal("connection did not shut down within timeout")
	}
}

// TestGetBlockRangeUnaffectedByPipelining verifies that the existing
// single-request path keeps its semantics on a client that has pipelining
// enabled: GetBlockRange waits for MsgStartBatch, blocks arrive through
// BlockFunc, and BatchDoneFunc reports completion.
func TestGetBlockRangeUnaffectedByPipelining(t *testing.T) {
	defer goleak.VerifyNone(t)
	wrapped1, point1 := testBlock(t, 100)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped1),
			blockfetch.NewMsgBatchDone(),
		),
	}
	batchDone := make(chan uint64, 4)
	h := newPipelineHarness(
		t,
		conversation,
		blockfetch.WithBatchDoneFunc(
			func(ctx blockfetch.CallbackContext) error {
				batchDone <- ctx.RequestId
				return nil
			},
		),
	)
	defer h.close(t)

	require.NoError(t, h.client.GetBlockRange(point1, point1))
	blk := h.nextBlock(t)
	require.Equal(t, deliveredBlock{requestId: 1, slot: 100}, blk)
	select {
	case id := <-batchDone:
		require.Equal(t, uint64(1), id)
	case err := <-h.connErrs:
		t.Fatalf("unexpected connection error: %s", err)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for BatchDoneFunc")
	}
	// A GetBlockRange request is not a pipelined request, so it must not also
	// be reported through RangeDoneFunc.
	select {
	case res := <-h.done:
		t.Fatalf("unexpected RangeDoneFunc call for request %d", res.requestId)
	case <-time.After(testTimeout):
	}
}

// TestRequestRangeExcessBatchDoneNotAppliedToNextRequest is the negative
// case: a peer that sends a second MsgBatchDone after completing the first
// batch must not have it applied to the next queued request. The connection
// fails and the still-outstanding request is resolved with an error rather
// than reported complete.
func TestRequestRangeExcessBatchDoneNotAppliedToNextRequest(t *testing.T) {
	defer goleak.VerifyNone(t)
	wrapped1, point1 := testBlock(t, 100)
	_, point2 := testBlock(t, 200)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped1),
			blockfetch.NewMsgBatchDone(),
			// Excess BatchDone for a request that has already been retired
			blockfetch.NewMsgBatchDone(),
		),
	}
	h := newPipelineHarness(t, conversation)
	defer h.close(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	id1, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
		Start: point1, End: point1, ExpectedBytes: 1024,
	})
	require.NoError(t, err)
	id2, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
		Start: point2, End: point2, ExpectedBytes: 1024,
	})
	require.NoError(t, err)

	require.Equal(t, deliveredBlock{requestId: id1, slot: 100}, h.nextBlock(t))
	first := h.nextDone(t)
	require.Equal(t, id1, first.requestId)
	require.NoError(t, first.err)

	// The second request must be resolved with a failure, never with the
	// excess BatchDone that belonged to the retired request.
	second := waitForDone(t, h, id2)
	require.Error(t, second.err)
	// No block was ever delivered for the second request
	select {
	case blk := <-h.blocks:
		t.Fatalf("unexpected block delivered: %+v", blk)
	default:
	}
}

// waitForDone waits for the completion of a specific request, tolerating a
// connection error arriving first since a protocol violation reports on both
// paths.
func waitForDone(
	t *testing.T,
	h *pipelineHarness,
	requestId uint64,
) rangeResult {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		select {
		case res := <-h.done:
			if res.requestId == requestId {
				return res
			}
		case <-deadline:
			t.Fatalf("timed out waiting for request %d to resolve", requestId)
		}
	}
}

// TestRequestRangeShutdownResolvesOutstanding verifies that a connection
// teardown mid-pipeline resolves every outstanding request exactly once, with
// no goroutine left waiting.
func TestRequestRangeShutdownResolvesOutstanding(t *testing.T) {
	defer goleak.VerifyNone(t)
	_, point1 := testBlock(t, 100)
	_, point2 := testBlock(t, 200)
	_, point3 := testBlock(t, 300)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		requestRangeInput(),
		requestRangeInput(),
		// The peer starts the first batch and then disappears
		batchOutput(blockfetch.NewMsgStartBatch()),
		ouroboros_mock.ConversationEntryClose{},
	}
	h := newPipelineHarness(t, conversation)
	defer h.close(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	ids := make([]uint64, 0, 3)
	for _, point := range []pcommon.Point{point1, point2, point3} {
		id, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
			Start: point, End: point, ExpectedBytes: 1024,
		})
		require.NoError(t, err)
		ids = append(ids, id)
	}

	resolved := make(map[uint64]error, len(ids))
	deadline := time.After(testTimeout)
	for len(resolved) < len(ids) {
		select {
		case res := <-h.done:
			_, dup := resolved[res.requestId]
			require.False(
				t,
				dup,
				"request %d resolved more than once",
				res.requestId,
			)
			resolved[res.requestId] = res.err
		case <-deadline:
			t.Fatalf(
				"only %d of %d outstanding requests resolved on shutdown",
				len(resolved),
				len(ids),
			)
		}
	}
	for _, id := range ids {
		require.ErrorIs(
			t,
			resolved[id],
			protocol.ErrProtocolShuttingDown,
			"request %d",
			id,
		)
	}
}

// TestRequestRangeInFlightByteBound verifies the in-flight bound is expressed
// in bytes and actually holds a request back. The first request is pinned
// open by blocking inside its block callback, which runs on the protocol
// receive goroutine, so nothing can retire it and release capacity until the
// test says so. While it is pinned, a third request cannot be admitted.
//
// This also covers the pipelined send path deterministically: the third
// request is admitted only once the first retires, with the state machine
// mid-batch for the second, so its MsgRequestRange can only reach the wire
// while the peer holds agency.
func TestRequestRangeInFlightByteBound(t *testing.T) {
	defer goleak.VerifyNone(t)
	wrapped1, point1 := testBlock(t, 100)
	wrapped2, point2 := testBlock(t, 200)
	wrapped3, point3 := testBlock(t, 300)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped1),
			blockfetch.NewMsgBatchDone(),
		),
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped2),
			blockfetch.NewMsgBatchDone(),
		),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped3),
			blockfetch.NewMsgBatchDone(),
		),
	}
	firstBlockEntered := make(chan struct{})
	releaseFirstBlock := make(chan struct{})
	var releaseFirstBlockOnce sync.Once
	release := func() {
		releaseFirstBlockOnce.Do(func() { close(releaseFirstBlock) })
	}
	blocks := make(chan deliveredBlock, 16)
	h := newPipelineHarness(
		t,
		conversation,
		blockfetch.WithMaxInFlightBytes(2048),
		blockfetch.WithBlockFunc(
			func(ctx blockfetch.CallbackContext, _ uint, block ledger.Block) error {
				if block.SlotNumber() == 100 {
					close(firstBlockEntered)
					<-releaseFirstBlock
				}
				blocks <- deliveredBlock{
					requestId: ctx.RequestId,
					slot:      block.SlotNumber(),
				}
				return nil
			},
		),
	)
	defer func() {
		release()
		h.close(t)
	}()
	nextBlock := func() deliveredBlock {
		t.Helper()
		select {
		case blk := <-blocks:
			return blk
		case err := <-h.connErrs:
			t.Fatalf("unexpected connection error: %s", err)
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for a block")
		}
		return deliveredBlock{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	for _, point := range []pcommon.Point{point1, point2} {
		_, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
			Start: point, End: point, ExpectedBytes: 1024,
		})
		require.NoError(t, err)
	}
	// Wait until the first request is pinned open inside its block callback.
	select {
	case <-firstBlockEntered:
	case err := <-h.connErrs:
		t.Fatalf("unexpected connection error: %s", err)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the first block callback")
	}
	// The bound is reached and nothing can release it, so a third request
	// must not be admitted.
	thirdAccepted := make(chan error, 1)
	go func() {
		_, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
			Start: point3, End: point3, ExpectedBytes: 1024,
		})
		thirdAccepted <- err
	}()
	select {
	case err := <-thirdAccepted:
		t.Fatalf(
			"third request was admitted past the in-flight byte bound: %v",
			err,
		)
	case <-time.After(100 * time.Millisecond):
		// Still blocked, as required
	}
	release()
	require.Equal(t, deliveredBlock{requestId: 1, slot: 100}, nextBlock())
	first := h.nextDone(t)
	require.Equal(t, uint64(1), first.requestId)
	require.NoError(t, first.err)
	select {
	case err := <-thirdAccepted:
		require.NoError(t, err)
	case <-time.After(testTimeout):
		t.Fatal("third request was not admitted after capacity was released")
	}
	for i, slot := range []uint64{200, 300} {
		id := uint64(i + 2)
		require.Equal(
			t,
			deliveredBlock{requestId: id, slot: slot},
			nextBlock(),
		)
		res := h.nextDone(t)
		require.Equal(t, id, res.requestId)
		require.NoError(t, res.err)
	}
}

// TestRequestRangeNoBlocksResolvesOnlyItsOwnRequest verifies a MsgNoBlocks
// answer fails the request it belongs to and leaves the following request
// outstanding.
func TestRequestRangeNoBlocksResolvesOnlyItsOwnRequest(t *testing.T) {
	defer goleak.VerifyNone(t)
	_, point1 := testBlock(t, 100)
	wrapped2, point2 := testBlock(t, 200)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		requestRangeInput(),
		batchOutput(blockfetch.NewMsgNoBlocks()),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBlock(wrapped2),
			blockfetch.NewMsgBatchDone(),
		),
	}
	h := newPipelineHarness(t, conversation)
	defer h.close(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	id1, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
		Start: point1, End: point1, ExpectedBytes: 1024,
	})
	require.NoError(t, err)
	id2, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
		Start: point2, End: point2, ExpectedBytes: 1024,
	})
	require.NoError(t, err)

	first := h.nextDone(t)
	require.Equal(t, id1, first.requestId)
	require.ErrorIs(t, first.err, blockfetch.ErrNoBlocks)

	require.Equal(t, deliveredBlock{requestId: id2, slot: 200}, h.nextBlock(t))
	second := h.nextDone(t)
	require.Equal(t, id2, second.requestId)
	require.NoError(t, second.err)
}

// TestGetBlockEmptyBatch verifies GetBlock reports a missing block rather than
// hanging when a peer completes a batch without sending one.
func TestGetBlockEmptyBatch(t *testing.T) {
	defer goleak.VerifyNone(t)
	_, point1 := testBlock(t, 100)
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(),
		batchOutput(
			blockfetch.NewMsgStartBatch(),
			blockfetch.NewMsgBatchDone(),
		),
	}
	h := newPipelineHarness(t, conversation)
	defer h.close(t)

	type result struct {
		block ledger.Block
		err   error
	}
	results := make(chan result, 1)
	go func() {
		blk, err := h.client.GetBlock(point1)
		results <- result{block: blk, err: err}
	}()
	select {
	case res := <-results:
		require.Nil(t, res.block)
		require.ErrorIs(t, res.err, blockfetch.ErrNoBlocks)
	case err := <-h.connErrs:
		t.Fatalf("unexpected connection error: %s", err)
	case <-time.After(testTimeout):
		t.Fatal("GetBlock did not return for a batch with no blocks")
	}
}
