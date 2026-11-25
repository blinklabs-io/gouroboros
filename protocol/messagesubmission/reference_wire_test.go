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

package messagesubmission

// These tests play the reference dmq-node's side of the connection byte for
// byte. Every payload the scripted peer sends, and every payload it expects,
// is copied from the reference codecs:
//
//   V1 is tx-submission: ouroboros-network 4646039f,
//   ouroboros-network/protocols/lib/Ouroboros/Network/Protocol/TxSubmission2/
//   Codec.hs, encodeTxSubmission2 (lines 316-360). dmq-node's V1 codec wraps
//   it (dmq-node a267003c, DMQ/Protocol/SigSubmission/Codec.hs 145-155).
//
//   V2: dmq-node a267003c, DMQ/Protocol/SigSubmissionV2/Codec.hs,
//   encodeSigSubmissionV2 (lines 139-180).
//
// The reference decoders read every collection with decodeListLenIndef, so
// only an indefinite-length list is accepted there.
//
// Which side starts follows dmq-node's DMQ/NodeToNode.hs: in V1 the
// initiator runs the outbound side (txSubmissionOutbound, lines 286-308) and
// the responder the inbound side (lines 330-357); in V2 the initiator runs
// the inbound side (lines 242-284) and the responder the outbound side
// (lines 311-328).

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

const referenceWireWait = 2 * time.Second

// referencePeer is the dmq-node end of a connection to one gOuroboros
// message-submission instance.
type referencePeer struct {
	t       *testing.T
	conn    net.Conn
	pending []byte
	errs    chan error
}

func newReferencePeer(t *testing.T) (*referencePeer, *muxer.Muxer) {
	t.Helper()
	ours, theirs := net.Pipe()
	m := muxer.New(ours)
	t.Cleanup(func() {
		_ = m.Stop(context.Background())
		_ = ours.Close()
		_ = theirs.Close()
	})
	return &referencePeer{
		t:    t,
		conn: theirs,
		errs: make(chan error, 8),
	}, m
}

// send writes one reference message. fromInitiator is the side dmq-node
// sends it from, which sets the muxer's direction bit.
func (p *referencePeer) send(fromInitiator bool, payload []byte) {
	p.t.Helper()
	id := uint16(ProtocolID)
	if !fromInitiator {
		id |= 0x8000
	}
	header := muxer.SegmentHeader{
		ProtocolId: id,
		// #nosec G115 -- test payloads are a few dozen bytes
		PayloadLength: uint16(len(payload)),
	}
	var buf bytes.Buffer
	require.NoError(p.t, binary.Write(&buf, binary.BigEndian, header))
	buf.Write(payload)
	require.NoError(
		p.t,
		p.conn.SetWriteDeadline(time.Now().Add(referenceWireWait)),
	)
	_, err := p.conn.Write(buf.Bytes())
	require.NoError(p.t, err, "gOuroboros did not read a reference message")
}

