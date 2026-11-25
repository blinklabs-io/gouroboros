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

package blockfetch_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
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
	// modelQueued: a client request pipelined while the server holds
	// agency. The server reads it once it is back in Idle and answers it
	// after the current range.
	modelQueued = "Queued"
	// modelHeld: the client's run is over and no new run reads the message.
	modelHeld = "Held"
)

// modelKind is a kind of message a peer can send.
type modelKind string

const (
	kindRequestRange modelKind = "MsgRequestRange"
	kindClientDone   modelKind = "MsgClientDone"
	kindStartBatch   modelKind = "MsgStartBatch"
	kindNoBlocks     modelKind = "MsgNoBlocks"
	kindBlock        modelKind = "MsgBlock"
	kindBatchDone    modelKind = "MsgBatchDone"
	// kindOversized is a message longer than the receiving state's size
	// limit.
	kindOversized modelKind = "Oversized"
)

// modelReceiveRow is one cell of the block-fetch receive table: what a role
// in a given state does with a kind of message from its peer.
type modelReceiveRow struct {
	role    protocol.ProtocolRole
	state   protocol.State
	kind    modelKind
	outcome string
}

// blockFetchModelReceive is the receive table of the block-fetch
// mini-protocol from the Ouroboros network specification ("Block-Fetch
// mini-protocol" in docs/network-spec/miniprotocols.tex: agencies,
// transitions, size limits), and from the reference implementation where the
// specification is silent. It is the full role x state x kind product, so a
// cell missing here is a cell nobody decided.
//
//   - Within a run a message is judged against the reader's state: one the
//     state has no transition for, or one over the state's size limit,
//     aborts the connection ("Protocols as State Machines", same file).
//   - A side reads only while its peer holds agency (runPeerWithDriver in
//     typed-protocols Network.TypedProtocol.Driver), so a message that
//     arrives while the reader holds agency waits (modelWaits). A
//     MsgRequestRange the client pipelines while the server streams is such
//     a message; it is legal once the server is back in Idle (modelQueued;
//     "Pipelining of Mini Protocols", same file).
//   - A server whose run ended restarts in Idle, so it judges a message
//     after MsgClientDone as the first of a new run. A client whose run
//     ended leaves the message unread (modelHeld).
var blockFetchModelReceive = buildModelReceive()

func buildModelReceive() []modelReceiveRow {
	server := protocol.ProtocolRoleServer
	client := protocol.ProtocolRoleClient
	idle := blockfetch.StateIdle
	busy := blockfetch.StateBusy
	streaming := blockfetch.StateStreaming
	done := blockfetch.StateDone
	v := modelViolation
	rows := []modelReceiveRow{}
	add := func(
		role protocol.ProtocolRole,
		state protocol.State,
		outcomes map[modelKind]string,
	) {
		for _, kind := range modelKinds {
			outcome, ok := outcomes[kind]
			if !ok {
				outcome = v
			}
			rows = append(rows, modelReceiveRow{role, state, kind, outcome})
		}
	}
	// The server reads client messages in Idle, and in Done as the first
	// message of the next run.
	add(server, idle, map[modelKind]string{
		kindRequestRange: "Busy",
		kindClientDone:   "Done",
	})
	add(server, busy, map[modelKind]string{
		kindRequestRange: modelQueued,
		kindClientDone:   modelWaits,
		kindStartBatch:   modelWaits,
		kindNoBlocks:     modelWaits,
		kindBlock:        modelWaits,
		kindBatchDone:    modelWaits,
		kindOversized:    modelWaits,
	})
	add(server, streaming, map[modelKind]string{
		kindRequestRange: modelQueued,
		kindClientDone:   modelWaits,
		kindStartBatch:   modelWaits,
		kindNoBlocks:     modelWaits,
		kindBlock:        modelWaits,
		kindBatchDone:    modelWaits,
		kindOversized:    modelWaits,
	})
	add(server, done, map[modelKind]string{
		kindRequestRange: "Busy",
		kindClientDone:   "Done",
	})
	// The client reads server messages in Busy and Streaming only.
	add(client, idle, map[modelKind]string{
		kindRequestRange: modelWaits,
		kindClientDone:   modelWaits,
		kindStartBatch:   modelWaits,
		kindNoBlocks:     modelWaits,
		kindBlock:        modelWaits,
		kindBatchDone:    modelWaits,
		kindOversized:    modelWaits,
	})
	add(client, busy, map[modelKind]string{
		kindStartBatch: "Streaming",
		kindNoBlocks:   "Idle",
	})
	add(client, streaming, map[modelKind]string{
		kindBlock:     "Streaming",
		kindBatchDone: "Idle",
	})
	add(client, done, map[modelKind]string{
		kindRequestRange: modelHeld,
		kindClientDone:   modelHeld,
		kindStartBatch:   modelHeld,
		kindNoBlocks:     modelHeld,
		kindBlock:        modelHeld,
		kindBatchDone:    modelHeld,
		kindOversized:    modelHeld,
	})
	return rows
}

