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

package leiosnotify_test

import (
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"

	"go.uber.org/goleak"
)

// TestStopBeforeStart tests that Stop works correctly when called before Start
func TestStopBeforeStart(t *testing.T) {
	defer goleak.VerifyNone(t)

	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			conversationEntryNtNResponseV15,
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

	client := oConn.LeiosNotify().Client
	if client == nil {
		t.Fatalf("LeiosNotify client is nil")
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
