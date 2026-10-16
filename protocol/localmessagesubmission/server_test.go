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

package localmessagesubmission

import (
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestServerRejectsExpiredMessageWithExpiredReason(t *testing.T) {
	for _, test := range []struct {
		name      string
		expiresAt uint32
		wantType  uint8
	}{
		{
			name:      "expired",
			expiresAt: 1,
			wantType:  2,
		},
		{
			name:      "too far in future remains invalid",
			expiresAt: uint32(time.Now().Add(time.Hour).Unix()),
			wantType:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			clientMuxer := muxer.New(clientConn)
			serverMuxer := muxer.New(serverConn)
			clientErrors := make(chan error, 1)
			serverErrors := make(chan error, 1)
			rejectReasons := make(chan pcommon.RejectReason, 1)
			clientCfg := NewConfig(
				WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
				WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
				WithRejectMessageFunc(func(
					_ CallbackContext,
					reason pcommon.RejectReason,
				) {
					rejectReasons <- reason
				}),
			)
			serverCfg := NewConfig(
				WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
				WithSubmitMessageFunc(func(
					CallbackContext,
					*pcommon.DmqMessage,
				) pcommon.RejectReason {
					return nil
				}),
			)
			client := NewClient(protocol.ProtocolOptions{
				Muxer:     clientMuxer,
				ErrorChan: clientErrors,
			}, &clientCfg)
			server := NewServer(protocol.ProtocolOptions{
				Muxer:     serverMuxer,
				ErrorChan: serverErrors,
			}, &serverCfg)
			client.Protocol.EnsureRegistered()
			server.Protocol.EnsureRegistered()
			clientMuxer.Start()
			serverMuxer.Start()
			client.Start()
			server.Start()
			t.Cleanup(func() {
				_ = client.Stop()
				server.Protocol.Stop()
				clientMuxer.Stop()
				serverMuxer.Stop()
				_ = clientConn.Close()
				_ = serverConn.Close()
			})

			require.NoError(t, client.SubmitMessage(&pcommon.DmqMessage{
				Payload: pcommon.DmqMessagePayload{ExpiresAt: test.expiresAt},
			}))
			select {
			case reason := <-rejectReasons:
				require.Equal(t, test.wantType, reason.RejectReasonType())
			case <-time.After(5 * time.Second):
				require.FailNow(t, "timed out waiting for rejection reason")
			}
		})
	}
}
