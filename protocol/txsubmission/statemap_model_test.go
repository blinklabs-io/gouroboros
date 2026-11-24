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

package txsubmission

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"slices"
	"sync"
	"sync/atomic"
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
	// message yet; it is judged once the peer holds agency again.
	modelWaits = "Waits"
	// modelHeld: the client's run is over and no new run reads the message.
	modelHeld = "Held"
	// modelQueued is gOuroboros' form of modelWaits for a request the
	// server pipelines behind one the client is still answering: the
	// client keeps it and handles it after its reply.
	modelQueued = "Queued"
)

const (
	roleClient = protocol.ProtocolRoleClient
	roleServer = protocol.ProtocolRoleServer
)

// modelKind is a kind of message a peer can send. MsgRequestTxIds is two
// kinds, because its blocking flag picks the state it leads to.
type modelKind string

const (
	kindInit                    modelKind = "MsgInit"
	kindRequestTxIdsBlocking    modelKind = "MsgRequestTxIdsBlocking"
	kindRequestTxIdsNonBlocking modelKind = "MsgRequestTxIdsNonBlocking"
	kindReplyTxIds              modelKind = "MsgReplyTxIds"
	kindRequestTxs              modelKind = "MsgRequestTxs"
	kindReplyTxs                modelKind = "MsgReplyTxs"
	kindDone                    modelKind = "MsgDone"
)

// modelReceiveRow is one cell of the receive table: what a role in a given
// state does with a kind of message from its peer.
type modelReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    modelKind
	outcome string
}

