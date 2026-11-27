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

// TestIdleClassificationCountsUnansweredPipelinedRequest drives a
// ChainSync-shaped requester through a request, a pipelined request, the
// first reply, the reservation of the second reply, the flush of the
// deferred transition and the second reply, and checks after each that a
// peer close is classified as clean only when the peer owes no reply.
// After the first reply returns the protocol to its initial state, the
// pipelined request is still unanswered until sendLoop flushes its deferred
// transition; a peer close in that window must not be treated as clean.
func TestIdleClassificationCountsUnansweredPipelinedRequest(t *testing.T) {
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
			AllowPipelinedSend:    true,
			PipelinedMessageTypes: []uint8{msgTypeRequestNext},
			Transitions: []StateTransition{
				{MsgType: msgTypeRollForward, NewState: idle},
			},
		},
	}
	p, errorChan := newStateLoopOnlyProtocol(t, stateMap, idle)
	requestNext := &MessageBase{MessageType: msgTypeRequestNext}
	rollForward := &MessageBase{MessageType: msgTypeRollForward}

	require.True(t, p.IsInTerminalOrIdleState(), "initial state, nothing sent")

	// Request: Idle -> CanAwait, the peer owes a reply.
	require.NoError(t, p.transitionState(requestNext))
	require.False(t, p.IsInTerminalOrIdleState(), "request unanswered")

	// PipelineRequest: written in CanAwait, transition deferred.
	p.addPendingPipelinedRequest()
	require.False(t, p.IsInTerminalOrIdleState(), "two requests unanswered")

	// AdmitInBusy: the first reply returns the protocol to Idle while the
	// pipelined request is still unanswered.
	require.NoError(t, p.transitionInboundState(rollForward, false))
	require.Equal(t, idle, p.getCurrentState())
	require.False(
		t,
		p.IsInTerminalOrIdleState(),
		"the peer still owes the pipelined request's reply",
	)

	// AdmitInIdle: that reply arrives and is held for the flush. The peer
	// owes nothing now.
	require.True(t, p.reservePipelinedReply())
	require.True(t, p.IsInTerminalOrIdleState(), "every reply received")

	// Flush, then the held reply: back in Idle with nothing owed.
	require.NoError(t, p.transitionPipelinedRequestState(requestNext))
	require.False(t, p.IsInTerminalOrIdleState(), "held reply not applied")
	require.NoError(t, p.transitionInboundState(rollForward, true))
	require.True(t, p.IsInTerminalOrIdleState(), "at rest")

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}
