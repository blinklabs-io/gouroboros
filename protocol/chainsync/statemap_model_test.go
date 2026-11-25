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

package chainsync

import (
	"fmt"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Receive outcomes besides the state the receiver moves to.
const (
	// modelViolation: the receiver refuses the message and reports a
	// protocol error, which tears the connection down.
	modelViolation = "Violation"
	// modelWaits: the receiver holds agency, so it does not read the
	// message yet; it is judged when the peer next holds agency.
	modelWaits = "Waits"
	// modelPipelined: the server holds agency and the message is a
	// MsgRequestNext the client pipelined; it is served after the current
	// request is answered.
	modelPipelined = "Pipelined"
)

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindRequestNext       modelKind = "MsgRequestNext"
	kindAwaitReply        modelKind = "MsgAwaitReply"
	kindRollForward       modelKind = "MsgRollForward"
	kindRollBackward      modelKind = "MsgRollBackward"
	kindFindIntersect     modelKind = "MsgFindIntersect"
	kindIntersectFound    modelKind = "MsgIntersectFound"
	kindIntersectNotFound modelKind = "MsgIntersectNotFound"
	kindDone              modelKind = "MsgDone"
	kindOversized         modelKind = "Oversized"
)

var modelKindTypes = map[modelKind]uint8{
	kindRequestNext:       MessageTypeRequestNext,
	kindAwaitReply:        MessageTypeAwaitReply,
	kindRollForward:       MessageTypeRollForward,
	kindRollBackward:      MessageTypeRollBackward,
	kindFindIntersect:     MessageTypeFindIntersect,
	kindIntersectFound:    MessageTypeIntersectFound,
	kindIntersectNotFound: MessageTypeIntersectNotFound,
	kindDone:              MessageTypeDone,
}

// modelReceive is one role in one state: what it does with each kind of
// message from its peer.
type modelReceive struct {
	role     protocol.ProtocolRole
	state    protocol.State
	outcomes map[modelKind]string
}

