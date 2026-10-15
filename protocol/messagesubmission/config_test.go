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

package messagesubmission

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestConstructorsApplyTTLDefaultToRawConfig(t *testing.T) {
	expired := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{ExpiresAt: 1},
	}
	for _, test := range []struct {
		name string
		cfg  Config
	}{
		{name: "zero value"},
		{
			name: "partial",
			cfg: Config{
				Authenticator: pcommon.NewNoOpAuthenticator(nil),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.cfg
			client := NewClient(protocol.ProtocolOptions{}, &cfg)
			server := NewServer(protocol.ProtocolOptions{}, &cfg)

			require.Nil(t, cfg.TTLValidator)
			require.Zero(t, cfg.MaxUnacknowledgedMessageIDs)
			require.Zero(t, cfg.MaxQueueSize)
			require.NotNil(t, client.config.TTLValidator)
			require.NotNil(t, server.config.TTLValidator)
			require.Equal(t, 100, client.config.MaxQueueSize)
			require.Equal(t, 100, server.config.MaxQueueSize)
			require.Equal(t, InitTimeout, client.config.InitTimeout)
			require.Equal(t, IdleTimeout, client.config.IdleTimeout)
			require.Equal(
				t,
				MessageIdsBlockingTimeout,
				client.config.MessageIdsBlockingTimeout,
			)
			require.Equal(t, MessagesTimeout, client.config.MessagesTimeout)
			require.Equal(t, DoneTimeout, client.config.DoneTimeout)
			require.Equal(
				t,
				DefaultMaxUnacknowledgedMessageIDs,
				client.config.MaxUnacknowledgedMessageIDs,
			)
			require.Error(
				t,
				client.config.TTLValidator.ValidateMessageTTLAt(
					expired,
					time.Unix(2, 0),
				),
			)
			require.ErrorContains(t, server.AddMessage(expired), "expired")
		})
	}
}

func TestConstructorsPreserveExplicitNoOpTTLValidator(t *testing.T) {
	cfg := Config{
		MaxQueueSize:  1,
		Authenticator: pcommon.NewNoOpAuthenticator(nil),
		TTLValidator:  pcommon.NewNoOpTTLValidator(nil),
	}
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	expired := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{ExpiresAt: 1},
	}

	require.NoError(
		t,
		client.config.TTLValidator.ValidateMessageTTLAt(
			expired,
			time.Unix(2, 0),
		),
	)
	require.NoError(t, server.AddMessage(expired))
	require.Equal(t, 1, client.config.MaxQueueSize)
	require.Equal(t, 1, server.config.MaxQueueSize)
}
