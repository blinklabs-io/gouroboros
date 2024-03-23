// Copyright 2023 Blink Labs Software
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
	"io"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/ouroboros-mock"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"go.uber.org/goleak"
)

func TestConnectionManagerTagString(t *testing.T) {
	testDefs := map[ouroboros.ConnectionManagerTag]string{
		ouroboros.ConnectionManagerTagHostP2PLedger: "HostP2PLedger",
		ouroboros.ConnectionManagerTagHostP2PGossip: "HostP2PGossip",
		ouroboros.ConnectionManagerTagRoleInitiator: "RoleInitiator",
		ouroboros.ConnectionManagerTagRoleResponder: "RoleResponder",
		ouroboros.ConnectionManagerTagNone:          "Unknown",
		ouroboros.ConnectionManagerTag(9999):        "Unknown",
	}
	for k, v := range testDefs {
		if k.String() != v {
			t.Fatalf(
				"did not get expected string for ID %d: got %s, expected %s",
				k,
				k.String(),
				v,
			)
		}
	}
}

func TestConnectionManagerConnError(t *testing.T) {
	defer goleak.VerifyNone(t)
	expectedConnId := 2
	expectedErr := io.EOF
	doneChan := make(chan any)
	connManager := ouroboros.NewConnectionManager(
		ouroboros.ConnectionManagerConfig{
			ConnClosedFunc: func(connId int, err error) {
				if err != nil {
					if connId != expectedConnId {
						t.Fatalf("did not receive error from expected connection: got %d, wanted %d", connId, expectedConnId)
					}
					if err != expectedErr {
						t.Fatalf("did not receive expected error: got: %s, expected: %s", err, expectedErr)
					}
					close(doneChan)
				}
			},
		},
	)
	for i := 0; i < 3; i++ {
		mockConversation := ouroboros_mock.ConversationKeepAlive
		if i == expectedConnId {
			mockConversation = ouroboros_mock.ConversationKeepAliveClose
		}
		mockConn := ouroboros_mock.NewConnection(
			ouroboros_mock.ProtocolRoleClient,
			mockConversation,
		)
		oConn, err := ouroboros.New(
			ouroboros.WithConnection(mockConn),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithKeepAlive(true),
			ouroboros.WithKeepAliveConfig(
				keepalive.NewConfig(
					keepalive.WithCookie(ouroboros_mock.MockKeepAliveCookie),
					keepalive.WithPeriod(2*time.Second),
					keepalive.WithTimeout(1*time.Second),
				),
			),
		)
		if err != nil {
			t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
		}
		connManager.AddConnection(i, oConn)
	}
	select {
	case <-doneChan:
		// Shutdown other connections
		for _, tmpConn := range connManager.GetConnectionsByTags() {
			if tmpConn.Id != expectedConnId {
				tmpConn.Conn.Close()
			}
		}
		// TODO: actually wait for shutdown
		time.Sleep(5 * time.Second)
		return
	case <-time.After(10 * time.Second):
		t.Fatalf("did not receive error within timeout")
	}
}

func TestConnectionManagerConnClosed(t *testing.T) {
	defer goleak.VerifyNone(t)
	expectedConnId := 42
	doneChan := make(chan any)
	connManager := ouroboros.NewConnectionManager(
		ouroboros.ConnectionManagerConfig{
			ConnClosedFunc: func(connId int, err error) {
				if connId != expectedConnId {
					t.Fatalf("did not receive closed signal from expected connection: got %d, wanted %d", connId, expectedConnId)
				}
				if err != nil {
					t.Fatalf("received unexpected error: %s", err)
				}
				close(doneChan)
			},
		},
	)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(false),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	connManager.AddConnection(expectedConnId, oConn)
	time.AfterFunc(
		1*time.Second,
		func() {
			oConn.Close()
		},
	)
	select {
	case <-doneChan:
		// TODO: actually wait for shutdown
		time.Sleep(5 * time.Second)
		return
	case <-time.After(10 * time.Second):
		t.Fatalf("did not receive error within timeout")
	}
}
