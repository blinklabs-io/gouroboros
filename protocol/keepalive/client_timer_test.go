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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
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

// newPingCountingPair connects a client with cfg to a keep-alive server
// over two muxers and returns the client and a channel that receives the
// cookie of every ping the server reads.
func newPingCountingPair(t *testing.T, cfg *Config) (*Client, <-chan uint16) {
	t.Helper()
	connA, connB := net.Pipe()
	clientMuxer := muxer.New(connA)
	serverMuxer := muxer.New(connB)
	pings := make(chan uint16, 1024)
	connId := connection.ConnectionId{
		LocalAddr:  connA.LocalAddr(),
		RemoteAddr: connA.RemoteAddr(),
	}
	serverCfg := NewConfig(WithOnKeepAliveReceived(
		func(_ connection.ConnectionId, cookie uint16) {
			select {
			case pings <- cookie:
			default:
			}
		},
	))
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        serverMuxer,
		Mode:         protocol.ProtocolModeNodeToNode,
	}, &serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        clientMuxer,
		Mode:         protocol.ProtocolModeNodeToNode,
	}, cfg)
	server.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(ctx)
		_ = server.Stop(ctx)
		_ = clientMuxer.Stop(ctx)
		_ = serverMuxer.Stop(ctx)
		_ = connA.Close()
		_ = connB.Close()
	})
	client.Start()
	return client, pings
}

// TestZeroPeriodDoesNotFloodPeer checks that a Config without a Period does
// not re-arm the timer at once after every answer, which would ping the
// server as fast as it can answer.
func TestZeroPeriodDoesNotFloodPeer(t *testing.T) {
	_, pings := newPingCountingPair(t, &Config{})
	select {
	case <-pings:
	case <-time.After(5 * time.Second):
		t.Fatal("the client did not send its first ping")
	}
	select {
	case cookie := <-pings:
		t.Fatalf("a second ping (cookie %d) followed at once: the zero Period was used", cookie)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestZeroTimeoutTimesTheAnswer checks that a Config without a Timeout still
// times the server's answer. Without that, a silent server is never noticed
// and the ping left in flight stops every later one.
func TestZeroTimeoutTimesTheAnswer(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "zero", timeout: 0, want: DefaultKeepAliveTimeout * time.Second},
		{name: "negative", timeout: -time.Second, want: DefaultKeepAliveTimeout * time.Second},
		{name: "configured", timeout: 3 * time.Second, want: 3 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := clientStateMap(tc.timeout)[StateServer]
			require.Equal(t, tc.want, entry.Timeout)
			require.Equal(t, ClientTimeout, clientStateMap(tc.timeout)[StateClient].Timeout)
		})
	}
}

// TestDefaultPeriodBelowServerLimit checks that the default ping period is
// below the server's limit on the next ping, so a server never hangs up on
// a client that keeps answering.
func TestDefaultPeriodBelowServerLimit(t *testing.T) {
	require.Less(t, DefaultKeepAlivePeriod*time.Second, ClientTimeout)
	require.Less(t, NewClient(protocol.ProtocolOptions{}, &Config{}).period, ClientTimeout)
}
