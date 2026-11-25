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

package ouroboros

import (
	"context"
	"errors"
	"testing"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestNoForwarderStartedAfterShutdown closes the connection after the
// handshake finished and before setup starts the protocol-error forwarder,
// with a protocol error already queued: the handshake protocol's SendError
// can pass its stop check, be stopped by shutdown, and still queue its
// error. Shutdown has then waited for its goroutines and closed errorChan,
// so a forwarder started now is one shutdown never waited for, and
// forwarding the queued error would send on the closed errorChan. Go
// picks at random between ready select cases, so the test repeats.
func TestNoForwarderStartedAfterShutdown(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, ignore)
	for range 64 {
		mockConn := ouroboros_mock.NewConnection(
			ouroboros_mock.ProtocolRoleClient,
			[]ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
				ouroboros_mock.ConversationEntryHandshakeNtCResponse,
			},
		)
		var conn *Connection
		_, err := New(
			WithConnection(mockConn),
			WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			func(c *Connection) {
				conn = c
				c.beforeProtocolErrorForwarderHook = func() {
					c.protoErrorChan <- errors.New("late protocol error")
					require.NoError(t, c.Close(context.Background()))
				}
			},
		)
		require.Error(t, err)
		_ = mockConn.Close()
		require.NoError(t, goleak.Find(ignore))
		require.NotNil(t, conn)
		require.Len(
			t,
			conn.protoErrorChan,
			1,
			"a forwarder started after shutdown took the queued error",
		)
	}
}
