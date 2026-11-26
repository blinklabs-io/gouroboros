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

package leiosnotify

import (
	"context"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncReportsNotificationLoopThatCannotStart closes the connection's
// protocol loops, as its shutdown does, after the client has started but
// before Sync. The notification loop cannot start, so Sync must report the
// shutdown instead of a running loop that does not exist, and must not leave
// the client refusing later calls as busy.
func TestSyncReportsNotificationLoopThatCannotStart(t *testing.T) {
	cfg := NewConfig(
		WithNotificationFunc(func(CallbackContext, protocol.Message) error {
			return nil
		}),
	)
	conn, peer := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = peer.Close()
	})
	m := muxer.New(conn)
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		Muxer: m,
	}, &cfg)
	client.Start()
	m.Start()
	t.Cleanup(func() {
		signalOnly, cancel := context.WithCancel(context.Background())
		cancel()
		_ = client.Protocol.Stop(signalOnly)
	})
	go func() {
		// Keep the send loop's writes from blocking on the pipe.
		buf := make([]byte, 1024)
		for {
			if _, err := peer.Read(buf); err != nil {
				return
			}
		}
	}()
	m.CloseProtocolLoops()

	require.ErrorIs(t, client.Sync(), protocol.ErrProtocolShuttingDown)
	client.busyMutex.Lock()
	running := client.notificationRunning
	client.busyMutex.Unlock()
	assert.False(t, running, "Sync left the client marked as syncing")
}
