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

package messagesubmission

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// modelViolation is the outcome of a message the receiver refuses: a
// protocol error, which tears the connection down.
const modelViolation = "Violation"

// modelKind is a kind of message a peer can send. The two request kinds
// differ only in the blocking flag, which selects the next state.
type modelKind string

const (
	kindInit                  modelKind = "MsgInit"
	kindRequestIdsBlocking    modelKind = "MsgRequestMessageIdsBlocking"
	kindRequestIdsNonBlocking modelKind = "MsgRequestMessageIdsNonBlocking"
	kindReplyIds              modelKind = "MsgReplyMessageIds"
	kindRequestMessages       modelKind = "MsgRequestMessages"
	kindReplyMessages         modelKind = "MsgReplyMessages"
	kindDone                  modelKind = "MsgDone"
)

var modelKinds = []modelKind{
	kindInit,
	kindRequestIdsBlocking,
	kindRequestIdsNonBlocking,
	kindReplyIds,
	kindRequestMessages,
	kindReplyMessages,
	kindDone,
}

// modelSide names who holds agency in a state of the model. In gOuroboros
// the outbound side, which has the messages, is the Client and the inbound
// side, which asks for them, is the Server, in both versions.
type modelSide int

const (
	sideNobody modelSide = iota
	sideOutbound
	sideInbound
)

// modelVersion is the state machine of one message-submission version, as
// CIP-0137 gives it ("Message Submission V1 mini-protocol" and "Message
// Submission V2 mini-protocol" in CIP-0137/README.md). Every state of the
// version is a key of agency; a (state, kind) pair absent from transitions
// is not allowed there.
type modelVersion struct {
	name        string
	stateMap    protocol.StateMap
	agency      map[protocol.State]modelSide
	transitions map[protocol.State]map[modelKind]protocol.State
}

var modelVersions = []modelVersion{
	{
		// V1 is tx-submission's shape: the outbound opens with MsgInit and
		// may end the run only instead of answering a blocking request.
		name:     "V1",
		stateMap: stateMapV1,
		agency: map[protocol.State]modelSide{
			protocolStateInit:             sideOutbound,
			protocolStateIdle:             sideInbound,
			protocolStateMessageIdsBlock:  sideOutbound,
			protocolStateMessageIdsNonBlk: sideOutbound,
			protocolStateMessages:         sideOutbound,
			protocolStateDone:             sideNobody,
		},
		transitions: map[protocol.State]map[modelKind]protocol.State{
			protocolStateInit: {kindInit: protocolStateIdle},
			protocolStateIdle: {
				kindRequestIdsBlocking:    protocolStateMessageIdsBlock,
				kindRequestIdsNonBlocking: protocolStateMessageIdsNonBlk,
				kindRequestMessages:       protocolStateMessages,
			},
			protocolStateMessageIdsBlock: {
				kindReplyIds: protocolStateIdle,
				kindDone:     protocolStateDone,
			},
			protocolStateMessageIdsNonBlk: {kindReplyIds: protocolStateIdle},
			protocolStateMessages:         {kindReplyMessages: protocolStateIdle},
		},
	},
	{
		// V2 has no StInit, and the inbound side ends the run from StIdle.
		// dmq-node's V2 also lets the outbound answer a blocking request
		// with MsgReplyNoSigIds; gOuroboros has no such message, a recorded
		// difference, so it is not a cell here.
		name:     "V2",
		stateMap: stateMapV2,
		agency: map[protocol.State]modelSide{
			protocolStateIdle:             sideInbound,
			protocolStateMessageIdsBlock:  sideOutbound,
			protocolStateMessageIdsNonBlk: sideOutbound,
			protocolStateMessages:         sideOutbound,
			protocolStateDone:             sideNobody,
		},
		transitions: map[protocol.State]map[modelKind]protocol.State{
			protocolStateIdle: {
				kindRequestIdsBlocking:    protocolStateMessageIdsBlock,
				kindRequestIdsNonBlocking: protocolStateMessageIdsNonBlk,
				kindRequestMessages:       protocolStateMessages,
				kindDone:                  protocolStateDone,
			},
			protocolStateMessageIdsBlock:  {kindReplyIds: protocolStateIdle},
			protocolStateMessageIdsNonBlk: {kindReplyIds: protocolStateIdle},
			protocolStateMessages:         {kindReplyMessages: protocolStateIdle},
		},
	},
}

