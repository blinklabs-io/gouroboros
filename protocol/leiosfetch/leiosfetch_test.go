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

package leiosfetch_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type testInnerFunc func(*testing.T, *ouroboros.Connection)

func runTest(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
		close(asyncErrChan)
	}()
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithLeiosFetchConfig(leiosfetch.Config{}),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	// Async error handler
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		// We can't call t.Fatalf() from a different Goroutine, so we panic instead
		panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
	}()
	// Run test inner function
	innerFunc(t, oConn)
	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close Ouroboros connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Ouroboros object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

// runTestCollectingConnErrors is runTest with the shared connection ErrorChan
// drained into a channel instead of panicking, so a test can assert that a
// desynchronised leios-fetch exchange fails the connection. Every wait is
// bounded: the bug under test is a park, so a hang must fail the test rather
// than hang the suite.
func runTestCollectingConnErrors(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc func(*testing.T, *ouroboros.Connection, <-chan error),
	connectionOptions ...ouroboros.ConnectionOptionFunc,
) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Drain the mock's error channel so the mock conversation goroutine is
	// never gated on a reader.
	mockDone := make(chan struct{})
	go func() {
		defer close(mockDone)
		for range mockConn.(*ouroboros_mock.Connection).ErrorChan() {
		}
	}()
	options := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithLeiosFetchConfig(leiosfetch.Config{}),
	}
	options = append(options, connectionOptions...)
	oConn, err := ouroboros.New(options...)
	require.NoError(t, err)
	connErrChan := make(chan error, 16)
	connDone := make(chan struct{})
	go func() {
		defer close(connDone)
		for err := range oConn.ErrorChan() {
			if err == nil {
				continue
			}
			select {
			case connErrChan <- err:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		// Ensure failed assertions also release the mock and protocol goroutines.
		_ = oConn.Close()
		select {
		case <-connDone:
		case <-time.After(10 * time.Second):
			t.Error("connection did not shut down within timeout")
		}
		select {
		case <-mockDone:
		case <-time.After(10 * time.Second):
			t.Error("mock connection did not shut down within timeout")
		}
	})
	innerFunc(t, oConn, connErrChan)
}

var conversationHandshake = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtNResponse,
}

// TestBlockRequestSuccess verifies the normal path: the server answers a
// BlockRequest with MsgBlock and the caller receives the exact block payload.
// Because the per-request delivery channel is registered before the request
// message is sent, the response cannot be dropped even if it arrives before
// the caller parks on the channel.
func TestBlockRequestSuccess(t *testing.T) {
	eb := testEB(t, 0x0a, 1)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgBlock(eb.raw),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			resp, err := oConn.LeiosFetch().Client.BlockRequest(
				context.Background(),
				pcommon.NewPoint(12345, eb.hash),
			)
			require.NoError(t, err)
			require.IsType(t, &leiosfetch.MsgBlock{}, resp)
			assert.Equal(
				t,
				eb.raw,
				[]byte(resp.(*leiosfetch.MsgBlock).BlockRaw),
			)
		},
	)
}