// expect reads the next message gOuroboros sends and requires it to be want,
// sent from the initiator side when fromInitiator is set.
func (p *referencePeer) expect(fromInitiator bool, want []byte) {
	p.t.Helper()
	for len(p.pending) < len(want) {
		require.NoError(
			p.t,
			p.conn.SetReadDeadline(time.Now().Add(referenceWireWait)),
		)
		var header muxer.SegmentHeader
		err := binary.Read(p.conn, binary.BigEndian, &header)
		require.NoError(
			p.t,
			err,
			"gOuroboros sent nothing; wanted %x from the %s",
			want,
			sideName(fromInitiator),
		)
		require.Equal(
			p.t,
			uint16(ProtocolID),
			header.GetProtocolId(),
			"segment for another mini-protocol",
		)
		require.Equal(
			p.t,
			sideName(fromInitiator),
			sideName(header.IsRequest()),
			"gOuroboros sent %x from the wrong side of the connection",
			want,
		)
		payload := make([]byte, header.PayloadLength)
		_, err = readFull(p.conn, payload)
		require.NoError(p.t, err)
		p.pending = append(p.pending, payload...)
	}
	got := p.pending[:len(want)]
	p.pending = p.pending[len(want):]
	require.Equal(p.t, want, got, "gOuroboros bytes differ from the reference")
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	read := 0
	for read < len(buf) {
		n, err := conn.Read(buf[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

func sideName(initiator bool) string {
	if initiator {
		return "initiator"
	}
	return "responder"
}

// await returns the next value on ch, failing on a protocol error first.
func await[T any](t *testing.T, ch <-chan T, errs <-chan error, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case err := <-errs:
		t.Fatalf("protocol error while waiting for %s: %v", what, err)
	case <-time.After(referenceWireWait):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

type idRequest struct {
	blocking bool
	ack      uint16
	req      uint16
}

func referenceID(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, 32)
}

// idAndSize is the reference encoding of one (sigId, SizeInBytes) pair: a
// 32-byte id and a size of 100.
func idAndSize(id []byte) []byte {
	out := []byte{0x82, 0x58, 0x20}
	out = append(out, id...)
	return append(out, 0x18, 0x64)
}

func byteString32(id []byte) []byte {
	return append([]byte{0x58, 0x20}, id...)
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

type clientHarness struct {
	peer       *referencePeer
	client     *Client
	idRequests chan idRequest
	bodyReqs   chan [][]byte
	done       chan struct{}
	errs       chan error
}

// newClientHarness connects a gOuroboros Client (the side that has the
// messages) to a scripted dmq-node peer that wants them.
func newClientHarness(t *testing.T, version uint16) *clientHarness {
	t.Helper()
	peer, m := newReferencePeer(t)
	h := &clientHarness{
		peer:       peer,
		idRequests: make(chan idRequest, 4),
		bodyReqs:   make(chan [][]byte, 4),
		done:       make(chan struct{}, 1),
		errs:       make(chan error, 8),
	}
	cfg := NewConfig(
		WithRequestMessageIdsFunc(func(
			_ CallbackContext,
			blocking bool,
			ack, req uint16,
		) {
			h.idRequests <- idRequest{blocking: blocking, ack: ack, req: req}
		}),
		WithRequestMessagesFunc(func(_ CallbackContext, ids [][]byte) {
			h.bodyReqs <- ids
		}),
	)
	cfg.DoneFunc = func(CallbackContext) error {
		h.done <- struct{}{}
		return nil
	}
	h.client = NewClient(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: h.errs,
		Version:   version,
	}, &cfg)
	h.client.Protocol.EnsureRegistered()
	m.Start()
	h.client.Start()
	t.Cleanup(func() { _ = h.client.Protocol.Stop(context.Background()) })
	return h
}

type serverHarness struct {
	peer      *referencePeer
	server    *Server
	idReplies chan int
	bodies    chan int
	errs      chan error
}

// newServerHarness connects a gOuroboros Server (the side that wants
// messages) to a scripted dmq-node peer that has them.
func newServerHarness(t *testing.T, version uint16) *serverHarness {
	t.Helper()
	peer, m := newReferencePeer(t)
	h := &serverHarness{
		peer:      peer,
		idReplies: make(chan int, 4),
		bodies:    make(chan int, 4),
		errs:      make(chan error, 8),
	}
	cfg := NewConfig(
		WithReplyMessageIdsFunc(func(
			_ CallbackContext,
			ids []pcommon.MessageIDAndSize,
		) {
			h.idReplies <- len(ids)
		}),
		WithReplyMessagesFunc(func(
			_ CallbackContext,
			msgs []pcommon.DmqMessage,
		) {
			h.bodies <- len(msgs)
		}),
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
	)
	h.server = NewServer(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: h.errs,
		Version:   version,
	}, &cfg)
	h.server.ProtocolInstance().EnsureRegistered()
	m.Start()
	h.server.Start()
	t.Cleanup(func() { _ = h.server.Stop(context.Background()) })
	return h
}

// In V1 the side with the messages is the initiator: it opens with MsgInit
// and answers the responder's requests in tx-submission's encoding.
func TestV1OutboundMatchesReferenceInbound(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN1)
	id := referenceID(0xa1)

	require.NoError(t, h.client.Init())
	// MsgInit = [6]
	h.peer.expect(true, []byte{0x81, 0x06})

	// MsgRequestTxIds blocking, ack 0, req 3 = [0, true, 0, 3]
	h.peer.send(false, []byte{0x84, 0x00, 0xf5, 0x00, 0x03})
	require.Equal(
		t,
		idRequest{blocking: true, ack: 0, req: 3},
		await(t, h.idRequests, h.errs, "the id request"),
	)
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	// MsgReplyTxIds = [1, [_ [id, 100]]]
	h.peer.expect(true, cat([]byte{0x82, 0x01, 0x9f}, idAndSize(id), []byte{0xff}))

	// MsgRequestTxs = [2, [_ id]]
	h.peer.send(false, cat([]byte{0x82, 0x02, 0x9f}, byteString32(id), []byte{0xff}))
	require.Equal(
		t,
		[][]byte{id},
		await(t, h.bodyReqs, h.errs, "the body request"),
	)
	require.NoError(t, h.client.ReplyMessages(nil))
	// MsgReplyTxs = [3, [_ ]]
	h.peer.expect(true, []byte{0x82, 0x03, 0x9f, 0xff})

	// MsgRequestTxIds blocking, ack 1, req 3 = [0, true, 1, 3]
	h.peer.send(false, []byte{0x84, 0x00, 0xf5, 0x01, 0x03})
	await(t, h.idRequests, h.errs, "the second id request")
	// Stop waits until MsgDone is written, and a pipe write completes only
	// once the peer reads it.
	stopped := make(chan error, 1)
	go func() { stopped <- h.client.Stop(context.Background()) }()
	// MsgDone = [4]
	h.peer.expect(true, []byte{0x81, 0x04})
	require.NoError(t, <-stopped)
}

// In V1 the side that wants messages is the responder: it waits for MsgInit
// and then asks.
func TestV1InboundMatchesReferenceOutbound(t *testing.T) {
	h := newServerHarness(t, protocol.ProtocolVersionDMQNtN1)
	id := referenceID(0xb2)

	h.peer.send(true, []byte{0x81, 0x06})
	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	h.peer.expect(false, []byte{0x84, 0x00, 0xf5, 0x00, 0x03})

	h.peer.send(true, cat([]byte{0x82, 0x01, 0x9f}, idAndSize(id), []byte{0xff}))
	require.Equal(t, 1, await(t, h.idReplies, h.errs, "the id reply"))

	require.NoError(t, h.server.RequestMessages([][]byte{id}))
	h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, byteString32(id), []byte{0xff}))

	h.peer.send(true, []byte{0x82, 0x03, 0x9f, 0xff})
	require.Equal(t, 0, await(t, h.bodies, h.errs, "the body reply"))

	require.NoError(t, h.server.RequestMessageIdsBlocking(1, 3))
	h.peer.expect(false, []byte{0x84, 0x00, 0xf5, 0x01, 0x03})
	// MsgDone = [4], the outbound's alternative to answering a blocking
	// request. The Server has no callback for it, so watch the run's
	// instance: the server then restarts on a new one, which is not done.
	run := h.server.ProtocolInstance()
	h.peer.send(true, []byte{0x81, 0x04})
	require.Eventually(
		t,
		run.IsDone,
		referenceWireWait,
		time.Millisecond,
		"MsgDone did not end the session",
	)
	require.Empty(t, h.errs)
}

