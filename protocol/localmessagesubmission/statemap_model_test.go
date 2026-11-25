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

package localmessagesubmission

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// modelViolation is the outcome of a message the receiver refuses: a
// protocol error, which tears the connection down.
const modelViolation = "Violation"

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindSubmit modelKind = "MsgSubmitMessage"
	kindAccept modelKind = "MsgAcceptMessage"
	kindReject modelKind = "MsgRejectMessage"
	kindDone   modelKind = "MsgDone"
)

var modelKinds = []modelKind{kindSubmit, kindAccept, kindReject, kindDone}

const modelDoneType = MessageTypeDone

// modelAgency and modelTransitions are the local message submission state
// machine of CIP-0137 ("Local Message Submission mini-protocol" in
// CIP-0137/README.md), which dmq-node implements as LocalTxSubmission
// (DMQ.Protocol.LocalMsgSubmission.Type). Every state is a key of
// modelAgency; a (state, kind) pair with no row is not allowed there.
var modelAgency = map[protocol.State]protocol.ProtocolStateAgency{
	protocolStateIdle: protocol.AgencyClient,
	protocolStateBusy: protocol.AgencyServer,
	protocolStateDone: protocol.AgencyNone,
}

var modelTransitions = []modelTransition{
	{protocolStateIdle, kindSubmit, protocolStateBusy},
	{protocolStateIdle, kindDone, protocolStateDone},
	{protocolStateBusy, kindAccept, protocolStateIdle},
	{protocolStateBusy, kindReject, protocolStateIdle},
}

func modelMessage(t *testing.T, kind modelKind) protocol.Message {
	t.Helper()
	switch kind {
	case kindSubmit:
		return NewMsgSubmitMessage(pcommon.DmqMessage{})
	case kindAccept:
		return NewMsgAcceptMessage()
	case kindReject:
		msg, err := NewMsgRejectMessage(pcommon.ExpiredReason{})
		require.NoError(t, err)
		return msg
	default:
		return NewMsgDone()
	}
}

// modelTransition is one row of the model's state machine: in from, a
// message of kind moves to to.
type modelTransition struct {
	from protocol.State
	kind modelKind
	to   protocol.State
}

func modelRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

// modelReceive is what role does with a message of kind from its peer in
// state: a message is legal only from the side holding agency and only if
// the state has a transition for it ("Protocols as State Machines" in
// docs/network-spec/miniprotocols.tex). A message sent while the receiver
// holds agency would wait in the reference; gOuroboros refuses it on
// arrival, as for every protocol, so the model's cell is Violation here.
func modelReceive(
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	peer := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peer = protocol.AgencyClient
	}
	if modelAgency[state] != peer {
		return modelViolation
	}
	for _, row := range modelTransitions {
		if row.from == state && row.kind == kind {
			return row.to.String()
		}
	}
	return modelViolation
}

// stateMapReceive derives one cell from StateMap alone, following the
// checks protocol.Protocol applies on arrival: the receiver's peer must hold
// agency, and the state must have a transition whose type and match function
// accept the message.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := stateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	peerAgency := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return modelViolation
	}
	msg := modelMessage(t, kind)
	for _, transition := range entry.Transitions {
		if transition.MsgType != msg.Type() {
			continue
		}
		if transition.MatchFunc != nil && !transition.MatchFunc(nil, msg) {
			continue
		}
		return transition.NewState.String()
	}
	return modelViolation
}

// TestStateMapMatchesModel checks every role x state x message cell against
// StateMap, including cells no running peer can reach, and pins the states
// and the agency of each.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	states := make([]protocol.State, 0, len(modelAgency))
	for state := range modelAgency {
		states = append(states, state)
	}
	mapStates := make([]protocol.State, 0, len(stateMap))
	for state := range stateMap {
		mapStates = append(mapStates, state)
	}
	require.ElementsMatch(t, states, mapStates,
		"StateMap must hold exactly the states of the model")
	for state, agency := range modelAgency {
		require.Equal(t, agency, stateMap[state].Agency, "agency in %s", state)
		require.Empty(t, stateMap[state].PipelinedMessageTypes,
			"nothing is pipelined in %s", state)
	}
	for _, role := range []protocol.ProtocolRole{
		protocol.ProtocolRoleClient,
		protocol.ProtocolRoleServer,
	} {
		for _, state := range states {
			for _, kind := range modelKinds {
				name := fmt.Sprintf("%s/in%s/%s", modelRoleName(role), state, kind)
				t.Run(name, func(t *testing.T) {
					require.Equal(
						t,
						modelReceive(role, state, kind),
						stateMapReceive(t, role, state, kind),
					)
				})
			}
		}
	}
}

// TestDoneCannotBeLeft checks that nothing leaves Done within a run and that
// Done is entered only by the client's MsgDone from StIdle.
func TestDoneCannotBeLeft(t *testing.T) {
	t.Parallel()
	require.Empty(t, stateMap[protocolStateDone].Transitions)
	for state, entry := range stateMap {
		for _, transition := range entry.Transitions {
			if transition.NewState != protocolStateDone {
				continue
			}
			require.Equal(t, protocolStateIdle, state)
			require.Equal(t, uint8(modelDoneType), transition.MsgType)
		}
	}
}
