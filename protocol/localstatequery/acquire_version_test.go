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

// newVersionedPair connects a client and a server that negotiated the given
// node-to-client version. targets receives each target the server is asked
// to acquire.
func newVersionedPair(
	t *testing.T,
	ntcVersion uint16,
) (*Client, chan AcquireTarget, chan error) {
	t.Helper()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	errs := make(chan error, 8)
	targets := make(chan AcquireTarget, 8)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	version := ntcVersion + protocol.ProtocolVersionNtCOffset
	server := NewServer(protocol.ProtocolOptions{
		Muxer: serverMuxer, ConnectionId: connID, ErrorChan: errs,
		Mode: protocol.ProtocolModeNodeToClient, Version: version,
	}, &Config{
		AcquireFunc: func(_ CallbackContext, target AcquireTarget, _ bool) error {
			targets <- target
			return nil
		},
		QueryFunc: func(CallbackContext, QueryWrapper) (any, error) {
			return []uint64{1, 42}, nil
		},
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer: clientMuxer, ConnectionId: connID, ErrorChan: errs,
		Mode: protocol.ProtocolModeNodeToClient, Version: version,
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
	return client, targets, errs
}

// TestAcquireImmutableTipNeedsVersion16 checks that the client acquires or
// re-acquires the immutable tip only on node-to-client version 16 or later,
// which introduced it. The reference codec refuses to encode it on an older
// version, and an older node cannot decode it, so sending it would tear the
// connection down. Refused, nothing is sent and the client keeps its state.
func TestAcquireImmutableTipNeedsVersion16(t *testing.T) {
	t.Parallel()
	t.Run("v15 refused", func(t *testing.T) {
		t.Parallel()
		client, targets, errs := newVersionedPair(t, 15)
		require.ErrorIs(
			t,
			client.AcquireImmutableTip(),
			ErrAcquireImmutableTipUnsupportedVersion,
		)
		require.NoError(t, client.AcquireVolatileTip())
		require.Equal(t, AcquireVolatileTip{}, <-targets)
		require.ErrorIs(
			t,
			client.AcquireImmutableTip(),
			ErrAcquireImmutableTipUnsupportedVersion,
		)
		// Still acquired: the query is answered without a new acquire.
		blockNo, err := client.GetChainBlockNo()
		require.NoError(t, err)
		require.Equal(t, int64(42), blockNo)
		require.Empty(t, targets)
		select {
		case err := <-errs:
			t.Fatalf("protocol error: %v", err)
		default:
		}
	})
	t.Run("v16 allowed", func(t *testing.T) {
		t.Parallel()
		client, targets, _ := newVersionedPair(t, 16)
		require.NoError(t, client.AcquireImmutableTip())
		require.Equal(t, AcquireImmutableTip{}, <-targets)
		require.NoError(t, client.AcquireImmutableTip())
		require.Equal(t, AcquireImmutableTip{}, <-targets)
	})
}
