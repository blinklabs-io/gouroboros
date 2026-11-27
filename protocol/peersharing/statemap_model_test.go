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

package peersharing

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
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
	// message yet; it is judged when the peer next holds agency.
	modelWaits = "Waits"
	// modelHeld: the client's run is over and no new run reads the message.
	modelHeld = "Held"
)

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindShareRequest        modelKind = "MsgShareRequest"
	kindSharePeers          modelKind = "MsgSharePeers"
	kindOverCountSharePeers modelKind = "OverCountSharePeers"
	kindDone                modelKind = "MsgDone"
	kindOversized           modelKind = "Oversized"
)

// modelRequested is the amount of the client's last request in every cell:
// kindSharePeers lists that many peers, and kindOverCountSharePeers one more.
const modelRequested = 1

// modelReceiveRow is one cell of the peer-sharing receive table: what a role
// in a given state does with a kind of message from its peer.
type modelReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    modelKind
	outcome string
}

// peerSharingModelReceive is the receive table of the peer-sharing
// mini-protocol from the Ouroboros network specification, and from the
// reference implementation where the specification is silent. It is the
// full role x state x kind product, so a cell missing here is a cell nobody
// decided.
//
//   - Within a run a message is judged against the reader's state: one that
//     is not legal there aborts the connection ("Protocols as State
//     Machines" in docs/network-spec/miniprotocols.tex), as does one over the
//     state's 5760-byte limit, and a MsgSharePeers listing more peers than
//     were requested ("Peer Sharing mini-protocol", same file;
//     peerSharingClient in Ouroboros.Network.PeerSharing).
//   - A side reads only while its peer holds agency (runPeerWithDriver in
//     typed-protocols Network.TypedProtocol.Driver), so a message that
//     arrives while the reader holds agency waits (modelWaits).
//   - A server whose run ended restarts in its initial state ("Inbound
//     side: the inbound governor" in docs/network-spec/connection-manager.tex),
//     so it judges a message after MsgDone as the first of a new run. A
//     client whose run ended leaves the message unread (modelHeld).
var peerSharingModelReceive = []modelReceiveRow{
	{protocol.ProtocolRoleServer, stateIdle, kindShareRequest, "Busy"},
	{protocol.ProtocolRoleServer, stateIdle, kindSharePeers, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindOverCountSharePeers, modelViolation},
	{protocol.ProtocolRoleServer, stateIdle, kindDone, "Done"},
	{protocol.ProtocolRoleServer, stateIdle, kindOversized, modelViolation},
	{protocol.ProtocolRoleServer, stateBusy, kindShareRequest, modelWaits},
	{protocol.ProtocolRoleServer, stateBusy, kindSharePeers, modelWaits},
	{protocol.ProtocolRoleServer, stateBusy, kindOverCountSharePeers, modelWaits},
	{protocol.ProtocolRoleServer, stateBusy, kindDone, modelWaits},
	{protocol.ProtocolRoleServer, stateBusy, kindOversized, modelWaits},
	{protocol.ProtocolRoleServer, stateDone, kindShareRequest, "Busy"},
	{protocol.ProtocolRoleServer, stateDone, kindSharePeers, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindOverCountSharePeers, modelViolation},
	{protocol.ProtocolRoleServer, stateDone, kindDone, "Done"},
	{protocol.ProtocolRoleServer, stateDone, kindOversized, modelViolation},
	{protocol.ProtocolRoleClient, stateIdle, kindShareRequest, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindSharePeers, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindOverCountSharePeers, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindDone, modelWaits},
	{protocol.ProtocolRoleClient, stateIdle, kindOversized, modelWaits},
	{protocol.ProtocolRoleClient, stateBusy, kindShareRequest, modelViolation},
	{protocol.ProtocolRoleClient, stateBusy, kindSharePeers, "Idle"},
	{protocol.ProtocolRoleClient, stateBusy, kindOverCountSharePeers, modelViolation},
	{protocol.ProtocolRoleClient, stateBusy, kindDone, modelViolation},
	{protocol.ProtocolRoleClient, stateBusy, kindOversized, modelViolation},
	{protocol.ProtocolRoleClient, stateDone, kindShareRequest, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindSharePeers, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindOverCountSharePeers, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindDone, modelHeld},
	{protocol.ProtocolRoleClient, stateDone, kindOversized, modelHeld},
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
// agency, and one the state has no transition for. The requested-count rule
// is the client's, not StateMap's, so over-count cells are left to the
// running tests.
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
	case kindShareRequest, kindOversized:
		return MessageTypeShareRequest
	case kindSharePeers, kindOverCountSharePeers:
		return MessageTypeSharePeers
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
	states := make([]protocol.State, 0, len(StateMap))
	for state := range StateMap {
		states = append(states, state)
	}
	require.ElementsMatch(
		t,
		[]protocol.State{stateIdle, stateBusy, stateDone},
		states,
		"StateMap must hold exactly the states of the reference",
	)
	require.Equal(t, protocol.AgencyClient, StateMap[stateIdle].Agency)
	require.Equal(t, protocol.AgencyServer, StateMap[stateBusy].Agency)
	require.Equal(t, protocol.AgencyNone, StateMap[stateDone].Agency)
	for state, entry := range StateMap {
		require.Empty(
			t,
			entry.PipelinedMessageTypes,
			"peer-sharing pipelines nothing, so %s must admit only the agency holder",
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
	for _, row := range peerSharingModelReceive {
		if row.state == stateDone || row.kind == kindOverCountSharePeers {
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

// TestDoneCannotBeLeft checks that no transition leaves Done within a run,
// and that Done is entered only by MsgDone from Idle. A message after Done
// belongs to a new run, not to a transition out of Done.
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

// TestStateMapTimeoutsMatchSpec pins the per-state timeouts to the
// specification's peer-sharing timeout table and the reference's
// timeLimitsPeerSharing: none in Idle, where the server may wait for ever
// between requests, and 60 s in Busy. NewClient overrides the Busy timeout
// from Config.Timeout.
func TestStateMapTimeoutsMatchSpec(t *testing.T) {
	t.Parallel()
	require.Zero(t, StateMap[stateIdle].Timeout)
	require.Nil(t, StateMap[stateIdle].TimeoutFunc)
	require.Equal(t, 60*time.Second, StateMap[stateBusy].Timeout)
	require.Equal(t, 60*time.Second, NewConfig().Timeout)
}

// modelHarness runs one peer-sharing role over a muxer and plays the other
// side of the connection by hand.
type modelHarness struct {
	conn    net.Conn
	errs    chan error
	payload chan []byte
	role    protocol.ProtocolRole
}

func newModelHarness(
	t *testing.T,
	role protocol.ProtocolRole,
) (*modelHarness, protocol.ProtocolOptions) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	h := &modelHarness{
		conn:    connB,
		errs:    make(chan error, 8),
		payload: make(chan []byte, 8),
		role:    role,
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
		m.Stop(context.Background())
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

// sendRaw writes data as the peer would, split into muxer segments. A write
// error is ignored, because the role under test may close the connection on
// refusing the message; the refusal is asserted through the error channel.
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

func (h *modelHarness) sendKind(t *testing.T, kind modelKind) {
	t.Helper()
	switch kind {
	case kindShareRequest:
		h.send(t, NewMsgShareRequest(modelRequested))
	case kindSharePeers:
		h.send(t, NewMsgSharePeers(smallPeerAddresses(modelRequested)))
	case kindOverCountSharePeers:
		h.send(t, NewMsgSharePeers(smallPeerAddresses(modelRequested+1)))
	case kindDone:
		h.send(t, NewMsgDone())
	case kindOversized:
		data, err := cbor.Encode([]any{
			MessageTypeShareRequest,
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

// requireAnswered checks that the server answered a request for
// modelRequested peers with exactly that many: the test's peer source always
// has more.
func (h *modelHarness) requireAnswered(t *testing.T) {
	t.Helper()
	reply, ok := h.receive(t).(*MsgSharePeers)
	require.True(t, ok, "MsgShareRequest must be answered with MsgSharePeers")
	require.Len(t, reply.PeerAddresses, modelRequested)
}

// Errors protocol.Protocol and the client report for refused messages.
const (
	errWithoutPeerAgency = "without peer agency"
	errNotAllowedInState = "not allowed in current protocol state"
	errOversized         = "received oversized message"
)

func violationError(row modelReceiveRow) string {
	switch {
	case row.outcome == modelWaits:
		return errWithoutPeerAgency
	case row.kind == kindOversized:
		return errOversized
	case row.kind == kindOverCountSharePeers && row.role == protocol.ProtocolRoleClient:
		return ErrTooManyPeersShared.Error()
	default:
		return errNotAllowedInState
	}
}

// modelPeerSource always has more peers than a cell asks for, so a reply's
// length shows the server clamped it to the request.
func modelPeerSource(CallbackContext, int) ([]PeerAddress, error) {
	return smallPeerAddresses(modelRequested + 2), nil
}

// driveToState starts the role under test and moves it into the row's
// state. It returns a channel carrying the result of a GetPeers call that is
// waiting for its reply, for a client in Busy.
func driveToState(t *testing.T, row modelReceiveRow) (*modelHarness, chan peersResult) {
	t.Helper()
	h, opts := newModelHarness(t, row.role)
	if row.role == protocol.ProtocolRoleServer {
		source := modelPeerSource
		// Stop waits for the peer source, so the cleanup releases it first.
		releaseSource := func() {}
		if row.state == stateBusy {
			// A server is in Busy while its peer source runs, so a source
			// that blocks holds it there.
			entered := make(chan struct{})
			release := make(chan struct{})
			releaseSource = func() { close(release) }
			source = func(CallbackContext, int) ([]PeerAddress, error) {
				close(entered)
				<-release
				return nil, nil
			}
			defer func() {
				h.send(t, NewMsgShareRequest(modelRequested))
				<-entered
			}()
		}
		cfg := NewConfig(WithShareRequestFunc(source))
		server := NewServer(opts, &cfg)
		server.Start()
		t.Cleanup(func() {
			releaseSource()
			_ = server.Stop(context.Background())
		})
		if row.state == stateDone {
			// The server cannot be observed resting in Done, so MsgDone and
			// the message under test go back to back; the pipe keeps order.
			h.send(t, NewMsgDone())
		}
		return h, nil
	}
	client := NewClient(opts, nil)
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	switch row.state {
	case stateBusy:
		results := make(chan peersResult, 1)
		go func() {
			peers, err := client.GetPeers(modelRequested)
			results <- peersResult{peers: peers, err: err}
		}()
		req, ok := h.receive(t).(*MsgShareRequest)
		require.True(t, ok, "GetPeers must send MsgShareRequest")
		require.Equal(t, uint8(modelRequested), req.Amount)
		return h, results
	case stateDone:
		// The client API never sends MsgDone, so the test sends it through
		// the embedded protocol.
		require.NoError(t, client.SendMessage(NewMsgDone()))
		_, ok := h.receive(t).(*MsgDone)
		require.True(t, ok, "client must put MsgDone on the wire")
		require.Eventually(t, client.IsDone, 5*time.Second, 5*time.Millisecond)
	}
	return h, nil
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table, sends the cell's message, and checks the outcome.
// Cells the reference table marks Waits are checked against gOuroboros's
// stricter behavior (see gouroborosRejectsEarly).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	for _, row := range peerSharingModelReceive {
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			h, results := driveToState(t, row)
			h.sendKind(t, row.kind)
			switch gouroborosRejectsEarly(row.outcome) {
			case "Busy":
				h.requireAnswered(t)
			case "Idle":
				select {
				case result := <-results:
					require.NoError(t, result.err)
					require.Len(t, result.peers, modelRequested)
				case err := <-h.errs:
					t.Fatalf("valid reply refused: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("valid reply was not delivered")
				}
			case modelViolation:
				h.requireViolation(t, violationError(row))
			case modelHeld:
				h.requireQuiet(t)
			case "Done":
				h.requireQuiet(t)
				// The run ended; the next must still be served.
				h.send(t, NewMsgShareRequest(modelRequested))
				h.requireAnswered(t)
			default:
				t.Fatalf("no check for outcome %q", row.outcome)
			}
		})
	}
}

// TestServerAnswersAmountZeroWithNoPeers checks the smallest request: the
// wire amount is a Word8, so 0 is legal, and a reply may list no more peers
// than were requested, so the server sends an empty list whatever its peer
// source returns.
func TestServerAnswersAmountZeroWithNoPeers(t *testing.T) {
	t.Parallel()
	h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
	cfg := NewConfig(WithShareRequestFunc(modelPeerSource))
	server := NewServer(opts, &cfg)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.send(t, NewMsgShareRequest(0))
	reply, ok := h.receive(t).(*MsgSharePeers)
	require.True(t, ok)
	require.Empty(t, reply.PeerAddresses)
	h.requireQuiet(t)
}

// TestClientRequestingZeroRefusesAnyPeer checks the requested-count rule at
// its edge: after asking for no peers, one peer is already too many.
func TestClientRequestingZeroRefusesAnyPeer(t *testing.T) {
	t.Parallel()
	h, opts := newModelHarness(t, protocol.ProtocolRoleClient)
	client := NewClient(opts, nil)
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	results := make(chan peersResult, 1)
	go func() {
		peers, err := client.GetPeers(0)
		results <- peersResult{peers: peers, err: err}
	}()
	req, ok := h.receive(t).(*MsgShareRequest)
	require.True(t, ok)
	require.Zero(t, req.Amount)
	h.send(t, NewMsgSharePeers(smallPeerAddresses(1)))
	h.requireViolation(t, "requested 0, received 1")
}

// TestShareRequestRefusesAmountAboveWord8 checks the amount's wire type: the
// reference's PeerSharingAmount is a Word8 and its CDDL is 0..255, so 256 is
// not a request.
func TestShareRequestRefusesAmountAboveWord8(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hex string
		ok  bool
	}{
		{"8200" + "18ff", true},    // [0, 255]
		{"8200" + "190100", false}, // [0, 256]
	} {
		data, err := hex.DecodeString(tc.hex)
		require.NoError(t, err)
		msg, err := NewMsgFromCbor(MessageTypeShareRequest, data)
		if !tc.ok {
			require.Error(t, err, tc.hex)
			continue
		}
		require.NoError(t, err, tc.hex)
		req, ok := msg.(*MsgShareRequest)
		require.True(t, ok)
		require.Equal(t, uint8(255), req.Amount)
	}
}

// TestPeerAddressRefusesWhatReferenceRefuses checks the address forms the
// reference decodeRemoteAddress cannot decode: a peer type other than 0
// (IPv4) or 1 (IPv6), and a word or port wider than its Word32 or Word16.
func TestPeerAddressRefusesWhatReferenceRefuses(t *testing.T) {
	t.Parallel()
	for name, h := range map[string]string{
		"peer type 2":         "83021a0100007f190bb9",
		"ipv4 port above u16": "83001a0100007f1a00010000",
		"ipv4 word above u32": "83001b00000001000000001a190bb9",
		"ipv6 port above u16": "8601000000011a00010000",
		"ipv4 extra element":  "84001a0100007f190bb900",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := hex.DecodeString(h)
			require.NoError(t, err)
			var decoded PeerAddress
			require.Error(t, decoded.UnmarshalCBOR(data))
		})
	}
}
