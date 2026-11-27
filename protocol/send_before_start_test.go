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

package protocol

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// newSendBeforeStartProtocol returns a client protocol on a started muxer
// whose initial state lets the client send message type 1, and the peer end
// of its connection.
func newSendBeforeStartProtocol(t *testing.T) (*Protocol, net.Conn) {
	t.Helper()
	conn, peer := net.Pipe()
	m := muxer.New(conn)
	m.Start()
	t.Cleanup(func() {
		_ = m.Stop(context.Background())
		_ = peer.Close()
	})
	idle := NewState(1, "Idle")
	busy := NewState(2, "Busy")
	p := New(ProtocolConfig{
		Name:       "send-before-start",
		ProtocolId: 1,
		Muxer:      m,
		ErrorChan:  make(chan error, 4),
		Role:       ProtocolRoleClient,
		StateMap: StateMap{
			idle: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType:  1,
					NewState: busy,
				}},
			},
			busy: {Agency: AgencyServer},
		},
		InitialState: idle,
	})
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p, peer
}

// TestSendMessageBeforeStartIsQueued checks that a message sent before Start
// is queued and written once the protocol starts, instead of blocking the
// sender until the protocol stops.
func TestSendMessageBeforeStartIsQueued(t *testing.T) {
	p, peer := newSendBeforeStartProtocol(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, p.SendMessageContext(ctx, &MessageBase{MessageType: 1}))
	p.Start()
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(5*time.Second)))
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(peer, binary.BigEndian, &header))
	require.Equal(t, uint16(1), header.GetProtocolId())
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(peer, payload)
	require.NoError(t, err)
	require.Equal(t, []byte{0x81, 0x01}, payload)
}

// TestSendMessageDuringStartDoesNotRace sends while another goroutine starts
// the protocol; run it with -race.
func TestSendMessageDuringStartDoesNotRace(t *testing.T) {
	p, peer := newSendBeforeStartProtocol(t)
	go func() { _, _ = io.Copy(io.Discard, peer) }()
	started := make(chan struct{})
	go func() {
		p.Start()
		close(started)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.SendMessageContextAndWait(ctx, &MessageBase{MessageType: 1}))
	<-started
}
