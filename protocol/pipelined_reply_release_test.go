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

// TestPipelinedReplyReservationEndsWithItsTransition pins when an admitted
// pipelined reply gives back its reservation: as part of the transition that
// consumes it, not afterwards.
//
// A ChainSync client with requests in flight reserves a reply admitted while
// it holds agency in Idle. Its next request moves to CanAwait and reserves
// peer agency for that reply, which then moves the protocol back to Idle. If
// the reservation is returned only after that transition, the client's
// following request can reach CanAwait first and count the consumed reply as
// still pending, reserving peer agency for a reply that will never arrive.
// Every later reply then waits for admission and the session stalls until
// something tears the connection down.
func TestPipelinedReplyReservationEndsWithItsTransition(t *testing.T) {
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
			Agency: AgencyServer,
			Transitions: []StateTransition{
				{MsgType: msgTypeRollForward, NewState: idle},
			},
		},
	}
	p, errorChan := newStateLoopOnlyProtocol(t, stateMap, idle)

	// Three requests in flight, and the reply to the first admitted while
	// the client held agency in Idle, as the read loop does.
	for range 3 {
		p.addPendingPipelinedRequest()
	}
	require.True(t, p.reservePipelinedReply())

	requestNext := &MessageBase{MessageType: msgTypeRequestNext}
	rollForward := &MessageBase{MessageType: msgTypeRollForward}
	require.NoError(t, p.transitionPipelinedRequestState(requestNext))
	require.NoError(t, p.transitionInboundState(rollForward, true))
	require.NoError(t, p.transitionPipelinedRequestState(requestNext))

	state, generation, hadUnreserved, _ := p.currentInboundSnapshot()
	require.Equal(t, canAwait, state)
	_, _, result := p.reserveInboundAdmission(
		uint(msgTypeRollForward),
		state,
		generation,
		hadUnreserved,
	)
	require.Equal(
		t,
		inboundAdmissionGranted,
		result,
		"the next reply must be admitted: the reply that held the "+
			"reservation was already consumed",
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}
