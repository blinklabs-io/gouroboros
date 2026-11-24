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

package leiosvotes

import (
	"fmt"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Receive outcomes besides the state the receiver moves to.
const (
	// modelViolation: the receiver refuses the message and the connection
	// is torn down.
	modelViolation = "Violation"
	// modelHeld: the receiver holds agency, and the message is one the peer
	// may pipeline, so it is kept and handled once the peer holds agency.
	modelHeld = "Held"
)

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindRequest          modelKind = "RequestNext"
	kindRequestZero      modelKind = "RequestNextZero"
	kindRequestOverLimit modelKind = "RequestNextOverLimit"
	kindVote             modelKind = "Vote"
	kindDone             modelKind = "Done"
)

var modelKinds = []modelKind{
	kindRequest, kindRequestZero, kindRequestOverLimit, kindVote, kindDone,
}

func (k modelKind) message() protocol.Message {
	switch k {
	case kindRequest:
		return NewMsgVotesRequestNext(2)
	case kindRequestZero:
		return NewMsgVotesRequestNext(0)
	case kindRequestOverLimit:
		return NewMsgVotesRequestNext(MaxRequestNextCount + 1)
	case kindVote:
		return NewMsgVote(testVote())
	default:
		return NewMsgDone()
	}
}

// modelState is a protocol state together with the votes still owed in it.
type modelState struct {
	state  protocol.State
	tokens uint64
}

func (s modelState) String() string {
	if s.state == StateBusy {
		return fmt.Sprintf("Busy%d", s.tokens)
	}
	return s.state.String()
}

var (
	modelIdle  = modelState{state: StateIdle}
	modelBusy2 = modelState{state: StateBusy, tokens: 2}
	modelBusy1 = modelState{state: StateBusy, tokens: 1}
)

// modelReceive is the receive table of leios-votes: what a role in each
// active state does with each kind of message from its peer. It is the full
// role x active state x kind product; a missing cell is a Violation. Done
// has no row: a message after Done belongs to the next run.
//
// Source: CIP-0164 (cardano-foundation/CIPs 03387471), Figure 6b, the
// LeiosVotes state machine: MsgLeiosVotesRequestNext(N) moves Idle to Busy
// with N tokens, MsgLeiosVote stays in Busy while more than one token is
// left and returns to Idle on the last, and MsgDone ends the protocol from
// Idle. The same section has the client send the next request before the
// previous batch is delivered, by protocol pipelining, so a server holds a
// request that arrives in Busy. The CIP does not bound N; zero tokens would
// leave Busy unreachable from its own rule, and gOuroboros refuses counts
// above MaxRequestNextCount. The node has no LeiosVotes reference
// implementation.
var modelReceive = map[protocol.ProtocolRole]map[modelState]map[modelKind]string{
	protocol.ProtocolRoleServer: {
		modelIdle: {
			kindRequest: "Busy",
			kindDone:    "Done",
		},
		modelBusy2: {
			kindRequest:          modelHeld,
			kindRequestZero:      modelHeld,
			kindRequestOverLimit: modelHeld,
		},
		modelBusy1: {
			kindRequest:          modelHeld,
			kindRequestZero:      modelHeld,
			kindRequestOverLimit: modelHeld,
		},
	},
	protocol.ProtocolRoleClient: {
		modelIdle:  {},
		modelBusy2: {kindVote: "Busy"},
		modelBusy1: {kindVote: "Idle"},
	},
}

func modelOutcome(role protocol.ProtocolRole, state modelState, kind modelKind) string {
	if outcome, ok := modelReceive[role][state][kind]; ok {
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
// protocol.Protocol applies to a received message: one sent while the
// receiver holds agency is admitted only if the state lists it as pipelined,
// and otherwise the first transition whose MatchFunc accepts it in a state
// context holding the cell's tokens decides. A held request is judged when
// it is handled, in Idle; that is the Idle row.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state modelState,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := StateMap[state.state]
	require.True(t, ok, "StateMap has no entry for %s", state.state)
	msg := kind.message()
	own := protocol.AgencyClient
	if role == protocol.ProtocolRoleServer {
		own = protocol.AgencyServer
	}
	if entry.Agency == own {
		if slices.Contains(entry.PipelinedMessageTypes, msg.Type()) {
			return modelHeld
		}
		return modelViolation
	}
	ctx := &stateContext{tokens: state.tokens}
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
		[]protocol.State{StateIdle, StateBusy, StateDone},
		stateMapStates(),
		"StateMap must hold exactly the states of the reference",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[StateBusy].Agency)
	require.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
	for role, states := range modelReceive {
		for state := range states {
			for _, kind := range modelKinds {
				name := fmt.Sprintf("%s/in%s/%s", modelRoleName(role), state, kind)
				t.Run(name, func(t *testing.T) {
					require.Equal(
						t,
						modelOutcome(role, state, kind),
						stateMapReceive(t, role, state, kind),
					)
				})
			}
		}
	}
}

