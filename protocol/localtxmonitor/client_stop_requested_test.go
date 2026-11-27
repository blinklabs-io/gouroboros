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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestCallsAfterStopRequestedSendNothing checks every public call made once
// Stop has closed stopRequested, which Stop does before it takes busyMutex:
// each returns ErrProtocolShuttingDown without sending and without marking
// a wait abandoned, so Stop still ends the run with MsgDone. The test
// closes stopRequested itself to hold the client in that window.
func TestCallsAfterStopRequestedSendNothing(t *testing.T) {
	calls := []struct {
		name string
		call func(*Client) error
	}{
		{"Acquire", func(c *Client) error { return c.Acquire() }},
		{"Release", func(c *Client) error { return c.Release() }},
	}
	for _, query := range testQueries() {
		calls = append(calls, struct {
			name string
			call func(*Client) error
		}{query.name, query.call})
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			client, conn, errorChan := startIdleTestClient(t)
			close(client.stopRequested)
			result := make(chan error, 1)
			go func() { result <- tc.call(client) }()
			select {
			case err := <-result:
				require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
			case <-time.After(time.Second):
				t.Fatal("call did not return after Stop was requested")
			}
			requireNothingSent(t, conn, errorChan)
			client.busyMutex.Lock()
			abandoned := client.abandoned
			client.busyMutex.Unlock()
			require.False(t, abandoned, "call marked a wait abandoned")
		})
	}
}
