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

package localtxmonitor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Receive outcomes besides the state the receiver moves to.
const (
	// modelViolation: the receiver refuses the message and reports a
	// protocol error, which tears the connection down.
	modelViolation = "Violation"
	// modelWaits: the receiver holds agency, so it does not read the
	// message yet.
	modelWaits = "Waits"
	// modelHeld: the run is over and nothing reads the message.
	modelHeld = "Held"
)

// The states of the local-tx-monitor model. The busy state's kind is part
// of its name, as in the reference's StBusy StBusyKind.
const (
	stIdle             = "StIdle"
	stAcquiring        = "StAcquiring"
	stAcquired         = "StAcquired"
	stBusyNextTx       = "StBusyNextTx"
	stBusyHasTx        = "StBusyHasTx"
	stBusyGetSizes     = "StBusyGetSizes"
	stBusyGetMeasures  = "StBusyGetMeasures"
	stDone             = "StDone"
	kindAcquire        = "MsgAcquire"
	kindAcquired       = "MsgAcquired"
	kindRelease        = "MsgRelease"
	kindNextTx         = "MsgNextTx"
	kindReplyNextTx    = "MsgReplyNextTx"
	kindHasTx          = "MsgHasTx"
	kindReplyHasTx     = "MsgReplyHasTx"
	kindGetSizes       = "MsgGetSizes"
	kindReplyGetSizes  = "MsgReplyGetSizes"
	kindGetMeasures    = "MsgGetMeasures"
	kindReplyMeasures  = "MsgReplyGetMeasures"
	kindDone           = "MsgDone"
	modelRoleClient    = "client"
	modelRoleServer    = "server"
	modelAgencyClient  = "Client"
	modelAgencyServer  = "Server"
	modelAgencyNobody  = "Nobody"
	modelMeasuresQuery = 11
	modelMeasuresReply = 12
)

var modelStates = []string{
	stIdle, stAcquiring, stAcquired,
	stBusyNextTx, stBusyHasTx, stBusyGetSizes, stBusyGetMeasures,
	stDone,
}

var modelKinds = []string{
	kindAcquire, kindAcquired, kindRelease,
	kindNextTx, kindReplyNextTx, kindHasTx, kindReplyHasTx,
	kindGetSizes, kindReplyGetSizes, kindGetMeasures, kindReplyMeasures,
	kindDone,
}

// modelAgency is who may send in each state: the agency table of the
// specification ("Local Tx-Monitor state agencies" in
// docs/network-spec/miniprotocols.tex) and StateAgency in the reference's
// LocalTxMonitor/Type.hs.
var modelAgency = map[string]string{
	stIdle:            modelAgencyClient,
	stAcquiring:       modelAgencyServer,
	stAcquired:        modelAgencyClient,
	stBusyNextTx:      modelAgencyServer,
	stBusyHasTx:       modelAgencyServer,
	stBusyGetSizes:    modelAgencyServer,
	stBusyGetMeasures: modelAgencyServer,
	stDone:            modelAgencyNobody,
}

// modelTransitions is Transitions in specs/09-localtx/LocalTxMonitor.tla of
// gouroboros-tla-models: the specification's transition table, with
// StBusy HasTx answered by MsgReplyHasTx as the reference's Type.hs has it
// (the table's MsgReplyNextTx there is a typo). MsgAcquire from StAcquired
// is MsgAwaitAcquire: the reference encodes both as [1].
var modelTransitions = []struct{ from, kind, to string }{
	{stIdle, kindAcquire, stAcquiring},
	{stIdle, kindDone, stDone},
	{stAcquiring, kindAcquired, stAcquired},
	{stAcquired, kindAcquire, stAcquiring},
	{stAcquired, kindRelease, stIdle},
	{stAcquired, kindNextTx, stBusyNextTx},
	{stAcquired, kindHasTx, stBusyHasTx},
	{stAcquired, kindGetSizes, stBusyGetSizes},
	{stAcquired, kindGetMeasures, stBusyGetMeasures},
	{stBusyNextTx, kindReplyNextTx, stAcquired},
	{stBusyHasTx, kindReplyHasTx, stAcquired},
	{stBusyGetSizes, kindReplyGetSizes, stAcquired},
	{stBusyGetMeasures, kindReplyMeasures, stAcquired},
}

