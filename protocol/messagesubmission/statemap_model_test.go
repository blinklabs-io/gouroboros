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
	modelInit                  modelKind = "MsgInit"
	modelRequestIdsBlocking    modelKind = "MsgRequestMessageIdsBlocking"
	modelRequestIdsNonBlocking modelKind = "MsgRequestMessageIdsNonBlocking"
	modelReplyIds              modelKind = "MsgReplyMessageIds"
	modelReplyNoIds            modelKind = "MsgReplyNoMessageIds"
	modelRequestMessages       modelKind = "MsgRequestMessages"
	modelReplyMessages         modelKind = "MsgReplyMessages"
	modelDone                  modelKind = "MsgDone"
)

var modelKinds = []modelKind{
	modelInit,
	modelRequestIdsBlocking,
	modelRequestIdsNonBlocking,
	modelReplyIds,
	modelReplyNoIds,
	modelRequestMessages,
	modelReplyMessages,
	modelDone,
}

// kinds is the kinds of message the version has. MsgInit exists only in V1
// and MsgReplyNoMessageIds only in V2; each reuses a tag of the other
// version (6 is V2's MsgDone, 3 is V1's MsgReplyMessages), so a cell for a
// message the version lacks would test a different message.
func (v modelVersion) kinds() []modelKind {
	ret := make([]modelKind, 0, len(modelKinds))
	for _, kind := range modelKinds {
		if kind == modelInit && v.initiator == sideInbound {
			continue
		}
		if kind == modelReplyNoIds && v.initiator == sideOutbound {
			continue
		}
		ret = append(ret, kind)
	}
	return ret
}

// modelSide names who holds agency in a reference state: the outbound side has
// the messages and the inbound side asks for them. In gOuroboros the outbound
// side is the Client type and the inbound side the Server type in both
// versions, but the initiator differs: the outbound side in V1 (tx-submission's
// shape) and the inbound side in V2 (dmq-node's SigSubmissionV2), so the
// StateMap's AgencyClient means the outbound side in V1 and the inbound side in
// V2.
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
	version     uint16
	initiator   modelSide
	stateMap    protocol.StateMap
	agency      map[protocol.State]modelSide
	transitions map[protocol.State]map[modelKind]protocol.State
}

var modelVersions = []modelVersion{
	{
		// V1 is tx-submission's shape: the outbound opens with MsgInit and
		// may end the run only instead of answering a blocking request.
		name:      "V1",
		version:   protocol.ProtocolVersionDMQNtN1,
		initiator: sideOutbound,
		stateMap:  stateMapV1,
		agency: map[protocol.State]modelSide{
			protocolStateInit:             sideOutbound,
			protocolStateIdle:             sideInbound,
			protocolStateMessageIdsBlock:  sideOutbound,
			protocolStateMessageIdsNonBlk: sideOutbound,
			protocolStateMessages:         sideOutbound,
			protocolStateDone:             sideNobody,
		},
		transitions: map[protocol.State]map[modelKind]protocol.State{
			protocolStateInit: {modelInit: protocolStateIdle},
			protocolStateIdle: {
				modelRequestIdsBlocking:    protocolStateMessageIdsBlock,
				modelRequestIdsNonBlocking: protocolStateMessageIdsNonBlk,
				modelRequestMessages:       protocolStateMessages,
			},
			protocolStateMessageIdsBlock: {
				modelReplyIds: protocolStateIdle,
				modelDone:     protocolStateDone,
			},
			protocolStateMessageIdsNonBlk: {modelReplyIds: protocolStateIdle},
			protocolStateMessages:         {modelReplyMessages: protocolStateIdle},
		},
	},
	{
		// V2 has no StInit, and the inbound side, the initiator, ends the
		// run from StIdle. The outbound side may answer a blocking request
		// with MsgReplyNoMessageIds (dmq-node's MsgReplyNoSigIds).
		name:      "V2",
		version:   protocol.ProtocolVersionDMQNtN2,
		initiator: sideInbound,
		stateMap:  stateMapV2,
		agency: map[protocol.State]modelSide{
			protocolStateIdle:             sideInbound,
			protocolStateMessageIdsBlock:  sideOutbound,
			protocolStateMessageIdsNonBlk: sideOutbound,
			protocolStateMessages:         sideOutbound,
			protocolStateDone:             sideNobody,
		},
		transitions: map[protocol.State]map[modelKind]protocol.State{
			protocolStateIdle: {
				modelRequestIdsBlocking:    protocolStateMessageIdsBlock,
				modelRequestIdsNonBlocking: protocolStateMessageIdsNonBlk,
				modelRequestMessages:       protocolStateMessages,
				modelDone:                  protocolStateDone,
			},
			protocolStateMessageIdsBlock: {
				modelReplyIds:   protocolStateIdle,
				modelReplyNoIds: protocolStateIdle,
			},
			protocolStateMessageIdsNonBlk: {modelReplyIds: protocolStateIdle},
			protocolStateMessages:         {modelReplyMessages: protocolStateIdle},
		},
	},
}

