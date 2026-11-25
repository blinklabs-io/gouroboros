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

package localtxsubmission

import (
	"bytes"
	"encoding/binary"
	"errors"
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

const (
	stIdle          = "StIdle"
	stBusy          = "StBusy"
	stDone          = "StDone"
	kindSubmitTx    = "MsgSubmitTx"
	kindAcceptTx    = "MsgAcceptTx"
	kindRejectTx    = "MsgRejectTx"
	kindDone        = "MsgDone"
	modelRoleClient = "client"
	modelRoleServer = "server"
)

var (
	modelStates = []string{stIdle, stBusy, stDone}
	modelKinds  = []string{kindSubmitTx, kindAcceptTx, kindRejectTx, kindDone}
)

// modelAgency is who may send in each state: "Local Tx-Submission state
// agencies" in docs/network-spec/miniprotocols.tex and StateAgency in the
// reference's LocalTxSubmission/Type.hs. Nobody may send in StDone.
var modelAgency = map[string]string{
	stIdle: modelRoleClient,
	stBusy: modelRoleServer,
	stDone: "",
}

// modelTransitions is the local tx-submission state machine of
// docs/network-spec/miniprotocols.tex (lines 1840-1886) and the reference's
// LocalTxSubmission/Type.hs.
var modelTransitions = []struct{ from, kind, to string }{
	{stIdle, kindSubmitTx, stBusy},
	{stIdle, kindDone, stDone},
	{stBusy, kindAcceptTx, stIdle},
	{stBusy, kindRejectTx, stIdle},
}

func modelPeer(role string) string {
	if role == modelRoleClient {
		return modelRoleServer
	}
	return modelRoleClient
}

func modelJudge(state, kind string) string {
	for _, tr := range modelTransitions {
		if tr.from == state && tr.kind == kind {
			return tr.to
		}
	}
	return modelViolation
}

// modelReceive is the receive table with the reference's settings: a message is
// judged against the reader's state, or waits while the reader holds agency; a
// server whose run ended restarts in StIdle, as the inbound governor restarts
// responders ("Inbound side: the inbound governor" in
// docs/network-spec/connection-manager.tex); a client whose run ended leaves it
// unread.
func modelReceive(role, state, kind string) string {
	if state == stDone {
		if role == modelRoleServer {
			return modelJudge(stIdle, kind)
		}
		return modelHeld
	}
	if modelAgency[state] != modelPeer(role) {
		return modelWaits
	}
	return modelJudge(state, kind)
}

// gouroborosReceive maps the reference onto gOuroboros where they differ, a
// recorded deviation: protocol.Protocol refuses a message sent while the
// receiver holds agency instead of leaving it to wait.
func gouroborosReceive(role, state, kind string) string {
	outcome := modelReceive(role, state, kind)
	if outcome == modelWaits {
		return modelViolation
	}
	return outcome
}

const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowedInState = "not allowed in current protocol state"
)

func gouroborosViolationError(role, state, kind string) string {
	if modelReceive(role, state, kind) == modelWaits {
		return errWithoutPeerAgency
	}
	return errNotAllowedInState
}

func modelGoState(state string) protocol.State {
	switch state {
	case stIdle:
		return stateIdle
	case stBusy:
		return stateBusy
	default:
		return stateDone
	}
}

func modelKindType(kind string) uint8 {
	switch kind {
	case kindSubmitTx:
		return MessageTypeSubmitTx
	case kindAcceptTx:
		return MessageTypeAcceptTx
	case kindRejectTx:
		return MessageTypeRejectTx
	default:
		return MessageTypeDone
	}
}

