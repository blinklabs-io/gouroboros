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

package handshake

import (
	"bytes"
	"context"
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
	// refViolation: the receiver refuses the message and reports a
	// protocol error, which tears the connection down.
	refViolation = "Violation"
	// refWaits: the receiver holds agency, so it does not read the
	// message yet.
	refWaits = "Waits"
	// refError: the run ends, but the client rejects the server's answer.
	refError = "Error"
)

// refKind is a kind of message a peer can send.
type refKind string

const (
	kindPropose    refKind = "MsgProposeVersions"
	kindAccept     refKind = "MsgAcceptVersion"
	kindRefuse     refKind = "MsgRefuse"
	kindQueryReply refKind = "MsgQueryReply"
	kindOversized  refKind = "Oversized"
)

// refReceiveRow is one cell of the handshake receive table: what a role in
// a given state does with a kind of message from its peer.
type refReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    refKind
	outcome string
}

// handshakeReferenceReceive is the receive table of the handshake mini-protocol
// from the Ouroboros network specification and, where they differ, the
// reference implementation. It is the full role x state x kind product. Both
// sides use the same one-version table, which does not ask for a query, so
// MsgQueryReply here is one the client did not ask for.
//
//   - A message that is not legal in the reader's state, or is over the
//     state's 5760-byte limit, aborts the connection ("Handshake
//     mini-protocol" in docs/network-spec/miniprotocols.tex).
//   - In StConfirm the reference reads MsgProposeVersions as
//     MsgReplyVersions, the simultaneous-open answer, and the client picks
//     the version itself (Handshake/Type.hs, Codec.hs, Client.hs).
//   - The reference client takes any MsgQueryReply as the query result
//     (Handshake/Client.hs).
//   - After the handshake the connection no longer runs mini-protocol 0, so
//     a further handshake message is for an unknown mini-protocol and tears
//     the connection down (network-mux Network/Mux/Ingress.hs).
var handshakeReferenceReceive = []refReceiveRow{
	{protocol.ProtocolRoleServer, statePropose, kindPropose, "Confirm"},
	{protocol.ProtocolRoleServer, statePropose, kindAccept, refViolation},
	{protocol.ProtocolRoleServer, statePropose, kindRefuse, refViolation},
	{protocol.ProtocolRoleServer, statePropose, kindQueryReply, refViolation},
	{protocol.ProtocolRoleServer, statePropose, kindOversized, refViolation},
	{protocol.ProtocolRoleServer, stateConfirm, kindPropose, refWaits},
	{protocol.ProtocolRoleServer, stateConfirm, kindAccept, refWaits},
	{protocol.ProtocolRoleServer, stateConfirm, kindRefuse, refWaits},
	{protocol.ProtocolRoleServer, stateConfirm, kindQueryReply, refWaits},
	{protocol.ProtocolRoleServer, stateConfirm, kindOversized, refWaits},
	{protocol.ProtocolRoleServer, stateDone, kindPropose, refViolation},
	{protocol.ProtocolRoleServer, stateDone, kindAccept, refViolation},
	{protocol.ProtocolRoleServer, stateDone, kindRefuse, refViolation},
	{protocol.ProtocolRoleServer, stateDone, kindQueryReply, refViolation},
	{protocol.ProtocolRoleServer, stateDone, kindOversized, refViolation},
	{protocol.ProtocolRoleClient, statePropose, kindPropose, refWaits},
	{protocol.ProtocolRoleClient, statePropose, kindAccept, refWaits},
	{protocol.ProtocolRoleClient, statePropose, kindRefuse, refWaits},
	{protocol.ProtocolRoleClient, statePropose, kindQueryReply, refWaits},
	{protocol.ProtocolRoleClient, statePropose, kindOversized, refWaits},
	{protocol.ProtocolRoleClient, stateConfirm, kindPropose, "Done"},
	{protocol.ProtocolRoleClient, stateConfirm, kindAccept, "Done"},
	{protocol.ProtocolRoleClient, stateConfirm, kindRefuse, "Done"},
	{protocol.ProtocolRoleClient, stateConfirm, kindQueryReply, "Done"},
	{protocol.ProtocolRoleClient, stateConfirm, kindOversized, refViolation},
	{protocol.ProtocolRoleClient, stateDone, kindPropose, refViolation},
	{protocol.ProtocolRoleClient, stateDone, kindAccept, refViolation},
	{protocol.ProtocolRoleClient, stateDone, kindRefuse, refViolation},
	{protocol.ProtocolRoleClient, stateDone, kindQueryReply, refViolation},
	{protocol.ProtocolRoleClient, stateDone, kindOversized, refViolation},
}

