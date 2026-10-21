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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPipelinedMessageAdmittedAfterItsOwnStateEnds covers a peer-pipelined
// message that starts arriving while this role holds agency in a state that
// declares it pipelined, and whose admission is settled only after our reply
// has handed agency back to the peer.
//
// A ChainSync server in CanAwait accepts a pipelined RequestNext. When its
// RollForward moves the protocol to Idle first, the request is admitted
// against Idle, where the client holds agency and RequestNext is allowed
// anyway. Denying it there ended the session with "received message type 0
// without peer agency" in the middle of an ordinary sync.
func TestPipelinedMessageAdmittedAfterItsOwnStateEnds(t *testing.T) {
	t.Parallel()

	const msgTypeRequestNext uint8 = 0
	const msgTypeRollForward uint8 = 2
	idle := NewState(1, "Idle")
	canAwait := NewState(2, "CanAwait")
	stateMap := StateMap{
		idle: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeRequestNext, NewState: canAwait},
			},
		},
		canAwait: StateMapEntry{
			Agency:                AgencyServer,
			PipelinedMessageTypes: []uint8{msgTypeRequestNext},
			Transitions: []StateTransition{
				{MsgType: msgTypeRollForward, NewState: idle},
			},
		},
	}
	errorChan := make(chan error, 1)
	p := New(ProtocolConfig{
		Name:         "test-pipelined-admission",
		ErrorChan:    errorChan,
		Role:         ProtocolRoleServer,
		StateMap:     stateMap,
		InitialState: idle,
	})
	p.sendReadyChan = make(chan bool, 1)
	stateTransitionChan := make(chan protocolStateTransition)
	p.stateTransitionChan = stateTransitionChan
	go p.stateLoop(stateTransitionChan)
	t.Cleanup(p.Stop)

	requestNext := &MessageBase{MessageType: msgTypeRequestNext}
	rollForward := &MessageBase{MessageType: msgTypeRollForward}
	require.NoError(t, p.transitionInboundState(requestNext, false))

	// The next request starts arriving while the server holds agency.
	state, generation, hadUnreserved, _ := p.currentInboundSnapshot()
	require.Equal(t, canAwait, state)

	// The server replies before that request's admission is settled.
	require.NoError(t, p.transitionState(rollForward))

	_, _, result := p.reserveInboundAdmission(
		uint(msgTypeRequestNext),
		state,
		generation,
		hadUnreserved,
	)
	require.Equal(
		t,
		inboundAdmissionGranted,
		result,
		"a request pipelined in CanAwait must still be admitted once the "+
			"reply hands agency back in Idle",
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}
