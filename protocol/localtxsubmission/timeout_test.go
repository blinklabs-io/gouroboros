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

package localtxsubmission

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfigHasNoBusyTimeout pins the specification's "No timeouts"
// for local-tx-submission. A node holds MsgSubmitTx until its mempool has
// room for the transaction, for as long as the mempool stays full, so a
// default timer would tear down the whole node-to-client connection while
// the node is working as intended. A caller may still opt in with
// WithTimeout.
func TestDefaultConfigHasNoBusyTimeout(t *testing.T) {
	t.Parallel()
	require.Zero(t, NewConfig().Timeout)
	require.Zero(t, StateMap[stateBusy].Timeout)
}

// TestClientBusyTimeout checks the Busy timer that reaches a running client
// through NewClient's copy of the state map: none by default, so a held
// submission does not tear the connection down, and the WithTimeout value
// when the caller opts in.
func TestClientBusyTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		h := newModelHarness(t, modelRoleClient)
		client := NewClient(h.opts, nil)
		client.Start()
		t.Cleanup(client.Protocol.Stop)
		go func() { _ = client.SubmitTx(6, placeholderTx) }()
		h.requireReceive(t, MessageTypeSubmitTx)
		select {
		case err := <-h.errs:
			t.Fatalf("held submission timed out: %v", err)
		case <-time.After(time.Second):
		}
	})
	t.Run("WithTimeout", func(t *testing.T) {
		h := newModelHarness(t, modelRoleClient)
		cfg := NewConfig(WithTimeout(50 * time.Millisecond))
		client := NewClient(h.opts, &cfg)
		client.Start()
		t.Cleanup(client.Protocol.Stop)
		go func() { _ = client.SubmitTx(6, placeholderTx) }()
		h.requireReceive(t, MessageTypeSubmitTx)
		h.requireViolation(
			t,
			"timeout waiting on transition from protocol state Busy",
		)
	})
}

// TestStopDoesNotWaitForSubmit holds a submission, as a node does while its
// mempool is full, and checks that Stop returns at once, that SubmitTx
// returns ErrProtocolShuttingDown, and that no MsgDone follows: the node
// holds agency in Busy. With no default timeout the wait has no bound.
func TestStopDoesNotWaitForSubmit(t *testing.T) {
	h := newModelHarness(t, modelRoleClient)
	client := NewClient(h.opts, nil)
	client.Start()
	t.Cleanup(client.Protocol.Stop)
	submitResult := make(chan error, 1)
	go func() { submitResult <- client.SubmitTx(6, placeholderTx) }()
	h.requireReceive(t, MessageTypeSubmitTx)

	stopResult := make(chan error, 1)
	go func() { stopResult <- client.Stop() }()
	select {
	case err := <-stopResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop waited for the node to answer")
	}
	select {
	case err := <-submitResult:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("SubmitTx did not return after Stop")
	}
	require.NoError(t, client.Stop())
	h.requireQuiet(t)
}