// TestBlockRequestSubsequentAfterAbandonedNoResponse verifies that when a
// BlockRequest is abandoned via its context and NO response ever arrives, a
// subsequent BlockRequest fails fast with ErrRequestSlotAbandoned and fails the
// connection.
//
// The peer holds leios-fetch server agency, so protocol.sendLoop can never
// regain the agency it needs to write another request on this bearer: the
// exchange is desynchronised for the life of the connection. Failing it is what
// lets peer governance drop and replace the peer instead of leaving a
// permanently dead leios-fetch client attached to an apparently healthy peer
// (dingo issue #3623).
func TestBlockRequestSubsequentAfterAbandonedNoResponse(t *testing.T) {
	conversation := append(
		conversationHandshake,
		// The server receives the first request and never responds. The
		// second request never reaches the wire because the client blocks it
		// until the first (never-arriving) response drains.
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRequest,
		},
	)
	runTestCollectingConnErrors(
		t,
		conversation,
		func(
			t *testing.T,
			oConn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			client := oConn.LeiosFetch().Client
			ctx1, cancel1 := context.WithTimeout(
				context.Background(),
				150*time.Millisecond,
			)
			defer cancel1()
			resp1, err1 := client.BlockRequest(
				ctx1,
				pcommon.NewPoint(12345, testPointHash(0x01)),
			)
			require.ErrorIs(t, err1, context.DeadlineExceeded)
			assert.Nil(t, resp1)

			// A subsequent request must fail after the bounded grace period
			// while the first request's response is still outstanding. Reusing
			// the slot would allow a late response to be mis-delivered here.
			// Bound the call itself: without the fix this is where #3623
			// parks.
			type reqResult struct {
				resp protocol.Message
				err  error
			}
			resultChan := make(chan reqResult, 1)
			go func() {
				resp, err := client.BlockRequest(
					context.Background(),
					pcommon.NewPoint(23456, testPointHash(0x05)),
				)
				resultChan <- reqResult{resp: resp, err: err}
			}()
			select {
			case result := <-resultChan:
				require.ErrorIs(
					t,
					result.err,
					leiosfetch.ErrRequestSlotAbandoned,
				)
				assert.Nil(t, result.resp)
			case <-time.After(10 * time.Second):
				t.Fatal(
					"subsequent BlockRequest parked instead of failing fast",
				)
			}

			// The desynchronised exchange must fail the connection so peer
			// governance can replace the peer.
			select {
			case err := <-connErrs:
				require.ErrorIs(t, err, leiosfetch.ErrRequestSlotAbandoned)
				require.ErrorContains(t, err, "retained agency")
			case <-time.After(10 * time.Second):
				t.Fatal(
					"desynchronised leios-fetch exchange did not fail the connection",
				)
			}
		},
	)
}

// TestBlockTxsRequestSubsequentAfterAbandonedNoResponse is the BlockTxs
// equivalent, which is the exact path dingo issue #3623 stalled on
// (fetchLeiosEbTxsBatchedUntilWithValidator -> BlockTxsRequest).
func TestBlockTxsRequestSubsequentAfterAbandonedNoResponse(t *testing.T) {
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockTxsRequest,
		},
	)
	runTestCollectingConnErrors(
		t,
		conversation,
		func(
			t *testing.T,
			oConn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			client := oConn.LeiosFetch().Client
			bitmaps := map[uint16]uint64{0: 0xff00000000000000}
			ctx1, cancel1 := context.WithTimeout(
				context.Background(),
				150*time.Millisecond,
			)
			defer cancel1()
			_, err1 := client.BlockTxsRequest(
				ctx1,
				pcommon.NewPoint(12345, testPointHash(0x01)),
				bitmaps,
			)
			require.ErrorIs(t, err1, context.DeadlineExceeded)

			errChan := make(chan error, 1)
			go func() {
				_, err := client.BlockTxsRequest(
					context.Background(),
					pcommon.NewPoint(23456, testPointHash(0x05)),
					bitmaps,
				)
				errChan <- err
			}()
			select {
			case err := <-errChan:
				require.ErrorIs(t, err, leiosfetch.ErrRequestSlotAbandoned)
			case <-time.After(10 * time.Second):
				t.Fatal(
					"subsequent BlockTxsRequest parked instead of failing fast",
				)
			}
			select {
			case err := <-connErrs:
				require.ErrorIs(t, err, leiosfetch.ErrRequestSlotAbandoned)
				require.ErrorContains(t, err, "BlockTxs")
			case <-time.After(10 * time.Second):
				t.Fatal(
					"desynchronised leios-fetch exchange did not fail the connection",
				)
			}
		},
	)
}

// TestBlockRequestContextCancelledNonFatal verifies that when a BlockRequest
// receives no response, the caller-supplied context bounds the wait and the
// request returns the context error. Critically, this must NOT emit a protocol
// error (the runTest harness panics on any error delivered to the shared
// connection ErrorChan), proving that an unanswered leios-fetch request does
// not tear down the multiplexed connection that other mini-protocols share.
func TestBlockRequestContextCancelledNonFatal(t *testing.T) {
	conversation := append(
		conversationHandshake,
		// The server receives the request but never responds.
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRequest,
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				200*time.Millisecond,
			)
			defer cancel()
			resp, err := oConn.LeiosFetch().Client.BlockRequest(
				ctx,
				pcommon.NewPoint(12345, testPointHash(0x01)),
			)
			require.Error(t, err)
			assert.True(
				t,
				errors.Is(err, context.DeadlineExceeded),
				"expected context deadline error, got %v",
				err,
			)
			assert.Nil(t, resp)
		},
	)
}

