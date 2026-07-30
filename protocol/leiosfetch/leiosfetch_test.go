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
				pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}),
				map[uint16]uint64{0: 0xff00000000000000},
			)
			require.Error(t, err)
			assert.ErrorIs(t, err, leiosfetch.ErrBlockTxsNotFound)
			assert.Nil(t, resp)
		},
	)
}