func modelPeerAgency(role string) string {
	if role == modelRoleClient {
		return modelAgencyServer
	}
	return modelAgencyClient
}

func modelJudge(state, kind string) string {
	for _, tr := range modelTransitions {
		if tr.from == state && tr.kind == kind {
			return tr.to
		}
	}
	return modelViolation
}

// modelReceive is ReceiveTable in LocalTxMonitor.tla with the reference's
// settings: a message is judged against the reader's state, or waits while
// the reader holds agency; a server whose run ended restarts in StIdle, as
// the inbound governor restarts responders ("Inbound side: the inbound
// governor" in docs/network-spec/connection-manager.tex); a client whose
// run ended leaves it unread.
func modelReceive(role, state, kind string) string {
	if state == stDone {
		if role == modelRoleServer {
			return modelJudge(stIdle, kind)
		}
		return modelHeld
	}
	if modelAgency[state] != modelPeerAgency(role) {
		return modelWaits
	}
	return modelJudge(state, kind)
}

func isMeasuresKind(kind string) bool {
	return kind == kindGetMeasures || kind == kindReplyMeasures
}

// gouroborosReceive maps the model onto gOuroboros where they differ, each
// a recorded deviation, not a defect:
//   - protocol.Protocol refuses a message sent while the receiver holds
//     agency instead of leaving it to wait.
//   - gOuroboros has no MsgGetMeasures or MsgReplyGetMeasures, so a reader
//     that would judge one cannot decode it.
//   - The server does not restart after MsgDone, so what follows it is
//     never read.
func gouroborosReceive(role, state, kind string) string {
	outcome := modelReceive(role, state, kind)
	switch {
	case outcome == modelWaits:
		return modelViolation
	case state == stDone && role == modelRoleServer:
		return modelHeld
	case outcome == modelHeld:
		return modelHeld
	case isMeasuresKind(kind):
		return modelViolation
	}
	return outcome
}

// Errors protocol.Protocol and NewMsgFromCbor report for refused messages.
const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowedInState = "not allowed in current protocol state"
	errUnknownType       = "unknown message type"
)

func gouroborosViolationError(role, state, kind string) string {
	switch {
	case modelReceive(role, state, kind) == modelWaits:
		return errWithoutPeerAgency
	case isMeasuresKind(kind):
		return errUnknownType
	default:
		return errNotAllowedInState
	}
}

var modelBusyQuery = map[string]uint8{
	stBusyNextTx:   MessageTypeNextTx,
	stBusyHasTx:    MessageTypeHasTx,
	stBusyGetSizes: MessageTypeGetSizes,
}

func modelBusyOf(query uint8) string {
	for state, q := range modelBusyQuery {
		if q == query {
			return state
		}
	}
	return ""
}

// modelGoState is the gOuroboros state for a model state, and the query
// its stateContext records in a busy state.
func modelGoState(state string) (protocol.State, uint8, bool) {
	switch state {
	case stIdle:
		return stateIdle, 0, true
	case stAcquiring:
		return stateAcquiring, 0, true
	case stAcquired:
		return stateAcquired, 0, true
	case stDone:
		return stateDone, 0, true
	}
	query, ok := modelBusyQuery[state]
	return stateBusy, query, ok
}

func modelKindType(kind string) uint8 {
	switch kind {
	case kindAcquire:
		return MessageTypeAcquire
	case kindAcquired:
		return MessageTypeAcquired
	case kindRelease:
		return MessageTypeRelease
	case kindNextTx:
		return MessageTypeNextTx
	case kindReplyNextTx:
		return MessageTypeReplyNextTx
	case kindHasTx:
		return MessageTypeHasTx
	case kindReplyHasTx:
		return MessageTypeReplyHasTx
	case kindGetSizes:
		return MessageTypeGetSizes
	case kindReplyGetSizes:
		return MessageTypeReplyGetSizes
	case kindGetMeasures:
		return modelMeasuresQuery
	case kindReplyMeasures:
		return modelMeasuresReply
	default:
		return MessageTypeDone
	}
}

