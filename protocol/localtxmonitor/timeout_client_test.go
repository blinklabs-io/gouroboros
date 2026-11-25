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

package localtxmonitor_test

import (
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

const optInTimeout = 100 * time.Millisecond

// conversationAcquireNoReply is a server that receives the acquire and
// never answers.
var conversationAcquireNoReply = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localtxmonitor.ProtocolId,
		MessageType: localtxmonitor.MessageTypeAcquire,
	},
}

// conversationQueryNoReply acquires, then receives a GetSizes query and
// never answers it.
var conversationQueryNoReply = append(
	append([]ouroboros_mock.ConversationEntry(nil), conversationHandshakeAcquire...),
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localtxmonitor.ProtocolId,
		MessageType: localtxmonitor.MessageTypeGetSizes,
	},
)

// newTimeoutConn connects a client with cfg to a mock server following
// conversation.
func newTimeoutConn(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	cfg localtxmonitor.Config,
) *ouroboros.Connection {
	t.Helper()
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithLocalTxMonitorConfig(cfg),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = oConn.Close() })
	return oConn
}

// requireTimedOut waits for the call to fail and the connection to report
// the timeout, then requires the connection to be torn down: its error
// channel, owned by the connection, closes only once it has shut down.
func requireTimedOut(t *testing.T, oConn *ouroboros.Connection, call func() error) {
	t.Helper()
	start := time.Now()
	callErr := make(chan error, 1)
	go func() { callErr <- call() }()
	select {
	case err := <-callErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return after its timeout")
	}
	require.GreaterOrEqual(t, time.Since(start), optInTimeout)
	select {
	case err, ok := <-oConn.ErrorChan():
		require.True(t, ok, "connection closed without reporting the timeout")
		require.ErrorContains(t, err, "timeout")
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not report the timeout")
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-oConn.ErrorChan():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("connection was not torn down after the timeout")
		}
	}
}

// requireStillWaiting checks that the call is still blocked well past the
// opt-in timeout, then closes the connection to release it.
func requireStillWaiting(t *testing.T, oConn *ouroboros.Connection, call func() error) {
	t.Helper()
	callErr := make(chan error, 1)
	go func() { callErr <- call() }()
	select {
	case err := <-callErr:
		t.Fatalf("call returned with no timeout configured: %v", err)
	case err := <-oConn.ErrorChan():
		t.Fatalf("connection failed with no timeout configured: %v", err)
	case <-time.After(5 * optInTimeout):
	}
	require.NoError(t, oConn.Close())
	select {
	case <-callErr:
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return after the connection closed")
	}
}

// TestAcquireTimeoutTearsDownConnection: with no reference time limit, a
// WithAcquireTimeout is the only bound on Acquiring, where the server has
// agency. Against a server that never answers it must fail the acquire and
// close the connection; without it the client waits.
func TestAcquireTimeoutTearsDownConnection(t *testing.T) {
	acquire := func(oConn *ouroboros.Connection) func() error {
		return func() error {
			return oConn.LocalTxMonitor().Client.Acquire()
		}
	}
	t.Run("opt-in timeout", func(t *testing.T) {
		oConn := newTimeoutConn(t, conversationAcquireNoReply, localtxmonitor.NewConfig(
			localtxmonitor.WithAcquireTimeout(optInTimeout),
		))
		requireTimedOut(t, oConn, acquire(oConn))
	})
	t.Run("default waits", func(t *testing.T) {
		oConn := newTimeoutConn(t, conversationAcquireNoReply, localtxmonitor.NewConfig())
		requireStillWaiting(t, oConn, acquire(oConn))
	})
}

// TestQueryTimeoutTearsDownConnection is the same for WithQueryTimeout and
// the Querying state.
func TestQueryTimeoutTearsDownConnection(t *testing.T) {
	query := func(oConn *ouroboros.Connection) func() error {
		return func() error {
			client := oConn.LocalTxMonitor().Client
			if err := client.Acquire(); err != nil {
				return err
			}
			_, _, _, err := client.GetSizes()
			return err
		}
	}
	t.Run("opt-in timeout", func(t *testing.T) {
		oConn := newTimeoutConn(t, conversationQueryNoReply, localtxmonitor.NewConfig(
			localtxmonitor.WithQueryTimeout(optInTimeout),
		))
		requireTimedOut(t, oConn, query(oConn))
	})
	t.Run("default waits", func(t *testing.T) {
		oConn := newTimeoutConn(t, conversationQueryNoReply, localtxmonitor.NewConfig())
		requireStillWaiting(t, oConn, query(oConn))
	})
}
