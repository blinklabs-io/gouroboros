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

package localstatequery

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
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
	// modelHeld: the client's run is over and no new run reads the message.
	modelHeld = "Held"
)

// modelKind is one of the twelve wire keys of the local-state-query codec.
// MsgAcquire and MsgReAcquire each have one key per acquire target.
type modelKind string

const (
	kindAcquire               modelKind = "MsgAcquire"
	kindAcquired              modelKind = "MsgAcquired"
	kindFailure               modelKind = "MsgFailure"
	kindQuery                 modelKind = "MsgQuery"
	kindResult                modelKind = "MsgResult"
	kindRelease               modelKind = "MsgRelease"
	kindReAcquire             modelKind = "MsgReAcquire"
	kindDone                  modelKind = "MsgDone"
	kindAcquireVolatileTip    modelKind = "MsgAcquireVolatileTip"
	kindReAcquireVolatileTip  modelKind = "MsgReAcquireVolatileTip"
	kindAcquireImmutableTip   modelKind = "MsgAcquireImmutableTip"
	kindReAcquireImmutableTip modelKind = "MsgReAcquireImmutableTip"
)

// modelKindTypes maps each kind to its message type, and so lists every kind.
var modelKindTypes = map[modelKind]uint8{
	kindAcquire:               MessageTypeAcquire,
	kindAcquired:              MessageTypeAcquired,
	kindFailure:               MessageTypeFailure,
	kindQuery:                 MessageTypeQuery,
	kindResult:                MessageTypeResult,
	kindRelease:               MessageTypeRelease,
	kindReAcquire:             MessageTypeReacquire,
	kindDone:                  MessageTypeDone,
	kindAcquireVolatileTip:    MessageTypeAcquireVolatileTip,
	kindReAcquireVolatileTip:  MessageTypeReacquireVolatileTip,
	kindAcquireImmutableTip:   MessageTypeAcquireImmutableTip,
	kindReAcquireImmutableTip: MessageTypeReacquireImmutableTip,
}

// modelReceiveRow is one cell of the receive table: what a role in a given
// state does with a kind of message from its peer.
type modelReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    modelKind
	outcome string
}