// In V2 the side that wants messages is the initiator, and it is the one
// that speaks first; dmq-node's outbound may end a blocking request with
// MsgReplyNoSigIds.
func TestV2InboundIsInitiatorAndMatchesReferenceOutbound(t *testing.T) {
	h := newServerHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xc3)

	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	// MsgRequestSigIds blocking, ack 0, req 3 = [1, true, 0, 3]
	h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})

	// MsgReplyNoSigIds = [3]: the outbound had nothing within its 17 s
	// (DMQ/SigSubmissionV2/Outbound.hs 129-153).
	h.peer.send(false, []byte{0x81, 0x03})
	require.Equal(t, 0, await(t, h.idReplies, h.errs, "the empty id reply"))

	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	// MsgReplySigIds = [2, [_ [id, 100]]]
	h.peer.send(false, cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff}))
	require.Equal(t, 1, await(t, h.idReplies, h.errs, "the id reply"))

	require.NoError(t, h.server.RequestMessages([][]byte{id}))
	// MsgRequestSigs = [4, [_ id]]
	h.peer.expect(true, cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff}))
	// MsgReplySigs = [5, [_ ]]
	h.peer.send(false, []byte{0x82, 0x05, 0x9f, 0xff})
	require.Equal(t, 0, await(t, h.bodies, h.errs, "the body reply"))

	require.NoError(t, h.server.Done())
	// MsgDone = [6]
	h.peer.expect(true, []byte{0x81, 0x06})
}

