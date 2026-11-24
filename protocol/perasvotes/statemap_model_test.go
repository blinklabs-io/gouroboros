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

package perasvotes

import (
	"fmt"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// modelViolation: the receiver refuses the message and the connection is
// torn down.
const modelViolation = "Violation"

// The reference's two messages gOuroboros does not implement, by their wire
// tags (ObjectDiffusion/Codec.hs lines 164-171).
const (
	refMessageTypeServerIdle uint8 = 6
	refMessageTypeAwaitReply uint8 = 7
)

var (
	modelIDA = VoteID{RoundNo: 1, SeatIndex: 1}
	modelIDB = VoteID{RoundNo: 1, SeatIndex: 2}
)

// modelState is a protocol state together with the window it is reached
// with: the object identifiers outstanding, and what the last request asked
// for.
type modelState struct {
	name  string
	state protocol.State
	setup func(*testing.T, *stateContext)
}

func noWindow(*testing.T, *stateContext) {}

func oneOutstanding(t *testing.T, ctx *stateContext) {
	t.Helper()
	require.True(t, matchBlockingObjectIDsRequest(ctx, NewMsgRequestObjectIDs(true, 0, 1)))
	require.True(t, matchReplyObjectIDs(ctx, NewMsgReplyObjectIDs([]VoteID{modelIDA})))
}

var (
	modelInit        = modelState{"Init", StateInit, noWindow}
	modelIdleEmpty   = modelState{"IdleEmpty", StateIdle, noWindow}
	modelIdleOne     = modelState{"IdleOne", StateIdle, oneOutstanding}
	modelIDsBlocking = modelState{"ObjectIDsBlocking", StateObjectIDsBlocking,
		func(t *testing.T, ctx *stateContext) {
			t.Helper()
			require.True(t, matchBlockingObjectIDsRequest(ctx, NewMsgRequestObjectIDs(true, 0, 1)))
		}}
	modelIDsNonBlocking = modelState{"ObjectIDsNonBlocking", StateObjectIDsNonBlocking,
		func(t *testing.T, ctx *stateContext) {
			t.Helper()
			oneOutstanding(t, ctx)
			require.True(t, matchNonBlockingObjectIDsRequest(ctx, NewMsgRequestObjectIDs(false, 0, 1)))
		}}
	modelObjects = modelState{"Objects", StateObjects,
		func(t *testing.T, ctx *stateContext) {
			t.Helper()
			oneOutstanding(t, ctx)
			require.True(t, matchRequestObjects(ctx, NewMsgRequestObjects([]VoteID{modelIDA})))
		}}
)

var modelStates = []modelState{
	modelInit, modelIdleEmpty, modelIdleOne,
	modelIDsBlocking, modelIDsNonBlocking, modelObjects,
}

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindInit              modelKind = "Init"
	kindReqIDsBlocking    modelKind = "RequestIDsBlocking"
	kindReqIDsBlockingAck modelKind = "RequestIDsBlockingAck"
	kindReqIDsNonBlocking modelKind = "RequestIDsNonBlocking"
	kindReqObjects        modelKind = "RequestObjects"
	kindDone              modelKind = "Done"
	kindReplyIDs          modelKind = "ReplyIDs"
	kindReplyIDsEmpty     modelKind = "ReplyIDsEmpty"
	kindReplyObjects      modelKind = "ReplyObjects"
	kindReplyObjectsOther modelKind = "ReplyObjectsUnrequested"
	kindAwaitReply        modelKind = "AwaitReply"
	kindServerIdle        modelKind = "ServerIdle"
)

var modelKinds = []modelKind{
	kindInit, kindReqIDsBlocking, kindReqIDsBlockingAck, kindReqIDsNonBlocking,
	kindReqObjects, kindDone, kindReplyIDs, kindReplyIDsEmpty, kindReplyObjects,
	kindReplyObjectsOther, kindAwaitReply, kindServerIdle,
}

func (k modelKind) message(t *testing.T) protocol.Message {
	t.Helper()
	switch k {
	case kindInit:
		return NewMsgInit()
	case kindReqIDsBlocking:
		return NewMsgRequestObjectIDs(true, 0, 1)
	case kindReqIDsBlockingAck:
		return NewMsgRequestObjectIDs(true, 1, 1)
	case kindReqIDsNonBlocking:
		return NewMsgRequestObjectIDs(false, 0, 1)
	case kindReqObjects:
		return NewMsgRequestObjects([]VoteID{modelIDA})
	case kindDone:
		return NewMsgDone()
	case kindReplyIDs:
		return NewMsgReplyObjectIDs([]VoteID{modelIDB})
	case kindReplyIDsEmpty:
		return NewMsgReplyObjectIDs(nil)
	case kindReplyObjects:
		return NewMsgReplyObjects([]VoteObject{testVoteObject(t, modelIDA)})
	case kindReplyObjectsOther:
		return NewMsgReplyObjects([]VoteObject{testVoteObject(t, modelIDB)})
	case kindAwaitReply:
		return &protocol.MessageBase{MessageType: refMessageTypeAwaitReply}
	default:
		return &protocol.MessageBase{MessageType: refMessageTypeServerIdle}
	}
}

