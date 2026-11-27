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

package peersharing

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestServerServesOnlyWhenNegotiatedOn sends MsgShareRequest under each
// combination of the disabled flags: when either flag says
// the connection negotiated NoPeerSharing, a MsgShareRequest is answered
// with an empty list and ShareRequestFunc is never asked for peers; only
// when both are clear does the server share peers.
func TestServerServesOnlyWhenNegotiatedOn(t *testing.T) {
	tests := []struct {
		name           string
		localDisabled  bool
		remoteDisabled bool
	}{
		{"on", false, false},
		{"local off", true, true},
		{"remote off", false, true},
		{"local flag only", true, false},
	}
	peer := PeerAddress{IP: net.IPv4(192, 0, 2, 1), Port: 3001}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			cfg := NewConfig(
				WithLocalDisabled(tc.localDisabled),
				WithRemoteDisabled(tc.remoteDisabled),
				WithShareRequestFunc(
					func(CallbackContext, int) ([]PeerAddress, error) {
						calls.Add(1)
						return []PeerAddress{peer}, nil
					},
				),
			)
			connB, _ := testPeerSharingServer(t, &cfg)
			sendPeerSharingRequest(t, connB, 5)
			require.NoError(
				t,
				connB.SetReadDeadline(time.Now().Add(time.Second)),
			)
			segment := readPeerSharingSegment(t, connB)
			msg, err := NewMsgFromCbor(
				MessageTypeSharePeers,
				segment.Payload,
			)
			require.NoError(t, err)
			peers := msg.(*MsgSharePeers).PeerAddresses
			off := tc.localDisabled || tc.remoteDisabled
			if off {
				require.Empty(t, peers)
				require.Zero(t, calls.Load(), "ShareRequestFunc called")
				return
			}
			require.Len(t, peers, 1)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}