// chainSyncModelReceive is the receive table of the chain-sync mini-protocol,
// for every role, active state and kind of message. Done is left out: a
// server restarts after MsgDone and judges what follows as a new run, and a
// client whose run ended reads nothing more, which the running tests cover.
//
//   - Agencies and transitions are the specification's chain-sync state
//     agencies and messages tables (docs/network-spec/miniprotocols.tex,
//     "Chain-Sync mini-protocol"); a message a state does not allow aborts
//     the connection ("Protocols as State Machines"), as does one over the
//     state's node-to-node size limit ("Node-to-node size limits per
//     state").
//   - A side reads only while its peer holds agency (typed-protocols
//     runPeerWithDriver), so a message that arrives while the reader holds
//     agency waits (modelWaits).
//   - The specification says chain-sync is not pipelined, but the
//     reference client pipelines MsgRequestNext (ClientPipelined.hs), and
//     the server reads each one in order once it is back in StIdle
//     (modelPipelined). Where the two differ the reference is followed.
var chainSyncModelReceive = []modelReceive{
	{protocol.ProtocolRoleServer, stateIdle, map[modelKind]string{
		kindRequestNext:       "CanAwait",
		kindAwaitReply:        modelViolation,
		kindRollForward:       modelViolation,
		kindRollBackward:      modelViolation,
		kindFindIntersect:     "Intersect",
		kindIntersectFound:    modelViolation,
		kindIntersectNotFound: modelViolation,
		kindDone:              "Done",
		kindOversized:         modelViolation,
	}},
	{protocol.ProtocolRoleServer, stateCanAwait, map[modelKind]string{
		kindRequestNext:       modelPipelined,
		kindAwaitReply:        modelWaits,
		kindRollForward:       modelWaits,
		kindRollBackward:      modelWaits,
		kindFindIntersect:     modelWaits,
		kindIntersectFound:    modelWaits,
		kindIntersectNotFound: modelWaits,
		kindDone:              modelWaits,
		kindOversized:         modelWaits,
	}},
	{protocol.ProtocolRoleServer, stateMustReply, map[modelKind]string{
		kindRequestNext:       modelPipelined,
		kindAwaitReply:        modelWaits,
		kindRollForward:       modelWaits,
		kindRollBackward:      modelWaits,
		kindFindIntersect:     modelWaits,
		kindIntersectFound:    modelWaits,
		kindIntersectNotFound: modelWaits,
		kindDone:              modelWaits,
		kindOversized:         modelWaits,
	}},
	{protocol.ProtocolRoleServer, stateIntersect, map[modelKind]string{
		kindRequestNext:       modelWaits,
		kindAwaitReply:        modelWaits,
		kindRollForward:       modelWaits,
		kindRollBackward:      modelWaits,
		kindFindIntersect:     modelWaits,
		kindIntersectFound:    modelWaits,
		kindIntersectNotFound: modelWaits,
		kindDone:              modelWaits,
		kindOversized:         modelWaits,
	}},
	{protocol.ProtocolRoleClient, stateIdle, map[modelKind]string{
		kindRequestNext:       modelWaits,
		kindAwaitReply:        modelWaits,
		kindRollForward:       modelWaits,
		kindRollBackward:      modelWaits,
		kindFindIntersect:     modelWaits,
		kindIntersectFound:    modelWaits,
		kindIntersectNotFound: modelWaits,
		kindDone:              modelWaits,
		kindOversized:         modelWaits,
	}},
	{protocol.ProtocolRoleClient, stateCanAwait, map[modelKind]string{
		kindRequestNext:       modelViolation,
		kindAwaitReply:        "MustReply",
		kindRollForward:       "Idle",
		kindRollBackward:      "Idle",
		kindFindIntersect:     modelViolation,
		kindIntersectFound:    modelViolation,
		kindIntersectNotFound: modelViolation,
		kindDone:              modelViolation,
		kindOversized:         modelViolation,
	}},
	{protocol.ProtocolRoleClient, stateMustReply, map[modelKind]string{
		kindRequestNext:       modelViolation,
		kindAwaitReply:        modelViolation,
		kindRollForward:       "Idle",
		kindRollBackward:      "Idle",
		kindFindIntersect:     modelViolation,
		kindIntersectFound:    modelViolation,
		kindIntersectNotFound: modelViolation,
		kindDone:              modelViolation,
		kindOversized:         modelViolation,
	}},
	{protocol.ProtocolRoleClient, stateIntersect, map[modelKind]string{
		kindRequestNext:       modelViolation,
		kindAwaitReply:        modelViolation,
		kindRollForward:       modelViolation,
		kindRollBackward:      modelViolation,
		kindFindIntersect:     modelViolation,
		kindIntersectFound:    "Idle",
		kindIntersectNotFound: "Idle",
		kindDone:              modelViolation,
		kindOversized:         modelViolation,
	}},
}

// gouroborosJudgesOnArrival maps the table onto gOuroboros where it is
// stricter than the reference: protocol.Protocol judges a message the
// moment it arrives, so one sent while the receiver holds agency is refused
// at once, unless the state admits it as pipelined. The specification does
// not say when such a message is judged, so this is a recorded difference.
// Replies to a pipelined MsgRequestNext that reach a client already back in
// Idle are judged by the engine against the request's own state, not by
// StateMap.
func gouroborosJudgesOnArrival(outcome string) string {
	if outcome == modelWaits {
		return modelViolation
	}
	return outcome
}

