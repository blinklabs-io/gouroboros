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

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestClientRefusesRepeatedMessageRequest checks that a V2 client serves a
// message body at most once while its ID is unacknowledged. dmq-node's V2
// outbound refuses such a request (ProtocolErrorDuplicateRequest); serving it
// again lets a peer make the client resend bodies without bound.
func TestClientRefusesRepeatedMessageRequest(t *testing.T) {
	id := testMessageID(0xa1)
	otherID := testMessageID(0xb2)
	calls := 0
	cfg := NewConfig(WithRequestMessagesFunc(
		func(CallbackContext, [][]byte) { calls++ },
	))
	client := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	client.pendingMessageIDs = [][]byte{id, otherID}

	require.NoError(t, client.handleRequestMessages(
		NewMsgRequestMessages(client.protoVersion, [][]byte{id}),
	))
	// The application has answered the first request.
	client.pendingMsgRequest = nil

	err := client.handleRequestMessages(
		NewMsgRequestMessages(client.protoVersion, [][]byte{otherID, id}),
	)
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	require.Equal(t, 1, calls)
	require.Nil(t, client.pendingMsgRequest)
}

// TestV1ClientServesRepeatedMessageRequest checks that a V1 client serves a
// body asked for again while its ID is unacknowledged. V1 is tx-submission,
// whose reference outbound (txSubmissionOutbound) refuses only an ID repeated
// within one request or not outstanding, so a reference V1 peer may repeat
// a request across requests and must not be disconnected for it.
func TestV1ClientServesRepeatedMessageRequest(t *testing.T) {
	id := testMessageID(0xa1)
	otherID := testMessageID(0xb2)
	calls := 0
	cfg := NewConfig(WithRequestMessagesFunc(
		func(CallbackContext, [][]byte) { calls++ },
	))
	client := NewClient(newTestProtoOptions(1), &cfg)
	client.pendingMessageIDs = [][]byte{id, otherID}

	require.NoError(t, client.handleRequestMessages(
		NewMsgRequestMessages(client.protoVersion, [][]byte{id}),
	))
	client.pendingMsgRequest = nil

	require.NoError(t, client.handleRequestMessages(
		NewMsgRequestMessages(client.protoVersion, [][]byte{otherID, id}),
	))
	require.Equal(t, 2, calls)

	// A repeat within one request is still refused, as the reference does.
	client.pendingMsgRequest = nil
	err := client.handleRequestMessages(
		NewMsgRequestMessages(client.protoVersion, [][]byte{id, id}),
	)
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
}

// TestServerRefusesToRepeatMessageRequest checks that RequestMessages does
// not ask for an ID it already asked for while the ID is unacknowledged, so
// that a gOuroboros server is never disconnected by a peer enforcing the
// rule.
func TestServerRefusesToRepeatMessageRequest(t *testing.T) {
	id := testMessageID(0xa1)
	server := newStartedMessageSubmissionServer(t)
	server.lock.Lock()
	server.pendingMessageIDs = [][]byte{id}
	server.lock.Unlock()

	require.NoError(t, server.RequestMessages([][]byte{id}))
	// The peer has answered the first request.
	server.lock.Lock()
	server.pendingMsgRequest = nil
	server.lock.Unlock()

	err := server.RequestMessages([][]byte{id})
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	server.lock.Lock()
	defer server.lock.Unlock()
	require.Nil(t, server.pendingMsgRequest)
}

// TestServerMayRequestReannouncedID checks that the record of requested IDs
// is released with the acknowledgement: an ID acknowledged and later
// announced again is a new announcement whose body may be requested.
func TestServerMayRequestReannouncedID(t *testing.T) {
	id := testMessageID(0xa1)
	server := newStartedMessageSubmissionServer(t)
	server.lock.Lock()
	server.pendingMessageIDs = [][]byte{id}
	server.lock.Unlock()
	require.NoError(t, server.RequestMessages([][]byte{id}))

	server.lock.Lock()
	server.pendingMsgRequest = nil
	server.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		ack:       1,
		requested: 1,
	}
	server.lock.Unlock()
	require.NoError(t, server.handleReplyMessageIds(NewMsgReplyMessageIds(
		server.protoVersion,
		[]pcommon.MessageIDAndSize{{MessageID: id, SizeInBytes: 1}},
	)))

	require.NoError(t, server.RequestMessages([][]byte{id}))
}