// lsqModelReceive is the receive table of the local-state-query
// mini-protocol, printed by the TLA+ model LocalStateQueryTable.tla. It is
// the full role x state x kind product, so a cell missing here is a cell
// nobody decided.
//
//   - Within a run a message is judged against the reader's state, by the
//     transition table of "Local State Query mini-protocol" in
//     docs/network-spec/miniprotocols.tex and the reference codec
//     (LocalStateQuery/Codec.hs); one that is not legal there aborts the
//     connection ("Protocols as State Machines", same file).
//   - A side reads only while its peer holds agency, so a message that
//     arrives while the reader holds agency waits (modelWaits).
//   - The node-to-client server runs under the reference inbound governor,
//     which restarts a responder that terminated cleanly, so a server judges
//     a message after MsgDone as the first of a new run. A client whose run
//     ended leaves the message unread (modelHeld).
var lsqModelReceive = []modelReceiveRow{
	{protocol.ProtocolRoleServer, stateIdle, kindAcquire, "Acquiring"},
	{protocol.ProtocolRoleServer, stateIdle, kindAcquired, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindFailure, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindQuery, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindResult, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindRelease, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindReAcquire, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindDone, "Done"},
	{protocol.ProtocolRoleServer, stateIdle, kindAcquireVolatileTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateIdle, kindReAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindAcquireImmutableTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateIdle, kindReAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquiring, kindAcquire, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindAcquired, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindFailure, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindQuery, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindResult, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindRelease, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindReAcquire, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindDone, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindReAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquiring, kindReAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleServer, stateAcquired, kindAcquire, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindAcquired, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindFailure, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindQuery, "Querying"},
	{protocol.ProtocolRoleServer, stateAcquired, kindResult, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindRelease, "Idle"},
	{protocol.ProtocolRoleServer, stateAcquired, kindReAcquire, "Acquiring"},
	{protocol.ProtocolRoleServer, stateAcquired, kindDone, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindReAcquireVolatileTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateAcquired, kindAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleServer, stateAcquired, kindReAcquireImmutableTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateQuerying, kindAcquire, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindAcquired, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindFailure, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindQuery, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindResult, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindRelease, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindReAcquire, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindDone, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindReAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleServer, stateQuerying, kindReAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleServer, stateDone, kindAcquire, "Acquiring"},
	{protocol.ProtocolRoleServer, stateDone, kindAcquired, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindFailure, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindQuery, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindResult, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindRelease, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindReAcquire, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindDone, "Done"},
	{protocol.ProtocolRoleServer, stateDone, kindAcquireVolatileTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateDone, kindReAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindAcquireImmutableTip, "Acquiring"},
	{protocol.ProtocolRoleServer, stateDone, kindReAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleClient, stateIdle, kindAcquire, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindAcquired, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindFailure, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindQuery, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindResult, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindRelease, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindReAcquire, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindDone, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindReAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindReAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquiring, kindAcquire, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindAcquired, "Acquired"},
	{protocol.ProtocolRoleClient, stateAcquiring, kindFailure, "Idle"},
	{protocol.ProtocolRoleClient, stateAcquiring, kindQuery, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindResult, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindRelease, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindReAcquire, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindDone, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindReAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquiring, kindReAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleClient, stateAcquired, kindAcquire, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindAcquired, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindFailure, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindQuery, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindResult, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindRelease, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindReAcquire, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindDone, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindReAcquireVolatileTip, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleClient, stateAcquired, kindReAcquireImmutableTip, modelWaits},
	{protocol.ProtocolRoleClient, stateQuerying, kindAcquire, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindAcquired, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindFailure, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindQuery, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindResult, "Acquired"},
	{protocol.ProtocolRoleClient, stateQuerying, kindRelease, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindReAcquire, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindDone, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindReAcquireVolatileTip, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleClient, stateQuerying, kindReAcquireImmutableTip, modelViolation},
	{protocol.ProtocolRoleClient, stateDone, kindAcquire, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindAcquired, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindFailure, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindQuery, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindResult, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindRelease, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindReAcquire, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindDone, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindAcquireVolatileTip, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindReAcquireVolatileTip, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindAcquireImmutableTip, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindReAcquireImmutableTip, modelHeld},
}

func modelRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

func (r modelReceiveRow) name() string {
	return fmt.Sprintf("%s/in%s/%s", modelRoleName(r.role), r.state, r.kind)
}

// gouroborosRejectsEarly maps the table onto gOuroboros where it is stricter
// than the reference implementation: protocol.Protocol judges a message the
// moment it arrives, and refuses one sent while the receiver holds agency
// instead of leaving it to wait. The specification does not say when such a
// message is judged, so this is a recorded difference, not a defect.
func gouroborosRejectsEarly(outcome string) string {
	if outcome == modelWaits {
		return modelViolation
	}
	return outcome
}

// stateMapReceive derives the outcome of one within-run cell from StateMap
// alone, following the checks protocol.Protocol applies: a message sent
// while the receiver holds agency is refused, as is one the state has no
// transition for.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := StateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	peerAgency := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return modelViolation
	}
	for _, transition := range entry.Transitions {
		if transition.MsgType == modelKindTypes[kind] {
			return transition.NewState.String()
		}
	}
	return modelViolation
}

// TestStateMapMatchesModel checks the within-run cells of the receive table
// against StateMap, including the cells no running client or server can be
// driven into, and pins the shape of StateMap the table depends on. The Done
// rows are about the next run, which the running tests cover.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	require.Len(t, modelKindTypes, 12, "the codec has twelve message keys")
	require.Len(t, lsqModelReceive, 2*5*12, "the table must be complete")
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	require.ElementsMatch(
		t,
		[]protocol.State{
			stateIdle, stateAcquiring, stateAcquired, stateQuerying, stateDone,
		},
		states,
		"StateMap must hold exactly the states of the model",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[stateIdle].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[stateAcquiring].Agency)
	require.Equal(t, protocol.AgencyClient, StateMap[stateAcquired].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[stateQuerying].Agency)
	require.Equal(t, protocol.AgencyNone, StateMap[stateDone].Agency)
	for state, entry := range StateMap {
		require.Empty(
			t,
			entry.PipelinedMessageTypes,
			"local-state-query pipelines nothing, so %s must admit only the agency holder",
			state,
		)
		for _, transition := range entry.Transitions {
			require.Nil(
				t,
				transition.MatchFunc,
				"%s transitions are decided by message type alone",
				state,
			)
		}
	}
	for _, row := range lsqModelReceive {
		if row.state == stateDone {
			continue
		}
		t.Run(row.name(), func(t *testing.T) {
			require.Equal(
				t,
				gouroborosRejectsEarly(row.outcome),
				stateMapReceive(t, row.role, row.state, row.kind),
			)
		})
	}
}

