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
	"reflect"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

const (
	mockProtocolVersionNtC    uint16 = (14 + protocol.ProtocolVersionNtCOffset)
	mockProtocolVersionNtN    uint16 = 13
	mockProtocolVersionNtNV11 uint16 = 11
)

var conversationEntryNtCResponse = ouroboros_mock.ConversationEntryOutput{
	ProtocolId: handshake.ProtocolId,
	IsResponse: true,
	Messages: []protocol.Message{
		handshake.NewMsgAcceptVersion(
			mockProtocolVersionNtC,
			mockNtCVersionData(),
		),
	},
}

var conversationEntryNtNResponse = ouroboros_mock.ConversationEntryOutput{
	ProtocolId: handshake.ProtocolId,
	IsResponse: true,
	Messages: []protocol.Message{
		handshake.NewMsgAcceptVersion(
			mockProtocolVersionNtN,
			mockNtNVersionData(),
		),
	},
}

var conversationEntryNtNResponseV11 = ouroboros_mock.ConversationEntryOutput{
	ProtocolId: handshake.ProtocolId,
	IsResponse: true,
	Messages: []protocol.Message{
		handshake.NewMsgAcceptVersion(
			mockProtocolVersionNtNV11,
			mockNtNVersionDataV11(),
		),
	},
}

func mockNtCVersionData() protocol.VersionData {
	return protocol.VersionDataNtC9to14(ouroboros_mock.MockNetworkMagic)
}

func mockNtNVersionDataV11() protocol.VersionData {
	return protocol.VersionDataNtN11to12{
		CborNetworkMagic:                       ouroboros_mock.MockNetworkMagic,
		CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
		CborPeerSharing:                        protocol.PeerSharingModeNoPeerSharing,
		CborQuery:                              protocol.QueryModeDisabled,
	}
}

func mockNtNVersionData() protocol.VersionData {
	return protocol.VersionDataNtN13andUp{
		VersionDataNtN11to12: mockNtNVersionDataV11().(protocol.VersionDataNtN11to12),
	}
}

func TestClientNtCAccept(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			conversationEntryNtCResponse,
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
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
		panic(fmt.Sprintf("unexpected Ouroboros connection error: %s", err))
	}()
	// Check negotiated version and version data
	protoVersion, protoVersionData := oConn.ProtocolVersion()
	if protoVersion != mockProtocolVersionNtC {
		t.Fatalf("did not get expected protocol version: got %d, wanted %d", protoVersion, mockProtocolVersionNtC)
	}
	if !reflect.DeepEqual(protoVersionData, mockNtCVersionData()) {
		t.Fatalf("did not get expected protocol version data:\n  got:   %#v\n  wanted: %#v", protoVersionData, mockNtCVersionData())
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

func TestClientNtNAccept(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			conversationEntryNtNResponse,
		},
	)
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
		panic(fmt.Sprintf("unexpected Ouroboros connection error: %s", err))
	}()
	// Check negotiated version and version data
	protoVersion, protoVersionData := oConn.ProtocolVersion()
	if protoVersion != mockProtocolVersionNtN {
		t.Fatalf("did not get expected protocol version: got %d, wanted %d", protoVersion, mockProtocolVersionNtN)
	}
	if !reflect.DeepEqual(protoVersionData, mockNtNVersionData()) {
		t.Fatalf("did not get expected protocol version data:\n  got:   %#v\n  wanted: %#v", protoVersionData, mockNtNVersionData())
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

func TestClientNtNAcceptV11(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			conversationEntryNtNResponseV11,
		},
	)
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
		panic(fmt.Sprintf("unexpected Ouroboros connection error: %s", err))
	}()
	// Check negotiated version and version data
	protoVersion, protoVersionData := oConn.ProtocolVersion()
	if protoVersion != mockProtocolVersionNtNV11 {
		t.Fatalf("did not get expected protocol version: got %d, wanted %d", protoVersion, mockProtocolVersionNtNV11)
	}
	if !reflect.DeepEqual(protoVersionData, mockNtNVersionDataV11()) {
		t.Fatalf("did not get expected protocol version data:\n  got:   %#v\n  wanted: %#v", protoVersionData, mockNtNVersionDataV11())
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

func TestClientNtCRefuseVersionMismatch(t *testing.T) {
	defer goleak.VerifyNone(t)
	expectedErr := fmt.Sprintf("%s: version mismatch", handshake.ProtocolName)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					handshake.NewMsgRefuse(
						[]any{
							handshake.RefuseReasonVersionMismatch,
							[]uint16{1, 2, 3},
						},
					),
				},
			},
		},
	)
	_, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err == nil {
		t.Fatalf("did not receive expected error")
	} else {
		if err.Error() != expectedErr {
			t.Fatalf("received unexpected error\n  got:   %v\n  wanted: %v", err, expectedErr)
		}
	}
}

func TestClientNtCRefuseDecodeError(t *testing.T) {
	defer goleak.VerifyNone(t)
	expectedErr := fmt.Sprintf("%s: decode error: foo", handshake.ProtocolName)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					handshake.NewMsgRefuse(
						[]any{
							handshake.RefuseReasonDecodeError,
							mockProtocolVersionNtC,
							"foo",
						},
					),
				},
			},
		},
	)
	_, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err == nil {
		t.Fatalf("did not receive expected error")
	} else {
		if err.Error() != expectedErr {
			t.Fatalf("received unexpected error\n  got:   %v\n  wanted: %v", err, expectedErr)
		}
	}
}

func TestClientNtCRefuseRefused(t *testing.T) {
	defer goleak.VerifyNone(t)
	expectedErr := fmt.Sprintf("%s: refused: foo", handshake.ProtocolName)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					handshake.NewMsgRefuse(
						[]any{
							handshake.RefuseReasonRefused,
							mockProtocolVersionNtC,
							"foo",
						},
					),
				},
			},
		},
	)
	_, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err == nil {
		t.Fatalf("did not receive expected error")
	} else {
		if err.Error() != expectedErr {
			t.Fatalf("received unexpected error\n  got:   %v\n  wanted: %v", err, expectedErr)
		}
	}
}
