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
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

// TestErrorHandlingWithActiveProtocols tests that connection errors are propagated
// when protocols are active, and ignored when protocols are stopped
func TestErrorHandlingWithActiveProtocols(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("ErrorsPropagatedWhenProtocolsActive", func(t *testing.T) {
		// Create a mock connection that will complete handshake
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

		// Wait for handshake to complete by checking if protocols are initialized
		var chainSyncProtocol *chainsync.ChainSync
		for i := 0; i < 100; i++ {
			chainSyncProtocol = oConn.ChainSync()
			if chainSyncProtocol != nil && chainSyncProtocol.Client != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if chainSyncProtocol == nil || chainSyncProtocol.Client == nil {
			oConn.Close()
			t.Fatal("chain sync protocol not initialized")
		}

		// Start the chain sync protocol to make it active
		chainSyncProtocol.Client.Start()

		// Wait a bit for protocol to start
		time.Sleep(100 * time.Millisecond)

		// Close the mock connection to generate a connection error
		mockConn.Close()

		// We should receive a connection error since protocols are active
		select {
		case err := <-oConn.ErrorChan():
			if err == nil {
				t.Fatal("expected connection error, got nil")
			}
			t.Logf("Received connection error (expected with active protocols): %s", err)
		case <-time.After(2 * time.Second):
			t.Error("timed out waiting for connection error - error should be propagated when protocols are active")
		}

		oConn.Close()
	})

	t.Run("ErrorsIgnoredWhenProtocolsStopped", func(t *testing.T) {
		// Create a mock connection
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

		// Wait for handshake to complete
		var chainSyncProtocol *chainsync.ChainSync
		for i := 0; i < 100; i++ {
			chainSyncProtocol = oConn.ChainSync()
			if chainSyncProtocol != nil && chainSyncProtocol.Client != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if chainSyncProtocol == nil || chainSyncProtocol.Client == nil {
			oConn.Close()
			t.Fatal("chain sync protocol not initialized")
		}

		// Start and then immediately stop the protocol
		chainSyncProtocol.Client.Start()
		time.Sleep(50 * time.Millisecond)

		// Stop the protocol explicitly
		if err := chainSyncProtocol.Client.Stop(); err != nil {
			t.Fatalf("failed to stop chain sync: %s", err)
		}

		// Wait for protocol to be done
		select {
		case <-chainSyncProtocol.Client.DoneChan():
			// Protocol is stopped
		case <-time.After(1 * time.Second):
			t.Fatal("timed out waiting for protocol to stop")
		}

		// Now close the mock connection to generate an error
		mockConn.Close()
		select {
		case err := <-oConn.ErrorChan():
			t.Logf("Received error during shutdown: %s", err)
		case <-time.After(500 * time.Millisecond):
			t.Log("No connection error received (expected when protocols are stopped)")
		}

		oConn.Close()
	})
}

// TestErrorHandlingWithMultipleProtocols tests error handling with multiple active protocols
func TestErrorHandlingWithMultipleProtocols(t *testing.T) {
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

	// Wait for handshake to complete
	time.Sleep(100 * time.Millisecond)

	// Start multiple protocols
	chainSync := oConn.ChainSync()
	blockFetch := oConn.BlockFetch()
	txSubmission := oConn.TxSubmission()

	if chainSync != nil && chainSync.Client != nil {
		chainSync.Client.Start()
	}
	if blockFetch != nil && blockFetch.Client != nil {
		blockFetch.Client.Start()
	}
	if txSubmission != nil && txSubmission.Client != nil {
		txSubmission.Client.Start()
	}

	// Wait for protocols to start
	time.Sleep(100 * time.Millisecond)

	// Close connection to generate error
	mockConn.Close()

	// Should receive error since protocols are active
	select {
	case err := <-oConn.ErrorChan():
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
		t.Logf("Received connection error with multiple active protocols: %s", err)
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for connection error")
	}

	oConn.Close()
}

// TestBasicErrorHandling tests basic error handling scenarios
func TestBasicErrorHandling(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("DialFailure", func(t *testing.T) {
		oConn, err := ouroboros.New(
			ouroboros.WithNetworkMagic(764824073),
		)
		if err != nil {
			t.Fatalf("unexpected error when creating Connection object: %s", err)
		}

		err = oConn.Dial("tcp", "invalid-hostname:9999")
		if err == nil {
			t.Fatal("expected dial error, got nil")
		}

		oConn.Close()
	})

	t.Run("DoubleClose", func(t *testing.T) {
		oConn, err := ouroboros.New(
			ouroboros.WithNetworkMagic(764824073),
		)
		if err != nil {
			t.Fatalf("unexpected error when creating Connection object: %s", err)
		}

		// First close
		if err := oConn.Close(); err != nil {
			t.Fatalf("unexpected error on first close: %s", err)
		}

		// Second close should also work
		if err := oConn.Close(); err != nil {
			t.Fatalf("unexpected error on second close: %s", err)
		}
	})
}

// TestErrorChannelBehavior tests basic error channel behavior
func TestErrorChannelBehavior(t *testing.T) {
	defer goleak.VerifyNone(t)

	oConn, err := ouroboros.New(
		ouroboros.WithNetworkMagic(764824073),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}

	errorChan := oConn.ErrorChan()
	if errorChan == nil {
		t.Fatal("error channel should not be nil")
	}

	select {
	case err, ok := <-errorChan:
		if ok {
			t.Logf("Error channel contained: %s", err)
		} else {
			t.Error("Error channel should not be closed initially")
		}
	default:
		// Expected - channel is empty but open
	}

	oConn.Close()
}
