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

package localtxmonitor

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestStopDoesNotWaitForPeer starts a call that waits for the node, which
// withholds its answer, and checks that Stop returns at once, that the call
// returns ErrProtocolShuttingDown, and that no message follows: the node
// holds agency, so the client may not send MsgDone. A re-acquire waits for a
// mempool change, which may never come, and a query waits up to
// QueryTimeout.
func TestStopDoesNotWaitForPeer(t *testing.T) {
	type waitingCall struct {
		name    string
		msgType uint8
		call    func(*Client) error
	}
	calls := []waitingCall{{
		name:    "AwaitAcquire",
		msgType: MessageTypeAcquire,
		call:    func(c *Client) error { return c.Acquire() },
	}}
	for _, query := range testQueries() {
		calls = append(calls, waitingCall{
			name:    query.name,
			msgType: query.queryType,
			call:    query.call,
		})
	}
	for _, wc := range calls {
		t.Run(wc.name, func(t *testing.T) {
			client, conn, errorChan := startAcquiredTestClient(t)
			callResult := make(chan error, 1)
			go func() { callResult <- wc.call(client) }()
			requireQueryTestMessageType(t, conn, wc.msgType)

			stopResult := make(chan error, 1)
			go func() { stopResult <- client.Stop(context.Background()) }()
			select {
			case err := <-stopResult:
				require.NoError(t, err)
			case err := <-errorChan:
				t.Fatalf("protocol error: %v", err)
			case <-time.After(time.Second):
				t.Fatal("Stop waited for the node to answer")
			}
			select {
			case err := <-callResult:
				require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
			case <-time.After(time.Second):
				t.Fatal("waiting call did not return after Stop")
			}
			require.NoError(t, client.Stop(context.Background()))

			require.NoError(
				t,
				conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)),
			)
			_, err := conn.Read(make([]byte, 1))
			var netErr net.Error
			require.True(
				t,
				errors.Is(err, os.ErrDeadlineExceeded) ||
					(errors.As(err, &netErr) && netErr.Timeout()),
				"client sent a message after Stop: %v", err,
			)
		})
	}
}
