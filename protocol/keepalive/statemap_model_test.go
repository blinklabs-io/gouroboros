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

package keepalive

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
	"github.com/stretchr/testify/assert"
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

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindKeepAlive           modelKind = "MsgKeepAlive"
	kindKeepAliveResponse   modelKind = "MsgKeepAliveResponse"
	kindWrongCookieResponse modelKind = "WrongCookieResponse"
	kindDone                modelKind = "MsgDone"
	kindOversized           modelKind = "Oversized"
)

var modelKinds = []modelKind{
	kindKeepAlive,
	kindKeepAliveResponse,
	kindWrongCookieResponse,
	kindDone,
	kindOversized,
}

// modelReceiveRow is one cell of the keep-alive receive table: what a role
// in a given state does with a kind of message from its peer.
type modelReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    modelKind
	outcome string
}

// keepAliveModelReceive is the receive table of the keep-alive mini-protocol
// from the Ouroboros network specification, and from the reference
// implementation where the specification is silent. It is the full
// role x state x kind product, so a cell missing here is a cell nobody
// decided.
//
//   - Within a run a message is judged against the reader's state: one that
//     is not legal there aborts the connection ("Protocols as State
//     Machines" in docs/network-spec/miniprotocols.tex), as does a response
//     with the wrong cookie or a message over the state's size limit ("Keep
//     Alive Mini Protocol", same file).
//   - A side reads only while its peer holds agency (runPeerWithDriver in
//     typed-protocols Network.TypedProtocol.Driver), so a message that
//     arrives while the reader holds agency waits (modelWaits).
//   - A server whose run ended restarts in its initial state
//     ("Inbound side: the inbound governor" in
//     docs/network-spec/connection-manager.tex), so it judges a message
//     after MsgDone as the first of a new run. A client whose run ended
//     leaves the message unread (modelHeld).
var keepAliveModelReceive = []modelReceiveRow{
	{protocol.ProtocolRoleServer, StateClient, kindKeepAlive, "Server"},
	{protocol.ProtocolRoleServer, StateClient, kindKeepAliveResponse, modelViolation},
	{protocol.ProtocolRoleServer, StateClient, kindWrongCookieResponse, modelViolation},
	{protocol.ProtocolRoleServer, StateClient, kindDone, "Done"},
	{protocol.ProtocolRoleServer, StateClient, kindOversized, modelViolation},
	{protocol.ProtocolRoleServer, StateServer, kindKeepAlive, modelWaits},
	{protocol.ProtocolRoleServer, StateServer, kindKeepAliveResponse, modelWaits},
	{protocol.ProtocolRoleServer, StateServer, kindWrongCookieResponse, modelWaits},
	{protocol.ProtocolRoleServer, StateServer, kindDone, modelWaits},
	{protocol.ProtocolRoleServer, StateServer, kindOversized, modelWaits},
	{protocol.ProtocolRoleServer, StateDone, kindKeepAlive, "Server"},
	{protocol.ProtocolRoleServer, StateDone, kindKeepAliveResponse, modelViolation},
	{protocol.ProtocolRoleServer, StateDone, kindWrongCookieResponse, modelViolation},
	{protocol.ProtocolRoleServer, StateDone, kindDone, "Done"},
	{protocol.ProtocolRoleServer, StateDone, kindOversized, modelViolation},
	{protocol.ProtocolRoleClient, StateClient, kindKeepAlive, modelWaits},
	{protocol.ProtocolRoleClient, StateClient, kindKeepAliveResponse, modelWaits},
	{protocol.ProtocolRoleClient, StateClient, kindWrongCookieResponse, modelWaits},
	{protocol.ProtocolRoleClient, StateClient, kindDone, modelWaits},
	{protocol.ProtocolRoleClient, StateClient, kindOversized, modelWaits},
	{protocol.ProtocolRoleClient, StateServer, kindKeepAlive, modelViolation},
	{protocol.ProtocolRoleClient, StateServer, kindKeepAliveResponse, "Client"},
	{protocol.ProtocolRoleClient, StateServer, kindWrongCookieResponse, modelViolation},
	{protocol.ProtocolRoleClient, StateServer, kindDone, modelViolation},
	{protocol.ProtocolRoleClient, StateServer, kindOversized, modelViolation},
	{protocol.ProtocolRoleClient, StateDone, kindKeepAlive, modelHeld},
	{protocol.ProtocolRoleClient, StateDone, kindKeepAliveResponse, modelHeld},
	{protocol.ProtocolRoleClient, StateDone, kindWrongCookieResponse, modelHeld},
	{protocol.ProtocolRoleClient, StateDone, kindDone, modelHeld},
	{protocol.ProtocolRoleClient, StateDone, kindOversized, modelHeld},
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
// alone, following the checks protocol.Protocol applies: a message over the
// state's size limit is refused, as is one sent while the receiver holds
// agency, and one the state has no transition for. Cookies are checked by the
// client, not StateMap, so wrong-cookie cells are left to the running tests.
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
	if kind == kindOversized {
		require.Positive(t, entry.PendingMessageByteLimit)
		return modelViolation
	}
	for _, transition := range entry.Transitions {
		if transition.MsgType == modelKindType(kind) {
			return transition.NewState.String()
		}
	}
	return modelViolation
}