// modelReceive is what role does with a message of kind from its peer in
// state: a message is legal only from the side holding agency and only if
// the state has a transition for it ("Protocols as State Machines" in
// docs/network-spec/miniprotocols.tex). A message sent while the receiver
// holds agency would wait in the reference; gOuroboros refuses it on
// arrival, as for every protocol, so the model's cell is Violation here.
func (v modelVersion) modelReceive(
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	peer := sideInbound
	if role == protocol.ProtocolRoleServer {
		peer = sideOutbound
	}
	if v.agency[state] != peer {
		return modelViolation
	}
	next, ok := v.transitions[state][kind]
	if !ok {
		return modelViolation
	}
	return next.String()
}

func modelMessage(kind modelKind) protocol.Message {
	switch kind {
	case kindInit:
		return NewMsgInit()
	case kindRequestIdsBlocking:
		return NewMsgRequestMessageIds(true, 0, 1)
	case kindRequestIdsNonBlocking:
		return NewMsgRequestMessageIds(false, 1, 1)
	case kindReplyIds:
		return NewMsgReplyMessageIds(nil)
	case kindRequestMessages:
		return NewMsgRequestMessages(nil)
	case kindReplyMessages:
		return NewMsgReplyMessages(nil)
	default:
		return NewMsgDone()
	}
}

// stateMapReceive derives one cell from a StateMap alone, following the
// checks protocol.Protocol applies on arrival: the receiver's peer must hold
// agency, and the state must have a transition whose type and match function
// accept the message.
func stateMapReceive(
	t *testing.T,
	stateMap protocol.StateMap,
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
	msg := modelMessage(kind)
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

func modelRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

func modelStates(stateMap protocol.StateMap) []protocol.State {
	states := make([]protocol.State, 0, len(stateMap))
	for state := range stateMap {
		states = append(states, state)
	}
	return states
}

// TestStateMapMatchesModel checks every role x state x message cell of both
// versions against the StateMap gOuroboros selects for that version,
// including cells no running peer can reach, and pins the states and the
// agency of each.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	roles := []protocol.ProtocolRole{
		protocol.ProtocolRoleClient,
		protocol.ProtocolRoleServer,
	}
	for _, v := range modelVersions {
		modelStatesList := make([]protocol.State, 0, len(v.agency))
		for state := range v.agency {
			modelStatesList = append(modelStatesList, state)
		}
		require.ElementsMatch(
			t,
			modelStatesList,
			modelStates(v.stateMap),
			"%s StateMap must hold exactly the states of the model",
			v.name,
		)
		for state, side := range v.agency {
			want := map[modelSide]protocol.ProtocolStateAgency{
				sideNobody:   protocol.AgencyNone,
				sideOutbound: protocol.AgencyClient,
				sideInbound:  protocol.AgencyServer,
			}[side]
			require.Equal(t, want, v.stateMap[state].Agency,
				"%s agency in %s", v.name, state)
			require.Empty(t, v.stateMap[state].PipelinedMessageTypes,
				"%s pipelines nothing in %s", v.name, state)
		}
		for _, role := range roles {
			for state := range v.agency {
				for _, kind := range modelKinds {
					name := fmt.Sprintf("%s/%s/in%s/%s",
						v.name, modelRoleName(role), state, kind)
					t.Run(name, func(t *testing.T) {
						require.Equal(
							t,
							v.modelReceive(role, state, kind),
							stateMapReceive(t, v.stateMap, role, state, kind),
						)
					})
				}
			}
		}
	}
}

// TestDoneIsEnteredOnlyAsModelled checks the states that cannot be entered
// except as the model allows: Done only by MsgDone, from StMessageIdsBlocking
// in V1 and from StIdle in V2; Init never re-entered and absent from V2; and
// nothing leaves Done within a run.
func TestDoneIsEnteredOnlyAsModelled(t *testing.T) {
	t.Parallel()
	doneFrom := map[string]protocol.State{
		"V1": protocolStateMessageIdsBlock,
		"V2": protocolStateIdle,
	}
	for _, v := range modelVersions {
		require.Empty(t, v.stateMap[protocolStateDone].Transitions, v.name)
		for state, entry := range v.stateMap {
			for _, transition := range entry.Transitions {
				require.NotEqual(t, protocolStateInit, transition.NewState,
					"%s re-enters Init from %s", v.name, state)
				if transition.NewState != protocolStateDone {
					continue
				}
				require.Equal(t, doneFrom[v.name], state, v.name)
				require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
			}
		}
	}
	_, hasInit := stateMapV2[protocolStateInit]
	require.False(t, hasInit, "V2 has no StInit")
}
