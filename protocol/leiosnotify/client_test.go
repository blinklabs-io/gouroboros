// Copyright 2025 Blink Labs Software
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

package leiosnotify_test

import (
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"

	"go.uber.org/goleak"
)

func mockNtNVersionData() protocol.VersionData {
	return protocol.VersionDataNtN13andUp{
		VersionDataNtN11to12: mockNtNVersionDataV11().(protocol.VersionDataNtN11to12),
	}
}

func mockNtNVersionDataV11() protocol.VersionData {
	return protocol.VersionDataNtN11to12{
		CborNetworkMagic:                       ouroboros_mock.MockNetworkMagic,
		CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
		CborPeerSharing:                        protocol.PeerSharingModeNoPeerSharing,
		CborQuery:                              protocol.QueryModeDisabled,
	}
}

var conversationEntryNtNResponseV15 = ouroboros_mock.ConversationEntryOutput{
	ProtocolId: handshake.ProtocolId,
	IsResponse: true,
	Messages: []protocol.Message{
		handshake.NewMsgAcceptVersion(
			15,
			mockNtNVersionData(),
		),
	},
}

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
	case <-time.After(5 * time.Second):
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

func TestClientShutdown(t *testing.T) {
	runTest(
		t,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			conversationEntryNtNResponseV15,
		},
		func(t *testing.T, oConn *ouroboros.Connection) {
			if oConn.LeiosNotify() == nil {
				t.Fatalf("LeiosNotify client is nil")
			}
			// Start the client
			oConn.LeiosNotify().Client.Start()
			// Stop the client
			if err := oConn.LeiosNotify().Client.Stop(); err != nil {
				t.Fatalf("unexpected error when stopping client: %s", err)
			}
		},
	)
}
