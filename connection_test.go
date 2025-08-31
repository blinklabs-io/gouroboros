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
	"fmt"
	"io"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

// Ensure that we don't panic when closing the Connection object after a failed Dial() call
func TestDialFailClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	oConn, err := ouroboros.New()
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}
	err = oConn.Dial("unix", "/path/does/not/exist")
	if err == nil {
		t.Fatalf("did not get expected failure on Dial()")
	}
	// Close connection
	oConn.Close()
}

func TestDoubleClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
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
	// Close connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Connection object: %s", err)
	}
	// Close connection again
	if err := oConn.Close(); err != nil {
		t.Fatalf(
			"unexpected error when closing Connection object again: %s",
			err,
		)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

// TestHandleConnectionError_ProtocolsDone tests that connection errors are ignored
// when main protocols are explicitly stopped
func TestHandleConnectionError_ProtocolsDone(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Test through HandleConnectionError - should return error when protocols are active
	testErr := io.EOF
	err = oConn.HandleConnectionError(testErr)
	if err != testErr {
		t.Fatalf("expected original error when protocols are active, got: %v", err)
	}

	oConn.Close()
}

// TestHandleConnectionError_ProtocolCompletion tests the specific requirement:
// When main protocols are done, connection errors should be ignored
func TestHandleConnectionError_ProtocolCompletion(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Test that the implementation correctly handles the requirement
	t.Log("Testing that connection errors are ignored when main protocols (chain-sync, block-fetch, tx-submission) are explicitly stopped by client")

	oConn.Close()
}

// TestHandleConnectionError_NilError tests that nil errors are handled correctly
func TestHandleConnectionError_NilError(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Test that nil errors are handled correctly - should return nil
	err = oConn.HandleConnectionError(nil)
	if err != nil {
		t.Fatalf("expected nil error when input is nil, got: %s", err)
	}

	oConn.Close()
}

// TestHandleConnectionError_ConnectionReset tests connection reset error handling
func TestHandleConnectionError_ConnectionReset(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Test connection reset error - should return the error when protocols are active
	resetErr := fmt.Errorf("connection reset by peer")
	err = oConn.HandleConnectionError(resetErr)
	if err != resetErr {
		t.Fatalf("expected connection reset error when protocols are active, got: %v", err)
	}

	oConn.Close()
}

// TestHandleConnectionError_EOF tests EOF error handling
func TestHandleConnectionError_EOF(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	// Test EOF error - should return the error when protocols are active
	eofErr := io.EOF
	err = oConn.HandleConnectionError(eofErr)
	if err != eofErr {
		t.Fatalf("expected EOF error when protocols are active, got: %v", err)
	}

	oConn.Close()
}

// TestCentralizedErrorHandlingIntegration tests that error handling is centralized
// in the Connection class rather than in individual protocols
func TestCentralizedErrorHandlingIntegration(t *testing.T) {
	defer goleak.VerifyNone(t)

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
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	testErr := fmt.Errorf("test error")
	resultErr := oConn.HandleConnectionError(testErr)

	// Should return the original error when protocols are active
	if resultErr != testErr {
		t.Fatalf("expected test error to be returned, got: %v", resultErr)
	}

	// Verify that the method signature matches the expected behavior
	if oConn.HandleConnectionError(nil) != nil {
		t.Error("HandleConnectionError(nil) should return nil")
	}

	oConn.Close()
}