// TestServerUnsentRequestReleasesItsIDs checks that a request the server
// could not send does not count as asked for, so it can be retried.
func TestServerUnsentRequestReleasesItsIDs(t *testing.T) {
	id := testMessageID(0xa1)
	server := newStartedMessageSubmissionServer(t)
	server.lock.Lock()
	server.pendingMessageIDs = [][]byte{id}
	server.lock.Unlock()
	server.Protocol.Stop()

	require.Error(t, server.RequestMessages([][]byte{id}))
	server.lock.Lock()
	defer server.lock.Unlock()
	require.Empty(t, server.requestedIDs)
}

// TestV2ClientServesReannouncedIDAfterNoIdsReply checks that
// MsgReplyNoMessageIds releases the record of requested IDs with the
// acknowledgement it applies, as a reply with IDs does: an ID acknowledged
// that way and later announced again is a new announcement whose body the
// peer may request.
func TestV2ClientServesReannouncedIDAfterNoIdsReply(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xf6)
	announce := cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff})
	request := cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff})

	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	await(t, h.idRequests, h.errs, "the first id request")
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	h.peer.expect(false, announce)
	h.peer.send(true, request)
	await(t, h.bodyReqs, h.errs, "the first body request")
	require.NoError(t, h.client.ReplyMessages(nil))
	h.peer.expect(false, []byte{0x82, 0x05, 0x9f, 0xff})

	// Blocking, acknowledging the id, answered with no ids.
	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x01, 0x03})
	await(t, h.idRequests, h.errs, "the second id request")
	require.NoError(t, h.client.ReplyNoMessageIds())
	h.peer.expect(false, []byte{0x81, 0x03})

	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	await(t, h.idRequests, h.errs, "the third id request")
	require.NoError(t, h.client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: id, SizeInBytes: 100},
	}))
	h.peer.expect(false, announce)
	h.peer.send(true, request)
	require.Equal(
		t,
		[][]byte{id},
		await(t, h.bodyReqs, h.errs, "the re-announced body request"),
	)
}

// TestV2ServerMayRequestReannouncedIDAfterNoIdsReply is the server side of
// TestV2ClientServesReannouncedIDAfterNoIdsReply.
func TestV2ServerMayRequestReannouncedIDAfterNoIdsReply(t *testing.T) {
	h := newServerHarness(t, protocol.ProtocolVersionDMQNtN2)
	id := referenceID(0xf7)
	announce := cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff})

	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	h.peer.send(false, announce)
	await(t, h.idReplies, h.errs, "the id reply")
	require.NoError(t, h.server.RequestMessages([][]byte{id}))
	h.peer.expect(true, cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff}))
	h.peer.send(false, []byte{0x82, 0x05, 0x9f, 0xff})
	await(t, h.bodies, h.errs, "the body reply")

	require.NoError(t, h.server.RequestMessageIdsBlocking(1, 3))
	h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x01, 0x03})
	h.peer.send(false, []byte{0x81, 0x03})
	await(t, h.idReplies, h.errs, "the empty id reply")

	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	h.peer.send(false, announce)
	await(t, h.idReplies, h.errs, "the re-announcement")
	require.NoError(t, h.server.RequestMessages([][]byte{id}))
}

// TestServerRestartForgetsRequestedIDs checks that the record of requested
// IDs goes with the run's outstanding IDs when the server restarts after a
// V1 client's MsgDone. That MsgDone answers a blocking request whose
// acknowledgement is never applied, so without the reset a client that
// announces the same ID in the next run could not have its body requested.
func TestServerRestartForgetsRequestedIDs(t *testing.T) {
	id := testMessageID(0xa1)
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	options := newTestProtoOptions(protocol.ProtocolVersionDMQNtN1)
	options.Muxer = m
	options.ErrorChan = make(chan error, 1)
	server := NewServer(options, nil)
	server.ProtocolInstance().EnsureRegistered()
	m.Start()
	server.Start()
	t.Cleanup(func() {
		server.Stop()
		m.Stop()
		_ = peerConn.Close()
		_ = localConn.Close()
	})
	server.lock.Lock()
	server.pendingMessageIDs = [][]byte{id}
	server.requestedIDs = map[string]struct{}{string(id): {}}
	server.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		ack:       1,
		requested: 1,
	}
	server.lock.Unlock()

	require.NoError(t, server.handleDone())

	server.lock.Lock()
	defer server.lock.Unlock()
	require.Empty(t, server.pendingMessageIDs)
	require.Empty(t, server.requestedIDs)
}