// TestBlockRequestSubsequentAfterAbandoned verifies that after a BlockRequest
// is abandoned via its context, a subsequent BlockRequest on the same client
// still completes normally. The late response to the abandoned request arrives
// after the caller has given up; it drives the mini-protocol state back to
// Idle (and is then delivered/dropped without blocking the receive loop), so
// the client remains usable for further requests.
func TestBlockRequestSubsequentAfterAbandoned(t *testing.T) {
	eb1 := testEB(t, 0x01, 1)
	eb2 := testEB(t, 0x03, 1)
	conversation := append(
		conversationHandshake,
		// First request: the server delays past the caller's context
		// deadline before finally sending a (now unawaited) response.
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRequest,
		},
		ouroboros_mock.ConversationEntrySleep{
			Duration: 300 * time.Millisecond,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgBlock(eb1.raw),
			},
		},
		// Second request: served normally.
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgBlock(eb2.raw),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.LeiosFetch().Client
			// First request: abandoned when its short context expires
			// before the (delayed) server response arrives.
			ctx1, cancel1 := context.WithTimeout(
				context.Background(),
				100*time.Millisecond,
			)
			defer cancel1()
			resp1, err1 := client.BlockRequest(
				ctx1,
				pcommon.NewPoint(12345, eb1.hash),
			)
			require.Error(t, err1)
			assert.True(
				t,
				errors.Is(err1, context.DeadlineExceeded),
				"expected context deadline error, got %v",
				err1,
			)
			assert.Nil(t, resp1)

			// Second request on the same client must still complete.
			ctx2, cancel2 := context.WithTimeout(
				context.Background(),
				2*time.Second,
			)
			defer cancel2()
			resp2, err2 := client.BlockRequest(
				ctx2,
				pcommon.NewPoint(23456, eb2.hash),
			)
			require.NoError(t, err2)
			require.NotNil(t, resp2)
			require.IsType(t, &leiosfetch.MsgBlock{}, resp2)
			// Assert the exact payload for the SECOND request. The late
			// response to the abandoned first request carried {0x82,0x01,0x02};
			// if correlation were broken it would be mis-delivered here. Only
			// the second request's own response {0x82,0x03,0x04} is correct.
			block2 := resp2.(*leiosfetch.MsgBlock)
			assert.Equal(
				t,
				eb2.raw,
				[]byte(block2.BlockRaw),
			)
		},
	)
}

// TestBlockRangeRequestSubsequentAfterAbandoned verifies that a late terminal
// response from an abandoned range exchange cannot be delivered to a later
// range request. Range responses use the shared request slot, so the first
// request must remain quarantined until its terminal response drains.
func TestBlockRangeRequestSubsequentAfterAbandoned(t *testing.T) {
	eb1 := testEB(t, 0x01, 1)
	eb2 := testEB(t, 0x03, 1)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntrySleep{
			Duration: 300 * time.Millisecond,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgLastBlockAndTxsInRange(
					eb1.raw,
					eb1.txs,
				),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgLastBlockAndTxsInRange(
					eb2.raw,
					eb2.txs,
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.LeiosFetch().Client
			ctx1, cancel1 := context.WithTimeout(
				context.Background(),
				100*time.Millisecond,
			)
			defer cancel1()
			resp1, err1 := client.BlockRangeRequest(
				ctx1,
				pcommon.NewPoint(12345, testPointHash(0x01)),
				pcommon.NewPoint(12346, testPointHash(0x02)),
			)
			require.ErrorIs(t, err1, context.DeadlineExceeded)
			assert.Nil(t, resp1)

			ctx2, cancel2 := context.WithTimeout(
				context.Background(),
				2*time.Second,
			)
			defer cancel2()
			resp2, err2 := client.BlockRangeRequest(
				ctx2,
				pcommon.NewPoint(23456, testPointHash(0x03)),
				pcommon.NewPoint(23457, testPointHash(0x04)),
			)
			require.NoError(t, err2)
			require.Len(t, resp2, 1)
			last, ok := resp2[0].(*leiosfetch.MsgLastBlockAndTxsInRange)
			require.True(t, ok)
			assert.Equal(t, eb2.raw, []byte(last.BlockRaw))
		},
	)
}

