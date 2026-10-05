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
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// TestReceiveLoopDefersDeclaredPeerPipelinedMessagesUntilPeerAgency checks
// that a peer-pipelined message admitted while this role holds agency is not
// handled until the peer holds agency again, and that it then applies its own
// transition. Handling it early would run its callback while the previous
// request is still unanswered.
func TestReceiveLoopDefersDeclaredPeerPipelinedMessagesUntilPeerAgency(
	t *testing.T,
) {
	const (
		requestType = uint8(0)
		replyType   = uint8(1)
	)
	stateIdle := NewState(1, "Idle")
	stateBusy := NewState(2, "Busy")
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
	}()
	t.Cleanup(func() {
		m.Stop()
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	errorChan := make(chan error, 1)
	handled := make(chan uint8, 1)
	p := New(ProtocolConfig{
		Name:       "pipelined-ingress",
		ProtocolId: 1,
		ErrorChan:  errorChan,
		Muxer:      m,
		Role:       ProtocolRoleServer,
		MessageHandlerFunc: func(msg Message) error {
			handled <- msg.Type()
			return nil
		},
		MessageFromCborFunc: func(msgType uint, _ []byte) (Message, error) {
			return &MessageBase{MessageType: uint8(msgType)}, nil
		},
		StateMap: StateMap{
			stateIdle: {
				Agency: AgencyClient,
				Transitions: []StateTransition{
					{MsgType: requestType, NewState: stateBusy},
				},
			},
			stateBusy: {
				Agency:                AgencyServer,
				PipelinedMessageTypes: []uint8{requestType},
				Transitions: []StateTransition{
					{MsgType: replyType, NewState: stateIdle},
				},
			},
		},
		InitialState: stateBusy,
	})
	p.Start()
	t.Cleanup(p.Stop)
	require.Eventually(t, func() bool {
		return p.getCurrentState() == stateBusy
	}, time.Second, time.Millisecond)
	segment := muxer.NewSegment(1, []byte{0x81, requestType}, false)
	require.NotNil(t, segment)
	select {
	case p.muxerRecvChan <- segment:
	case <-time.After(time.Second):
		t.Fatal("protocol did not accept the peer message")
	}
	select {
	case got := <-handled:
		t.Fatalf("message type %d was handled while this role held agency", got)
	case err := <-errorChan:
		t.Fatalf("protocol rejected declared pipelined message: %s", err)
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, p.SendMessage(&MessageBase{MessageType: replyType}))
	select {
	case got := <-handled:
		require.Equal(t, requestType, got)
		require.Equal(t, stateBusy, p.getCurrentState())
	case err := <-errorChan:
		t.Fatalf("protocol error after agency returned: %s", err)
	case <-time.After(time.Second):
		t.Fatal(
			"protocol did not handle the peer message after agency returned",
		)
	}
}