// gouroborosOutcome maps a cell of the reference table onto what gOuroboros
// does, where the two differ. Each difference is recorded, not a defect:
//
//   - protocol.Protocol judges a message the moment it arrives, so one sent
//     while the receiver holds agency is refused rather than left to wait.
//   - The client refuses a MsgQueryReply it did not ask for. An honest
//     server only answers a query, so this is stricter, not incompatible.
//   - After Done, the protocol is stopped and the muxer drops further
//     handshake segments instead of closing the connection.
func gouroborosOutcome(row refReceiveRow) string {
	switch {
	case row.outcome == refWaits:
		return refViolation
	case row.role == protocol.ProtocolRoleClient && row.state == stateConfirm &&
		row.kind == kindQueryReply:
		return refError
	case row.state == stateDone:
		return refDropped
	}
	return row.outcome
}

// refDropped is gOuroboros's outcome for a handshake message after Done:
// nothing is reported and nothing is sent.
const refDropped = "Dropped"

func refRoleName(role protocol.ProtocolRole) string {
	if role == protocol.ProtocolRoleClient {
		return "client"
	}
	return "server"
}

func (r refReceiveRow) name() string {
	return fmt.Sprintf("%s/in%s/%s", refRoleName(r.role), r.state, r.kind)
}

func refKindType(kind refKind) uint8 {
	switch kind {
	case kindPropose, kindOversized:
		return MessageTypeProposeVersions
	case kindAccept:
		return MessageTypeAcceptVersion
	case kindRefuse:
		return MessageTypeRefuse
	default:
		return MessageTypeQueryReply
	}
}

// stateMapReceive derives the outcome of one within-run cell from a state
// map alone, following the checks protocol.Protocol applies: a message sent
// while the receiver holds agency is refused, as is one over the state's
// size limit and one the state has no transition for. What the client makes
// of a legal answer is left to the running tests.
func stateMapReceive(
	t *testing.T,
	stateMap protocol.StateMap,
	row refReceiveRow,
) string {
	t.Helper()
	entry, ok := stateMap[row.state]
	require.True(t, ok, "state map has no entry for %s", row.state)
	peerAgency := protocol.AgencyServer
	if row.role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		return refViolation
	}
	if row.kind == kindOversized {
		require.Equal(t, MaxPendingMessageBytes, entry.PendingMessageByteLimit)
		return refViolation
	}
	for _, transition := range entry.Transitions {
		if transition.MsgType == refKindType(row.kind) {
			return transition.NewState.String()
		}
	}
	return refViolation
}

// stateMapOutcome is the reference outcome as far as the state map decides
// it: a client's Error ends in Done.
func stateMapOutcome(row refReceiveRow) string {
	outcome := gouroborosOutcome(row)
	if outcome == refError {
		return stateDone.String()
	}
	return outcome
}

var refStateMaps = map[string]protocol.StateMap{
	"NtN": StateMapNtN,
	"NtC": StateMapNtC,
}

// TestStateMapMatchesReference checks the within-run cells of the receive table
// against both state maps, including the cells no running client or server
// can be driven into, and pins the shape the table depends on. The Done rows
// are about what happens after the protocol stops, which the running tests
// cover.
func TestStateMapMatchesReference(t *testing.T) {
	t.Parallel()
	for mode, stateMap := range refStateMaps {
		require.Len(t, stateMap, 3, "%s: exactly Propose, Confirm and Done", mode)
		require.Equal(t, protocol.AgencyClient, stateMap[statePropose].Agency, mode)
		require.Equal(t, protocol.AgencyServer, stateMap[stateConfirm].Agency, mode)
		require.Equal(t, protocol.AgencyNone, stateMap[stateDone].Agency, mode)
		for state, entry := range stateMap {
			require.Empty(t, entry.PipelinedMessageTypes, "%s %s pipelines nothing", mode, state)
			for _, transition := range entry.Transitions {
				require.Nil(t, transition.MatchFunc,
					"%s %s transitions are decided by message type alone", mode, state)
			}
		}
		for _, row := range handshakeReferenceReceive {
			if row.state == stateDone {
				continue
			}
			t.Run(mode+"/"+row.name(), func(t *testing.T) {
				require.Equal(t, stateMapOutcome(row), stateMapReceive(t, stateMap, row))
			})
		}
	}
}