func TestBlockRangeRequestStreamsMultipleMessages(t *testing.T) {
	eb1 := testEB(t, 0x01, 1)
	eb2 := testEB(t, 0x02, 1)
	eb3 := testEB(t, 0x03, 1)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgNextBlockAndTxsInRange(eb1.raw, eb1.txs),
				leiosfetch.NewMsgNextBlockAndTxsInRange(eb2.raw, eb2.txs),
				leiosfetch.NewMsgLastBlockAndTxsInRange(eb3.raw, eb3.txs),
			},
		},
	)
	runTest(t, conversation, func(t *testing.T, oConn *ouroboros.Connection) {
		client := oConn.LeiosFetch().Client
		resp, err := client.BlockRangeRequest(
			context.Background(),
			pcommon.NewPoint(12345, testPointHash(0x01)),
			pcommon.NewPoint(12346, testPointHash(0x02)),
		)
		require.NoError(t, err)
		require.Len(t, resp, 3)
	})
}

func TestBlockRangeRequestRejectsAggregateTransactionHeaders(t *testing.T) {
	ebA := testEB(t, 0x01, 3)
	ebB := testEB(t, 0x02, 3)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgNextBlockAndTxsInRange(ebA.raw, ebA.txs),
				leiosfetch.NewMsgLastBlockAndTxsInRange(ebB.raw, ebB.txs),
			},
		},
	)
	cfg := leiosfetch.NewConfig(
		leiosfetch.WithMaxBlockRangeResponseBytes(256),
	)
	runTestCollectingConnErrors(
		t,
		conversation,
		func(
			t *testing.T,
			conn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				2*time.Second,
			)
			defer cancel()
			_, err := conn.LeiosFetch().Client.BlockRangeRequest(
				ctx,
				pcommon.NewPoint(12345, testPointHash(0x01)),
				pcommon.NewPoint(12346, testPointHash(0x02)),
			)
			require.Error(t, err)
			select {
			case connErr := <-connErrs:
				require.ErrorContains(t, connErr, "maximum is 1")
			case <-time.After(2 * time.Second):
				t.Fatal("aggregate transaction headers did not fail the connection")
			}
		},
		ouroboros.WithLeiosFetchConfig(cfg),
	)
}

func TestBlockRangeRequestLimitFailsConnection(t *testing.T) {
	ebs := []testEBFixture{
		testEB(t, 0x01, 1),
		testEB(t, 0x02, 1),
		testEB(t, 0x03, 1),
	}
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgNextBlockAndTxsInRange(
					ebs[0].raw, ebs[0].txs,
				),
				leiosfetch.NewMsgNextBlockAndTxsInRange(
					ebs[1].raw, ebs[1].txs,
				),
				leiosfetch.NewMsgLastBlockAndTxsInRange(
					ebs[2].raw, ebs[2].txs,
				),
			},
		},
	)
	cfg := leiosfetch.NewConfig(
		leiosfetch.WithMaxBlockRangeResponses(2),
	)
	runTestCollectingConnErrors(
		t,
		conversation,
		func(
			t *testing.T,
			oConn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				time.Second,
			)
			defer cancel()
			_, err := oConn.LeiosFetch().Client.BlockRangeRequest(
				ctx,
				pcommon.NewPoint(12345, testPointHash(0x01)),
				pcommon.NewPoint(12346, testPointHash(0x02)),
			)
			require.Error(t, err)
			require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
			select {
			case <-oConn.LeiosFetch().Client.DoneChan():
			case <-time.After(time.Second):
				t.Fatal("over-limit range stream did not stop the protocol")
			}
			select {
			case connErr := <-connErrs:
				require.Error(t, connErr)
			case <-time.After(time.Second):
				t.Fatal("over-limit range stream did not fail the connection")
			}
		},
		ouroboros.WithLeiosFetchConfig(cfg),
	)
}

// A request the reply could not answer in full is refused locally rather than
// sent, because the full answer would fail the whole connection.
func TestVotesRequestRejectsMoreIDsThanOneReplyCarries(t *testing.T) {
	runTestCollectingConnErrors(
		t,
		conversationHandshake,
		func(
			t *testing.T,
			conn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				2*time.Second,
			)
			defer cancel()
			_, err := conn.LeiosFetch().Client.VotesRequest(
				ctx,
				make(
					[]leiosfetch.MsgVotesRequestVoteId,
					leiosfetch.MaxVotesPerMessage+1,
				),
			)
			require.ErrorIs(t, err, leiosfetch.ErrVoteRequestTooLarge)
			select {
			case connErr := <-connErrs:
				t.Fatalf("local refusal failed the connection: %v", connErr)
			case <-time.After(100 * time.Millisecond):
			}
		},
	)
}