var modelKinds = []modelKind{
	kindRequestRange,
	kindClientDone,
	kindStartBatch,
	kindNoBlocks,
	kindBlock,
	kindBatchDone,
	kindOversized,
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

// gouroborosOutcome maps the table onto gOuroboros where it differs from
// the reference implementation without being a defect: protocol.Protocol
// judges a message the moment it arrives, and refuses one sent while the
// receiver holds agency instead of leaving it to wait. The specification
// does not say when such a message is judged. A pipelined MsgRequestRange
// is the exception: StateMap lists it in PipelinedMessageTypes, so the
// server admits it at once and answers it after the current range, which is
// what the reference does when it reads it in Idle.
func gouroborosOutcome(outcome string) string {
	if outcome == modelWaits {
		return modelViolation
	}
	return outcome
}

func modelKindType(kind modelKind) uint8 {
	switch kind {
	case kindRequestRange:
		return blockfetch.MessageTypeRequestRange
	case kindClientDone:
		return blockfetch.MessageTypeClientDone
	case kindStartBatch:
		return blockfetch.MessageTypeStartBatch
	case kindNoBlocks:
		return blockfetch.MessageTypeNoBlocks
	case kindBlock, kindOversized:
		return blockfetch.MessageTypeBlock
	default:
		return blockfetch.MessageTypeBatchDone
	}
}

// stateMapReceive derives the outcome of one within-run cell from StateMap
// alone, following the checks protocol.Protocol applies: a message over the
// state's size limit is refused, a pipelined type is admitted while the
// receiver holds agency, any other message sent while the receiver holds
// agency is refused, and so is one the state has no transition for.
func stateMapReceive(
	t *testing.T,
	role protocol.ProtocolRole,
	state protocol.State,
	kind modelKind,
) string {
	t.Helper()
	entry, ok := blockfetch.StateMap[state]
	require.True(t, ok, "StateMap has no entry for %s", state)
	peerAgency := protocol.AgencyServer
	if role == protocol.ProtocolRoleServer {
		peerAgency = protocol.AgencyClient
	}
	if entry.Agency != peerAgency {
		if role == protocol.ProtocolRoleServer && kind != kindOversized {
			for _, msgType := range entry.PipelinedMessageTypes {
				if msgType == modelKindType(kind) {
					return modelQueued
				}
			}
		}
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

// TestStateMapMatchesModel checks the within-run cells of the receive table
// against StateMap, including the cells no running client or server can be
// driven into, and pins the shape of StateMap the table depends on. The Done
// rows are about the next run, which the running tests cover.
func TestStateMapMatchesModel(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(
		t,
		[]protocol.State{
			blockfetch.StateIdle,
			blockfetch.StateBusy,
			blockfetch.StateStreaming,
			blockfetch.StateDone,
		},
		stateMapStates(),
		"StateMap must hold exactly the states of the reference",
	)
	require.Equal(t, protocol.AgencyClient, blockfetch.StateMap[blockfetch.StateIdle].Agency)
	require.Equal(t, protocol.AgencyServer, blockfetch.StateMap[blockfetch.StateBusy].Agency)
	require.Equal(t, protocol.AgencyServer, blockfetch.StateMap[blockfetch.StateStreaming].Agency)
	require.Equal(t, protocol.AgencyNone, blockfetch.StateMap[blockfetch.StateDone].Agency)
	for state, entry := range blockfetch.StateMap {
		for _, transition := range entry.Transitions {
			require.Nil(
				t,
				transition.MatchFunc,
				"%s transitions are decided by message type alone",
				state,
			)
		}
	}
	for _, row := range blockFetchModelReceive {
		if row.state == blockfetch.StateDone {
			continue
		}
		t.Run(row.name(), func(t *testing.T) {
			require.Equal(
				t,
				gouroborosOutcome(row.outcome),
				stateMapReceive(t, row.role, row.state, row.kind),
			)
		})
	}
}

func stateMapStates() []protocol.State {
	states := make([]protocol.State, 0, len(blockfetch.StateMap))
	for state := range blockfetch.StateMap {
		states = append(states, state)
	}
	return states
}

// TestStateMapEntryAndExitRules checks the states that cannot be entered or
// left except by the specification's transitions: Done is entered only by
// MsgClientDone from Idle and never left within a run; Streaming only by
// MsgStartBatch from Busy or MsgBlock from Streaming; Busy only by
// MsgRequestRange from Idle; and Idle only by MsgNoBlocks or MsgBatchDone.
func TestStateMapEntryAndExitRules(t *testing.T) {
	t.Parallel()
	type edge struct {
		from    protocol.State
		msgType uint8
	}
	want := map[protocol.State][]edge{
		blockfetch.StateBusy: {
			{blockfetch.StateIdle, blockfetch.MessageTypeRequestRange},
		},
		blockfetch.StateDone: {
			{blockfetch.StateIdle, blockfetch.MessageTypeClientDone},
		},
		blockfetch.StateStreaming: {
			{blockfetch.StateBusy, blockfetch.MessageTypeStartBatch},
			{blockfetch.StateStreaming, blockfetch.MessageTypeBlock},
		},
		blockfetch.StateIdle: {
			{blockfetch.StateBusy, blockfetch.MessageTypeNoBlocks},
			{blockfetch.StateStreaming, blockfetch.MessageTypeBatchDone},
		},
	}
	got := map[protocol.State][]edge{}
	for from, entry := range blockfetch.StateMap {
		for _, transition := range entry.Transitions {
			got[transition.NewState] = append(
				got[transition.NewState],
				edge{from, transition.MsgType},
			)
		}
	}
	require.Empty(t, blockfetch.StateMap[blockfetch.StateDone].Transitions)
	for state, edges := range want {
		require.ElementsMatch(t, edges, got[state], "edges into %s", state)
	}
	require.Len(t, got, len(want))
}

// TestStateMapLimitsAgainstSpec pins the per-state size limits and timeouts
// to the specification's block-fetch tables, and records where gOuroboros
// differs: Busy admits a full block (2,500,000 bytes, against 65,535),
// because protocol.Protocol may judge an early MsgBlock against Busy before
// MsgStartBatch has been applied.
func TestStateMapLimitsAgainstSpec(t *testing.T) {
	t.Parallel()
	entry := blockfetch.StateMap
	require.Equal(t, 65535, entry[blockfetch.StateIdle].PendingMessageByteLimit)
	require.Equal(t, 2500000, entry[blockfetch.StateStreaming].PendingMessageByteLimit)
	require.Equal(t, 2500000, entry[blockfetch.StateBusy].PendingMessageByteLimit)
	require.Zero(t, entry[blockfetch.StateIdle].Timeout)
	require.Equal(t, 60*time.Second, entry[blockfetch.StateBusy].Timeout)
	require.Equal(t, 60*time.Second, entry[blockfetch.StateStreaming].Timeout)
}

// modelHarness runs one block-fetch role over a muxer and plays the other
// side of the connection by hand.
type modelHarness struct {
	conn net.Conn
	errs chan error
	// muxErrs carries the muxer's own errors, such as an ingress overflow.
	muxErrs chan error
	payload chan []byte
	role    protocol.ProtocolRole
	// blocks and ranges record what a client delivered.
	blocks chan uint64
	ranges chan error
	// served counts the ranges a server was asked for.
	served chan struct{}
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
		muxErrs: m.ErrorChan(),
		payload: make(chan []byte, 8),
		role:    role,
		blocks:  make(chan uint64, 8),
		ranges:  make(chan error, 8),
		served:  make(chan struct{}, 8),
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

// sendRaw writes data as the peer would, split into muxer segments. A write
// error is ignored, because the role under test may close the connection on
// refusing the message; the refusal is asserted through the error channel.
func (h *modelHarness) sendRaw(t *testing.T, data []byte) {
	t.Helper()
	const segmentMax = 0xffff
	for len(data) > 0 {
		n := min(len(data), segmentMax)
		segment := muxer.NewSegment(
			blockfetch.ProtocolId,
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

// receiveType returns the type of the next message the role under test sent.
func (h *modelHarness) receiveType(t *testing.T) uint8 {
	t.Helper()
	select {
	case data, ok := <-h.payload:
		require.True(t, ok, "connection closed before a message arrived")
		msgType, err := cbor.DecodeIdFromList(data)
		require.NoError(t, err)
		return uint8(msgType)
	case err := <-h.errs:
		t.Fatalf("protocol error while waiting for a message: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no message from the role under test")
	}
	return 0
}

func (h *modelHarness) requireReceive(t *testing.T, msgType uint8) {
	t.Helper()
	require.Equal(t, msgType, h.receiveType(t))
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

// requireRefusal checks that the role under test refuses the row's message.
// The muxer's ingress limit for block-fetch is the largest per-state size
// limit, so an oversized message the read loop has not started taking, while
// it lags or while a server restarts, is refused by the muxer instead, which
// also ends the connection.
func (h *modelHarness) requireRefusal(t *testing.T, row modelReceiveRow) {
	t.Helper()
	if row.kind != kindOversized {
		h.requireViolation(t, violationError(row))
		return
	}
	select {
	case err := <-h.errs:
		require.ErrorContains(t, err, violationError(row))
	case err := <-h.muxErrs:
		require.ErrorIs(t, err, muxer.ErrIngressOverflow)
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

// Errors protocol.Protocol reports for refused messages.
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
	default:
		return errNotAllowedInState
	}
}

// modelSlot is the slot of the one block in every range the tests request.
const modelSlot = 100

// kindMessage builds a message of the given kind. A block is the one the
// client's range asks for, so a client in Streaming accepts it.
func kindMessage(t *testing.T, kind modelKind, state protocol.State) []byte {
	t.Helper()
	var msg protocol.Message
	switch kind {
	case kindRequestRange:
		_, point := testBlock(t, modelSlot)
		msg = blockfetch.NewMsgRequestRange(point, point)
	case kindClientDone:
		msg = blockfetch.NewMsgClientDone()
	case kindStartBatch:
		msg = blockfetch.NewMsgStartBatch()
	case kindNoBlocks:
		msg = blockfetch.NewMsgNoBlocks()
	case kindBlock:
		wrapped, _ := testBlock(t, modelSlot)
		msg = blockfetch.NewMsgBlock(wrapped)
	case kindBatchDone:
		msg = blockfetch.NewMsgBatchDone()
	case kindOversized:
		limit := blockfetch.StateMap[state].PendingMessageByteLimit
		if limit == 0 {
			limit = blockfetch.StreamingMaxPendingMessageBytes
		}
		data, err := cbor.Encode([]any{
			blockfetch.MessageTypeBlock,
			make([]byte, limit+1),
		})
		require.NoError(t, err)
		return data
	}
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	return data
}

// startModelClient starts a pipelining client that records each delivered
// block's slot and each range's outcome.
func startModelClient(
	t *testing.T,
	h *modelHarness,
	opts protocol.ProtocolOptions,
) *blockfetch.Client {
	t.Helper()
	cfg, err := blockfetch.NewConfig(
		blockfetch.WithRequestPipelining(true),
		blockfetch.WithBlockRawFunc(
			func(_ blockfetch.CallbackContext, _ uint, data []byte) error {
				h.blocks <- modelSlot
				return nil
			},
		),
		blockfetch.WithRangeDoneFunc(
			func(_ blockfetch.CallbackContext, err error) error {
				h.ranges <- err
				return nil
			},
		),
	)
	require.NoError(t, err)
	// The test blocks carry no body, so their body hash cannot match.
	cfg.SkipBlockValidation = true
	client := blockfetch.NewClient(opts, &cfg)
	client.Start()
	t.Cleanup(func() { _ = client.Stop() })
	return client
}

// requestModelRange sends one MsgRequestRange for the block at modelSlot
// and waits until it is on the wire, which puts the client in Busy.
func requestModelRange(
	t *testing.T,
	h *modelHarness,
	client *blockfetch.Client,
) {
	t.Helper()
	_, point := testBlock(t, modelSlot)
	_, err := client.RequestRange(context.Background(), blockfetch.RangeRequest{
		Start: point,
		End:   point,
	})
	require.NoError(t, err)
	h.requireReceive(t, blockfetch.MessageTypeRequestRange)
}

// driveClient starts a client and moves it into the row's state. For
// Streaming the prefix depends on the cell: MsgBatchDone is legal only once
// the range's block has arrived, so that cell's prefix includes the block.
func driveClient(t *testing.T, row modelReceiveRow) *modelHarness {
	t.Helper()
	h, opts := newModelHarness(t, protocol.ProtocolRoleClient)
	client := startModelClient(t, h, opts)
	switch row.state {
	case blockfetch.StateIdle:
	case blockfetch.StateBusy:
		requestModelRange(t, h, client)
	case blockfetch.StateStreaming:
		requestModelRange(t, h, client)
		h.send(t, blockfetch.NewMsgStartBatch())
		if row.kind == kindBatchDone {
			wrapped, _ := testBlock(t, modelSlot)
			h.send(t, blockfetch.NewMsgBlock(wrapped))
			requireBlock(t, h)
		}
	case blockfetch.StateDone:
		require.NoError(t, client.Stop())
		h.requireReceive(t, blockfetch.MessageTypeClientDone)
	}
	return h
}

func requireBlock(t *testing.T, h *modelHarness) {
	t.Helper()
	select {
	case slot := <-h.blocks:
		require.Equal(t, uint64(modelSlot), slot)
	case err := <-h.errs:
		t.Fatalf("valid block refused: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("valid block was not delivered")
	}
}

func requireRange(t *testing.T, h *modelHarness, want error) {
	t.Helper()
	select {
	case err := <-h.ranges:
		if want == nil {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, want)
		}
	case err := <-h.errs:
		t.Fatalf("valid answer refused: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("range was not resolved")
	}
}

// driveServer starts a server and moves it into the row's state. Its
// RequestRangeFunc answers MsgNoBlocks for every range after the first; the
// first is held in Busy, or in Streaming after MsgStartBatch, so the server
// rests there until release is called.
func driveServer(
	t *testing.T,
	row modelReceiveRow,
) (h *modelHarness, release func()) {
	t.Helper()
	h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
	hold := row.state == blockfetch.StateBusy ||
		row.state == blockfetch.StateStreaming
	released := make(chan struct{})
	var releaseOnce sync.Once
	release = func() { releaseOnce.Do(func() { close(released) }) }
	t.Cleanup(release)
	first := true
	cfg, err := blockfetch.NewConfig(
		blockfetch.WithBatchStartTimeout(time.Minute),
		blockfetch.WithRequestRangeFunc(func(
			ctx blockfetch.CallbackContext, _ pcommon.Point, _ pcommon.Point,
		) error {
			h.served <- struct{}{}
			if hold && first {
				first = false
				if row.state == blockfetch.StateStreaming {
					if err := ctx.Server.StartBatch(); err != nil {
						return err
					}
				}
				go func() {
					<-released
					if row.state == blockfetch.StateStreaming {
						_ = ctx.Server.BatchDone()
						return
					}
					_ = ctx.Server.NoBlocks()
				}()
				return nil
			}
			return ctx.Server.NoBlocks()
		}),
	)
	require.NoError(t, err)
	server := blockfetch.NewServer(opts, &cfg)
	server.Start()
	t.Cleanup(server.Stop)
	switch row.state {
	case blockfetch.StateBusy, blockfetch.StateStreaming:
		h.sendRaw(t, kindMessage(t, kindRequestRange, blockfetch.StateIdle))
		requireServed(t, h)
		if row.state == blockfetch.StateStreaming {
			h.requireReceive(t, blockfetch.MessageTypeStartBatch)
		}
	case blockfetch.StateDone:
		// The server cannot be observed resting in Done, so MsgClientDone
		// and the message under test go back to back; the pipe keeps order.
		h.send(t, blockfetch.NewMsgClientDone())
	}
	return h, release
}

func requireServed(t *testing.T, h *modelHarness) {
	t.Helper()
	select {
	case <-h.served:
	case err := <-h.errs:
		t.Fatalf("valid request refused: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("request was not served")
	}
}

// TestReceiveMatchesModel drives a running client or server into each cell
// of the receive table, sends the cell's message, and checks the outcome.
// Cells the reference table marks Waits are checked against gOuroboros's
// stricter behavior (see gouroborosOutcome).
func TestReceiveMatchesModel(t *testing.T) {
	t.Parallel()
	for _, row := range blockFetchModelReceive {
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			if row.role == protocol.ProtocolRoleServer {
				checkServerCell(t, row)
				return
			}
			checkClientCell(t, row)
		})
	}
}

func checkClientCell(t *testing.T, row modelReceiveRow) {
	t.Helper()
	h := driveClient(t, row)
	h.sendRaw(t, kindMessage(t, row.kind, row.state))
	switch gouroborosOutcome(row.outcome) {
	case "Streaming":
		if row.kind == kindBlock {
			requireBlock(t, h)
		}
		h.requireQuiet(t)
	case "Idle":
		if row.kind == kindNoBlocks {
			requireRange(t, h, blockfetch.ErrNoBlocks)
		} else {
			requireRange(t, h, nil)
		}
		h.requireQuiet(t)
	case modelViolation:
		h.requireRefusal(t, row)
	case modelHeld:
		h.requireQuiet(t)
	default:
		t.Fatalf("no check for outcome %q", row.outcome)
	}
}

func checkServerCell(t *testing.T, row modelReceiveRow) {
	t.Helper()
	h, release := driveServer(t, row)
	h.sendRaw(t, kindMessage(t, row.kind, row.state))
	switch gouroborosOutcome(row.outcome) {
	case "Busy":
		requireServed(t, h)
		h.requireReceive(t, blockfetch.MessageTypeNoBlocks)
	case "Done":
		h.requireQuiet(t)
		// The run ended; the next must still be served.
		h.sendRaw(t, kindMessage(t, kindRequestRange, blockfetch.StateIdle))
		requireServed(t, h)
		h.requireReceive(t, blockfetch.MessageTypeNoBlocks)
	case modelQueued:
		// Admitted, but not served until the current range is finished.
		h.requireQuiet(t)
		select {
		case <-h.served:
			t.Fatal("pipelined request served before the current range ended")
		default:
		}
		release()
		if row.state == blockfetch.StateStreaming {
			h.requireReceive(t, blockfetch.MessageTypeBatchDone)
		} else {
			h.requireReceive(t, blockfetch.MessageTypeNoBlocks)
		}
		requireServed(t, h)
		h.requireReceive(t, blockfetch.MessageTypeNoBlocks)
	case modelViolation:
		h.requireRefusal(t, row)
	default:
		t.Fatalf("no check for outcome %q", row.outcome)
	}
}

// TestClientTimesOutInServerAgency checks the Busy and Streaming rows of the
// specification's timeout table: a client whose peer falls silent in either
// state gives up instead of waiting forever. Idle has no timeout, and the
// client is never left waiting in it.
func TestClientTimesOutInServerAgency(t *testing.T) {
	t.Parallel()
	for _, state := range []protocol.State{
		blockfetch.StateBusy,
		blockfetch.StateStreaming,
	} {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			h, opts := newModelHarness(t, protocol.ProtocolRoleClient)
			cfg, err := blockfetch.NewConfig(
				blockfetch.WithRequestPipelining(true),
				blockfetch.WithBatchStartTimeout(50*time.Millisecond),
				blockfetch.WithBlockTimeout(50*time.Millisecond),
				blockfetch.WithRangeDoneFunc(
					func(blockfetch.CallbackContext, error) error { return nil },
				),
				blockfetch.WithBlockRawFunc(
					func(blockfetch.CallbackContext, uint, []byte) error { return nil },
				),
			)
			require.NoError(t, err)
			client := blockfetch.NewClient(opts, &cfg)
			client.Start()
			t.Cleanup(func() { _ = client.Stop() })
			requestModelRange(t, h, client)
			if state == blockfetch.StateStreaming {
				h.send(t, blockfetch.NewMsgStartBatch())
			}
			h.requireViolation(
				t,
				"timeout waiting on transition from protocol state "+state.String(),
			)
		})
	}
}

// depthRecorder keeps the last ingress queue depth the muxer reported.
type depthRecorder struct {
	depth atomic.Int64
}

func (d *depthRecorder) IngressQueueDepth(_ uint16, _ muxer.ProtocolRole, bytes int) {
	d.depth.Store(int64(bytes))
}

func (*depthRecorder) IngressDeliveryBlocked(uint16, muxer.ProtocolRole, time.Duration) {}

func (*depthRecorder) IngressBackpressure(uint16, muxer.ProtocolRole, time.Duration) {}

// TestOversizedRefusedWhileReaderStalled sends the Busy cell's oversized
// message while the server reads nothing: its RequestRangeFunc holds the
// receive loop, and a receive queue of one leaves the read loop blocked
// behind it. The muxer then holds the whole message and refuses it, as it
// does when the read loop lags under load or is restarting after
// MsgClientDone.
func TestOversizedRefusedWhileReaderStalled(t *testing.T) {
	t.Parallel()
	h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
	depths := &depthRecorder{}
	opts.Muxer.SetMetrics(depths)
	released := make(chan struct{})
	cfg, err := blockfetch.NewConfig(
		blockfetch.WithBatchStartTimeout(time.Minute),
		blockfetch.WithRecvQueueSize(1),
		blockfetch.WithRequestRangeFunc(func(
			blockfetch.CallbackContext, pcommon.Point, pcommon.Point,
		) error {
			h.served <- struct{}{}
			<-released
			return nil
		}),
	)
	require.NoError(t, err)
	server := blockfetch.NewServer(opts, &cfg)
	server.Start()
	t.Cleanup(func() { stopServer(server) })
	t.Cleanup(func() { close(released) })
	request := kindMessage(t, kindRequestRange, blockfetch.StateIdle)
	h.sendRaw(t, request)
	requireServed(t, h)
	// The read loop blocks on the third request, holding at most one batch
	// of ten segments, and the muxer's channel to it holds ten more. Forty
	// one-segment requests therefore leave some on the muxer's queue, which
	// nothing drains while the callback holds the receive loop.
	for range 40 {
		h.sendRaw(t, request)
	}
	require.Eventually(t, func() bool {
		return depths.depth.Load() > 0
	}, 5*time.Second, time.Millisecond, "the read loop kept taking segments")
	row := modelReceiveRow{
		role:    protocol.ProtocolRoleServer,
		state:   blockfetch.StateBusy,
		kind:    kindOversized,
		outcome: modelWaits,
	}
	h.sendRaw(t, kindMessage(t, row.kind, row.state))
	h.requireRefusal(t, row)
}

// stopServer stops s through whichever Stop signature the package has: a
// plain Stop() or Stop(ctx) error. Both forms exist while the change that
// adds a context to Stop is in flight, and this test must build against each.
func stopServer(s any) {
	switch v := s.(type) {
	case interface{ Stop(context.Context) error }:
		_ = v.Stop(context.Background())
	case interface{ Stop() }:
		v.Stop()
	default:
		panic(fmt.Sprintf("%T has no Stop method", s))
	}
}
