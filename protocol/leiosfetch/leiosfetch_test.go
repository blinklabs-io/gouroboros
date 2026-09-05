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
) {
	defer goleak.VerifyNone(t)
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
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
	)
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
	innerFunc(t, oConn, connErrChan)
	// The connection may already be closing because the test asserted a
	// protocol error, so a Close error is not itself a failure.
	_ = oConn.Close()
	select {
	case <-connDone:
	case <-time.After(10 * time.Second):
		t.Fatal("connection did not shut down within timeout")
	}
	select {
	case <-mockDone:
	case <-time.After(10 * time.Second):
		t.Fatal("mock connection did not shut down within timeout")
	}
}

var conversationHandshake = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtNResponse,
}

// TestBlockRequestNoBlock verifies that when the server responds to a
// BlockRequest with MsgNoBlock, the client's BlockRequest returns the typed
// ErrBlockNotFound sentinel rather than a protocol error, and the connection
// stays usable.
func TestBlockRequestNoBlock(t *testing.T) {
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
				leiosfetch.NewMsgNoBlock(),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			resp, err := oConn.LeiosFetch().Client.BlockRequest(
				context.Background(),
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
			)
			require.Error(t, err)
			assert.ErrorIs(t, err, leiosfetch.ErrBlockNotFound)
			assert.Nil(t, resp)
		},
	)
}

// TestBlockTxsRequestNoBlockTxs verifies the equivalent behavior for
// BlockTxsRequest / MsgNoBlockTxs.
func TestBlockTxsRequestNoBlockTxs(t *testing.T) {
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
				leiosfetch.NewMsgNoBlockTxs(),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			resp, err := oConn.LeiosFetch().Client.BlockTxsRequest(
				context.Background(),
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
				map[uint16]uint64{0: 0xff00000000000000},
			)
			require.Error(t, err)
			assert.ErrorIs(t, err, leiosfetch.ErrBlockTxsNotFound)
			assert.Nil(t, resp)
		},
	)
}

// TestBlockRequestSuccess verifies the normal path: the server answers a
// BlockRequest with MsgBlock and the caller receives the exact block payload.
// Because the per-request delivery channel is registered before the request
// message is sent, the response cannot be dropped even if it arrives before
// the caller parks on the channel.
func TestBlockRequestSuccess(t *testing.T) {
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
				leiosfetch.NewMsgBlock([]byte{0x82, 0x0a, 0x0b}),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			resp, err := oConn.LeiosFetch().Client.BlockRequest(
				context.Background(),
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
			)
			require.NoError(t, err)
			require.IsType(t, &leiosfetch.MsgBlock{}, resp)
			assert.Equal(
				t,
				[]byte{0x82, 0x0a, 0x0b},
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
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
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
					pcommon.NewPoint(23456, []byte{0x05, 0x06, 0x07, 0x08}),
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
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
				bitmaps,
			)
			require.ErrorIs(t, err1, context.DeadlineExceeded)

			errChan := make(chan error, 1)
			go func() {
				_, err := client.BlockTxsRequest(
					context.Background(),
					pcommon.NewPoint(23456, []byte{0x05, 0x06, 0x07, 0x08}),
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
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
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
				leiosfetch.NewMsgBlock([]byte{0x82, 0x01, 0x02}),
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
				leiosfetch.NewMsgBlock([]byte{0x82, 0x03, 0x04}),
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
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
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
				pcommon.NewPoint(23456, []byte{0x05, 0x06, 0x07, 0x08}),
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
				[]byte{0x82, 0x03, 0x04},
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
					[]byte{0x82, 0x01, 0x02},
					nil,
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
					[]byte{0x82, 0x03, 0x04},
					nil,
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
				pcommon.NewPoint(12345, []byte{0x01}),
				pcommon.NewPoint(12346, []byte{0x02}),
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
				pcommon.NewPoint(23456, []byte{0x03}),
				pcommon.NewPoint(23457, []byte{0x04}),
			)
			require.NoError(t, err2)
			require.Len(t, resp2, 1)
			last, ok := resp2[0].(*leiosfetch.MsgLastBlockAndTxsInRange)
			require.True(t, ok)
			assert.Equal(t, []byte{0x82, 0x03, 0x04}, []byte(last.BlockRaw))
		},
	)
}

func TestBlockRangeRequestStreamsMultipleMessages(t *testing.T) {
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
				leiosfetch.NewMsgNextBlockAndTxsInRange([]byte{0x82, 0x01, 0x01}, nil),
				leiosfetch.NewMsgNextBlockAndTxsInRange([]byte{0x82, 0x01, 0x02}, nil),
				leiosfetch.NewMsgLastBlockAndTxsInRange([]byte{0x82, 0x01, 0x03}, nil),
			},
		},
	)
	runTest(t, conversation, func(t *testing.T, oConn *ouroboros.Connection) {
		client := oConn.LeiosFetch().Client
		resp, err := client.BlockRangeRequest(
			context.Background(),
			pcommon.NewPoint(12345, []byte{0x01}),
			pcommon.NewPoint(12346, []byte{0x02}),
		)
		require.NoError(t, err)
		require.Len(t, resp, 3)
	})
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
					pcommon.NewPoint(23456, []byte{0x05, 0x06, 0x07, 0x08}),
					pcommon.NewPoint(34567, []byte{0x09, 0x0a, 0x0b, 0x0c}),
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
							[]byte{0x01, 0x02, 0x03, 0x04},
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
