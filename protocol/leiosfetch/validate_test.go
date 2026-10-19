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

package leiosfetch

import (
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEB struct {
	raw  []byte
	hash lcommon.Blake2b256
}

// testEndorserBlockN builds an endorser block of txCount references. Different
// seeds give different blocks and so different hashes.
func testEndorserBlockN(t *testing.T, seed byte, txCount int) testEB {
	t.Helper()
	refs := make([]lcommon.LeiosTransactionReference, 0, txCount)
	for i := range txCount {
		refs = append(refs, lcommon.LeiosTransactionReference{
			TransactionHash: lcommon.Blake2b256{seed, byte(i)},
			TransactionSize: 1,
		})
	}
	raw, err := lcommon.LeiosEndorserBlock{TransactionReferences: refs}.
		MarshalCBOR()
	require.NoError(t, err)
	eb, err := lcommon.NewLeiosEndorserBlockFromCbor(raw)
	require.NoError(t, err)
	return testEB{raw: raw, hash: eb.Hash()}
}

func testEndorserBlock(t *testing.T, seed byte) testEB {
	t.Helper()
	return testEndorserBlockN(t, seed, 1)
}

func testBlockRequest(t *testing.T, seed byte) *MsgBlockRequest {
	t.Helper()
	return NewMsgBlockRequest(
		pcommon.NewPoint(10, testEndorserBlock(t, seed).hash.Bytes()),
	)
}

func testRangeRequest() *MsgBlockRangeRequest {
	return NewMsgBlockRangeRequest(
		pcommon.NewPoint(10, testPointHash(0x01)),
		pcommon.NewPoint(20, testPointHash(0x02)),
	)
}

// requestFor returns a request that the given reply answers.
func requestFor(t *testing.T, resp protocol.Message) protocol.Message {
	t.Helper()
	switch resp.Type() {
	case MessageTypeBlock:
		return testBlockRequest(t, 1)
	case MessageTypeBlockTxs:
		return NewMsgBlockTxsRequest(
			pcommon.NewPoint(10, testPointHash(0x01)),
			nil,
		)
	case MessageTypeVotes:
		return NewMsgVotesRequest(nil)
	default:
		return testRangeRequest()
	}
}

func requireDelivered(
	t *testing.T,
	slot *requestSlot,
	msg protocol.Message,
) {
	t.Helper()
	delivered, err := slot.deliver(msg, true)
	require.NoError(t, err)
	require.True(t, delivered, "response was dropped")
}

func testVote(t *testing.T, slotNo, voterId uint64) cbor.RawMessage {
	t.Helper()
	vote := lcommon.LeiosVote{
		SlotNo:        slotNo,
		VoterId:       voterId,
		VoteSignature: make([]byte, lcommon.LeiosBlsSignatureSize),
	}
	raw, err := cbor.Encode(vote)
	require.NoError(t, err)
	return cbor.RawMessage(raw)
}

func TestValidateResponse(t *testing.T) {
	point := pcommon.NewPoint(10, testPointHash(0x01))
	bitmaps := map[uint16]uint64{0: 0b101, 3: 0b1}
	txs := []cbor.RawMessage{{0x01}, {0x02}, {0x03}}
	voteIds := []MsgVotesRequestVoteId{
		{SlotNo: 5, VoterId: 1},
		{SlotNo: 5, VoterId: 2},
	}
	eb1 := testEndorserBlock(t, 1)
	eb2 := testEndorserBlock(t, 2)
	eb3 := testEndorserBlockN(t, 3, 3)
	oneTx := []cbor.RawMessage{{0x01}}
	rangeReq := testRangeRequest()

	for _, tc := range []struct {
		name    string
		request protocol.Message
		resp    protocol.Message
		wantErr bool
	}{
		{
			name:    "block accepted",
			request: testBlockRequest(t, 1),
			resp:    NewMsgBlock(eb1.raw),
		},
		{
			name:    "block for another point",
			request: testBlockRequest(t, 1),
			resp:    NewMsgBlock(eb2.raw),
			wantErr: true,
		},
		{
			name:    "block that is not an endorser block",
			request: testBlockRequest(t, 1),
			resp:    NewMsgBlock([]byte{0x82, 0x01, 0x02}),
			wantErr: true,
		},
		{
			name:    "block request answered by block txs",
			request: testBlockRequest(t, 1),
			resp:    NewMsgBlockTxs(nil),
			wantErr: true,
		},
		{
			name:    "block txs request answered by block",
			request: NewMsgBlockTxsRequest(point, bitmaps),
			resp:    NewMsgBlock(eb1.raw),
			wantErr: true,
		},
		{
			name:    "votes request answered by block",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgBlock(eb1.raw),
			wantErr: true,
		},
		{
			name:    "range request answered by block",
			request: rangeReq,
			resp:    NewMsgBlock(eb1.raw),
			wantErr: true,
		},
		{
			name:    "block txs accepted",
			request: NewMsgBlockTxsRequest(point, bitmaps),
			resp:    NewMsgBlockTxs(txs),
		},
		{
			name:    "block txs too few",
			request: NewMsgBlockTxsRequest(point, bitmaps),
			resp:    NewMsgBlockTxs(txs[:2]),
			wantErr: true,
		},
		{
			name:    "block txs too many",
			request: NewMsgBlockTxsRequest(point, bitmaps),
			resp: NewMsgBlockTxs(
				append(slicesClone(txs), cbor.RawMessage{0x04}),
			),
			wantErr: true,
		},
		{
			name:    "votes accepted",
			request: NewMsgVotesRequest(voteIds),
			resp: NewMsgVotes([]cbor.RawMessage{
				testVote(t, 5, 2),
				testVote(t, 5, 1),
			}),
		},
		{
			name:    "votes subset accepted",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgVotes([]cbor.RawMessage{testVote(t, 5, 2)}),
		},
		{
			name:    "votes empty accepted",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgVotes(nil),
		},
		{
			name:    "vote from other voter",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgVotes([]cbor.RawMessage{testVote(t, 5, 9)}),
			wantErr: true,
		},
		{
			name:    "vote from other slot",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgVotes([]cbor.RawMessage{testVote(t, 6, 1)}),
			wantErr: true,
		},
		{
			name:    "duplicate vote",
			request: NewMsgVotesRequest(voteIds),
			resp: NewMsgVotes([]cbor.RawMessage{
				testVote(t, 5, 1),
				testVote(t, 5, 1),
			}),
			wantErr: true,
		},
		{
			name:    "more votes than requested",
			request: NewMsgVotesRequest(voteIds[:1]),
			resp: NewMsgVotes([]cbor.RawMessage{
				testVote(t, 5, 1),
				testVote(t, 5, 2),
			}),
			wantErr: true,
		},
		{
			name:    "undecodable vote",
			request: NewMsgVotesRequest(voteIds),
			resp:    NewMsgVotes([]cbor.RawMessage{{0x01}}),
			wantErr: true,
		},
		{
			name:    "range next accepted",
			request: rangeReq,
			resp:    NewMsgNextBlockAndTxsInRange(eb1.raw, oneTx),
		},
		{
			name:    "range last accepted",
			request: rangeReq,
			resp:    NewMsgLastBlockAndTxsInRange(eb3.raw, txs),
		},
		{
			name:    "range block that is not an endorser block",
			request: rangeReq,
			resp:    NewMsgNextBlockAndTxsInRange([]byte{0x82, 1, 2}, nil),
			wantErr: true,
		},
		{
			name:    "range block with too few transactions",
			request: rangeReq,
			resp:    NewMsgNextBlockAndTxsInRange(eb3.raw, txs[:2]),
			wantErr: true,
		},
		{
			name:    "range block with too many transactions",
			request: rangeReq,
			resp:    NewMsgLastBlockAndTxsInRange(eb1.raw, txs),
			wantErr: true,
		},
		{
			name:    "block request answered by range block",
			request: testBlockRequest(t, 1),
			resp:    NewMsgNextBlockAndTxsInRange(eb1.raw, oneTx),
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(map[lcommon.Blake2b256]struct{})
			err := validateResponse(tc.request, tc.resp, seen)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrUnexpectedResponse)
				return
			}
			require.NoError(t, err)
		})
	}

	t.Run("range block repeats", func(t *testing.T) {
		seen := make(map[lcommon.Blake2b256]struct{})
		require.NoError(t, validateResponse(
			rangeReq,
			NewMsgNextBlockAndTxsInRange(eb1.raw, oneTx),
			seen,
		))
		require.NoError(t, validateResponse(
			rangeReq,
			NewMsgNextBlockAndTxsInRange(eb2.raw, oneTx),
			seen,
		))
		require.ErrorIs(t, validateResponse(
			rangeReq,
			NewMsgLastBlockAndTxsInRange(eb1.raw, oneTx),
			seen,
		), ErrUnexpectedResponse)
	})

	t.Run("no active request", func(t *testing.T) {
		require.ErrorIs(
			t,
			validateResponse(nil, NewMsgBlock(eb1.raw), nil),
			ErrUnexpectedResponse,
		)
	})
}