// TestTokensFollowTheModel drives one request through the transitions in a
// single state context, as the protocol does: the request sets the tokens,
// each vote but the last keeps Busy and takes one, and the last returns to
// Idle with none left.
func TestTokensFollowTheModel(t *testing.T) {
	ctx := &stateContext{}
	next := func(state protocol.State, msg protocol.Message) protocol.State {
		for _, transition := range StateMap[state].Transitions {
			if transition.MsgType != msg.Type() {
				continue
			}
			if transition.MatchFunc != nil &&
				!transition.MatchFunc(ctx, msg) {
				continue
			}
			return transition.NewState
		}
		t.Fatalf("%T refused in %s with %d tokens", msg, state, ctx.tokens)
		return protocol.State{}
	}
	state := next(StateIdle, NewMsgVotesRequestNext(3))
	require.Equal(t, StateBusy, state)
	require.Equal(t, uint64(3), ctx.tokens)
	for want := uint64(2); want > 0; want-- {
		state = next(state, NewMsgVote(testVote()))
		require.Equal(t, StateBusy, state)
		require.Equal(t, want, ctx.tokens)
	}
	state = next(state, NewMsgVote(testVote()))
	require.Equal(t, StateIdle, state)
	require.Zero(t, ctx.tokens)
}

// TestStatesThatCannotBeEntered checks the edges the reference table has no row
// for: nothing leaves Done, Done is entered only by MsgDone from Idle, Busy is
// entered only by a request from Idle or a vote from Busy, and every state is
// reachable from Idle. The client never writes without agency: it holds a
// pipelined request until the batch before it has arrived.
func TestStatesThatCannotBeEntered(t *testing.T) {
	require.Empty(t, StateMap[StateDone].Transitions)
	for state, entry := range StateMap {
		require.False(t, entry.AllowPipelinedSend, "%s pipelines sends", state)
		for _, transition := range entry.Transitions {
			switch transition.NewState {
			case StateDone:
				require.Equal(t, StateIdle, state)
				require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
			case StateBusy:
				if state == StateIdle {
					require.Equal(t, uint8(MessageTypeVotesRequestNext), transition.MsgType)
				} else {
					require.Equal(t, StateBusy, state)
					require.Equal(t, uint8(MessageTypeVote), transition.MsgType)
				}
			}
		}
	}
	require.ElementsMatch(t, stateMapStates(), reachableStates(StateIdle))
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

// TestRequestNextAfterStoppedSyncGetsOwnVote stops a Sync loop with one of
// its votes still owed, then calls RequestNext. The loop and RequestNext read
// the same channel, and votes carry no request identifier, so the loop must
// take the owed vote before RequestNext may run; otherwise RequestNext
// returns the loop's vote as the answer to its own request.
func TestRequestNextAfterStoppedSyncGetsOwnVote(t *testing.T) {
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		_ = connA.Close()
		_ = connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(m.Stop)
	cfg := NewConfig(
		WithPipelineLimit(1),
		WithRequestNextCount(2),
		WithVoteFunc(func(CallbackContext, Vote) error {
			return ErrStopVoteProcess
		}),
	)
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId(), Muxer: m},
		&cfg,
	)
	client.Start()
	t.Cleanup(client.Protocol.Stop)
	m.Start()
	require.NoError(t, client.Sync())
	requireReadTestSegment(t, connB)

	sendVote := func(slot uint64) {
		vote := testVote()
		vote.SlotNo = slot
		data, err := cbor.Encode(NewMsgVote(vote))
		require.NoError(t, err)
		writeTestSegment(t, connB, muxer.NewSegment(ProtocolId, data, true))
	}
	// The loop's two votes: the first stops it, the second is owed.
	sendVote(1)
	sendVote(2)

	type result struct {
		votes []Vote
		err   error
	}
	results := make(chan result, 1)
	require.Eventually(t, func() bool {
		client.busyMutex.Lock()
		running := client.syncRunning
		client.busyMutex.Unlock()
		return !running
	}, 5*time.Second, time.Millisecond, "the loop never ended")
	go func() {
		votes, err := client.RequestNext(1)
		results <- result{votes, err}
	}()
	requireReadTestSegment(t, connB)
	sendVote(3)
	select {
	case r := <-results:
		require.NoError(t, r.err)
		require.Len(t, r.votes, 1)
		require.Equal(t, uint64(3), r.votes[0].SlotNo,
			"RequestNext returned a vote the stopped loop was owed")
	case <-time.After(5 * time.Second):
		t.Fatal("RequestNext did not return")
	}
}