func TestVotesRequestPinsReplyToRequestedCount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		requests int
		replies  int
		limitErr string
	}{
		{name: "at requested count", requests: 2, replies: 2},
		{name: "above requested count", requests: 2, replies: 3, limitErr: "maximum is 2"},
		{
			// The reply stays under one 65,535-byte mux segment, which the
			// mock peer cannot split; a full-size reply is covered by the
			// decoder tests.
			name:     "at protocol maximum request",
			requests: leiosfetch.MaxVotesPerMessage,
			replies:  700,
		},
		{
			name:     "above protocol maximum",
			requests: leiosfetch.MaxVotesPerMessage,
			replies:  leiosfetch.MaxVotesPerMessage + 1,
			limitErr: "maximum is 1000",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			voteIds := make([]leiosfetch.MsgVotesRequestVoteId, tc.requests)
			for idx := range voteIds {
				voteIds[idx] = leiosfetch.MsgVotesRequestVoteId{
					SlotNo:  1,
					VoterId: uint64(idx),
				}
			}
			votes := make([]cbor.RawMessage, tc.replies)
			for idx := range votes {
				if tc.limitErr != "" {
					// The decoder rejects the count before reading any
					// vote, and small entries keep an oversized reply inside
					// one mux segment.
					votes[idx] = cbor.RawMessage{0x80}
					continue
				}
				raw, err := cbor.Encode(lcommon.LeiosVote{
					SlotNo:  1,
					VoterId: uint64(idx),
					VoteSignature: make(
						[]byte,
						lcommon.LeiosBlsSignatureSize,
					),
				})
				require.NoError(t, err)
				votes[idx] = raw
			}
			conversation := append(
				conversationHandshake,
				ouroboros_mock.ConversationEntryInput{
					ProtocolId:  leiosfetch.ProtocolId,
					MessageType: leiosfetch.MessageTypeVotesRequest,
				},
				ouroboros_mock.ConversationEntryOutput{
					ProtocolId: leiosfetch.ProtocolId,
					IsResponse: true,
					Messages: []protocol.Message{
						leiosfetch.NewMsgVotes(votes),
					},
				},
			)
			runTestCollectingConnErrors(
				t,
				conversation,
				func(
					t *testing.T,
					conn *ouroboros.Connection,
					connErrs <-chan error,
				) {
					ctx, cancel := context.WithTimeout(
						context.Background(),
						2*time.Second,
					)
					defer cancel()
					msg, err := conn.LeiosFetch().Client.VotesRequest(
						ctx,
						voteIds,
					)
					if tc.limitErr == "" {
						require.NoError(t, err)
						require.Len(t, msg.(*leiosfetch.MsgVotes).VotesRaw, tc.replies)
						return
					}
					require.Error(t, err)
					select {
					case connErr := <-connErrs:
						require.ErrorContains(t, connErr, tc.limitErr)
					case <-time.After(2 * time.Second):
						t.Fatal("oversized Votes reply did not fail the connection")
					}
				},
			)
		})
	}
}

func TestBlockRangeRequestStallFailsConnection(t *testing.T) {
	stallEB := testEB(t, 0x01, 1)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgNextBlockAndTxsInRange(
					stallEB.raw,
					stallEB.txs,
				),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeBlockRangeRequest,
		},
	)
	cfg := leiosfetch.NewConfig(leiosfetch.WithTimeout(100 * time.Millisecond))
	runTestCollectingConnErrors(
		t,
		conversation,
		func(
			t *testing.T,
			oConn *ouroboros.Connection,
			connErrs <-chan error,
		) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				3*time.Second,
			)
			defer cancel()
			startedAt := time.Now()
			_, err := oConn.LeiosFetch().Client.BlockRangeRequest(
				ctx,
				pcommon.NewPoint(12345, testPointHash(0x01)),
				pcommon.NewPoint(12346, testPointHash(0x02)),
			)
			require.Error(t, err)
			require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
			require.GreaterOrEqual(t, time.Since(startedAt), 50*time.Millisecond)
			select {
			case <-oConn.LeiosFetch().Client.DoneChan():
			case <-time.After(time.Second):
				t.Fatal("stalled range stream did not stop the protocol")
			}
			select {
			case connErr := <-connErrs:
				require.Error(t, connErr)
			case <-time.After(time.Second):
				t.Fatal("stalled range stream did not fail the connection")
			}
		},
		ouroboros.WithLeiosFetchConfig(cfg),
	)
}

