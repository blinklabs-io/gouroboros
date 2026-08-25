// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package ouroboros_test

import (
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	mocknet "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestUnconfiguredResponderKeepsConnectionUsable(t *testing.T) {
	defer goleak.VerifyNone(t)
	conversation := []mocknet.ConversationEntry{
		mocknet.ConversationEntryHandshakeRequestOutput,
		mocknet.ConversationEntryHandshakeNtNResponseInput,
		mocknet.ConversationEntryOutput{
			ProtocolId: leiosvotes.ProtocolId,
			Messages: []protocol.Message{
				leiosvotes.NewMsgVotesRequestNext(1),
			},
		},
		mocknet.ConversationEntryOutput{
			ProtocolId: keepalive.ProtocolId,
			Messages: []protocol.Message{
				keepalive.NewMsgKeepAlive(mocknet.MockKeepAliveCookie),
			},
		},
		mocknet.ConversationEntryInput{
			ProtocolId: keepalive.ProtocolId,
			IsResponse: true,
			Message: keepalive.NewMsgKeepAliveResponse(
				mocknet.MockKeepAliveCookie,
			),
			MsgFromCborFunc: keepalive.NewMsgFromCbor,
		},
	}
	mockConn := mocknet.NewConnection(
		mocknet.ProtocolRoleServer,
		conversation,
	).(*mocknet.Connection)
	votesCfg := leiosvotes.NewConfig(
		leiosvotes.WithTimeout(5 * time.Millisecond),
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(mocknet.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithServer(true),
		ouroboros.WithLeiosVotesConfig(votesCfg),
	)
	require.NoError(t, err)
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("close shared connection: %v", err)
		}
		if err := mockConn.Close(); err != nil {
			t.Errorf("close mock connection: %v", err)
		}
		select {
		case <-oConn.ErrorChan():
		case <-time.After(2 * time.Second):
			t.Error("shared connection did not shut down")
		}
	}()
	require.Eventually(t, func() bool {
		return !oConn.LeiosVotes().Server.ProtocolInstance().
			IsInTerminalOrIdleState()
	}, time.Second, time.Millisecond,
		"unconfigured responder did not remain pending in Busy",
	)
	require.Never(t, func() bool {
		select {
		case <-oConn.ErrorChan():
			return true
		default:
			return false
		}
	}, 50*time.Millisecond, time.Millisecond,
		"unconfigured responder closed or errored the shared connection",
	)
	select {
	case err, ok := <-mockConn.ErrorChan():
		if ok {
			require.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mock conversation did not complete")
	}
	select {
	case err, ok := <-oConn.ErrorChan():
		if !ok {
			t.Fatal("shared connection closed unexpectedly")
		}
		require.NoError(t, err)
	default:
	}
}