// txSubmissionModelReceive is the receive table of the node-to-node
// tx-submission mini-protocol. It is the full role x state x kind product, so a
// cell missing here is a cell nobody decided. Sources, at
// IntersectMBO/ouroboros-network 5f356486ab3e:
//
//   - States, agency and transitions: "Tx-Submission mini-protocol" in
//     docs/network-spec/miniprotocols.tex, and TxSubmission2/Type.hs.
//   - A message the reader's state does not allow aborts the connection
//     ("Protocols as State Machines" in the same file); the reference codec
//     decodes each message against the reader's state (TxSubmission2/Codec.hs).
//   - A side reads only while its peer holds agency (typed-protocols
//     Network.TypedProtocol.Driver), so a message that arrives while the
//     reader holds agency waits (modelWaits). This is how the reference
//     client takes requests the server pipelines.
//   - A responder that terminates cleanly restarts in its initial state
//     ("Inbound side: the inbound governor" in connection-manager.tex), so
//     the server judges a message after MsgDone as the first of a new run.
//     A client whose run ended leaves the message unread (modelHeld).
var txSubmissionModelReceive = []modelReceiveRow{
	{roleServer, stateInit, kindInit, "Idle"},
	{roleServer, stateInit, kindRequestTxIdsBlocking, modelViolation},
	{roleServer, stateInit, kindRequestTxIdsNonBlocking, modelViolation},
	{roleServer, stateInit, kindReplyTxIds, modelViolation},
	{roleServer, stateInit, kindRequestTxs, modelViolation},
	{roleServer, stateInit, kindReplyTxs, modelViolation},
	{roleServer, stateInit, kindDone, modelViolation},
	{roleServer, stateIdle, kindInit, modelWaits},
	{roleServer, stateIdle, kindRequestTxIdsBlocking, modelWaits},
	{roleServer, stateIdle, kindRequestTxIdsNonBlocking, modelWaits},
	{roleServer, stateIdle, kindReplyTxIds, modelWaits},
	{roleServer, stateIdle, kindRequestTxs, modelWaits},
	{roleServer, stateIdle, kindReplyTxs, modelWaits},
	{roleServer, stateIdle, kindDone, modelWaits},
	{roleServer, stateTxIdsBlocking, kindInit, modelViolation},
	{roleServer, stateTxIdsBlocking, kindRequestTxIdsBlocking, modelViolation},
	{
		roleServer,
		stateTxIdsBlocking,
		kindRequestTxIdsNonBlocking,
		modelViolation,
	},
	{roleServer, stateTxIdsBlocking, kindReplyTxIds, "Idle"},
	{roleServer, stateTxIdsBlocking, kindRequestTxs, modelViolation},
	{roleServer, stateTxIdsBlocking, kindReplyTxs, modelViolation},
	{roleServer, stateTxIdsBlocking, kindDone, "Done"},
	{roleServer, stateTxIdsNonblocking, kindInit, modelViolation},
	{
		roleServer,
		stateTxIdsNonblocking,
		kindRequestTxIdsBlocking,
		modelViolation,
	},
	{
		roleServer,
		stateTxIdsNonblocking,
		kindRequestTxIdsNonBlocking,
		modelViolation,
	},
	{roleServer, stateTxIdsNonblocking, kindReplyTxIds, "Idle"},
	{roleServer, stateTxIdsNonblocking, kindRequestTxs, modelViolation},
	{roleServer, stateTxIdsNonblocking, kindReplyTxs, modelViolation},
	{roleServer, stateTxIdsNonblocking, kindDone, modelViolation},
	{roleServer, stateTxs, kindInit, modelViolation},
	{roleServer, stateTxs, kindRequestTxIdsBlocking, modelViolation},
	{roleServer, stateTxs, kindRequestTxIdsNonBlocking, modelViolation},
	{roleServer, stateTxs, kindReplyTxIds, modelViolation},
	{roleServer, stateTxs, kindRequestTxs, modelViolation},
	{roleServer, stateTxs, kindReplyTxs, "Idle"},
	{roleServer, stateTxs, kindDone, modelViolation},
	{roleServer, stateDone, kindInit, "Idle"},
	{roleServer, stateDone, kindRequestTxIdsBlocking, modelViolation},
	{roleServer, stateDone, kindRequestTxIdsNonBlocking, modelViolation},
	{roleServer, stateDone, kindReplyTxIds, modelViolation},
	{roleServer, stateDone, kindRequestTxs, modelViolation},
	{roleServer, stateDone, kindReplyTxs, modelViolation},
	{roleServer, stateDone, kindDone, modelViolation},
	{roleClient, stateInit, kindInit, modelWaits},
	{roleClient, stateInit, kindRequestTxIdsBlocking, modelWaits},
	{roleClient, stateInit, kindRequestTxIdsNonBlocking, modelWaits},
	{roleClient, stateInit, kindReplyTxIds, modelWaits},
	{roleClient, stateInit, kindRequestTxs, modelWaits},
	{roleClient, stateInit, kindReplyTxs, modelWaits},
	{roleClient, stateInit, kindDone, modelWaits},
	{roleClient, stateIdle, kindInit, modelViolation},
	{roleClient, stateIdle, kindRequestTxIdsBlocking, "TxIdsBlocking"},
	{roleClient, stateIdle, kindRequestTxIdsNonBlocking, "TxIdsNonBlocking"},
	{roleClient, stateIdle, kindReplyTxIds, modelViolation},
	{roleClient, stateIdle, kindRequestTxs, "Txs"},
	{roleClient, stateIdle, kindReplyTxs, modelViolation},
	{roleClient, stateIdle, kindDone, modelViolation},
	{roleClient, stateTxIdsBlocking, kindInit, modelWaits},
	{roleClient, stateTxIdsBlocking, kindRequestTxIdsBlocking, modelWaits},
	{roleClient, stateTxIdsBlocking, kindRequestTxIdsNonBlocking, modelWaits},
	{roleClient, stateTxIdsBlocking, kindReplyTxIds, modelWaits},
	{roleClient, stateTxIdsBlocking, kindRequestTxs, modelWaits},
	{roleClient, stateTxIdsBlocking, kindReplyTxs, modelWaits},
	{roleClient, stateTxIdsBlocking, kindDone, modelWaits},
	{roleClient, stateTxIdsNonblocking, kindInit, modelWaits},
	{roleClient, stateTxIdsNonblocking, kindRequestTxIdsBlocking, modelWaits},
	{
		roleClient,
		stateTxIdsNonblocking,
		kindRequestTxIdsNonBlocking,
		modelWaits,
	},
	{roleClient, stateTxIdsNonblocking, kindReplyTxIds, modelWaits},
	{roleClient, stateTxIdsNonblocking, kindRequestTxs, modelWaits},
	{roleClient, stateTxIdsNonblocking, kindReplyTxs, modelWaits},
	{roleClient, stateTxIdsNonblocking, kindDone, modelWaits},
	{roleClient, stateTxs, kindInit, modelWaits},
	{roleClient, stateTxs, kindRequestTxIdsBlocking, modelWaits},
	{roleClient, stateTxs, kindRequestTxIdsNonBlocking, modelWaits},
	{roleClient, stateTxs, kindReplyTxIds, modelWaits},
	{roleClient, stateTxs, kindRequestTxs, modelWaits},
	{roleClient, stateTxs, kindReplyTxs, modelWaits},
	{roleClient, stateTxs, kindDone, modelWaits},
	{roleClient, stateDone, kindInit, modelHeld},
	{roleClient, stateDone, kindRequestTxIdsBlocking, modelHeld},
	{roleClient, stateDone, kindRequestTxIdsNonBlocking, modelHeld},
	{roleClient, stateDone, kindReplyTxIds, modelHeld},
	{roleClient, stateDone, kindRequestTxs, modelHeld},
	{roleClient, stateDone, kindReplyTxs, modelHeld},
	{roleClient, stateDone, kindDone, modelHeld},
}