// TestDoneCannotBeLeft checks that no transition leaves Done, that only the
// server's three answers enter it, and that only MsgProposeVersions enters
// Confirm, in both state maps.
func TestDoneCannotBeLeft(t *testing.T) {
	t.Parallel()
	for mode, stateMap := range refStateMaps {
		require.Empty(t, stateMap[stateDone].Transitions, mode)
		for state, entry := range stateMap {
			for _, transition := range entry.Transitions {
				switch transition.NewState {
				case stateDone:
					require.Equal(t, stateConfirm, state, mode)
					require.Contains(t,
						[]uint8{
							MessageTypeAcceptVersion,
							MessageTypeRefuse,
							MessageTypeQueryReply,
							// MsgReplyVersions, the simultaneous-open answer.
							MessageTypeProposeVersions,
						},
						transition.MsgType, mode)
				case stateConfirm:
					require.Equal(t, statePropose, state, mode)
					require.Equal(t, uint8(MessageTypeProposeVersions), transition.MsgType, mode)
				default:
					t.Fatalf("%s: unexpected transition %s -> %s", mode, state, transition.NewState)
				}
			}
		}
	}
}

// TestStateMapLimitsMatchSpec pins the per-state limits to the
// specification's handshake tables: 5760 bytes in both states, in both modes;
// a 10 s timeout in both states for node-to-node, and none for
// node-to-client.
func TestStateMapLimitsMatchSpec(t *testing.T) {
	t.Parallel()
	for _, state := range []protocol.State{statePropose, stateConfirm} {
		require.Equal(t, 5760, StateMapNtN[state].PendingMessageByteLimit)
		require.Equal(t, 5760, StateMapNtC[state].PendingMessageByteLimit)
		require.Equal(t, 10*time.Second, StateMapNtN[state].Timeout)
		require.Zero(t, StateMapNtC[state].Timeout)
	}
	require.Equal(t, 10*time.Second, NewConfig().Timeout,
		"the default Config.Timeout overrides the waiting side's state timeout")
}

// refVersion and refTable are the one-version table of the receive
// table, for each mode.
func refVersion(mode protocol.ProtocolMode) uint16 {
	if mode == protocol.ProtocolModeNodeToClient {
		return 16 + protocol.ProtocolVersionNtCOffset
	}
	return 14
}

func refTable(mode protocol.ProtocolMode) protocol.ProtocolVersionMap {
	full := protocol.GetProtocolVersionMap(mode, 42, protocol.DiffusionModeInitiatorOnly, false, false)
	v := refVersion(mode)
	return protocol.ProtocolVersionMap{v: full[v]}
}

// refHarness runs one handshake role over a muxer and plays the other side
// of the connection by hand.
type refHarness struct {
	conn     net.Conn
	errs     chan error
	payload  chan []byte
	role     protocol.ProtocolRole
	mode     protocol.ProtocolMode
	finished chan uint16
}

func newReferenceHarness(
	t *testing.T,
	role protocol.ProtocolRole,
	mode protocol.ProtocolMode,
) (*refHarness, protocol.ProtocolOptions) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	h := &refHarness{
		conn:     connB,
		errs:     make(chan error, 8),
		payload:  make(chan []byte, 8),
		role:     role,
		mode:     mode,
		finished: make(chan uint16, 2),
	}
	opts := protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		Muxer:     m,
		ErrorChan: h.errs,
		Mode:      mode,
		Role:      role,
	}
	// net.Pipe writes block until read, so the peer must always be reading.
	go h.readPeer()
	t.Cleanup(func() {
		_ = m.Stop(context.Background())
		_ = connA.Close()
		_ = connB.Close()
	})
	m.Start()
	return h, opts
}

func (h *refHarness) config() *Config {
	cfg := NewConfig(
		WithProtocolVersionMap(refTable(h.mode)),
		WithFinishedFunc(func(_ CallbackContext, v uint16, _ protocol.VersionData) error {
			h.finished <- v
			return nil
		}),
	)
	return &cfg
}