// TestUnreachableStates checks the states each side can never enter within
// a run: no transition leaves Done, only MsgDone from Idle enters it,
// Querying is entered only by MsgQuery from Acquired, and Acquired only by
// MsgAcquired or MsgResult. So a query can only be sent on an acquired state.
func TestUnreachableStates(t *testing.T) {
	t.Parallel()
	require.Empty(t, StateMap[stateDone].Transitions)
	entries := map[protocol.State][]string{}
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			entries[transition.NewState] = append(
				entries[transition.NewState],
				fmt.Sprintf("%s/%d", state, transition.MsgType),
			)
		}
	}
	require.ElementsMatch(t,
		[]string{fmt.Sprintf("%s/%d", stateIdle, MessageTypeDone)},
		entries[stateDone])
	require.ElementsMatch(t,
		[]string{fmt.Sprintf("%s/%d", stateAcquired, MessageTypeQuery)},
		entries[stateQuerying])
	require.ElementsMatch(t,
		[]string{
			fmt.Sprintf("%s/%d", stateAcquiring, MessageTypeAcquired),
			fmt.Sprintf("%s/%d", stateQuerying, MessageTypeResult),
		},
		entries[stateAcquired])
	require.ElementsMatch(t,
		[]string{
			fmt.Sprintf("%s/%d", stateAcquiring, MessageTypeFailure),
			fmt.Sprintf("%s/%d", stateAcquired, MessageTypeRelease),
		},
		entries[stateIdle],
		"a failed acquire or re-acquire returns to Idle")
}

// modelHarness runs one local-state-query role over a muxer and plays the
// other side of the connection by hand.
type modelHarness struct {
	conn     net.Conn
	errs     chan error
	payload  chan []byte
	role     protocol.ProtocolRole
	released chan struct{}
}

