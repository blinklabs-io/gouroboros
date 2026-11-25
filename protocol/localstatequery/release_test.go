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

package localstatequery

import (
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestReleaseWithNothingAcquired checks that Release with no state acquired
// returns ErrNotAcquired and sends nothing. MsgRelease is legal only in
// Acquired, so sending it from Idle is a protocol error that tears the
// connection down; after the refusal the client must still be usable.
func TestReleaseWithNothingAcquired(t *testing.T) {
	t.Parallel()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	errs := make(chan error, 8)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	server := NewServer(protocol.ProtocolOptions{
		Muxer: serverMuxer, ConnectionId: connID, ErrorChan: errs,
	}, &Config{
		AcquireFunc: func(CallbackContext, AcquireTarget, bool) error {
			return nil
		},
		QueryFunc: func(CallbackContext, QueryWrapper) (any, error) {
			return []uint64{1, 42}, nil
		},
		ReleaseFunc: func(CallbackContext) error { return nil },
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer: clientMuxer, ConnectionId: connID, ErrorChan: errs,
	}, &Config{AcquireTimeout: time.Minute, QueryTimeout: time.Minute})
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
		server.Stop()
		client.Stop()
		serverMuxer.Stop()
		clientMuxer.Stop()
	})
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()

	require.ErrorIs(t, client.Release(), ErrNotAcquired)
	require.NoError(t, client.AcquireVolatileTip())
	require.NoError(t, client.Release())
	require.ErrorIs(t, client.Release(), ErrNotAcquired)
	// The connection is intact: a query acquires and is answered.
	blockNo, err := client.GetChainBlockNo()
	require.NoError(t, err)
	require.Equal(t, int64(42), blockNo)
	select {
	case err := <-errs:
		t.Fatalf("protocol error: %v", err)
	default:
	}
}