// modelReceive is the receive table of ObjectDiffusion as Peras vote
// diffusion uses it: what a role in each active state, reached with a given
// window, does with each kind of message from its peer. A missing cell is a
// Violation. Done has no row.
//
// Source: ouroboros-network 5f356486ab3e,
// ouroboros-network/protocols/lib/Ouroboros/Network/Protocol/ObjectDiffusion/Type.hs
// lines 263-393: the agencies; MsgInit from Init; a blocking request MUST be
// used exactly when no identifier stays unacknowledged, and its reply is
// non-empty; a non-blocking reply may be empty; only outstanding
// identifiers may be requested; MsgDone from Idle; and, in a blocking
// request's state, MsgAwaitReply and then MsgServerIdle.
var modelReceive = map[protocol.ProtocolRole]map[string]map[modelKind]string{
	protocol.ProtocolRoleServer: {
		modelInit.name: {kindInit: "Idle"},
		modelIdleEmpty.name: {
			kindReqIDsBlocking: "ObjectIDsBlocking",
			kindDone:           "Done",
		},
		modelIdleOne.name: {
			kindReqIDsBlockingAck: "ObjectIDsBlocking",
			kindReqIDsNonBlocking: "ObjectIDsNonBlocking",
			kindReqObjects:        "Objects",
			kindDone:              "Done",
		},
		modelIDsBlocking.name:    {},
		modelIDsNonBlocking.name: {},
		modelObjects.name:        {},
	},
	protocol.ProtocolRoleClient: {
		modelInit.name:      {},
		modelIdleEmpty.name: {},
		modelIdleOne.name:   {},
		modelIDsBlocking.name: {
			kindReplyIDs:   "Idle",
			kindAwaitReply: "ObjectIDsBlockingMustReply",
		},
		modelIDsNonBlocking.name: {
			kindReplyIDs:      "Idle",
			kindReplyIDsEmpty: "Idle",
		},
		modelObjects.name: {
			kindReplyObjects: "Idle",
		},
	},
}

// gouroborosDeviations are the cells where gOuroboros differs from the
// reference, with what it does instead. It has no MsgAwaitReply or
// MsgServerIdle and no MustReply state, so it refuses the reference
// server's await in a blocking request's state.
var gouroborosDeviations = map[string]string{
	"client/inObjectIDsBlocking/AwaitReply": modelViolation,
}

func modelOutcome(role protocol.ProtocolRole, state modelState, kind modelKind) string {
	if outcome, ok := modelReceive[role][state.name][kind]; ok {
		return outcome
	}
	return modelViolation
}

func modelRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

// stateMapReceive derives one cell from StateMap alone, following the checks
// protocol.Protocol applies to a received message: nothing is pipelined, so
// one sent while the receiver holds agency is refused, and otherwise the
// first transition whose MatchFunc accepts it in the cell's window decides.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state modelState,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := StateMap[state.state]
	require.True(t, ok, "StateMap has no entry for %s", state.state)
	own := protocol.AgencyClient
	if role == protocol.ProtocolRoleServer {
		own = protocol.AgencyServer
	}
	if entry.Agency == own {
		return modelViolation
	}
	ctx := newStateContext(DefaultMaxObjectsUnacknowledged)
	state.setup(t, ctx)
	msg := kind.message(t)
	for _, transition := range entry.Transitions {
		if transition.MsgType != msg.Type() {
			continue
		}
		if transition.MatchFunc != nil && !transition.MatchFunc(ctx, msg) {
			continue
		}
		return transition.NewState.String()
	}
	return modelViolation
}

// TestStateMapMatchesModel checks every cell of the receive table against
// StateMap, for both roles, and pins the agencies the table depends on.
func TestStateMapMatchesModel(t *testing.T) {
	require.ElementsMatch(
		t,
		[]protocol.State{
			StateInit, StateIdle, StateObjectIDsBlocking,
			StateObjectIDsNonBlocking, StateObjects, StateDone,
		},
		stateMapStates(),
		"StateMap must hold the reference states except MustReply",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[StateInit].Agency)
	require.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	for _, state := range []protocol.State{
		StateObjectIDsBlocking, StateObjectIDsNonBlocking, StateObjects,
	} {
		require.Equal(t, protocol.AgencyServer, StateMap[state].Agency, state)
	}
	require.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
	for role := range modelReceive {
		for _, state := range modelStates {
			for _, kind := range modelKinds {
				name := fmt.Sprintf("%s/in%s/%s", modelRoleName(role), state.name, kind)
				t.Run(name, func(t *testing.T) {
					want := modelOutcome(role, state, kind)
					if deviation, ok := gouroborosDeviations[name]; ok {
						want = deviation
					}
					require.Equal(t, want, stateMapReceive(t, role, state, kind))
				})
			}
		}
	}
}

// TestStatesThatCannotBeEntered checks the edges the model has no row for:
// nothing leaves Done or enters Init, Done is entered only by MsgDone from
// Idle, nothing is pipelined, and every state is reachable from Init.
func TestStatesThatCannotBeEntered(t *testing.T) {
	require.Empty(t, StateMap[StateDone].Transitions)
	for state, entry := range StateMap {
		require.False(t, entry.AllowPipelinedSend, "%s pipelines sends", state)
		require.Empty(t, entry.PipelinedMessageTypes, "%s pipelines", state)
		for _, transition := range entry.Transitions {
			require.NotEqual(t, StateInit, transition.NewState)
			if transition.NewState == StateDone {
				require.Equal(t, StateIdle, state)
				require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
			}
		}
	}
	require.ElementsMatch(t, stateMapStates(), reachableStates(StateInit))
}

func stateMapStates() []protocol.State {
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	return states
}

func reachableStates(initial protocol.State) []protocol.State {
	seen := []protocol.State{initial}
	for i := 0; i < len(seen); i++ {
		for _, transition := range StateMap[seen[i]].Transitions {
			if !slices.Contains(seen, transition.NewState) {
				seen = append(seen, transition.NewState)
			}
		}
	}
	return seen
}