func (h *refHarness) readPeer() {
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

// sendRaw writes data as the peer would, in one muxer segment: from the
// responder when the role under test is the client, and from the initiator
// when it is the server. A write error is ignored, because the role under
// test may close the connection on refusing the message.
func (h *refHarness) sendRaw(t *testing.T, data []byte) {
	t.Helper()
	segment := muxer.NewSegment(ProtocolId, data, h.role == protocol.ProtocolRoleClient)
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	buf.Write(segment.Payload)
	require.NoError(t, h.conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	_, _ = h.conn.Write(buf.Bytes())
}

func (h *refHarness) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	h.sendRaw(t, data)
}

func (h *refHarness) sendKind(t *testing.T, kind refKind) {
	t.Helper()
	table := refTable(h.mode)
	v := refVersion(h.mode)
	switch kind {
	case kindPropose:
		h.send(t, NewMsgProposeVersions(table))
	case kindAccept:
		h.send(t, NewMsgAcceptVersion(v, table[v]))
	case kindRefuse:
		h.send(t, NewMsgRefuse([]any{RefuseReasonVersionMismatch, []uint16{v}}))
	case kindQueryReply:
		h.send(t, NewMsgQueryReply(table))
	case kindOversized:
		data, err := cbor.Encode([]any{
			MessageTypeProposeVersions,
			make([]byte, MaxPendingMessageBytes+1),
		})
		require.NoError(t, err)
		h.sendRaw(t, data)
	}
}

// receive returns the next message the role under test sent to the peer.
func (h *refHarness) receive(t *testing.T) protocol.Message {
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

func (h *refHarness) requireError(t *testing.T, contains string) error {
	t.Helper()
	select {
	case err := <-h.errs:
		require.ErrorContains(t, err, contains)
		return err
	case v := <-h.finished:
		t.Fatalf("handshake finished with version %d; the reference forbids it", v)
	case <-time.After(5 * time.Second):
		t.Fatal("message the reference forbids was accepted without an error")
	}
	return nil
}

func (h *refHarness) requireFinished(t *testing.T, version uint16) {
	t.Helper()
	select {
	case v := <-h.finished:
		require.Equal(t, version, v)
	case err := <-h.errs:
		t.Fatalf("handshake did not finish: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("handshake did not finish")
	}
}

// requireQuiet checks that, for a short while, the role under test neither
// reports an error, sends anything, nor closes the connection.
func (h *refHarness) requireQuiet(t *testing.T) {
	t.Helper()
	select {
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	case data, ok := <-h.payload:
		if !ok {
			t.Fatal("connection closed")
		}
		t.Fatalf("unexpected message %x", data)
	case v := <-h.finished:
		t.Fatalf("unexpected second finish with version %d", v)
	case <-time.After(200 * time.Millisecond):
	}
}

// Errors protocol.Protocol and the client report for refused messages.
const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowed        = "not allowed in current protocol state"
	errOversized         = "received oversized message"
	errUnsolicitedQuery  = "without proposing query mode"
)

// driveToState starts the role under test and moves it into the row's
// state. A client cannot rest in Propose, since Start sends its proposal at
// once, and a server cannot rest in Confirm, since it answers inside its
// message handler; the caller skips those cells.
func driveToState(t *testing.T, row refReceiveRow, mode protocol.ProtocolMode) *refHarness {
	t.Helper()
	h, opts := newReferenceHarness(t, row.role, mode)
	v := refVersion(mode)
	if row.role == protocol.ProtocolRoleServer {
		server := NewServer(opts, h.config())
		server.Start()
		t.Cleanup(func() { _ = server.Stop(context.Background()) })
		if row.state == stateDone {
			h.send(t, NewMsgProposeVersions(refTable(mode)))
			accept, ok := h.receive(t).(*MsgAcceptVersion)
			require.True(t, ok, "server must accept the shared version")
			require.Equal(t, v, accept.Version)
			h.requireFinished(t, v)
		}
		return h
	}
	client := NewClient(opts, h.config())
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	_, ok := h.receive(t).(*MsgProposeVersions)
	require.True(t, ok, "client must open with MsgProposeVersions")
	if row.state == stateDone {
		h.sendKind(t, kindAccept)
		h.requireFinished(t, v)
		require.Eventually(t, client.IsDone, 5*time.Second, 5*time.Millisecond)
	}
	return h
}

// TestReceiveMatchesReference drives a running client or server into each cell
// of the receive table it can rest in, sends the cell's message, and checks
// the outcome against gouroborosOutcome.
func TestReceiveMatchesReference(t *testing.T) {
	t.Parallel()
	for modeName, mode := range map[string]protocol.ProtocolMode{
		"NtN": protocol.ProtocolModeNodeToNode,
		"NtC": protocol.ProtocolModeNodeToClient,
	} {
		for _, row := range handshakeReferenceReceive {
			if row.role == protocol.ProtocolRoleServer && row.state == stateConfirm ||
				row.role == protocol.ProtocolRoleClient && row.state == statePropose {
				continue
			}
			t.Run(modeName+"/"+row.name(), func(t *testing.T) {
				t.Parallel()
				h := driveToState(t, row, mode)
				h.sendKind(t, row.kind)
				switch outcome := gouroborosOutcome(row); outcome {
				case "Confirm":
					accept, ok := h.receive(t).(*MsgAcceptVersion)
					require.True(t, ok, "MsgProposeVersions must be answered")
					require.Equal(t, refVersion(mode), accept.Version)
				case "Done":
					checkClientDone(t, h, row.kind, refVersion(mode))
				case refError:
					h.requireError(t, errUnsolicitedQuery)
				case refViolation:
					h.requireError(t, violationError(row))
				case refDropped:
					h.requireQuiet(t)
				default:
					t.Fatalf("no check for outcome %q", outcome)
				}
			})
		}
	}
}

// checkClientDone checks how a client in Confirm ends on a legal answer.
func checkClientDone(t *testing.T, h *refHarness, kind refKind, version uint16) {
	t.Helper()
	switch kind {
	case kindAccept, kindPropose:
		// On a simultaneous open the peer's MsgProposeVersions is read as
		// MsgReplyVersions and the client picks the version itself.
		h.requireFinished(t, version)
	case kindRefuse:
		err := h.requireError(t, "version mismatch")
		var mismatch *VersionMismatchError
		require.True(t, errors.As(err, &mismatch), "refusal must surface as VersionMismatchError: %v", err)
		require.Equal(t, []uint16{version}, mismatch.SupportedVersions)
	default:
		t.Fatalf("no Done check for %s", kind)
	}
}

func violationError(row refReceiveRow) string {
	switch {
	case row.kind == kindOversized:
		return errOversized
	case row.outcome == refWaits:
		return errWithoutPeerAgency
	default:
		return errNotAllowed
	}
}

// TestClientConfirmTimeoutMatchesSpec checks the Confirm timeout from the
// waiting client's side: a node-to-node client whose proposal is never
// answered gives up, and a node-to-client one keeps waiting, as the
// specification sets no node-to-client timeouts. The server's Propose
// timeout is covered by TestServerInitialProposeTimeout.
func TestClientConfirmTimeoutMatchesSpec(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		mode        protocol.ProtocolMode
		wantTimeout bool
	}{
		"NtN": {protocol.ProtocolModeNodeToNode, true},
		"NtC": {protocol.ProtocolModeNodeToClient, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, opts := newReferenceHarness(t, protocol.ProtocolRoleClient, test.mode)
			cfg := h.config()
			cfg.Timeout = 50 * time.Millisecond
			client := NewClient(opts, cfg)
			client.Start()
			t.Cleanup(func() { _ = client.Stop(context.Background()) })
			_, ok := h.receive(t).(*MsgProposeVersions)
			require.True(t, ok)
			if test.wantTimeout {
				h.requireError(t, "timeout waiting on transition from protocol state Confirm")
				return
			}
			h.requireQuiet(t)
		})
	}
}

