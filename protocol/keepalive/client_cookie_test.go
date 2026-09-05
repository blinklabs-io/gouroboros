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

package keepalive

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestClientAdvancesCookieAfterValidResponse(t *testing.T) {
	cfg := NewConfig(WithCookie(0xffff))
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{LocalAddr: local.LocalAddr(), RemoteAddr: local.RemoteAddr()},
	}, &cfg)

	require.NoError(t, client.handleKeepAliveResponse(NewMsgKeepAliveResponse(0xffff)))
	client.cookieMutex.Lock()
	require.Zero(t, client.cookie)
	client.cookieMutex.Unlock()
}

func TestClientDoesNotAdvanceCookieAfterInvalidResponse(t *testing.T) {
	cfg := NewConfig(WithCookie(7))
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{LocalAddr: local.LocalAddr(), RemoteAddr: local.RemoteAddr()},
	}, &cfg)

	require.Error(t, client.handleKeepAliveResponse(NewMsgKeepAliveResponse(8)))
	client.cookieMutex.Lock()
	require.Equal(t, uint16(7), client.cookie)
	client.cookieMutex.Unlock()
}
