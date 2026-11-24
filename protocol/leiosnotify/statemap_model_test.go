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

package leiosnotify

import (
	"fmt"
	"slices"
	"testing"
	"time"

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
	MessageTypeQuit,
	MessageTypeCanceled,
	MessageTypeNotificationRequestNext,
	MessageTypeBlockAnnouncement,
	MessageTypeBlockOffer,
	MessageTypeBlockTxsOffer,
	MessageTypeVotesOffer,
}

// modelReceive is the receive table of leios-notify: what a role in each
// active state does with each message type from its peer. It is the full
// role x active state x message type product, so a missing cell is one
// nobody decided. Done has no row: a message after Done belongs to the next
// run.
//
// Sources: ouroboros-consensus ec239d57f618,
// Ouroboros/Consensus/MiniProtocol/LeiosNotify/Type.hs lines 126-162 (the
// messages and agencies), and LeiosNotify/Client.hs lines 131-226 (the
// client pipelines requests and MsgQuit behind outstanding replies, so a
// server reads them while it holds agency in Busy). A message the state
// allows nobody to send fails the codec (Type.hs lines 354-358), which
// tears the connection down.
var modelReceive = map[protocol.ProtocolRole]map[protocol.State]map[uint8]string{
	protocol.ProtocolRoleServer: {
		StateIdle: {
			MessageTypeNotificationRequestNext: "Busy",
			MessageTypeQuit:                    "Quit",
		},
		StateBusy: {
			MessageTypeNotificationRequestNext: modelHeld,
			MessageTypeQuit:                    modelHeld,
		},
		StateQuit: {},
	},
	protocol.ProtocolRoleClient: {
		StateIdle: {},
		StateBusy: {
			MessageTypeCanceled:          "Idle",
			MessageTypeBlockAnnouncement: "Idle",
			MessageTypeBlockOffer:        "Idle",
			MessageTypeBlockTxsOffer:     "Idle",
			MessageTypeVotesOffer:        "Idle",
		},
		StateQuit: {
			MessageTypeDone: "Done",
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
		[]protocol.State{StateIdle, StateBusy, StateQuit, StateDone},
		stateMapStates(),
		"StateMap must hold exactly the states of the model",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[StateBusy].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[StateQuit].Agency)
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

// TestClientPipelinesOnlyRequestsAndQuit checks the send side of the model:
// the client may write while the server holds agency in Busy, and only
// NotificationRequestNext and MsgQuit, as the reference client does.
func TestClientPipelinesOnlyRequestsAndQuit(t *testing.T) {
	for state, entry := range StateMap {
		if state == StateBusy {
			require.True(t, entry.AllowPipelinedSend)
			require.ElementsMatch(
				t,
				[]uint8{MessageTypeNotificationRequestNext, MessageTypeQuit},
				entry.PipelinedMessageTypes,
			)
			continue
		}
		require.False(t, entry.AllowPipelinedSend, "%s pipelines", state)
		require.Empty(t, entry.PipelinedMessageTypes, "%s pipelines", state)
	}
}

// TestStatesThatCannotBeEntered checks the edges the model has no row for:
// nothing leaves Done, Done is entered only by MsgDone from Quit, Quit only
// by MsgQuit from Idle, and every state is reachable from Idle.
func TestStatesThatCannotBeEntered(t *testing.T) {
	require.Empty(t, StateMap[StateDone].Transitions)
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			require.Nil(t, transition.MatchFunc)
			switch transition.NewState {
			case StateDone:
				require.Equal(t, StateQuit, state)
				require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
			case StateQuit:
				require.Equal(t, StateIdle, state)
				require.Equal(t, uint8(MessageTypeQuit), transition.MsgType)
			}
		}
	}
	require.ElementsMatch(t, stateMapStates(), reachableStates(StateIdle))
}

// TestStateMapTimeoutsMatchReference pins the timeouts of the reference's
// timeLimitsLeiosNotify (Type.hs lines 221-227): Idle and Busy wait
// forever, and only Quit is bounded. Config.Timeout sets Quit's bound.
func TestStateMapTimeoutsMatchReference(t *testing.T) {
	cfg := NewConfig()
	stateMap := cfg.protocolConfig(protocol.ProtocolConfig{}).StateMap
	require.Zero(t, stateMap[StateIdle].Timeout)
	require.Zero(t, stateMap[StateBusy].Timeout)
	require.Equal(t, 60*time.Second, stateMap[StateQuit].Timeout)
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