func modelRoleName(role protocol.ProtocolRole) string {
	if role == roleClient {
		return "client"
	}
	return "server"
}

func (r modelReceiveRow) name() string {
	return fmt.Sprintf("%s/in%s/%s", modelRoleName(r.role), r.state, r.kind)
}

func isRequestKind(kind modelKind) bool {
	return kind == kindRequestTxIdsBlocking ||
		kind == kindRequestTxIdsNonBlocking ||
		kind == kindRequestTxs
}

// gouroborosOutcome maps a cell onto gOuroboros, which judges a message the
// moment it arrives. A request the server pipelines while the client
// answers an earlier non-blocking or body request is kept and handled after
// the reply (modelQueued). Any other message sent while the receiver holds
// agency is refused at once. The specification does not say when such a
// message is judged, so this is a recorded difference, not a defect: the
// reference server never sends one (it pipelines only behind those two
// requests, Server.hs), and a peer that does is out of protocol either way.
func gouroborosOutcome(row modelReceiveRow) string {
	if row.outcome != modelWaits {
		return row.outcome
	}
	if row.role == roleClient &&
		(row.state == stateTxIdsNonblocking || row.state == stateTxs) &&
		isRequestKind(row.kind) {
		return modelQueued
	}
	return modelViolation
}

// modelMessage returns a message of the given kind whose content is valid
// wherever the kind is allowed, so that only the state decides the outcome.
// ack is what a blocking request acknowledges: every id the client will
// hold when it reads the request.
func modelMessage(kind modelKind, ack uint16) protocol.Message {
	switch kind {
	case kindInit:
		return NewMsgInit()
	case kindRequestTxIdsBlocking:
		return NewMsgRequestTxIds(true, ack, 1)
	case kindRequestTxIdsNonBlocking:
		return NewMsgRequestTxIds(false, 0, 1)
	case kindReplyTxIds:
		return NewMsgReplyTxIds(
			[]TxIdAndSize{{TxId: modelTxId(1), Size: modelTxSize}},
		)
	case kindRequestTxs:
		return NewMsgRequestTxs([]TxId{modelTxId(1)})
	case kindReplyTxs:
		return NewMsgReplyTxs(nil)
	default:
		return NewMsgDone()
	}
}

const modelTxSize = 100

func modelTxId(n byte) TxId {
	return TxId{EraId: 6, TxId: [32]byte{n}}
}