// In V2 the side that has the messages is the responder: it waits for the
// initiator's request.
func TestV2OutboundIsResponderAndMatchesReferenceInbound(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xd4)

	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	require.Equal(
		t,
		idRequest{blocking: true, ack: 0, req: 3},
		await(t, h.idRequests, h.errs, "the id request"),
	)
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff}))

	h.peer.send(true, cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff}))
	require.Equal(
		t,
		[][]byte{id},
		await(t, h.bodyReqs, h.errs, "the body request"),
	)
	require.NoError(t, h.client.ReplyMessages(nil))
	h.peer.expect(false, []byte{0x82, 0x05, 0x9f, 0xff})

	h.peer.send(true, []byte{0x81, 0x06})
	await(t, h.done, h.errs, "MsgDone")
}

// A V2 Client with nothing to announce answers a blocking request with
// MsgReplyNoSigIds, which applies the request's acknowledgement, as
// dmq-node's outbound does after its 17 s wait (Outbound.hs 129-153).
func TestV2OutboundAnswersBlockingRequestWithNoIds(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xe5)

	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	await(t, h.idRequests, h.errs, "the first id request")
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff}))

	// Blocking, acknowledging the one outstanding id.
	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x01, 0x03})
	await(t, h.idRequests, h.errs, "the second id request")
	require.NoError(t, h.client.ReplyNoMessageIds())
	// MsgReplyNoSigIds = [3]
	h.peer.expect(false, []byte{0x81, 0x03})
	require.Empty(t, h.client.GetPendingMessageIDs())
	require.Error(t, h.client.ReplyNoMessageIds(), "nothing is outstanding")

	// The window is empty again, so the next request must be blocking and
	// acknowledge nothing.
	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	await(t, h.idRequests, h.errs, "the third id request")
}

func TestReplyNoMessageIdsRefusals(t *testing.T) {
	cfg := NewConfig()
	v1 := NewClient(newTestProtoOptions(protocol.ProtocolVersionDMQNtN1), &cfg)
	v1.pendingIDRequest = &messageIDRequest{blocking: true, requested: 1}
	require.ErrorContains(t, v1.ReplyNoMessageIds(), "only supported")

	v2 := NewClient(newTestProtoOptions(protocol.ProtocolVersionDMQNtN2), &cfg)
	v2.pendingMessageIDs = [][]byte{referenceID(1)}
	v2.pendingIDRequest = &messageIDRequest{requested: 1}
	require.ErrorContains(t, v2.ReplyNoMessageIds(), "non-blocking")
	require.Len(t, v2.GetPendingMessageIDs(), 1)
}

// The side each version starts on, as dmq-node's DMQ/NodeToNode.hs runs them.
func TestConnectionRolesFollowReference(t *testing.T) {
	for _, tc := range []struct {
		version    uint16
		clientRole protocol.ProtocolRole
		serverRole protocol.ProtocolRole
	}{
		{
			protocol.ProtocolVersionDMQNtN1,
			protocol.ProtocolRoleClient,
			protocol.ProtocolRoleServer,
		},
		{
			protocol.ProtocolVersionDMQNtN2,
			protocol.ProtocolRoleServer,
			protocol.ProtocolRoleClient,
		},
	} {
		cfg := NewConfig()
		opts := newTestProtoOptions(tc.version)
		require.Equal(t, tc.clientRole, NewClient(opts, &cfg).Role())
		require.Equal(t, tc.serverRole, NewServer(opts, &cfg).ProtocolInstance().Role())
	}
}

