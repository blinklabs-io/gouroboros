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

package handshake_test

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

func TestServerBasicHandshake(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			// MsgProposeVersions from mock client
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							(10 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
							(11 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
							(12 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
						},
					),
				},
			},
			// MsgAcceptVersion from server
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:      handshake.ProtocolId,
				IsResponse:      true,
				MsgFromCborFunc: handshake.NewMsgFromCbor,
				Message: handshake.NewMsgAcceptVersion(
					(12 + protocol.ProtocolVersionNtCOffset),
					protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
				),
			},
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithServer(true),
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

func TestServerHandshakeRefuseVersionMismatch(t *testing.T) {
	defer func() {
		goleak.VerifyNone(t)
	}()
	expectedErr := fmt.Errorf("handshake failed: refused due to version mismatch")
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			// MsgProposeVersions from mock client
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							(100 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
							(101 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
							(102 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic),
						},
					),
				},
			},
			// MsgRefuse from server
			ouroboros_mock.ConversationEntryInput{
				IsResponse:      true,
				ProtocolId:      handshake.ProtocolId,
				MsgFromCborFunc: handshake.NewMsgFromCbor,
				MessageType:     handshake.MessageTypeRefuse,
				Message: handshake.NewMsgRefuse(
					[]any{
						handshake.RefuseReasonVersionMismatch,
						// Convert []uint16 to []any
						func(in []uint16) []any {
							var ret []any
							for _, item := range in {
								ret = append(ret, uint64(item))
							}
							return ret
						}(protocol.GetProtocolVersionsNtC()),
					},
				),
			},
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithServer(true),
	)
	if err != nil {
		if err.Error() != expectedErr.Error() {
			t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
		}
	} else {
		oConn.Close()
		// Wait for connection shutdown
		select {
		case <-oConn.ErrorChan():
		case <-time.After(10 * time.Second):
			t.Errorf("did not shutdown within timeout")
		}
		t.Fatalf("did not receive expected error")
	}
}
