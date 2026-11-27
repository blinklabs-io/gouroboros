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
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestClientResponseTransitions checks that only a response carrying the
// expected cookie clears inFlight and moves to the next cookie; any other
// is refused and changes nothing.
func TestClientResponseTransitions(t *testing.T) {
	tests := []struct {
		name         string
		response     uint16
		wantErr      bool
		wantCookie   uint16
		wantInFlight bool
	}{
		{name: "expected cookie", response: 7, wantCookie: 8, wantInFlight: false},
		{name: "previous cookie", response: 6, wantErr: true, wantCookie: 7, wantInFlight: true},
		{name: "next cookie", response: 8, wantErr: true, wantCookie: 7, wantInFlight: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient(protocol.ProtocolOptions{}, &Config{Period: time.Hour})
			client.cookie = 7
			client.inFlight = true
			err := client.handleKeepAliveResponse(NewMsgKeepAliveResponse(tc.response))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantCookie, client.cookie)
			require.Equal(t, tc.wantInFlight, client.inFlight)
		})
	}
}

// TestSendKeepAliveTransitions checks sendKeepAlive: a ping in flight is
// skipped and the timer re-armed, so at most one ping is in flight; once
// the protocol is stopping, the send fails, inFlight is cleared and no
// timer is armed.
func TestSendKeepAliveTransitions(t *testing.T) {
	t.Run("in flight skips and re-arms", func(t *testing.T) {
		client := NewClient(protocol.ProtocolOptions{}, &Config{Period: time.Hour})
		client.inFlight = true
		client.sendKeepAlive()
		client.timerMutex.Lock()
		timer := client.timer
		client.timerMutex.Unlock()
		require.NotNil(t, timer, "a skipped ping did not re-arm the timer")
		timer.Stop()
		require.True(t, client.inFlight)
	})
	t.Run("stopping refuses and arms nothing", func(t *testing.T) {
		client := NewClient(protocol.ProtocolOptions{}, &Config{Period: time.Hour})
		require.NoError(t, client.Protocol.Stop(context.Background()))
		client.sendKeepAlive()
		client.timerMutex.Lock()
		timer := client.timer
		client.timerMutex.Unlock()
		require.Nil(t, timer, "a stopping client armed a timer")
		require.False(t, client.inFlight, "a refused ping stayed in flight")
	})
}
