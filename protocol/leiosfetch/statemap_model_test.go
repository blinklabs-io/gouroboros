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

package leiosfetch

import (
	"fmt"
	"slices"
	"testing"

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

var modelMessageTypes = []uint8{
	MessageTypeDone,
	MessageTypeBlockRequest,
	MessageTypeBlock,
	MessageTypeBlockTxsRequest,
	MessageTypeBlockTxs,
	MessageTypeVotesRequest,
	MessageTypeVotes,
	MessageTypeBlockRangeRequest,
	MessageTypeNextBlockAndTxsInRange,
	MessageTypeLastBlockAndTxsInRange,
}

// modelReceive is the receive table of leios-fetch: what a role in each
// active state does with each message type from its peer. It is the full
// role x active state x message type product; a missing cell is a
// Violation. Done has no row: a message after Done belongs to the next run.
//
// Sources: ouroboros-consensus ec239d57f618,
// Ouroboros/Consensus/MiniProtocol/LeiosFetch/Type.hs lines 164-183 for
// Idle, Block, BlockTxs and Done and messages 0-4, and LeiosFetch/Client.hs
// lines 70-168, where the client pipelines block and block transactions
// requests, so a server reads them while it holds agency. The reference has
// no Votes or BlockRange state and no messages 10-14; those rows are
// gOuroboros' own extension (README.md) and are checked for consistency only:
// each request has its own server-agency state, and nothing is pipelined
// into or out of it.
var modelReceive = map[protocol.ProtocolRole]map[protocol.State]map[uint8]string{
	protocol.ProtocolRoleServer: {
		StateIdle: {
			MessageTypeDone:              "Done",
			MessageTypeBlockRequest:      "Block",
			MessageTypeBlockTxsRequest:   "BlockTxs",
			MessageTypeVotesRequest:      "Votes",
			MessageTypeBlockRangeRequest: "BlockRange",
		},
		StateBlock: {
			MessageTypeBlockRequest:    modelHeld,
			MessageTypeBlockTxsRequest: modelHeld,
		},
		StateBlockTxs: {
			MessageTypeBlockRequest:    modelHeld,
			MessageTypeBlockTxsRequest: modelHeld,
		},
		StateVotes:      {},
		StateBlockRange: {},
	},
	protocol.ProtocolRoleClient: {
		StateIdle: {},
		StateBlock: {
			MessageTypeBlock: "Idle",
		},
		StateBlockTxs: {
			MessageTypeBlockTxs: "Idle",
		},
		StateVotes: {
			MessageTypeVotes: "Idle",
		},
		StateBlockRange: {
			MessageTypeNextBlockAndTxsInRange: "BlockRange",
			MessageTypeLastBlockAndTxsInRange: "Idle",
		},
	},
}

func modelOutcome(role protocol.ProtocolRole, state protocol.State, msgType uint8) string {
	if outcome, ok := modelReceive[role][state][msgType]; ok {
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
// and otherwise the state must have a transition for it.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state protocol.State,
	msgType uint8,
) string {
	t.Helper()
	entry, ok := StateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	own := protocol.AgencyClient
	if role == protocol.ProtocolRoleServer {
		own = protocol.AgencyServer
	}
	if entry.Agency == own {
		if slices.Contains(entry.PipelinedMessageTypes, msgType) {
			return modelHeld
		}
		return modelViolation
	}
	for _, transition := range entry.Transitions {
		if transition.MsgType == msgType {
			return transition.NewState.String()
		}
	}
	return modelViolation
}

// TestStateMapMatchesModel checks every cell of the receive table against
// StateMap, for both roles, and pins the agencies the table depends on.
func TestStateMapMatchesModel(t *testing.T) {
	require.ElementsMatch(
		t,
		[]protocol.State{
			StateIdle, StateBlock, StateBlockTxs,
			StateVotes, StateBlockRange, StateDone,
		},
		stateMapStates(),
		"StateMap must hold exactly the states of the model",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	for _, state := range []protocol.State{
		StateBlock, StateBlockTxs, StateVotes, StateBlockRange,
	} {
		require.Equal(t, protocol.AgencyServer, StateMap[state].Agency, state)
	}
	require.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
	for role, states := range modelReceive {
		for state := range states {
			for _, msgType := range modelMessageTypes {
				name := fmt.Sprintf("%s/in%s/type%d", modelRoleName(role), state, msgType)
				t.Run(name, func(t *testing.T) {
					require.Equal(
						t,
						modelOutcome(role, state, msgType),
						stateMapReceive(t, role, state, msgType),
					)
				})
			}
		}
	}
}

// TestClientNeverWritesWithoutAgency checks the send side of the model. The
// client holds one request at a time (requestSlot), so no state lets it
// write while the server holds agency; MsgDone, and any request made after
// it, waits for Idle.
func TestClientNeverWritesWithoutAgency(t *testing.T) {
	for state, entry := range StateMap {
		require.False(t, entry.AllowPipelinedSend, "%s pipelines sends", state)
	}
}

// TestStatesThatCannotBeEntered checks the edges the model has no row for:
// nothing leaves Done, Done is entered only by MsgDone from Idle, every
// server-agency state is left only for Idle or itself, and every state is
// reachable from Idle.
func TestStatesThatCannotBeEntered(t *testing.T) {
	require.Empty(t, StateMap[StateDone].Transitions)
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			require.Nil(t, transition.MatchFunc)
			if transition.NewState == StateDone {
				require.Equal(t, StateIdle, state)
				require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
			}
			if state != StateIdle {
				require.Contains(
					t,
					[]protocol.State{StateIdle, state},
					transition.NewState,
				)
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