func newModelHarness(
	t *testing.T,
	role protocol.ProtocolRole,
) (*modelHarness, protocol.ProtocolOptions) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	h := &modelHarness{
		conn:     connB,
		errs:     make(chan error, 8),
		payload:  make(chan []byte, 8),
		role:     role,
		released: make(chan struct{}, 4),
	}
	opts := protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
			RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
		},
		Muxer:     m,
		ErrorChan: h.errs,
		Mode:      protocol.ProtocolModeNodeToClient,
		Version:   23 + protocol.ProtocolVersionNtCOffset,
	}
	// net.Pipe writes block until read, so the peer must always be reading.
	go h.readPeer()
	t.Cleanup(func() {
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	m.Start()
	return h, opts
}

func (h *modelHarness) readPeer() {
	defer close(h.payload)
	for {
		var header muxer.SegmentHeader
		if err := binary.Read(h.conn, binary.BigEndian, &header); err != nil {
			return
		}
		data := make([]byte, header.PayloadLength)
		if _, err := io.ReadFull(h.conn, data); err != nil {
			return
		}
		h.payload <- data
	}
}

// send writes msg as the peer would. A write error is ignored, because the
// role under test may close the connection on refusing the message; the
// refusal is asserted through the error channel.
func (h *modelHarness) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	segment := muxer.NewSegment(
		ProtocolId,
		data,
		h.role == protocol.ProtocolRoleClient,
	)
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(
		t,
		binary.Write(buf, binary.BigEndian, &segment.SegmentHeader),
	)
	buf.Write(segment.Payload)
	require.NoError(t, h.conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	_, _ = h.conn.Write(buf.Bytes())
}

var modelPoint = pcommon.NewPoint(42, make([]byte, 32))

// modelBlockNo is the block number every result in these tests carries, in
// the [1, n] shape GetChainBlockNo decodes.
const modelBlockNo = 42

func modelResult(t *testing.T) []byte {
	t.Helper()
	data, err := cbor.Encode([]uint64{1, modelBlockNo})
	require.NoError(t, err)
	return data
}

func (h *modelHarness) sendKind(t *testing.T, kind modelKind) {
	t.Helper()
	switch kind {
	case kindAcquire:
		h.send(t, NewMsgAcquire(modelPoint))
	case kindAcquired:
		h.send(t, NewMsgAcquired())
	case kindFailure:
		h.send(t, NewMsgFailure(AcquireFailurePointTooOld))
	case kindQuery:
		h.send(t, NewMsgQuery(buildQuery(QueryTypeChainBlockNo)))
	case kindResult:
		h.send(t, NewMsgResult(modelResult(t)))
	case kindRelease:
		h.send(t, NewMsgRelease())
	case kindReAcquire:
		h.send(t, NewMsgReAcquire(modelPoint))
	case kindDone:
		h.send(t, NewMsgDone())
	case kindAcquireVolatileTip:
		h.send(t, NewMsgAcquireVolatileTip())
	case kindReAcquireVolatileTip:
		h.send(t, NewMsgReAcquireVolatileTip())
	case kindAcquireImmutableTip:
		h.send(t, NewMsgAcquireImmutableTip())
	case kindReAcquireImmutableTip:
		h.send(t, NewMsgReAcquireImmutableTip())
	default:
		t.Fatalf("no message for kind %q", kind)
	}
}

// receive returns the next message the role under test sent to the peer.
func (h *modelHarness) receive(t *testing.T) protocol.Message {
	t.Helper()
	select {
	case data, ok := <-h.payload:
		require.True(t, ok, "connection closed before a message arrived")
		msgType, err := cbor.DecodeIdFromList(data)
		require.NoError(t, err)
		msg, err := NewMsgFromCbor(uint(msgType), data)
		require.NoError(t, err)
		return msg
	case err := <-h.errs:
		t.Fatalf("protocol error while waiting for a message: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no message from the role under test")
	}
	return nil
}

func (h *modelHarness) receiveType(t *testing.T, msgType uint8) {
	t.Helper()
	msg := h.receive(t)
	require.Equal(t, msgType, msg.Type(), "got %T", msg)
}

func (h *modelHarness) requireViolation(t *testing.T, contains string) {
	t.Helper()
	select {
	case err := <-h.errs:
		require.ErrorContains(t, err, contains)
	case <-time.After(5 * time.Second):
		t.Fatal("message the model forbids was accepted without an error")
	}
}

// requireQuiet checks that, for a short while, the role under test neither
// reports an error, nor sends anything, nor closes the connection.
func (h *modelHarness) requireQuiet(t *testing.T) {
	t.Helper()
	select {
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	case data, ok := <-h.payload:
		if !ok {
			t.Fatal("connection closed")
		}
		t.Fatalf("unexpected message %x", data)
	case <-time.After(200 * time.Millisecond):
	}
}

// Errors protocol.Protocol reports for refused messages.
const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowedInState = "not allowed in current protocol state"
)

func violationError(row modelReceiveRow) string {
	if row.outcome == modelWaits {
		return errWithoutPeerAgency
	}
	return errNotAllowedInState
}

func newModelServer(t *testing.T, h *modelHarness, opts protocol.ProtocolOptions) *Server {
	t.Helper()
	server := NewServer(opts, &Config{
		AcquireFunc: func(CallbackContext, AcquireTarget, bool) error {
			return nil
		},
		QueryFunc: func(CallbackContext, QueryWrapper) (any, error) {
			return []uint64{1, modelBlockNo}, nil
		},
		ReleaseFunc: func(CallbackContext) error {
			h.released <- struct{}{}
			return nil
		},
	})
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	return server
}

// driveServer starts a server and moves it into state. A server cannot rest
// in Acquiring or Querying, since it answers inside its message handler; the
// caller skips those cells, which TestStateMapMatchesModel covers.
func driveServer(t *testing.T, state protocol.State) *modelHarness {
	t.Helper()
	h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
	newModelServer(t, h, opts)
	switch state {
	case stateAcquired:
		h.send(t, NewMsgAcquireVolatileTip())
		h.receiveType(t, MessageTypeAcquired)
	case stateDone:
		// The server cannot be observed resting in Done, so MsgDone and
		// the message under test go back to back; the pipe keeps order.
		h.send(t, NewMsgDone())
	}
	return h
}