func modelKindType(kind modelKind) uint8 {
	switch kind {
	case kindKeepAlive, kindOversized:
		return MessageTypeKeepAlive
	case kindKeepAliveResponse, kindWrongCookieResponse:
		return MessageTypeKeepAliveResponse
	default:
		return MessageTypeDone
	}
}

// TestStateMapMatchesModel checks the within-run cells of the receive table
// against StateMap, including the cells no running client or server can be
// driven into, and pins the shape of StateMap the table depends on. The Done
// rows are about the next run, which the running tests cover.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(
		t,
		[]protocol.State{StateClient, StateServer, StateDone},
		stateMapStates(),
		"StateMap must hold exactly the states of the model",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[StateClient].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[StateServer].Agency)
	require.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
	for state, entry := range StateMap {
		require.Empty(
			t,
			entry.PipelinedMessageTypes,
			"keep-alive pipelines nothing, so %s must admit only the agency holder",
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
	for _, row := range keepAliveModelReceive {
		if row.state == StateDone || row.kind == kindWrongCookieResponse {
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

func stateMapStates() []protocol.State {
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	return states
}

// TestDoneCannotBeLeft checks that no transition leaves Done within a run,
// and that Done is entered only by MsgDone from Client. A message after Done
// belongs to a new run, not to a transition out of Done.
func TestDoneCannotBeLeft(t *testing.T) {
	t.Parallel()
	require.Empty(t, StateMap[StateDone].Transitions)
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			if transition.NewState != StateDone {
				continue
			}
			require.Equal(t, StateClient, state)
			require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
		}
	}
}

// modelHarness runs one keep-alive role over a muxer and plays the other
// side of the connection by hand.
type modelHarness struct {
	conn      net.Conn
	errs      chan error
	payload   chan []byte
	role      protocol.ProtocolRole
	responses chan uint16
}

func newModelHarness(t *testing.T, role protocol.ProtocolRole) (*modelHarness, protocol.ProtocolOptions) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	h := &modelHarness{
		conn:      connB,
		errs:      make(chan error, 8),
		payload:   make(chan []byte, 8),
		role:      role,
		responses: make(chan uint16, 4),
	}
	opts := protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		Muxer:     m,
		ErrorChan: h.errs,
		Mode:      protocol.ProtocolModeNodeToNode,
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

// sendRaw writes data as the peer would, split into muxer segments: from
// the responder when the role under test is the client, and from the
// initiator when it is the server. A write error is ignored, because the
// role under test may close the connection on refusing the message; the
// refusal is asserted through the error channel.
func (h *modelHarness) sendRaw(t *testing.T, data []byte) {
	t.Helper()
	const segmentMax = 0xffff
	for len(data) > 0 {
		n := min(len(data), segmentMax)
		segment := muxer.NewSegment(
			ProtocolId,
			data[:n],
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
		if _, err := h.conn.Write(buf.Bytes()); err != nil {
			return
		}
		data = data[n:]
	}
}

func (h *modelHarness) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	h.sendRaw(t, data)
}

// sendKind sends a message of the given kind. expectedCookie is the cookie
// a well-formed response must echo.
func (h *modelHarness) sendKind(t *testing.T, kind modelKind, expectedCookie uint16) {
	t.Helper()
	switch kind {
	case kindKeepAlive:
		h.send(t, NewMsgKeepAlive(expectedCookie))
	case kindKeepAliveResponse:
		h.send(t, NewMsgKeepAliveResponse(expectedCookie))
	case kindWrongCookieResponse:
		h.send(t, NewMsgKeepAliveResponse(expectedCookie+1))
	case kindDone:
		h.send(t, NewMsgDone())
	case kindOversized:
		data, err := cbor.Encode([]any{
			MessageTypeKeepAlive,
			make([]byte, MaxPendingMessageBytes+1),
		})
		require.NoError(t, err)
		h.sendRaw(t, data)
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

func (h *modelHarness) requireResponse(t *testing.T, cookie uint16) {
	t.Helper()
	reply, ok := h.receive(t).(*MsgKeepAliveResponse)
	require.True(t, ok, "MsgKeepAlive must be answered")
	require.Equal(t, cookie, reply.Cookie)
}

func (h *modelHarness) requireResponseHandled(t *testing.T, cookie uint16) {
	t.Helper()
	select {
	case got := <-h.responses:
		require.Equal(t, cookie, got)
	case err := <-h.errs:
		t.Fatalf("valid response refused: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("valid response was not handled")
	}
}

// Errors protocol.Protocol and the client report for refused messages.
const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowedInState = "not allowed in current protocol state"
	errOversized         = "received oversized message"
	errWrongCookie       = "unexpected cookie"
)

func violationError(row modelReceiveRow) string {
	switch {
	case row.outcome == modelWaits:
		return errWithoutPeerAgency
	case row.kind == kindOversized:
		return errOversized
	case row.kind == kindWrongCookieResponse && row.role == protocol.ProtocolRoleClient:
		return errWrongCookie
	default:
		return errNotAllowedInState
	}
}

const modelCookie uint16 = 0x0abc

// driveToState starts the role under test and moves it into state, returning
// the cookie a well-formed response must now echo. A server cannot rest in
// Server, since it answers a ping inside its message handler; the caller
// skips those cells.
func driveToState(t *testing.T, row modelReceiveRow) (*modelHarness, uint16) {
	t.Helper()
	h, opts := newModelHarness(t, row.role)
	if row.role == protocol.ProtocolRoleServer {
		server := NewServer(opts, nil)
		server.Start()
		t.Cleanup(server.Stop)
		if row.state == StateDone {
			// The server cannot be observed resting in Done, so MsgDone and
			// the message under test go back to back; the pipe keeps order.
			h.send(t, NewMsgDone())
		}
		return h, modelCookie
	}
	cfg := NewConfig(
		WithCookie(modelCookie),
		WithPeriod(time.Hour),
		WithOnKeepAliveResponseReceived(
			func(_ connection.ConnectionId, c uint16) { h.responses <- c },
		),
	)
	client := NewClient(opts, &cfg)
	client.Start()
	t.Cleanup(client.Stop)
	ping, ok := h.receive(t).(*MsgKeepAlive)
	require.True(t, ok, "client must open with MsgKeepAlive")
	require.Equal(t, modelCookie, ping.Cookie)
	if row.state == StateServer {
		return h, modelCookie
	}
	h.send(t, NewMsgKeepAliveResponse(modelCookie))
	h.requireResponseHandled(t, modelCookie)
	nextCookie := modelCookie + 1
	if row.state == StateDone {
		// The client API never sends MsgDone, so the test sends it through
		// the embedded protocol.
		require.NoError(t, client.SendMessage(NewMsgDone()))
		_, ok := h.receive(t).(*MsgDone)
		require.True(t, ok, "client must put MsgDone on the wire")
		require.Eventually(t, client.IsDone, 5*time.Second, 5*time.Millisecond)
	}
	return h, nextCookie
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table it can rest in, sends the cell's message, and checks
// the outcome. Cells the model marks Waits are checked against gOuroboros's
// stricter behavior (see gouroborosRejectsEarly).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	for _, row := range keepAliveModelReceive {
		if row.role == protocol.ProtocolRoleServer && row.state == StateServer {
			continue
		}
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			h, cookie := driveToState(t, row)
			h.sendKind(t, row.kind, cookie)
			switch gouroborosRejectsEarly(row.outcome) {
			case "Server":
				h.requireResponse(t, cookie)
			case "Client":
				h.requireResponseHandled(t, cookie)
			case modelViolation:
				h.requireViolation(t, violationError(row))
			case modelHeld:
				h.requireQuiet(t)
			case "Done":
				h.requireQuiet(t)
				// The run ended; the next must still be served.
				h.send(t, NewMsgKeepAlive(cookie))
				h.requireResponse(t, cookie)
			default:
				t.Fatalf("no check for outcome %q", row.outcome)
			}
		})
	}
}

// TestClientTimesOutWhenServerFallsSilent checks the Server state timeout of
// the specification's keep-alive timeout table: a client whose ping is never
// answered gives up instead of waiting forever.
func TestClientTimesOutWhenServerFallsSilent(t *testing.T) {
	t.Parallel()
	h, opts := newModelHarness(t, protocol.ProtocolRoleClient)
	cfg := NewConfig(
		WithPeriod(time.Hour),
		WithTimeout(50*time.Millisecond),
	)
	client := NewClient(opts, &cfg)
	client.Start()
	t.Cleanup(client.Stop)
	_, ok := h.receive(t).(*MsgKeepAlive)
	require.True(t, ok)
	h.requireViolation(t, "timeout waiting on transition from protocol state Server")
}

// TestStateMapTimeoutsMatchSpec pins the per-state timeouts to the
// specification's keep-alive timeout table: 97 s in Client and 60 s in
// Server. NewClient overrides the Server timeout from Config.Timeout.
func TestStateMapTimeoutsMatchSpec(t *testing.T) {
	t.Parallel()
	require.Equal(t, 97*time.Second, StateMap[StateClient].Timeout)
	require.Equal(t, 60*time.Second, StateMap[StateServer].Timeout)
}

// TestServerStopWinsOverRestart stops the server while it handles MsgDone,
// between reading MsgDone and installing the next run's instance. Stop must
// still stop the server: the restart must not bring keep-alive back.
func TestServerStopWinsOverRestart(t *testing.T) {
	t.Parallel()
	h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
	var server *Server
	stopped := make(chan struct{})
	cfg := NewConfig(WithDoneFunc(func(CallbackContext) error {
		server.Stop()
		close(stopped)
		return nil
	}))
	server = NewServer(opts, &cfg)
	original := server.ProtocolInstance()
	server.Start()
	t.Cleanup(server.Stop)
	h.send(t, NewMsgDone())
	<-stopped
	// The ping must arrive after any restart has had time to run; one sent
	// with MsgDone is read, and dropped, by the instance being stopped.
	restarted := func() bool { return server.ProtocolInstance() != original }
	if !assert.Never(t, restarted, 200*time.Millisecond, 5*time.Millisecond,
		"a stopped server installed a new protocol instance") {
		return
	}
	h.send(t, NewMsgKeepAlive(modelCookie))
	h.requireQuiet(t)
}
