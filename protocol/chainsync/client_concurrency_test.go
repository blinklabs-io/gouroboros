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

package chainsync_test

import (
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"

	"go.uber.org/goleak"
)

// TestConcurrentStartStop tests that concurrent Start/Stop operations don't cause deadlocks or races
func TestConcurrentStartStop(t *testing.T) {
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
		ouroboros.WithChainSyncConfig(chainsync.NewConfig()),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("unexpected error when closing Ouroboros object: %s", err)
		}
	}()

	client := oConn.ChainSync().Client
	if client == nil {
		t.Fatalf("ChainSync client is nil")
	}

	// Run concurrent Start/Stop operations
	var wg sync.WaitGroup
	const numGoroutines = 10
	const numOperations = 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// Start the client
				client.Start()

				// Small delay to allow operations to interleave
				time.Sleep(time.Millisecond)

				// Stop the client
				if err := client.Stop(); err != nil {
					t.Errorf(
						"goroutine %d: unexpected error when stopping client: %s",
						id,
						err,
					)
				}
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
	case <-time.After(10 * time.Second):
		t.Fatal(
			"concurrent Start/Stop operations timed out - possible deadlock",
		)
	}
}

// TestStopBeforeStart tests that Stop works correctly when called before Start
func TestStopBeforeStart(t *testing.T) {
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
		ouroboros.WithChainSyncConfig(chainsync.NewConfig()),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("unexpected error when closing Ouroboros object: %s", err)
		}
	}()

	client := oConn.ChainSync().Client
	if client == nil {
		t.Fatalf("ChainSync client is nil")
	}

	// Stop before Start - should not panic or deadlock
	if err := client.Stop(); err != nil {
		t.Errorf("unexpected error when stopping unstarted client: %s", err)
	}

	// Now Start should work normally (but should not actually start due to stopped flag)
	client.Start()

	// Stop again should work
	if err := client.Stop(); err != nil {
		t.Errorf("unexpected error when stopping client: %s", err)
	}
}
