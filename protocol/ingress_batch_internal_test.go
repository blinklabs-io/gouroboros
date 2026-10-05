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

// TestReadLoopBatchStaysWithinReadBufferCap delivers many complete messages
// at once, far more in total than the protocol's read buffer cap, as the
// muxer does when it releases a queue that built up behind a slow consumer.
// Each message fits the cap on its own, so every one must be handled: the
// read loop may batch queued segments before scanning, but not so many that
// the batch itself exceeds the cap meant for a single message.
func TestReadLoopBatchStaysWithinReadBufferCap(t *testing.T) {
	t.Parallel()
	const (
		messages    = 200
		messageSize = 100
		readBufCap  = 2048
	)
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	t.Cleanup(func() {
		m.Stop()
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	state := NewState(1, "Streaming")
	errorChan := make(chan error, 1)
	handled := make(chan struct{}, messages)
	p := New(ProtocolConfig{
		Name:       "ingress-batch",
		ProtocolId: 1,
		ErrorChan:  errorChan,
		Muxer:      m,
		Role:       ProtocolRoleServer,
		MessageHandlerFunc: func(Message) error {
			handled <- struct{}{}
			return nil
		},
		MessageFromCborFunc: func(msgType uint, _ []byte) (Message, error) {
			return &MessageBase{MessageType: uint8(msgType)}, nil
		},
		StateMap: StateMap{
			state: {
				Agency: AgencyClient,
				Transitions: []StateTransition{
					{MsgType: 0, NewState: state},
				},
			},
		},
		InitialState:      state,
		RecvQueueSize:     messages,
		MaxReadBufferSize: readBufCap,
	})
	p.EnsureRegistered()
	// [0, bytes(96)]: a complete 100-byte message per segment.
	payload := append([]byte{0x82, 0x00, 0x58, messageSize - 4},
		make([]byte, messageSize-4)...)
	require.Len(t, payload, messageSize)
	queued := make(chan *muxer.Segment, messages)
	for range messages {
		seg := muxer.NewSegment(1, payload, false)
		require.NotNil(t, seg)
		queued <- seg
	}
	p.muxerRecvChan = queued
	p.Start()
	t.Cleanup(p.Stop)
	for i := range messages {
		select {
		case <-handled:
		case err := <-errorChan:
			t.Fatalf(
				"protocol failed after %d of %d messages: %v",
				i,
				messages,
				err,
			)
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d messages handled", i, messages)
		}
	}
}
