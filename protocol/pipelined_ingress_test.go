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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

func TestReceiveLoopHandlesDeclaredPeerPipelinedMessagesWithLocalAgency(t *testing.T) {
	const pipelinedMessageType = uint8(0)
	for _, test := range []struct {
		name  string
		state State
	}{
		{name: "busy", state: NewState(1, "Busy")},
		{name: "streaming", state: NewState(2, "Streaming")},
	} {
		t.Run(test.name, func(t *testing.T) {
			localConn, peerConn := net.Pipe()
			m := muxer.New(localConn)
			m.Start()
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
					test.state: {
						Agency:                AgencyServer,
						PipelinedMessageTypes: []uint8{pipelinedMessageType},
					},
				},
				InitialState: test.state,
			})
			p.Start()
			t.Cleanup(p.Stop)
			require.Eventually(t, func() bool {
				return p.getCurrentState() == test.state
			}, time.Second, time.Millisecond)
			segment := muxer.NewSegment(
				1,
				[]byte{0x81, pipelinedMessageType},
				false,
			)
			require.NotNil(t, segment)
			select {
			case p.muxerRecvChan <- segment:
			case <-time.After(time.Second):
				t.Fatal("protocol did not accept the peer message")
			}
			select {
			case got := <-handled:
				require.Equal(t, pipelinedMessageType, got)
				require.Equal(t, test.state, p.getCurrentState())
			case err := <-errorChan:
				t.Fatalf("protocol rejected declared pipelined message: %s", err)
			case <-time.After(time.Second):
				t.Fatal("protocol did not handle the peer message")
			}
		})
	}
}