// stateMapReceive derives the outcome of one within-run cell from StateMap
// and NewMsgFromCbor, following the checks protocol.Protocol applies: a
// message sent while the receiver holds agency is refused, one that does
// not decode is refused, and otherwise a transition must exist for it and
// its MatchFunc must accept it against the query the busy state recorded.
func stateMapReceive(t *testing.T, role, state, kind string) string {
	t.Helper()
	goState, query, ok := modelGoState(state)
	require.True(t, ok, "no gOuroboros state for %s", state)
	entry, ok := StateMap[goState]
	require.True(t, ok, "StateMap has no entry for %s", goState)
	peerAgency := protocol.AgencyServer
	if role == modelRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return modelViolation
	}
	msgType := modelKindType(kind)
	if _, err := NewMsgFromCbor(uint(msgType), modelKindCbor(t, kind)); err != nil {
		return modelViolation
	}
	msg := &protocol.MessageBase{MessageType: msgType}
	for _, transition := range entry.Transitions {
		if transition.MsgType != msgType {
			continue
		}
		ctx := &stateContext{}
		ctx.query.Store(uint32(query))
		if transition.MatchFunc != nil && !transition.MatchFunc(ctx, msg) {
			continue
		}
		if transition.NewState == stateBusy {
			return modelBusyOf(uint8(ctx.query.Load())) //nolint:gosec // stores a uint8
		}
		return "St" + transition.NewState.String()
	}
	return modelViolation
}

func modelRows(f func(role, state, kind string)) {
	for _, role := range []string{modelRoleClient, modelRoleServer} {
		for _, state := range modelStates {
			for _, kind := range modelKinds {
				f(role, state, kind)
			}
		}
	}
}

// TestStateMapMatchesModel checks every within-run cell of the receive table
// against StateMap, including the cells no running client or server can be
// driven into. StBusy GetMeasures cannot be entered at all.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(
		t,
		[]protocol.State{stateIdle, stateAcquiring, stateAcquired, stateBusy, stateDone},
		stateMapStates(),
		"StateMap must hold the model's states, with one Busy for the kinds",
	)
	require.Len(t, queryReplyTypes, len(modelBusyQuery),
		"Busy must have one kind per query gOuroboros knows")
	modelRows(func(role, state, kind string) {
		if state == stDone || state == stBusyGetMeasures {
			return
		}
		t.Run(role+"/in"+state+"/"+kind, func(t *testing.T) {
			require.Equal(
				t,
				gouroborosReceive(role, state, kind),
				stateMapReceive(t, role, state, kind),
			)
		})
	})
}

func stateMapStates() []protocol.State {
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	return states
}

// TestDoneCannotBeLeft checks that no transition leaves Done, and that Done
// is entered only by MsgDone from Idle: in particular not from Acquired, so
// a client must release its snapshot before it may finish.
func TestDoneCannotBeLeft(t *testing.T) {
	t.Parallel()
	require.Empty(t, StateMap[stateDone].Transitions)
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			if transition.NewState != stateDone {
				continue
			}
			require.Equal(t, stateIdle, state)
			require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
		}
	}
}

// TestBusyGetMeasuresCannotBeEntered checks that gOuroboros has no
// MsgGetMeasures: the query does not decode, so neither role can reach the
// model's StBusy GetMeasures. The reference understands it at every
// node-to-client version.
func TestBusyGetMeasuresCannotBeEntered(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{kindGetMeasures, kindReplyMeasures} {
		_, err := NewMsgFromCbor(uint(modelKindType(kind)), modelKindCbor(t, kind))
		require.ErrorContains(t, err, errUnknownType, kind)
	}
	for _, entry := range StateMap {
		for _, transition := range entry.Transitions {
			require.NotEqual(t, uint8(modelMeasuresQuery), transition.MsgType)
		}
	}
}

// modelHarness runs one local-tx-monitor role over a muxer and plays the
// other side of the connection by hand.
type modelHarness struct {
	conn    net.Conn
	errs    chan error
	payload chan []byte
	role    string
	opts    protocol.ProtocolOptions
}