// TestNonBlockRequestsAfterAbandonedBlockRequestFailFast pins that every
// leios-fetch request path goes through the same admission slot.
//
// The states share one connection-wide agency, so a block request whose
// caller gave up leaves the peer holding agency and nothing further can be
// written to the bearer. Before these paths acquired the slot they skipped
// that diagnosis and blocked forever on their own result channels, with no
// context and no recovery.
func TestNonBlockRequestsAfterAbandonedBlockRequestFailFast(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request func(*leiosfetch.Client) error
	}{
		{
			name: "VotesRequest",
			request: func(c *leiosfetch.Client) error {
				_, err := c.VotesRequest(
					context.Background(),
					[]leiosfetch.MsgVotesRequestVoteId{},
				)
				return err
			},
		},
		{
			name: "BlockRangeRequest",
			request: func(c *leiosfetch.Client) error {
				_, err := c.BlockRangeRequest(
					context.Background(),
					pcommon.NewPoint(23456, testPointHash(0x05)),
					pcommon.NewPoint(34567, testPointHash(0x09)),
				)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conversation := append(
				conversationHandshake,
				ouroboros_mock.ConversationEntryInput{
					ProtocolId:  leiosfetch.ProtocolId,
					MessageType: leiosfetch.MessageTypeBlockRequest,
				},
			)
			runTestCollectingConnErrors(
				t,
				conversation,
				func(
					t *testing.T,
					oConn *ouroboros.Connection,
					connErrs <-chan error,
				) {
					client := oConn.LeiosFetch().Client
					ctx1, cancel1 := context.WithTimeout(
						context.Background(),
						150*time.Millisecond,
					)
					defer cancel1()
					_, err1 := client.BlockRequest(
						ctx1,
						pcommon.NewPoint(
							12345,
							testPointHash(0x01),
						),
					)
					require.ErrorIs(t, err1, context.DeadlineExceeded)

					errChan := make(chan error, 1)
					go func() { errChan <- tc.request(client) }()
					select {
					case err := <-errChan:
						require.ErrorIs(
							t,
							err,
							leiosfetch.ErrRequestSlotAbandoned,
						)
					case <-time.After(10 * time.Second):
						t.Fatal(
							"request parked behind an abandoned block request",
						)
					}

					select {
					case err := <-connErrs:
						require.ErrorIs(
							t,
							err,
							leiosfetch.ErrRequestSlotAbandoned,
						)
						require.ErrorContains(t, err, "retained agency")
					case <-time.After(10 * time.Second):
						t.Fatal(
							"desynchronised leios-fetch exchange did not fail the connection",
						)
					}
				},
			)
		})
	}
}

func TestBlockTxsRequestPinsReplyToBitmapCardinality(t *testing.T) {
	point := pcommon.NewPoint(12345, testPointHash(0x01))
	bitmaps := map[uint16]uint64{0: 0x3}
	t.Run("accepts requested count", func(t *testing.T) {
		conversation := append(
			conversationHandshake,
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  leiosfetch.ProtocolId,
				MessageType: leiosfetch.MessageTypeBlockTxsRequest,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: leiosfetch.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					leiosfetch.NewMsgBlockTxs([]cbor.RawMessage{{0x80}, {0x80}}),
				},
			},
		)
		runTest(t, conversation, func(t *testing.T, conn *ouroboros.Connection) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			msg, err := conn.LeiosFetch().Client.BlockTxsRequest(ctx, point, bitmaps)
			require.NoError(t, err)
			require.Len(t, msg.(*leiosfetch.MsgBlockTxs).TxsRaw, 2)
		})
	})

	t.Run("rejects above requested count", func(t *testing.T) {
		conversation := append(
			conversationHandshake,
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  leiosfetch.ProtocolId,
				MessageType: leiosfetch.MessageTypeBlockTxsRequest,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: leiosfetch.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					leiosfetch.NewMsgBlockTxs([]cbor.RawMessage{{0x80}, {0x80}, {0x80}}),
				},
			},
		)
		runTestCollectingConnErrors(
			t,
			conversation,
			func(t *testing.T, conn *ouroboros.Connection, connErrs <-chan error) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, err := conn.LeiosFetch().Client.BlockTxsRequest(ctx, point, bitmaps)
				require.Error(t, err)
				select {
				case connErr := <-connErrs:
					require.ErrorContains(t, connErr, "maximum is 2")
				case <-time.After(2 * time.Second):
					t.Fatal("oversized BlockTxs reply did not fail the connection")
				}
			},
		)
	})

	t.Run("rejects below requested count", func(t *testing.T) {
		conversation := append(
			conversationHandshake,
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  leiosfetch.ProtocolId,
				MessageType: leiosfetch.MessageTypeBlockTxsRequest,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: leiosfetch.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					leiosfetch.NewMsgBlockTxs([]cbor.RawMessage{{0x80}}),
				},
			},
		)
		runTestCollectingConnErrors(
			t,
			conversation,
			func(t *testing.T, conn *ouroboros.Connection, connErrs <-chan error) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, err := conn.LeiosFetch().Client.BlockTxsRequest(ctx, point, bitmaps)
				require.Error(t, err)
				select {
				case connErr := <-connErrs:
					require.ErrorContains(t, connErr, "do not match the 2 requested")
				case <-time.After(2 * time.Second):
					t.Fatal("short BlockTxs reply did not fail the connection")
				}
			},
		)
	})
}

