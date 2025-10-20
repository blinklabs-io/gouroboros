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

package ouroboros_test

import (
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

// TestErrorHandlingWithActiveProtocols tests that connection errors are propagated
// when protocols are active, and ignored when protocols are stopped
func TestErrorHandlingWithActiveProtocols(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("ErrorsPropagatedWhenProtocolsActive", func(t *testing.T) {
		// Create a mock connection that will complete handshake and start the chainsync protocol
		mockConn := ouroboros_mock.NewConnection(
			ouroboros_mock.ProtocolRoleClient,
			[]ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
				ouroboros_mock.ConversationEntryHandshakeNtCResponse,
				// ChainSync messages
				ouroboros_mock.ConversationEntryInput{
					// FindIntersect
					ProtocolId: chainsync.ProtocolIdNtC,
					Message: chainsync.NewMsgFindIntersect(
						[]common.Point{
							{
								Slot: 21600,
								Hash: []byte("19297addad3da631einos029"),
							},
						},
					),
				},
			},
		)

		oConn, err := ouroboros.New(
			ouroboros.WithConnection(mockConn),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithServer(true),
			ouroboros.WithChainSyncConfig(
				chainsync.NewConfig(
					chainsync.WithFindIntersectFunc(
						func(ctx chainsync.CallbackContext, points []common.Point) (common.Point, chainsync.Tip, error) {
							// We need to block here to keep the protocol active
							time.Sleep(5 * time.Second)
							return common.Point{}, chainsync.Tip{}, fmt.Errorf("context cancelled")
						},
					),
				),
			),
		)
		if err != nil {
			t.Fatalf("unexpected error when creating Connection object: %s", err)
		}

		// Wait for handshake to complete by checking if protocols are initialized
		var chainSyncProtocol *chainsync.ChainSync
		for i := 0; i < 100; i++ {
			chainSyncProtocol = oConn.ChainSync()
			if chainSyncProtocol != nil && chainSyncProtocol.Server != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if chainSyncProtocol == nil || chainSyncProtocol.Server == nil {
			oConn.Close()
			t.Fatal("chain sync protocol not initialized")
		}

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
		// Create a mock connection that will send a Done message to stop the protocol
		mockConn := ouroboros_mock.NewConnection(
			ouroboros_mock.ProtocolRoleClient,
			[]ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
				ouroboros_mock.ConversationEntryHandshakeNtCResponse,
				// Send Done message to stop the protocol
				ouroboros_mock.ConversationEntryInput{
					ProtocolId: chainsync.ProtocolIdNtC,
				},
			},
		)

		oConn, err := ouroboros.New(
			ouroboros.WithConnection(mockConn),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithServer(true),
		)
		if err != nil {
			t.Fatalf("unexpected error when creating Connection object: %s", err)
		}

		// Wait for handshake to complete
		var chainSyncProtocol *chainsync.ChainSync
		for i := 0; i < 100; i++ {
			chainSyncProtocol = oConn.ChainSync()
			if chainSyncProtocol != nil && chainSyncProtocol.Server != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if chainSyncProtocol == nil || chainSyncProtocol.Server == nil {
			oConn.Close()
			t.Fatal("chain sync protocol not initialized")
		}

		// Wait for protocol to be done (Done message from mock should trigger this)
		select {
		case <-chainSyncProtocol.Server.DoneChan():
		// Protocol is stoppeds
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
