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

package txsubmission_test

import (
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"

	"go.uber.org/goleak"
)

// TestServerConcurrentStop tests that concurrent Stop calls don't cause deadlocks
func TestServerConcurrentStop(t *testing.T) {
	t.Skip("Skipping server test due to mock server issues with NtN protocol")
	defer goleak.VerifyNone(t)

	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
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
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("unexpected error when closing Ouroboros object: %s", err)
		}
	}()

	server := oConn.TxSubmission().Server
	if server == nil {
		t.Fatalf("TxSubmission server is nil")
	}

	// Start the server
	server.Start()

	// Run concurrent Stop operations
	var wg sync.WaitGroup
	const numGoroutines = 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// All Stop calls should succeed (idempotent)
			if err := server.Stop(); err != nil {
				t.Errorf(
					"goroutine %d: unexpected error when stopping server: %s",
					id,
					err,
				)
			}
		}(i)
	}

	// Wait for all goroutines to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Stop operations timed out - possible deadlock")
	}
}

// TestServerStopSetsStoppedFlag tests that calling Stop() properly sets the stopped flag
// TODO: Once mock infrastructure supports triggering handleDone(), add a test that verifies
// Stop() prevents handleDone() from restarting the protocol
func TestServerStopSetsStoppedFlag(t *testing.T) {
	t.Skip("Skipping server test due to mock server issues with NtN protocol")
	defer goleak.VerifyNone(t)

	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
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
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("unexpected error when closing Ouroboros object: %s", err)
		}
	}()

	server := oConn.TxSubmission().Server
	if server == nil {
		t.Fatalf("TxSubmission server is nil")
	}

	// Start the server
	server.Start()

	// Stop the server
	if err := server.Stop(); err != nil {
		t.Errorf("unexpected error when stopping server: %s", err)
	}

	// Verify that the server is marked as stopped
	if !server.IsStopped() {
		t.Error("server should be marked as stopped after calling Stop()")
	}
}