// The reference node refuses these bitmaps while decoding the request, which
// fails the whole connection, so the client refuses them before sending.
func TestBlockTxsRequestRejectsBitmapsThePeerRefuses(t *testing.T) {
	point := pcommon.NewPoint(12345, testPointHash(0x01))
	for _, tc := range []struct {
		name    string
		bitmaps map[uint16]uint64
	}{
		{name: "key beyond an endorser block", bitmaps: map[uint16]uint64{268: 1}},
		{name: "bitmap selecting nothing", bitmaps: map[uint16]uint64{0: 1, 1: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runTestCollectingConnErrors(
				t,
				conversationHandshake,
				func(t *testing.T, conn *ouroboros.Connection, connErrs <-chan error) {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_, err := conn.LeiosFetch().Client.BlockTxsRequest(ctx, point, tc.bitmaps)
					require.ErrorIs(t, err, leiosfetch.ErrInvalidBlockTxsBitmaps)
					select {
					case connErr := <-connErrs:
						t.Fatalf("local refusal failed the connection: %v", connErr)
					case <-time.After(100 * time.Millisecond):
					}
				},
			)
		})
	}
	t.Run("last bitmap key", func(t *testing.T) {
		bitmaps := map[uint16]uint64{267: 1}
		conversation := append(
			conversationHandshake,
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  leiosfetch.ProtocolId,
				MessageType: leiosfetch.MessageTypeBlockTxsRequest,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: leiosfetch.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					leiosfetch.NewMsgBlockTxs([]cbor.RawMessage{{0x80}}),
				},
			},
		)
		runTest(t, conversation, func(t *testing.T, conn *ouroboros.Connection) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			msg, err := conn.LeiosFetch().Client.BlockTxsRequest(ctx, point, bitmaps)
			require.NoError(t, err)
			require.Len(t, msg.(*leiosfetch.MsgBlockTxs).TxsRaw, 1)
		})
	})
}

type testEBFixture struct {
	raw  []byte
	hash []byte
	txs  []cbor.RawMessage
}

// testEB builds an endorser block of txCount transactions and returns it with
// its hash and those transactions. Distinct seeds give distinct blocks.
func testEB(t *testing.T, seed byte, txCount int) testEBFixture {
	t.Helper()
	refs := make([]lcommon.LeiosTransactionReference, 0, txCount)
	txs := make([]cbor.RawMessage, 0, txCount)
	for i := range txCount {
		tx, err := cbor.Encode([]uint64{uint64(seed), uint64(i)})
		require.NoError(t, err)
		txs = append(txs, tx)
		refs = append(refs, lcommon.LeiosTransactionReference{
			TransactionHash: lcommon.Blake2b256Hash(tx),
			TransactionSize: uint32(len(tx)),
		})
	}
	raw, err := lcommon.LeiosEndorserBlock{TransactionReferences: refs}.
		MarshalCBOR()
	require.NoError(t, err)
	eb, err := lcommon.NewLeiosEndorserBlockFromCbor(raw)
	require.NoError(t, err)
	hash := eb.Hash()
	return testEBFixture{raw: raw, hash: hash.Bytes(), txs: txs}
}

func testTxs(count int) []cbor.RawMessage {
	txs := make([]cbor.RawMessage, count)
	for i := range txs {
		txs[i] = cbor.RawMessage{0x80}
	}
	return txs
}