func slicesClone(txs []cbor.RawMessage) []cbor.RawMessage {
	return append([]cbor.RawMessage{}, txs...)
}

// A mismatched reply must not reach the waiting caller or free the slot.
func TestRequestSlotRejectsMismatchedReply(t *testing.T) {
	slot := &requestSlot{}
	w, err := slot.acquire(
		context.Background(),
		make(chan struct{}),
		testBlockRequest(t, 1),
	)
	require.NoError(t, err)

	delivered, err := slot.deliver(
		NewMsgBlock(testEndorserBlock(t, 2).raw),
		true,
	)
	require.ErrorIs(t, err, ErrUnexpectedResponse)
	assert.False(t, delivered)

	delivered, err = slot.deliverRange(
		NewMsgLastBlockAndTxsInRange(
			testEndorserBlock(t, 1).raw,
			[]cbor.RawMessage{{0x01}},
		),
		true,
	)
	require.ErrorIs(t, err, ErrUnexpectedResponse)
	assert.False(t, delivered)

	select {
	case got := <-w:
		t.Fatalf("mismatched reply was delivered: %v", got)
	case <-time.After(50 * time.Millisecond):
	}
	slot.mu.Lock()
	assert.True(t, slot.busy)
	slot.mu.Unlock()

	requireDelivered(t, slot, NewMsgBlock(testEndorserBlock(t, 1).raw))
	select {
	case <-w:
	case <-time.After(time.Second):
		t.Fatal("matching reply was not delivered")
	}
}