// modelCall is a client call running in the background.
type modelCall chan error

func (c modelCall) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-c:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("client call did not return")
	}
	return nil
}

// driveClient starts a client and moves it into state. It returns the call
// that is waiting for the server's answer in Acquiring and Querying.
func driveClient(t *testing.T, state protocol.State) (*modelHarness, modelCall) {
	t.Helper()
	h, opts := newModelHarness(t, protocol.ProtocolRoleClient)
	client := NewClient(opts, &Config{
		AcquireTimeout: time.Minute,
		QueryTimeout:   time.Minute,
	})
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	call := make(modelCall, 1)
	switch state {
	case stateAcquiring, stateAcquired:
		go func() { call <- client.AcquireVolatileTip() }()
		h.receiveType(t, MessageTypeAcquireVolatileTip)
		if state == stateAcquired {
			h.send(t, NewMsgAcquired())
			require.NoError(t, call.wait(t))
		}
	case stateQuerying:
		go func() {
			blockNo, err := client.GetChainBlockNo()
			if err == nil && blockNo != modelBlockNo {
				err = fmt.Errorf("block number %d", blockNo)
			}
			call <- err
		}()
		// The query acquires the volatile tip itself first.
		h.receiveType(t, MessageTypeAcquireVolatileTip)
		h.send(t, NewMsgAcquired())
		h.receiveType(t, MessageTypeQuery)
	case stateDone:
		// The client API never sends MsgDone, so the test sends it through
		// the embedded protocol.
		require.NoError(t, client.SendMessage(NewMsgDone()))
		h.receiveType(t, MessageTypeDone)
		require.Eventually(t, client.IsDone, 5*time.Second, 5*time.Millisecond)
	}
	return h, call
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table it can rest in, sends the cell's message, and checks
// the outcome. Cells the model marks Waits are checked against gOuroboros's
// stricter behavior (see gouroborosRejectsEarly).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	for _, row := range lsqModelReceive {
		if row.role == protocol.ProtocolRoleServer &&
			(row.state == stateAcquiring || row.state == stateQuerying) {
			continue
		}
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			if row.role == protocol.ProtocolRoleServer {
				h := driveServer(t, row.state)
				h.sendKind(t, row.kind)
				checkServerOutcome(t, h, row)
				return
			}
			h, call := driveClient(t, row.state)
			h.sendKind(t, row.kind)
			checkClientOutcome(t, h, call, row)
		})
	}
}

func checkServerOutcome(t *testing.T, h *modelHarness, row modelReceiveRow) {
	t.Helper()
	switch gouroborosRejectsEarly(row.outcome) {
	case "Acquiring":
		h.receiveType(t, MessageTypeAcquired)
	case "Querying":
		h.receiveType(t, MessageTypeResult)
	case "Idle":
		select {
		case <-h.released:
		case err := <-h.errs:
			t.Fatalf("release refused: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("release was not handled")
		}
		h.requireQuiet(t)
	case "Done":
		h.requireQuiet(t)
		// The run ended; the next must still be served.
		h.send(t, NewMsgAcquireVolatileTip())
		h.receiveType(t, MessageTypeAcquired)
	case modelHeld:
		h.requireQuiet(t)
	case modelViolation:
		h.requireViolation(t, violationError(row))
	default:
		t.Fatalf("no check for outcome %q", row.outcome)
	}
}

func checkClientOutcome(
	t *testing.T,
	h *modelHarness,
	call modelCall,
	row modelReceiveRow,
) {
	t.Helper()
	switch gouroborosRejectsEarly(row.outcome) {
	case "Acquired":
		require.NoError(t, call.wait(t))
	case "Idle":
		require.ErrorIs(t, call.wait(t), ErrAcquireFailurePointTooOld)
	case modelHeld:
		h.requireQuiet(t)
	case modelViolation:
		h.requireViolation(t, violationError(row))
		if row.state == stateAcquiring || row.state == stateQuerying {
			// The waiting call must not be left blocked.
			require.ErrorIs(t, call.wait(t), protocol.ErrProtocolShuttingDown)
		}
	default:
		t.Fatalf("no check for outcome %q", row.outcome)
	}
}

// newModelPair connects a client to a gOuroboros server over a pipe.
func newModelPair(
	t *testing.T,
	queryFunc QueryFunc,
) (*Client, *Server, chan error) {
	t.Helper()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	errs := make(chan error, 16)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	version := uint16(23 + protocol.ProtocolVersionNtCOffset)
	server := NewServer(protocol.ProtocolOptions{
		Muxer: serverMuxer, ConnectionId: connID, ErrorChan: errs,
		Mode: protocol.ProtocolModeNodeToClient, Version: version,
	}, &Config{
		AcquireFunc: func(CallbackContext, AcquireTarget, bool) error {
			return nil
		},
		QueryFunc: queryFunc,
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer: clientMuxer, ConnectionId: connID, ErrorChan: errs,
		Mode: protocol.ProtocolModeNodeToClient, Version: version,
	}, &Config{AcquireTimeout: time.Minute, QueryTimeout: time.Minute})
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
		server.Stop(context.Background())
		client.Stop(context.Background())
		serverMuxer.Stop()
		clientMuxer.Stop()
	})
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	return client, server, errs
}