// stateMapReceive derives the outcome of one within-run cell from StateMap
// alone, following the checks protocol.Protocol applies: a message sent
// while the receiver holds agency is queued if the state lists it as
// pipelined and refused otherwise, and one the state has no transition for
// is refused.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := StateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	msg := modelMessage(kind, 0)
	peerAgency := protocol.AgencyServer
	if role == roleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		if slices.Contains(entry.PipelinedMessageTypes, msg.Type()) {
			return modelQueued
		}
		return modelViolation
	}
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

// TestStateMapMatchesModel checks every within-run cell of the receive
// table against StateMap, including cells no running client or server can
// be driven into, and pins the shape of StateMap the table depends on. The
// Done rows are about the next run, which TestReceiveMatchesModel covers.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	require.ElementsMatch(
		t,
		[]protocol.State{
			stateInit, stateIdle, stateTxIdsBlocking,
			stateTxIdsNonblocking, stateTxs, stateDone,
		},
		states,
		"StateMap must hold exactly the states of the reference",
	)
	agency := map[protocol.State]protocol.ProtocolStateAgency{
		stateInit:             protocol.AgencyClient,
		stateIdle:             protocol.AgencyServer,
		stateTxIdsBlocking:    protocol.AgencyClient,
		stateTxIdsNonblocking: protocol.AgencyClient,
		stateTxs:              protocol.AgencyClient,
		stateDone:             protocol.AgencyNone,
	}
	for state, want := range agency {
		require.Equal(t, want, StateMap[state].Agency, "agency of %s", state)
	}
	require.Len(t, txSubmissionModelReceive, 2*len(agency)*7)
	for _, row := range txSubmissionModelReceive {
		if row.state == stateDone {
			continue
		}
		t.Run(row.name(), func(t *testing.T) {
			require.Equal(
				t,
				gouroborosOutcome(row),
				stateMapReceive(t, row.role, row.state, row.kind),
			)
		})
	}
}

// TestStatesThatCannotBeEntered checks the transitions into the two states the
// reference enters only one way: nothing leads back into Init, and only MsgDone
// in TxIdsBlocking leads into Done, which nothing leaves within a run. MsgDone
// answering a non-blocking request is the case the specification rules out by
// name.
func TestStatesThatCannotBeEntered(t *testing.T) {
	t.Parallel()
	require.Empty(t, StateMap[stateDone].Transitions)
	for state, entry := range StateMap {
		for _, transition := range entry.Transitions {
			require.NotEqual(
				t,
				stateInit,
				transition.NewState,
				"%s leads back into Init",
				state,
			)
			if transition.NewState != stateDone {
				continue
			}
			require.Equal(t, stateTxIdsBlocking, state)
			require.Equal(t, uint8(MessageTypeDone), transition.MsgType)
		}
	}
}

// modelHarness runs one tx-submission role over a muxer and plays the
// other side of the connection by hand.
type modelHarness struct {
	conn    net.Conn
	errs    chan error
	payload chan []byte
	role    protocol.ProtocolRole
	opts    protocol.ProtocolOptions
}

func newModelHarness(
	t *testing.T,
	role protocol.ProtocolRole,
) *modelHarness {
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
	return h
}

// readPeer delivers each message the role under test sends, one per
// h.payload value. A segment is not a message: the sender may put several
// queued messages in one segment, or split one across segments, so the
// payloads are joined and cut at CBOR item boundaries.
func (h *modelHarness) readPeer() {
	defer close(h.payload)
	var buf []byte
	for {
		var header muxer.SegmentHeader
		if err := binary.Read(h.conn, binary.BigEndian, &header); err != nil {
			return
		}
		data := make([]byte, header.PayloadLength)
		if _, err := io.ReadFull(h.conn, data); err != nil {
			return
		}
		buf = append(buf, data...)
		for len(buf) > 0 {
			var item cbor.RawMessage
			n, err := cbor.Decode(buf, &item)
			if err != nil {
				// An incomplete item waits for the next segment.
				break
			}
			h.payload <- buf[:n:n]
			buf = buf[n:]
		}
	}
}