// A range stream must not repeat an endorser block across its messages.
func TestRequestSlotRejectsRepeatedRangeBlock(t *testing.T) {
	slot := &requestSlot{}
	w, err := slot.acquireWithRangeLimits(
		context.Background(),
		make(chan struct{}),
		testRangeRequest(),
		10,
		1<<20,
	)
	require.NoError(t, err)
	defer slot.release(w)

	eb := testEndorserBlock(t, 1).raw
	txs := []cbor.RawMessage{{0x01}}
	delivered, err := slot.deliverRange(
		NewMsgNextBlockAndTxsInRange(eb, txs),
		false,
	)
	require.NoError(t, err)
	require.True(t, delivered)
	<-w

	delivered, err = slot.deliverRange(
		NewMsgLastBlockAndTxsInRange(eb, txs),
		true,
	)
	require.ErrorIs(t, err, ErrUnexpectedResponse)
	assert.False(t, delivered)
}

// A late reply to an abandoned request has no waiter and is drained, not
// rejected, so quarantine still frees the slot.
func TestRequestSlotAbandonedReplyIsNotValidated(t *testing.T) {
	slot := &requestSlot{}
	w, err := slot.acquire(
		context.Background(),
		make(chan struct{}),
		testBlockRequest(t, 1),
	)
	require.NoError(t, err)
	slot.abandon(w)

	delivered, err := slot.deliver(
		NewMsgBlock(testEndorserBlock(t, 2).raw),
		true,
	)
	require.NoError(t, err)
	assert.True(t, delivered)
	slot.mu.Lock()
	assert.False(t, slot.busy)
	slot.mu.Unlock()
}