// stateMapReceive derives the outcome of one cell from a state map alone,
// following the checks protocol.Protocol applies: a message over the
// state's size limit is refused; one sent while the receiver holds agency
// is admitted only if the state lists its type as pipelined; otherwise the
// state must have a transition for its type.
func stateMapReceive(
	t *testing.T,
	stateMap protocol.StateMap,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := stateMap[state]
	require.True(t, ok, "state map has no entry for %s", state)
	if kind == kindOversized {
		require.Positive(t, entry.PendingMessageByteLimit)
		return modelViolation
	}
	msgType := modelKindType(t, kind)
	peerAgency := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		if slices.Contains(entry.PipelinedMessageTypes, msgType) {
			return modelPipelined
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

func modelKindType(t *testing.T, kind modelKind) uint8 {
	t.Helper()
	msgType, ok := modelKindTypes[kind]
	require.True(t, ok, "no message type for %s", kind)
	return msgType
}

func modelRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

// TestStateMapMatchesModel checks every cell of the receive table against
// the node-to-node and node-to-client state maps, including cells no
// running client or server can be driven into. Node-to-client has no size
// limits ("Node-to-client size limits and timeouts"), so its Oversized cells
// are not judged by size.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	for _, mode := range []struct {
		name     string
		stateMap protocol.StateMap
		sized    bool
	}{
		{"NtN", StateMapNtN, true},
		{"NtC", StateMapNtC, false},
	} {
		for _, row := range chainSyncModelReceive {
			require.Len(t, row.outcomes, len(modelKindTypes)+1,
				"row %s/%s must decide every kind", modelRoleName(row.role), row.state)
			for kind, outcome := range row.outcomes {
				if kind == kindOversized && !mode.sized {
					continue
				}
				name := fmt.Sprintf("%s/%s/in%s/%s",
					mode.name, modelRoleName(row.role), row.state, kind)
				t.Run(name, func(t *testing.T) {
					require.Equal(t,
						gouroborosJudgesOnArrival(outcome),
						stateMapReceive(t, mode.stateMap, row.role, row.state, kind))
				})
			}
		}
	}
}

// TestStateMapAgencies checks each state's agency in both maps against the
// specification's chain-sync state agencies table.
func TestStateMapAgencies(t *testing.T) {
	t.Parallel()
	want := map[protocol.State]protocol.ProtocolStateAgency{
		stateIdle:      protocol.AgencyClient,
		stateCanAwait:  protocol.AgencyServer,
		stateMustReply: protocol.AgencyServer,
		stateIntersect: protocol.AgencyServer,
		stateDone:      protocol.AgencyNone,
	}
	for _, stateMap := range []protocol.StateMap{StateMapNtN, StateMapNtC} {
		require.Len(t, stateMap, len(want))
		for state, agency := range want {
			require.Equal(t, agency, stateMap[state].Agency, "state %s", state)
		}
	}
}

type modelEdge struct {
	from    protocol.State
	msgType uint8
}

// TestStatesEnteredOnlyAsSpecified checks, for both maps, every way into
// each state, so a state cannot be entered from anywhere the
// specification's messages table does not allow: MustReply only by
// MsgAwaitReply from CanAwait, Done only by MsgDone from Idle, and nothing
// leaves Done.
func TestStatesEnteredOnlyAsSpecified(t *testing.T) {
	t.Parallel()
	want := map[protocol.State][]modelEdge{
		stateIdle: {
			{stateCanAwait, MessageTypeRollForward},
			{stateCanAwait, MessageTypeRollBackward},
			{stateMustReply, MessageTypeRollForward},
			{stateMustReply, MessageTypeRollBackward},
			{stateIntersect, MessageTypeIntersectFound},
			{stateIntersect, MessageTypeIntersectNotFound},
		},
		stateCanAwait:  {{stateIdle, MessageTypeRequestNext}},
		stateMustReply: {{stateCanAwait, MessageTypeAwaitReply}},
		stateIntersect: {{stateIdle, MessageTypeFindIntersect}},
		stateDone:      {{stateIdle, MessageTypeDone}},
	}
	for _, stateMap := range []protocol.StateMap{StateMapNtN, StateMapNtC} {
		require.Empty(t, stateMap[stateDone].Transitions)
		got := map[protocol.State][]modelEdge{}
		for from, entry := range stateMap {
			for _, transition := range entry.Transitions {
				require.Nil(t, transition.MatchFunc,
					"%s transitions are decided by message type alone", from)
				got[transition.NewState] = append(got[transition.NewState],
					modelEdge{from, transition.MsgType})
			}
		}
		for state, edges := range want {
			require.ElementsMatch(t, edges, got[state], "ways into %s", state)
		}
		require.Len(t, got, len(want))
	}
}