// send writes msg as the peer would: from the responder when the role
// under test is the client, and from the initiator when it is the server.
// A write error is ignored, because the role under test may close the
// connection on refusing an earlier message; refusals are asserted through
// the error channel.
func (h *modelHarness) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	segment := muxer.NewSegment(ProtocolId, data, h.role == roleClient)
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(
		t,
		binary.Write(buf, binary.BigEndian, &segment.SegmentHeader),
	)
	buf.Write(segment.Payload)
	require.NoError(
		t,
		h.conn.SetWriteDeadline(time.Now().Add(5*time.Second)),
	)
	_, _ = h.conn.Write(buf.Bytes())
}

// receive returns the next message the role under test sent to the peer,
// and checks its type.
func (h *modelHarness) receive(t *testing.T, msgType uint8) protocol.Message {
	t.Helper()
	select {
	case data, ok := <-h.payload:
		require.True(t, ok, "connection closed before a message arrived")
		id, err := cbor.DecodeIdFromList(data)
		require.NoError(t, err)
		msg, err := NewMsgFromCbor(uint(id), data)
		require.NoError(t, err)
		require.Equal(t, msgType, msg.Type(), "unexpected message %T", msg)
		return msg
	case err := <-h.errs:
		t.Fatalf("protocol error while waiting for a message: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("no message of type %d from the role under test", msgType)
	}
	return nil
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
// reports an error nor sends anything.
func (h *modelHarness) requireQuiet(t *testing.T) {
	t.Helper()
	select {
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	case data, ok := <-h.payload:
		if ok {
			t.Fatalf("unexpected message %x", data)
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func waitFor[T any](t *testing.T, h *modelHarness, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case err := <-h.errs:
		t.Fatalf("protocol error while waiting for %s: %v", what, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

// clientModel is a client under test whose callbacks report each call and
// can be told to hold, to let the test act while the client has agency, or
// to stop the session.
type clientModel struct {
	*modelHarness
	client   *Client
	idsCalls chan bool
	txsCalls chan int
	hold     atomic.Bool
	stopNext atomic.Bool
	holding  chan struct{}
	release  chan struct{}
	released sync.Once
	nextId   atomic.Uint32
}

func (c *clientModel) maybeHold() {
	if c.hold.CompareAndSwap(true, false) {
		c.holding <- struct{}{}
		<-c.release
	}
}

func (c *clientModel) releaseHold() {
	c.released.Do(func() { close(c.release) })
}

// holdAndSend makes the client's next callback hold, sends msg, and waits
// until the callback holds.
func (c *clientModel) holdAndSend(t *testing.T, msg protocol.Message) {
	t.Helper()
	c.hold.Store(true)
	c.send(t, msg)
	waitFor(t, c.modelHarness, c.holding, "the callback to hold")
}

// driveClient starts a client and moves it into state. Every state but
// Init starts the same way: one blocking request announces id 1, which
// the client then holds unacknowledged, so every later request below is
// one the reference client accepts too.
func driveClient(t *testing.T, state protocol.State) *clientModel {
	t.Helper()
	c := &clientModel{
		modelHarness: newModelHarness(t, roleClient),
		idsCalls:     make(chan bool, 8),
		txsCalls:     make(chan int, 8),
		holding:      make(chan struct{}, 1),
		release:      make(chan struct{}),
	}
	cfg := &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			blocking bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			c.idsCalls <- blocking
			if c.stopNext.CompareAndSwap(true, false) {
				return nil, ErrStopServerProcess
			}
			c.maybeHold()
			if req == 0 {
				return nil, nil
			}
			id := modelTxId(byte(c.nextId.Add(1)))
			return []TxIdAndSize{{TxId: id, Size: modelTxSize}}, nil
		},
		RequestTxsFunc: func(_ CallbackContext, ids []TxId) ([]TxBody, error) {
			c.txsCalls <- len(ids)
			c.maybeHold()
			return nil, nil
		},
	}
	c.client = NewClient(c.opts, cfg)
	c.client.Start()
	t.Cleanup(func() {
		c.releaseHold()
		_ = c.client.Stop()
	})
	if state == stateInit {
		return c
	}
	c.client.Init()
	c.receive(t, MessageTypeInit)
	c.send(t, NewMsgRequestTxIds(true, 0, 1))
	require.True(t, waitFor(t, c.modelHarness, c.idsCalls, "a txid request"))
	c.receive(t, MessageTypeReplyTxIds)
	switch state {
	case stateTxIdsBlocking:
		c.holdAndSend(t, NewMsgRequestTxIds(true, 1, 1))
		<-c.idsCalls
	case stateTxIdsNonblocking:
		c.holdAndSend(t, NewMsgRequestTxIds(false, 0, 1))
		<-c.idsCalls
	case stateTxs:
		c.holdAndSend(t, NewMsgRequestTxs([]TxId{modelTxId(1)}))
		<-c.txsCalls
	case stateDone:
		c.stopNext.Store(true)
		c.send(t, NewMsgRequestTxIds(true, 1, 1))
		<-c.idsCalls
		c.receive(t, MessageTypeDone)
	}
	return c
}

// clientAck is what a blocking request sent in state acknowledges to leave
// nothing outstanding when the client reads it: id 1, and in
// TxIdsNonBlocking also the id the held reply announces.
func clientAck(state protocol.State) uint16 {
	if state == stateTxIdsNonblocking {
		return 2
	}
	return 1
}

// checkClientServes checks the client handles kind as a request: its
// callback runs with the right blocking flag and the reply goes out.
func (c *clientModel) checkClientServes(t *testing.T, kind modelKind) {
	t.Helper()
	if kind == kindRequestTxs {
		waitFor(t, c.modelHarness, c.txsCalls, "a body request")
		c.receive(t, MessageTypeReplyTxs)
		return
	}
	blocking := waitFor(t, c.modelHarness, c.idsCalls, "a txid request")
	require.Equal(t, kind == kindRequestTxIdsBlocking, blocking)
	c.receive(t, MessageTypeReplyTxIds)
}

// serverModel is a server under test with each result and notification it
// reports.
type serverModel struct {
	*modelHarness
	server    *Server
	inits     chan struct{}
	dones     chan struct{}
	idsResult chan error
	txsResult chan error
}

func (s *serverModel) requestTxIds(t *testing.T) {
	t.Helper()
	go func() {
		_, err := s.server.RequestTxIds(true, 1)
		s.idsResult <- err
	}()
	s.receive(t, MessageTypeRequestTxIds)
}

// driveServer starts a server and moves it into state.
func driveServer(t *testing.T, state protocol.State) *serverModel {
	t.Helper()
	s := &serverModel{
		modelHarness: newModelHarness(t, roleServer),
		inits:        make(chan struct{}, 2),
		dones:        make(chan struct{}, 1),
		idsResult:    make(chan error, 2),
		txsResult:    make(chan error, 1),
	}
	cfg := NewConfig(
		WithOnInit(func(connection.ConnectionId) { s.inits <- struct{}{} }),
		WithOnDone(func(connection.ConnectionId) { s.dones <- struct{}{} }),
	)
	s.server = NewServer(s.opts, &cfg)
	s.server.Start()
	t.Cleanup(s.server.Stop)
	if state == stateInit {
		return s
	}
	s.send(t, NewMsgInit())
	waitFor(t, s.modelHarness, s.inits, "MsgInit")
	switch state {
	case stateTxIdsBlocking:
		s.requestTxIds(t)
	case stateTxIdsNonblocking:
		// The server's own RequestTxIds acknowledges every id it holds, so
		// a request that leaves ids outstanding, which a non-blocking one
		// must, is sent through the protocol directly.
		s.server.stateMu.Lock()
		s.server.pendingTxIds = &pendingTxIdsRequest{requested: 1}
		s.server.stateMu.Unlock()
		require.NoError(
			t,
			s.server.ProtocolInstance().
				SendMessage(NewMsgRequestTxIds(false, 0, 1)),
		)
		s.receive(t, MessageTypeRequestTxIds)
	case stateTxs:
		s.requestTxIds(t)
		s.send(t, modelMessage(kindReplyTxIds, 0))
		require.NoError(t, waitFor(t, s.modelHarness, s.idsResult, "txids"))
		go func() {
			_, err := s.server.RequestTxs([]TxId{modelTxId(1)})
			s.txsResult <- err
		}()
		s.receive(t, MessageTypeRequestTxs)
	case stateDone:
		s.requestTxIds(t)
		s.send(t, NewMsgDone())
		require.ErrorIs(
			t,
			waitFor(t, s.modelHarness, s.idsResult, "the Done result"),
			ErrStopServerProcess,
		)
		waitFor(t, s.modelHarness, s.dones, "OnDone")
	}
	return s
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

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table, sends the cell's message, and checks the outcome
// against gouroborosOutcome. Every message is one the receiver would accept
// in a state that allows it, so only the state decides.
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	for _, row := range txSubmissionModelReceive {
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			if row.role == roleClient {
				checkClientRow(t, row)
				return
			}
			checkServerRow(t, row)
		})
	}
}

func checkClientRow(t *testing.T, row modelReceiveRow) {
	t.Helper()
	c := driveClient(t, row.state)
	c.send(t, modelMessage(row.kind, clientAck(row.state)))
	switch outcome := gouroborosOutcome(row); outcome {
	case stateTxIdsBlocking.String(),
		stateTxIdsNonblocking.String(),
		stateTxs.String():
		c.checkClientServes(t, row.kind)
	case modelQueued:
		c.requireQuiet(t)
		c.releaseHold()
		if row.state == stateTxs {
			c.receive(t, MessageTypeReplyTxs)
		} else {
			c.receive(t, MessageTypeReplyTxIds)
		}
		c.checkClientServes(t, row.kind)
	case modelViolation:
		c.requireViolation(t, violationError(row))
	case modelHeld:
		c.requireQuiet(t)
	default:
		t.Fatalf("no check for outcome %q", outcome)
	}
}

func checkServerRow(t *testing.T, row modelReceiveRow) {
	t.Helper()
	s := driveServer(t, row.state)
	s.send(t, modelMessage(row.kind, 1))
	switch outcome := gouroborosOutcome(row); outcome {
	case stateIdle.String():
		switch row.kind {
		case kindInit:
			waitFor(t, s.modelHarness, s.inits, "a second MsgInit")
		case kindReplyTxs:
			require.NoError(
				t,
				waitFor(t, s.modelHarness, s.txsResult, "bodies"),
			)
		default:
			ids := s.idsResult
			if row.state == stateTxIdsNonblocking {
				s.server.stateMu.Lock()
				ch := s.server.requestTxIdsResultChan
				s.server.stateMu.Unlock()
				result := waitFor(t, s.modelHarness, ch, "txids")
				require.NoError(t, result.err)
				require.Len(t, result.txIds, 1)
				return
			}
			require.NoError(t, waitFor(t, s.modelHarness, ids, "txids"))
		}
	case stateDone.String():
		require.ErrorIs(
			t,
			waitFor(t, s.modelHarness, s.idsResult, "the Done result"),
			ErrStopServerProcess,
		)
		waitFor(t, s.modelHarness, s.dones, "OnDone")
	case modelViolation:
		s.requireViolation(t, violationError(row))
	default:
		t.Fatalf("no check for outcome %q", outcome)
	}
}

// TestClientSendsDoneOnlyForBlocking checks the sending side of the rule
// that MsgDone answers only a blocking request: a RequestTxIdsFunc asking
// to stop during a non-blocking request fails the client instead of
// putting MsgDone on the wire, where the server would refuse it.
func TestClientSendsDoneOnlyForBlocking(t *testing.T) {
	t.Parallel()
	c := driveClient(t, stateIdle)
	c.stopNext.Store(true)
	c.send(t, NewMsgRequestTxIds(false, 0, 1))
	c.requireViolation(t, "cannot stop server process during a non-blocking")
	select {
	case data, ok := <-c.payload:
		if ok {
			t.Fatalf("the client sent %x after refusing to stop", data)
		}
	default:
	}
}
