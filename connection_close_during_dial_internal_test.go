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
	"net"
	"sync"
	"testing"
	"time"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type closeRecordingConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *closeRecordingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestCloseBeforeMuxerCreatedClosesConn closes the connection after setup
// has published doneChan and before it creates the muxer, as a Close from
// another goroutine during Dial can. Shutdown then finds no muxer to stop,
// so setup must not leave one running on the connection it was given.
func TestCloseBeforeMuxerCreatedClosesConn(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	transport := &closeRecordingConn{
		Conn:   mockConn,
		closed: make(chan struct{}),
	}
	defer func() { _ = mockConn.Close() }()
	_, err := New(
		WithConnection(transport),
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		func(c *Connection) {
			c.beforeMuxerHook = func() {
				require.NoError(t, c.Close(context.Background()))
			}
		},
	)
	require.Error(t, err)
	select {
	case <-transport.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("connection closed during setup left its conn open")
	}
}
