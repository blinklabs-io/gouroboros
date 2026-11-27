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

package leiosvotes

import (
	"context"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncOutcomeFollowsLifecycle checks what Sync returns in each
// lifecycle state: a running client starts its loop; one whose Stop has
// run, or whose connection has begun to close so that RunLoop starts
// nothing, reports the shutdown and is not left marked as syncing. In the
// last case Sync must not return nil with no loop running.
func TestSyncOutcomeFollowsLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		before  func(*Client, *muxer.Muxer)
		wantErr error
		running bool
	}{
		{
			name:    "running",
			before:  func(*Client, *muxer.Muxer) {},
			running: true,
		},
		{
			name: "after Stop",
			before: func(c *Client, _ *muxer.Muxer) {
				require.NoError(t, c.Stop(context.Background()))
			},
			wantErr: protocol.ErrProtocolShuttingDown,
		},
		{
			name: "loops closed",
			before: func(_ *Client, m *muxer.Muxer) {
				m.CloseProtocolLoops()
			},
			wantErr: protocol.ErrProtocolShuttingDown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, m := newSyncTestClient(t)
			tc.before(client, m)
			err := client.Sync()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			client.busyMutex.Lock()
			running := client.syncRunning
			client.busyMutex.Unlock()
			assert.Equal(t, tc.running, running, "syncRunning after Sync")
		})
	}
}

func newSyncTestClient(t *testing.T) (*Client, *muxer.Muxer) {
	t.Helper()
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		_ = connA.Close()
		_ = connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	cfg := NewConfig(
		WithPipelineLimit(1),
		WithRequestNextCount(1),
		WithVoteFunc(func(CallbackContext, Vote) error { return nil }),
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        m,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	t.Cleanup(func() {
		signalOnly, cancel := context.WithCancel(context.Background())
		cancel()
		_ = client.Stop(signalOnly)
	})
	go func() {
		// Keep the send loop's writes from blocking on the pipe.
		buf := make([]byte, 1024)
		for {
			if _, err := connB.Read(buf); err != nil {
				return
			}
		}
	}()
	return client, m
}