// responder is the side that is not the version's initiator.
func (v modelVersion) responder() modelSide {
	if v.initiator == sideOutbound {
		return sideInbound
	}
	return sideOutbound
}

// modelReceive is what role does with a message of kind from its peer in
// state: a message is legal only from the side holding agency and only if
// the state has a transition for it ("Protocols as State Machines" in
// docs/network-spec/miniprotocols.tex). A message sent while the receiver
// holds agency would wait in the reference; gOuroboros refuses it on
// arrival, as for every protocol, so the reference cell is Violation here.
func (v modelVersion) modelReceive(
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	peer := v.responder()
	if role == protocol.ProtocolRoleServer {
		peer = v.initiator
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

func modelMessage(version uint16, kind modelKind) protocol.Message {
	switch kind {
	case modelInit:
		return NewMsgInit()
	case modelRequestIdsBlocking:
		return NewMsgRequestMessageIds(version, true, 0, 1)
	case modelRequestIdsNonBlocking:
		return NewMsgRequestMessageIds(version, false, 1, 1)
	case modelReplyIds:
		return NewMsgReplyMessageIds(version, nil)
	case modelReplyNoIds:
		return NewMsgReplyNoMessageIds()
	case modelRequestMessages:
		return NewMsgRequestMessages(version, nil)
	case modelReplyMessages:
		return NewMsgReplyMessages(version, nil)
	default:
		return NewMsgDone(version)
	}
}

// stateMapReceive derives one cell from a StateMap alone, following the
// checks protocol.Protocol applies on arrival: the receiver's peer must hold
// agency, and the state must have a transition whose type and match function
// accept the message.
func stateMapReceive(
	t *testing.T,
	v modelVersion,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := v.stateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	peerAgency := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return modelViolation
	}
	msg := modelMessage(v.version, kind)
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
			"%s StateMap must hold exactly the states of the reference",
			v.name,
		)
		for state, side := range v.agency {
			want := map[modelSide]protocol.ProtocolStateAgency{
				sideNobody:    protocol.AgencyNone,
				v.initiator:   protocol.AgencyClient,
				v.responder(): protocol.AgencyServer,
			}[side]
			require.Equal(t, want, v.stateMap[state].Agency,
				"%s agency in %s", v.name, state)
			require.Empty(t, v.stateMap[state].PipelinedMessageTypes,
				"%s pipelines nothing in %s", v.name, state)
		}
		for _, role := range roles {
			for state := range v.agency {
				for _, kind := range v.kinds() {
					name := fmt.Sprintf("%s/%s/in%s/%s",
						v.name, modelRoleName(role), state, kind)
					t.Run(name, func(t *testing.T) {
						require.Equal(
							t,
							v.modelReceive(role, state, kind),
							stateMapReceive(t, v, role, state, kind),
						)
					})
				}
			}
		}
	}
}

// TestDoneIsEnteredOnlyAsModelled checks the states that cannot be entered
// except as the reference allows: Done only by MsgDone, from
// StMessageIdsBlocking in V1 and from StIdle in V2; Init never re-entered and
// absent from V2; and nothing leaves Done within a run.
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
				require.Equal(t, wireTag(v.version, kindDone), transition.MsgType)
			}
		}
	}
	_, hasInit := stateMapV2[protocolStateInit]
	require.False(t, hasInit, "V2 has no StInit")
}
