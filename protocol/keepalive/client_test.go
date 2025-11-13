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

package keepalive_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

const (
	// MockKeepAliveWrongCookie is the wrong cookie for a keep-alive response
	MockKeepAliveWrongCookie     uint16 = 0x3e8
	MockKeepAliveDifferentCookie uint16 = 0xADA
)

// ConversationKeepAlive is a pre-defined conversation with a NtN handshake and repeated keep-alive requests
// and responses
var ConversationKeepAliveWrongResponse = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtNResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:      keepalive.ProtocolId,
		Message:         keepalive.NewMsgKeepAlive(MockKeepAliveWrongCookie),
		MsgFromCborFunc: keepalive.NewMsgFromCbor,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: keepalive.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			keepalive.NewMsgKeepAliveResponse(
				MockKeepAliveWrongCookie,
			), // Incorrect Cookie value
		},
	},
}

var ConversationKeepAliveDifferentCookie = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtNResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId: keepalive.ProtocolId,
		Message: keepalive.NewMsgKeepAlive(
			MockKeepAliveDifferentCookie,
		),
		MsgFromCborFunc: keepalive.NewMsgFromCbor,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: keepalive.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			keepalive.NewMsgKeepAliveResponse(MockKeepAliveDifferentCookie),
		},
	},
}

func TestServerKeepaliveHandling(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		ouroboros_mock.ConversationKeepAlive,
	).(*ouroboros_mock.Connection)

	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error\n  got:   %v\n  wanted: no error", err)
		}
		close(asyncErrChan)
	}()

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithKeepAliveConfig(keepalive.NewConfig(
			keepalive.WithPeriod(time.Millisecond*100),
			// Set correct cookie
			keepalive.WithCookie(ouroboros_mock.MockKeepAliveCookie),
		)),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}

	// Close connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Connection object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestServerKeepaliveHandlingWithWrongResponse(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		ConversationKeepAliveWrongResponse,
	).(*ouroboros_mock.Connection)

	// Expected cookie is 0x3e8 instead of 0x3e7 based on the mock connection
	expectedErr := "input error: parsed message does not match expected value: " +
		"got &keepalive.MsgKeepAlive{MessageBase:protocol.MessageBase{_:struct {}{}, rawCbor:[]uint8(nil), MessageType:0x0}, Cookie:0x3e7}, " +
		"expected &keepalive.MsgKeepAlive{MessageBase:protocol.MessageBase{_:struct {}{}, rawCbor:[]uint8(nil), MessageType:0x0}, Cookie:0x3e8}"
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.ErrorChan()
		if err == nil {
			asyncErrChan <- errors.New("did not receive expected error")
		} else {
			if err.Error() != expectedErr {
				asyncErrChan <- fmt.Errorf("did not receive expected error\n  got:    %v\n  wanted: %s", err, expectedErr)
			}
		}
		close(asyncErrChan)
	}()

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithKeepAliveConfig(keepalive.NewConfig(
			keepalive.WithPeriod(time.Millisecond*10),
			keepalive.WithCookie(ouroboros_mock.MockKeepAliveCookie),
		)),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Connection object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestServerKeepaliveHandlingWithDifferentCookie(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		ConversationKeepAliveDifferentCookie,
	).(*ouroboros_mock.Connection)

	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error\n  got:   %v\n  wanted: no error", err)
		}
		close(asyncErrChan)
	}()

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithKeepAliveConfig(keepalive.NewConfig(
			keepalive.WithPeriod(time.Millisecond*10),
			keepalive.WithCookie(MockKeepAliveDifferentCookie),
		)),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Connection object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
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

func TestClientShutdown(t *testing.T) {
	runTest(
		t,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		},
		func(t *testing.T, oConn *ouroboros.Connection) {
			if oConn.KeepAlive() == nil {
				t.Fatalf("KeepAlive client is nil")
			}
			// Start the client
			oConn.KeepAlive().Client.Start()
			// Stop the client
			if err := oConn.KeepAlive().Client.Stop(); err != nil {
				t.Fatalf("unexpected error when stopping client: %s", err)
			}
		},
	)
}