// refData is node-to-node version data for versions 13 to 15.
func refData(magic uint32, query bool) protocol.VersionData {
	return protocol.VersionDataNtN13andUp{
		VersionDataNtN11to12: protocol.VersionDataNtN11to12{
			CborNetworkMagic: magic,
			CborQuery:        query,
		},
	}
}

// refRawTable encodes a proposal; a nil entry stands for version data the
// server cannot decode.
func refRawTable(t *testing.T, table map[uint16]protocol.VersionData) map[uint16]cbor.RawMessage {
	t.Helper()
	raw := make(map[uint16]cbor.RawMessage, len(table))
	for v, data := range table {
		if data == nil {
			raw[v] = cbor.RawMessage{0x01}
			continue
		}
		encoded, err := cbor.Encode(&data)
		require.NoError(t, err)
		raw[v] = encoded
	}
	return raw
}

// refAnswer is the server's answer in the negotiation table: the message
// type, the accepted or refused version, and the refusal reason.
type refAnswer struct {
	msgType uint8
	version uint16
	reason  uint64
}

func (a refAnswer) String() string {
	return fmt.Sprintf("type %d version %d reason %d", a.msgType, a.version, a.reason)
}

// TestServerNegotiationMatchesReference checks the server's answer against the
// specification's server algorithm and acceptOrRefuse in the reference
// (Handshake/Client.hs, Server.hs): the highest common version, then
// VersionMismatch, HandshakeDecodeError or Refused, and a query reply only when
// the version would have been accepted. Rows where gOuroboros answers otherwise
// would give both answers; there are none.
func TestServerNegotiationMatchesReference(t *testing.T) {
	t.Parallel()
	accept := func(v uint16) refAnswer { return refAnswer{MessageTypeAcceptVersion, v, 0} }
	refuse := func(reason uint64, v uint16) refAnswer { return refAnswer{MessageTypeRefuse, v, reason} }
	query := refAnswer{msgType: MessageTypeQueryReply}
	server := map[uint16]protocol.VersionData{13: refData(42, false), 14: refData(42, false)}
	for _, test := range []struct {
		name      string
		proposal  map[uint16]protocol.VersionData
		server    map[uint16]protocol.VersionData
		reference refAnswer
		// gouroboros is set only where gOuroboros differs from the reference.
		gouroboros *refAnswer
	}{
		{"highest common", map[uint16]protocol.VersionData{13: refData(42, false), 14: refData(42, false)}, server, accept(14), nil},
		{"highest common below proposed", map[uint16]protocol.VersionData{14: refData(42, false), 15: refData(42, false)}, server, accept(14), nil},
		{"no common version", map[uint16]protocol.VersionData{15: refData(42, false)}, server, refuse(RefuseReasonVersionMismatch, 0), nil},
		{"highest common undecodable", map[uint16]protocol.VersionData{13: refData(42, false), 14: nil}, server, refuse(RefuseReasonDecodeError, 14), nil},
		{"magic mismatch", map[uint16]protocol.VersionData{14: refData(43, false)}, server, refuse(RefuseReasonRefused, 14), nil},
		{"lower version magic ignored", map[uint16]protocol.VersionData{13: refData(43, false), 14: refData(42, false)}, server, accept(14), nil},
		{"query", map[uint16]protocol.VersionData{14: refData(42, true)}, server, query, nil},
		{"query without common version", map[uint16]protocol.VersionData{15: refData(42, true)}, server, refuse(RefuseReasonVersionMismatch, 0), nil},
		{"query with magic mismatch", map[uint16]protocol.VersionData{14: refData(43, true)}, server, refuse(RefuseReasonRefused, 14), nil},
		{"query on a version not chosen", map[uint16]protocol.VersionData{13: refData(42, true), 14: refData(42, false)}, server, accept(14), nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			want := test.reference
			if test.gouroboros != nil {
				want = *test.gouroboros
			}
			h, opts := newReferenceHarness(t, protocol.ProtocolRoleServer, protocol.ProtocolModeNodeToNode)
			cfg := h.config()
			cfg.ProtocolVersionMap = test.server
			s := NewServer(opts, cfg)
			s.Start()
			t.Cleanup(func() { _ = s.Stop(context.Background()) })
			h.send(t, &MsgProposeVersions{
				MessageBase: protocol.MessageBase{MessageType: MessageTypeProposeVersions},
				VersionMap:  refRawTable(t, test.proposal),
			})
			require.Equal(t, want.String(), refAnswerOf(t, h.receiveAnswer(t)).String())
		})
	}
}