// TestConcurrentCallsGetOwnAnswers runs two kinds of query from many
// goroutines at once. busyMutex keeps one request outstanding, so each call
// must decode the answer to its own query; an answer reaching another call
// fails to decode or carries the other query's value (OwnAnswer in
// LocalStateQueryClient.tla).
func TestConcurrentCallsGetOwnAnswers(t *testing.T) {
	t.Parallel()
	client, _, errs := newModelPair(t, func(
		_ CallbackContext,
		w QueryWrapper,
	) (any, error) {
		switch w.Query.(type) {
		case *ChainBlockNoQuery:
			return []uint64{1, modelBlockNo}, nil
		case *ChainPointQuery:
			return modelPoint, nil
		default:
			return nil, fmt.Errorf("unexpected query %T", w.Query)
		}
	})
	const goroutines, calls = 8, 25
	failures := make(chan error, goroutines*calls)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			for range calls {
				if i%2 == 0 {
					blockNo, err := client.GetChainBlockNo()
					if err == nil && blockNo != modelBlockNo {
						err = fmt.Errorf("block number %d", blockNo)
					}
					if err != nil {
						failures <- fmt.Errorf("GetChainBlockNo: %w", err)
					}
					continue
				}
				point, err := client.GetChainPoint()
				if err == nil && point.Slot != modelPoint.Slot {
					err = fmt.Errorf("point slot %d", point.Slot)
				}
				if err != nil {
					failures <- fmt.Errorf("GetChainPoint: %w", err)
				}
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	select {
	case err := <-errs:
		t.Fatalf("protocol error: %v", err)
	default:
	}
}

// TestStopWithQueryOutstanding stops the client while the server is still
// computing a query's answer. The waiting call must return
// ErrProtocolShuttingDown rather than block, and Start must not bring the
// stopped client back: a restarted instance would meet the late MsgResult in
// Idle, the defect chain-sync's Stop and Start had.
func TestStopWithQueryOutstanding(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	unblock := make(chan struct{})
	client, _, _ := newModelPair(t, func(CallbackContext, QueryWrapper) (any, error) {
		close(entered)
		<-unblock
		return []uint64{1, modelBlockNo}, nil
	})
	// Registered after newModelPair's cleanup, so it runs first and frees
	// the server's handler before the server is stopped.
	t.Cleanup(func() { close(unblock) })
	call := make(modelCall, 1)
	go func() {
		_, err := client.GetChainBlockNo()
		call <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("query did not reach the server")
	}
	client.Stop(context.Background())
	require.ErrorIs(t, call.wait(t), protocol.ErrProtocolShuttingDown)
	client.Start()
	again := make(modelCall, 1)
	go func() {
		_, err := client.GetChainBlockNo()
		again <- err
	}()
	require.ErrorIs(t, again.wait(t), protocol.ErrProtocolShuttingDown)
}