func newModelHarness(t *testing.T, role string) *modelHarness {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	h := &modelHarness{
		conn:    connB,
		errs:    make(chan error, 8),
		payload: make(chan []byte, 8),
		role:    role,
	}
	h.opts = protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		Muxer:     m,
		ErrorChan: h.errs,
		Mode:      protocol.ProtocolModeNodeToClient,
	}
	// net.Pipe writes block until read, so the peer must always be reading.
	go h.readPeer()
	t.Cleanup(func() {
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	m.Start()
	return h
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

// sendRaw writes data as the peer would. A write error is ignored, because
// the role under test may close the connection on refusing the message; the
// refusal is asserted through the error channel.
func (h *modelHarness) sendRaw(t *testing.T, data []byte) {
	t.Helper()
	segment := muxer.NewSegment(ProtocolId, data, h.role == modelRoleClient)
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	buf.Write(segment.Payload)
	require.NoError(t, h.conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	_, _ = h.conn.Write(buf.Bytes())
}

var modelTxId = bytes.Repeat([]byte{0xab}, 32)

// modelKindCbor is the concrete message each kind stands for. gOuroboros
// cannot build the GetMeasures pair, so their CBOR is written out from the
// reference codec: [11] and [12, txCount, {measure => [size, capacity]}].
func modelKindCbor(t *testing.T, kind string) []byte {
	t.Helper()
	var msg protocol.Message
	switch kind {
	case kindAcquire:
		msg = NewMsgAcquire()
	case kindAcquired:
		msg = NewMsgAcquired(7)
	case kindRelease:
		msg = NewMsgRelease()
	case kindNextTx:
		msg = NewMsgNextTx()
	case kindReplyNextTx:
		msg = NewMsgReplyNextTx(0, nil)
	case kindHasTx:
		msg = NewMsgHasTxForEra(6, modelTxId)
	case kindReplyHasTx:
		msg = NewMsgReplyHasTx(true)
	case kindGetSizes:
		msg = NewMsgGetSizes()
	case kindReplyGetSizes:
		msg = NewMsgReplyGetSizes(1, 2, 3)
	case kindGetMeasures:
		return []byte{0x81, 0x0b}
	case kindReplyMeasures:
		return []byte{0x83, 0x0c, 0x00, 0xa0}
	default:
		msg = NewMsgDone()
	}
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	return data
}

// receive returns the type of the next message the role under test sent.
func (h *modelHarness) receive(t *testing.T) uint8 {
	t.Helper()
	select {
	case data, ok := <-h.payload:
		require.True(t, ok, "connection closed before a message arrived")
		msgType, err := cbor.DecodeIdFromList(data)
		require.NoError(t, err)
		return uint8(msgType) //nolint:gosec // message types are small
	case err := <-h.errs:
		t.Fatalf("protocol error while waiting for a message: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no message from the role under test")
	}
	return 0
}

func (h *modelHarness) requireReceive(t *testing.T, want uint8) {
	t.Helper()
	require.Equal(t, want, h.receive(t))
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

func requireCallResult(t *testing.T, h *modelHarness, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		require.NoError(t, err)
	case err := <-h.errs:
		t.Fatalf("protocol error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return")
	}
}

// modelQueryCall starts the client call that sends the query of a busy
// state, and returns its result.
func modelQueryCall(client *Client, state string) <-chan error {
	result := make(chan error, 1)
	go func() {
		var err error
		switch state {
		case stBusyNextTx:
			_, err = client.NextTx()
		case stBusyHasTx:
			_, err = client.HasTxForEra(6, modelTxId)
		default:
			_, _, _, err = client.GetSizes()
		}
		result <- err
	}()
	return result
}

// driveClient starts a client and moves it into state. It returns the
// result of the call left waiting there, if any.
func driveClient(t *testing.T, h *modelHarness, state string) <-chan error {
	t.Helper()
	client := NewClient(h.opts, nil)
	client.Start()
	t.Cleanup(client.Protocol.Stop)
	if state == stIdle {
		return nil
	}
	if state == stDone {
		require.NoError(t, client.Stop())
		h.requireReceive(t, MessageTypeDone)
		return nil
	}
	acquired := make(chan error, 1)
	go func() { acquired <- client.Acquire() }()
	h.requireReceive(t, MessageTypeAcquire)
	if state == stAcquiring {
		return acquired
	}
	h.sendRaw(t, modelKindCbor(t, kindAcquired))
	requireCallResult(t, h, acquired)
	if state == stAcquired {
		return nil
	}
	result := modelQueryCall(client, state)
	h.requireReceive(t, modelBusyQuery[state])
	return result
}

func modelMempool(CallbackContext) (uint64, uint32, []TxAndEraId, error) {
	return 7, 100, nil, nil
}

// driveServer starts a server and moves it into state. A server cannot rest
// in StAcquiring or a busy state, since it answers inside its message
// handler; the caller skips those cells.
func driveServer(t *testing.T, h *modelHarness, state string) {
	t.Helper()
	cfg := NewConfig(WithGetMempoolFunc(modelMempool))
	server := NewServer(h.opts, &cfg)
	server.Start()
	t.Cleanup(server.Stop)
	switch state {
	case stAcquired:
		h.sendRaw(t, modelKindCbor(t, kindAcquire))
		h.requireReceive(t, MessageTypeAcquired)
	case stDone:
		// The server cannot be observed resting in Done, so MsgDone and the
		// message under test go back to back; the pipe keeps order.
		h.sendRaw(t, modelKindCbor(t, kindDone))
	}
}

var modelReplyOf = map[string]uint8{
	stBusyNextTx:   MessageTypeReplyNextTx,
	stBusyHasTx:    MessageTypeReplyHasTx,
	stBusyGetSizes: MessageTypeReplyGetSizes,
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table it can rest in, sends the cell's message, and checks
// the outcome against the model as gOuroboros deviates from it (see
// gouroborosReceive).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	modelRows(func(role, state, kind string) {
		if state == stBusyGetMeasures {
			return
		}
		if role == modelRoleServer && state != stIdle && state != stAcquired && state != stDone {
			return
		}
		t.Run(fmt.Sprintf("%s/in%s/%s", role, state, kind), func(t *testing.T) {
			t.Parallel()
			h := newModelHarness(t, role)
			var pending <-chan error
			if role == modelRoleClient {
				pending = driveClient(t, h, state)
			} else {
				driveServer(t, h, state)
			}
			h.sendRaw(t, modelKindCbor(t, kind))
			outcome := gouroborosReceive(role, state, kind)
			switch {
			case outcome == modelViolation:
				h.requireViolation(t, gouroborosViolationError(role, state, kind))
			case outcome == modelHeld:
				h.requireQuiet(t)
			case role == modelRoleClient:
				// Every state a client reads its way into is StAcquired:
				// the waiting Acquire or query call returns.
				require.Equal(t, stAcquired, outcome)
				requireCallResult(t, h, pending)
			case outcome == stAcquiring:
				h.requireReceive(t, MessageTypeAcquired)
			case outcome == stIdle:
				// Released: MsgDone is legal now, MsgNextTx would not be.
				h.requireQuiet(t)
				h.sendRaw(t, modelKindCbor(t, kindDone))
				h.requireQuiet(t)
			case outcome == stDone:
				h.requireQuiet(t)
			default:
				h.requireReceive(t, modelReplyOf[outcome])
			}
		})
	})
}

// TestAwaitAcquireIsAcquireOnTheWire checks that MsgAcquire is [1], which the reference decodes in StAcquired as
// MsgAwaitAcquire, and that the gOuroboros server answers it at once with a
// fresh snapshot. The reference server waits instead until the mempool
// differs from the held snapshot, so this pins a recorded deviation.
func TestAwaitAcquireIsAcquireOnTheWire(t *testing.T) {
	t.Parallel()
	awaitBytes, err := cbor.Encode(NewMsgAcquire())
	require.NoError(t, err)
	require.Equal(t, []byte{0x81, 0x01}, awaitBytes)

	server := newModelHarness(t, modelRoleServer)
	driveServer(t, server, stAcquired)
	server.sendRaw(t, awaitBytes)
	server.requireReceive(t, MessageTypeAcquired)
}
