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

package connection

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectionIdString(t *testing.T) {
	t.Parallel()
	local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3000}
	remote := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3001}
	for _, tc := range []struct {
		name string
		id   ConnectionId
		want string
	}{
		{name: "zero value", want: "<nil><-><nil>"},
		{
			name: "missing local", id: ConnectionId{RemoteAddr: remote},
			want: "<nil><->127.0.0.1:3001",
		},
		{
			name: "missing remote", id: ConnectionId{LocalAddr: local},
			want: "127.0.0.1:3000<-><nil>",
		},
		{
			name: "TCP", id: ConnectionId{local, remote},
			want: "127.0.0.1:3000<->127.0.0.1:3001",
		},
		{
			name: "Unix",
			id: ConnectionId{
				LocalAddr:  &net.UnixAddr{Name: "node.socket", Net: "unix"},
				RemoteAddr: &net.UnixAddr{Net: "unix"},
			},
			want: "node.socket<->",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got string
			require.NotPanics(t, func() { got = tc.id.String() })
			require.Equal(t, tc.want, got)
		})
	}
}