// A reply of the expected type that does not describe the request must fail
// the connection with ErrUnexpectedResponse and never reach the caller.
func TestMismatchedReplyFailsConnection(t *testing.T) {
	eb1 := testEB(t, 0x01, 1)
	eb2 := testEB(t, 0x02, 1)
	vote, err := cbor.Encode(lcommon.LeiosVote{
		SlotNo:        9,
		VoterId:       9,
		VoteSignature: make([]byte, lcommon.LeiosBlsSignatureSize),
	})
	require.NoError(t, err)
	for _, tc := range []struct {
		name        string
		requestType uint
		reply       protocol.Message
		request     func(*leiosfetch.Client) (any, error)
	}{
		{
			name:        "block",
			requestType: leiosfetch.MessageTypeBlockRequest,
			reply:       leiosfetch.NewMsgBlock(eb2.raw),
			request: func(c *leiosfetch.Client) (any, error) {
				return c.BlockRequest(
					context.Background(),
					pcommon.NewPoint(12345, eb1.hash),
				)
			},
		},
		{
			name:        "transactions",
			requestType: leiosfetch.MessageTypeBlockTxsRequest,
			reply:       leiosfetch.NewMsgBlockTxs(testTxs(1)),
			request: func(c *leiosfetch.Client) (any, error) {
				return c.BlockTxsRequest(
					context.Background(),
					pcommon.NewPoint(12345, eb1.hash),
					map[uint16]uint64{0: 0b11},
				)
			},
		},
		{
			name:        "votes",
			requestType: leiosfetch.MessageTypeVotesRequest,
			reply:       leiosfetch.NewMsgVotes([]cbor.RawMessage{vote}),
			request: func(c *leiosfetch.Client) (any, error) {
				return c.VotesRequest(
					context.Background(),
					[]leiosfetch.MsgVotesRequestVoteId{
						{SlotNo: 1, VoterId: 1},
					},
				)
			},
		},
		{
			name:        "range",
			requestType: leiosfetch.MessageTypeBlockRangeRequest,
			reply: leiosfetch.NewMsgLastBlockAndTxsInRange(
				eb1.raw,
				eb2.txs,
			),
			request: func(c *leiosfetch.Client) (any, error) {
				return c.BlockRangeRequest(
					context.Background(),
					pcommon.NewPoint(12345, testPointHash(0x01)),
					pcommon.NewPoint(12346, testPointHash(0x02)),
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conversation := append(
				conversationHandshake,
				ouroboros_mock.ConversationEntryInput{
					ProtocolId:  leiosfetch.ProtocolId,
					MessageType: tc.requestType,
				},
				ouroboros_mock.ConversationEntryOutput{
					ProtocolId: leiosfetch.ProtocolId,
					IsResponse: true,
					Messages:   []protocol.Message{tc.reply},
				},
			)
			runTestCollectingConnErrors(
				t,
				conversation,
				func(
					t *testing.T,
					oConn *ouroboros.Connection,
					connErrs <-chan error,
				) {
					got, err := tc.request(oConn.LeiosFetch().Client)
					require.Error(t, err)
					// The mismatched reply must never reach the caller.
					require.Nil(t, got)
					select {
					case connErr := <-connErrs:
						require.ErrorIs(
							t,
							connErr,
							leiosfetch.ErrUnexpectedResponse,
						)
					case <-time.After(2 * time.Second):
						t.Fatal("mismatched reply did not fail the connection")
					}
				},
			)
		})
	}
}

// A request that repeats a vote ID is answered once per occurrence, and the
// reply is delivered.
func TestVotesRequestWithRepeatedIdsAcceptsMatchingReply(t *testing.T) {
	vote, err := cbor.Encode(lcommon.LeiosVote{
		SlotNo:        5,
		VoterId:       1,
		VoteSignature: make([]byte, lcommon.LeiosBlsSignatureSize),
	})
	require.NoError(t, err)
	conversation := append(
		conversationHandshake,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  leiosfetch.ProtocolId,
			MessageType: leiosfetch.MessageTypeVotesRequest,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: leiosfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				leiosfetch.NewMsgVotes([]cbor.RawMessage{vote, vote}),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			id := leiosfetch.MsgVotesRequestVoteId{SlotNo: 5, VoterId: 1}
			resp, err := oConn.LeiosFetch().Client.VotesRequest(
				context.Background(),
				[]leiosfetch.MsgVotesRequestVoteId{id, id},
			)
			require.NoError(t, err)
			require.Len(t, resp.(*leiosfetch.MsgVotes).VotesRaw, 2)
		},
	)
}