// A V2 Client holds a request for ids that dmq-node's requester pipelines
// behind its request for messages (SendMsgRequestSigsPipelined, then
// SendMsgRequestSigIdsPipelined, DMQ/SigSubmissionV2/Inbound.hs), and
// answers it after the messages instead of dropping the connection.
func TestV2OutboundHoldsPipelinedIdRequest(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xf6)

	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	await(t, h.idRequests, h.errs, "the id request")
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff}))

	h.peer.send(true, cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff}))
	// Non-blocking, acknowledging nothing, asking for one more id.
	h.peer.send(true, []byte{0x84, 0x01, 0xf4, 0x00, 0x01})
	require.Equal(
		t,
		[][]byte{id},
		await(t, h.bodyReqs, h.errs, "the body request"),
	)
	require.NoError(t, h.client.ReplyMessages(nil))
	h.peer.expect(false, []byte{0x82, 0x05, 0x9f, 0xff})
	require.Equal(
		t,
		idRequest{blocking: false, ack: 0, req: 1},
		await(t, h.idRequests, h.errs, "the pipelined id request"),
	)
}

// Both versions hold a request for messages that dmq-node's requester
// pipelines behind an unanswered non-blocking request for ids, or behind an
// unanswered request for messages, and answer it after the reply they owe.
// After collecting one pipelined reply with others still owed, the V2
// inbound's pipelinedDispatch may send SendMsgRequestSigsPipelined again
// (DMQ/SigSubmissionV2/Inbound.hs); the V1 inbound,
// txSubmissionServerPeerPipelined, does the same (TxSubmission/Inbound/V1.hs
// serverReqTxs, serverReqTxIds, serverIdle).
func TestOutboundHoldsPipelinedMessageRequests(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version uint16
		// The initiator's direction flag for the peer's messages.
		fromInitiator bool
		reqIds        byte
		replyIds      byte
		reqMsgs       byte
		replyMsgs     byte
	}{
		{"V1", protocol.ProtocolVersionDMQNtN1, false, 0x00, 0x01, 0x02, 0x03},
		{"V2", protocol.ProtocolVersionDMQNtN2, true, 0x01, 0x02, 0x04, 0x05},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newClientHarness(t, tc.version)
			a := referenceID(0xc1)
			b := referenceID(0xc2)
			if tc.version == protocol.ProtocolVersionDMQNtN1 {
				require.NoError(t, h.client.Init())
				h.peer.expect(true, []byte{0x81, 0x06})
			}
			// Blocking, ack 0, req 2.
			h.peer.send(tc.fromInitiator, []byte{0x84, tc.reqIds, 0xf5, 0x00, 0x02})
			await(t, h.idRequests, h.errs, "the id request")
			require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
				{MessageID: a, SizeInBytes: 100},
				{MessageID: b, SizeInBytes: 100},
			}))
			h.peer.expect(!tc.fromInitiator, cat(
				[]byte{0x82, tc.replyIds, 0x9f},
				idAndSize(a), idAndSize(b),
				[]byte{0xff},
			))

			// Non-blocking, ack 0, req 1, with a request for a's message
			// pipelined behind it.
			h.peer.send(tc.fromInitiator, []byte{0x84, tc.reqIds, 0xf4, 0x00, 0x01})
			h.peer.send(tc.fromInitiator, cat(
				[]byte{0x82, tc.reqMsgs, 0x9f}, byteString32(a), []byte{0xff},
			))
			require.Equal(
				t,
				idRequest{blocking: false, ack: 0, req: 1},
				await(t, h.idRequests, h.errs, "the non-blocking id request"),
			)
			require.NoError(t, h.client.ReplyMessageIds(nil))
			h.peer.expect(!tc.fromInitiator, []byte{0x82, tc.replyIds, 0x9f, 0xff})
			require.Equal(
				t,
				[][]byte{a},
				await(t, h.bodyReqs, h.errs, "the message request behind ids"),
			)

			// A request for b's message pipelined behind the one for a's.
			h.peer.send(tc.fromInitiator, cat(
				[]byte{0x82, tc.reqMsgs, 0x9f}, byteString32(b), []byte{0xff},
			))
			require.NoError(t, h.client.ReplyMessages(nil))
			h.peer.expect(!tc.fromInitiator, []byte{0x82, tc.replyMsgs, 0x9f, 0xff})
			require.Equal(
				t,
				[][]byte{b},
				await(t, h.bodyReqs, h.errs, "the message request behind messages"),
			)
		})
	}
}
