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
// subsequent BlockRequest still fails cleanly with its own context error and
// does NOT tear down the shared multiplexed connection. The runTest harness
// panics on any error delivered to the connection ErrorChan, so completing
// without a panic proves the connection stays alive (no SendError / fatal
// teardown from a request issued after an abandoned one).
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
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
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
			require.Error(t, err1)
			assert.True(
				t,
				errors.Is(err1, context.DeadlineExceeded),
				"expected context deadline error, got %v",
				err1,
			)
			assert.Nil(t, resp1)

			// Subsequent request: also bounded by its own context. It must
			// return a context error, not tear down the connection.
			ctx2, cancel2 := context.WithTimeout(
				context.Background(),
				150*time.Millisecond,
			)
			defer cancel2()
			resp2, err2 := client.BlockRequest(
				ctx2,
				pcommon.NewPoint(23456, []byte{0x05, 0x06, 0x07, 0x08}),
			)
			require.Error(t, err2)
			assert.True(
				t,
				errors.Is(err2, context.DeadlineExceeded),
				"expected context deadline error, got %v",
				err2,
			)
			assert.Nil(t, resp2)
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