// stateMapReceive derives the outcome of one within-run cell from StateMap
// alone, following the checks protocol.Protocol applies: a message sent
// while the receiver holds agency is refused, as is one the state has no
// transition for.
func stateMapReceive(t *testing.T, role, state, kind string) string {
	t.Helper()
	entry, ok := StateMap[modelGoState(state)]
	require.True(t, ok, "StateMap has no entry for %s", state)
	peerAgency := protocol.AgencyServer
	if role == modelRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return modelViolation
	}
	for _, transition := range entry.Transitions {
		if transition.MsgType == modelKindType(kind) {
			return "St" + transition.NewState.String()
		}
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

// TestStateMapMatchesModel checks the within-run cells of the receive table
// against StateMap, including the server's StBusy cells, which no running
// server can be driven into, and pins the shape of StateMap the table
// depends on.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	states := make([]protocol.State, 0, len(StateMap))
	for state, entry := range StateMap {
		states = append(states, state)
		require.Empty(t, entry.PipelinedMessageTypes,
			"local-tx-submission pipelines nothing, so %s admits only the agency holder", state)
		for _, transition := range entry.Transitions {
			require.Nil(t, transition.MatchFunc,
				"%s transitions are decided by message type alone", state)
		}
	}
	require.ElementsMatch(t, []protocol.State{stateIdle, stateBusy, stateDone}, states)
	modelRows(func(role, state, kind string) {
		if state == stDone {
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

// TestDoneCannotBeLeft checks that no transition leaves Done, and that Done
// is entered only by MsgDone from Idle.
func TestDoneCannotBeLeft(t *testing.T) {
	t.Parallel()
	require.Equal(t, protocol.AgencyNone, StateMap[stateDone].Agency)
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

// modelHarness runs one local-tx-submission role over a muxer and plays the
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

// send writes msg as the peer would. A write error is ignored, because the
// role under test may close the connection on refusing the message; the
// refusal is asserted through the error channel.
func (h *modelHarness) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	segment := muxer.NewSegment(ProtocolId, data, h.role == modelRoleClient)
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	buf.Write(segment.Payload)
	require.NoError(t, h.conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	_, _ = h.conn.Write(buf.Bytes())
}

// modelRejectReason is a reason the client's decoder accepts:
// [0, [1, ["foo"]]].
var modelRejectReason = []byte{0x82, 0x00, 0x82, 0x01, 0x81, 0x63, 0x66, 0x6f, 0x6f}

func modelKindMessage(kind string) protocol.Message {
	switch kind {
	case kindSubmitTx:
		return NewMsgSubmitTx(6, placeholderTx)
	case kindAcceptTx:
		return NewMsgAcceptTx()
	case kindRejectTx:
		return NewMsgRejectTx(modelRejectReason)
	default:
		return NewMsgDone()
	}
}

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
		t.Fatal("message the reference forbids was accepted without an error")
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

// driveClient starts a client and moves it into state, returning the
// result of a SubmitTx left waiting in StBusy.
func driveClient(t *testing.T, h *modelHarness, state string) <-chan error {
	t.Helper()
	client := NewClient(h.opts, nil)
	client.Start()
	t.Cleanup(client.Protocol.Stop)
	switch state {
	case stBusy:
		result := make(chan error, 1)
		go func() { result <- client.SubmitTx(6, placeholderTx) }()
		h.requireReceive(t, MessageTypeSubmitTx)
		return result
	case stDone:
		require.NoError(t, client.Stop())
		h.requireReceive(t, MessageTypeDone)
	}
	return nil
}

// driveServer starts a server that accepts every transaction. A server
// cannot rest in StBusy, since it answers inside its message handler; the
// caller skips those cells.
func driveServer(t *testing.T, h *modelHarness, state string) {
	t.Helper()
	cfg := NewConfig(WithSubmitTxFunc(
		func(CallbackContext, MsgSubmitTxTransaction) error { return nil },
	))
	server := NewServer(h.opts, &cfg)
	server.Start()
	t.Cleanup(server.Stop)
	if state == stDone {
		// The server cannot be observed resting in Done, so MsgDone and the
		// message under test go back to back; the pipe keeps order.
		h.send(t, NewMsgDone())
	}
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table it can rest in, sends the cell's message, and checks
// the outcome against the reference as gOuroboros deviates from it (see
// gouroborosReceive).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	modelRows(func(role, state, kind string) {
		if role == modelRoleServer && state == stBusy {
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
			h.send(t, modelKindMessage(kind))
			outcome := gouroborosReceive(role, state, kind)
			switch {
			case outcome == modelViolation:
				h.requireViolation(t, gouroborosViolationError(role, state, kind))
			case outcome == modelHeld, outcome == stDone:
				h.requireQuiet(t)
			case role == modelRoleServer:
				// The only state a server reads its way into and leaves on
				// its own is StBusy: it answers.
				require.Equal(t, stBusy, outcome)
				h.requireReceive(t, MessageTypeAcceptTx)
			default:
				// A client reads its way back to StIdle: SubmitTx returns
				// the answer.
				require.Equal(t, stIdle, outcome)
				select {
				case err := <-pending:
					if kind == kindAcceptTx {
						require.NoError(t, err)
					} else {
						var rejected TransactionRejectedError
						require.True(t, errors.As(err, &rejected), "got %v", err)
					}
				case err := <-h.errs:
					t.Fatalf("protocol error: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("SubmitTx did not return")
				}
			}
		})
	})
}