// receiveAnswer returns the server's answer. Unlike receive, it ignores
// errors: the server reports a refusal or a query as a terminal error once
// the answer is on the wire, and that error may be seen first.
func (h *refHarness) receiveAnswer(t *testing.T) protocol.Message {
	t.Helper()
	select {
	case data, ok := <-h.payload:
		require.True(t, ok, "connection closed before the answer arrived")
		msgType, err := cbor.DecodeIdFromList(data)
		require.NoError(t, err)
		msg, err := NewMsgFromCbor(uint(msgType), data)
		require.NoError(t, err)
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("no answer from the server")
	}
	return nil
}

func refAnswerOf(t *testing.T, msg protocol.Message) refAnswer {
	t.Helper()
	switch m := msg.(type) {
	case *MsgAcceptVersion:
		return refAnswer{MessageTypeAcceptVersion, m.Version, 0}
	case *MsgQueryReply:
		return refAnswer{msgType: MessageTypeQueryReply}
	case *MsgRefuse:
		require.NotEmpty(t, m.Reason)
		reason, ok := m.Reason[0].(uint64)
		require.True(t, ok)
		answer := refAnswer{msgType: MessageTypeRefuse, reason: reason}
		if reason != RefuseReasonVersionMismatch {
			v, ok := m.Reason[1].(uint64)
			require.True(t, ok)
			answer.version = uint16(v) // #nosec G115 -- a handshake version number
		}
		return answer
	}
	t.Fatalf("unexpected answer %T", msg)
	return refAnswer{}
}
