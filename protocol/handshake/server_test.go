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

package handshake_test

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

func TestServerInitialProposeTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, test := range []struct {
		name        string
		mode        protocol.ProtocolMode
		timeout     time.Duration
		wantTimeout bool
	}{
		{"node-to-node", protocol.ProtocolModeNodeToNode, 20 * time.Millisecond, true},
		{"node-to-client", protocol.ProtocolModeNodeToClient, 20 * time.Millisecond, false},
		{"node-to-node zero timeout", protocol.ProtocolModeNodeToNode, 0, false},
		{"node-to-node negative timeout", protocol.ProtocolModeNodeToNode, -time.Second, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverConn, peerConn := net.Pipe()
			defer serverConn.Close()
			defer peerConn.Close()
			m := muxer.New(serverConn)
			m.Start()
			defer m.Stop()
			errorChan := make(chan error, 1)
			cfg := handshake.NewConfig(
				handshake.WithTimeout(test.timeout),
			)
			s := handshake.NewServer(protocol.ProtocolOptions{
				Muxer:     m,
				ErrorChan: errorChan,
				Mode:      test.mode,
				Role:      protocol.ProtocolRoleServer,
			}, &cfg)
			s.Start()
			defer s.Stop()

			if test.wantTimeout {
				select {
				case err := <-errorChan:
					if err == nil {
						t.Fatal("received nil timeout error")
					}
					if !strings.Contains(err.Error(), "timeout waiting on transition") {
						t.Fatalf("unexpected timeout error: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("server did not enforce initial Propose timeout")
				}
				return
			}
			select {
			case err := <-errorChan:
				t.Fatalf("unexpected initial timeout: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

func TestServerBasicN2NHandshake(t *testing.T) {
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
							10: protocol.VersionDataNtN7to10{
								CborNetworkMagic:                       ouroboros_mock.MockNetworkMagic,
								CborInitiatorAndResponderDiffusionMode: true,
							},
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
					10,
					protocol.VersionDataNtN7to10{
						CborNetworkMagic:                       ouroboros_mock.MockNetworkMagic,
						CborInitiatorAndResponderDiffusionMode: true,
					},
				),
			},
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
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

func TestServerBasicHandshake(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							(10 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
							(11 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
							(12 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
						},
					),
				},
			},
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
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
	}()
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Ouroboros object: %s", err)
	}
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
	expectedErr := errors.New(
		"handshake failed: refused due to version mismatch",
	)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			// MsgProposeVersions from mock client
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							(100 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
							(101 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
							(102 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic,
							),
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

func TestServerHandshakeRefuseNetworkMagicMismatchSanitized(t *testing.T) {
	defer goleak.VerifyNone(t)
	testVersion := uint16(12 + protocol.ProtocolVersionNtCOffset)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			// MsgProposeVersions from mock client with wrong network magic
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							testVersion: protocol.VersionDataNtC9to14(
								ouroboros_mock.MockNetworkMagic + 1,
							),
						},
					),
				},
			},
			// MsgRefuse from server with sanitized reason string
			ouroboros_mock.ConversationEntryInput{
				IsResponse:      true,
				ProtocolId:      handshake.ProtocolId,
				MsgFromCborFunc: handshake.NewMsgFromCbor,
				MessageType:     handshake.MessageTypeRefuse,
				Message: handshake.NewMsgRefuse(
					[]any{
						handshake.RefuseReasonRefused,
						uint64(testVersion),
						"network magic mismatch",
					},
				),
			},
		},
	)
	_, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithServer(true),
	)
	if err == nil {
		t.Fatal("did not receive expected error")
	}
	if !strings.Contains(err.Error(), "network magic mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
	if strings.Contains(err.Error(), "VersionData") {
		t.Fatalf("error leaked version struct details: %v", err)
	}
}

func TestServerQueryMode(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				Messages: []protocol.Message{
					handshake.NewMsgProposeVersions(
						protocol.ProtocolVersionMap{
							(15 + protocol.ProtocolVersionNtCOffset): protocol.VersionDataNtC15andUp{
								CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
								CborQuery:        true,
							},
						},
					),
				},
			},
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:      handshake.ProtocolId,
				IsResponse:      true,
				MsgFromCborFunc: handshake.NewMsgFromCbor,
				MessageType:     handshake.MessageTypeQueryReply,
				Message: handshake.NewMsgQueryReply(
					protocol.GetProtocolVersionMap(
						protocol.ProtocolModeNodeToClient,
						ouroboros_mock.MockNetworkMagic,
						protocol.DiffusionModeInitiatorOnly,
						false,
						false,
					),
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
		if !strings.Contains(err.Error(), "handshake query mode") {
			t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
		}
	} else {
		oConn.Close()
		select {
		case <-oConn.ErrorChan():
		case <-time.After(10 * time.Second):
			t.Errorf("did not shutdown within timeout")
		}
	}
}
