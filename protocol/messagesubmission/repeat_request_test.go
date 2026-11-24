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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestClientRefusesRepeatedMessageRequest checks that the client serves a
// message body at most once while its ID is unacknowledged. The protocol
// forbids asking for an ID "already asked for", and dmq-node's outbound
// refuses it (ProtocolErrorDuplicateRequest); serving it again lets a peer
// make the client resend bodies without bound.
func TestClientRefusesRepeatedMessageRequest(t *testing.T) {
	for _, version := range []uint16{1, MessageSubmissionV2MinVersion} {
		t.Run(fmt.Sprintf("version=%d", version), func(t *testing.T) {
			id := testMessageID(0xa1)
			otherID := testMessageID(0xb2)
			calls := 0
			cfg := NewConfig(WithRequestMessagesFunc(
				func(CallbackContext, [][]byte) { calls++ },
			))
			client := NewClient(newTestProtoOptions(version), &cfg)
			client.pendingMessageIDs = [][]byte{id, otherID}

			require.NoError(t, client.handleRequestMessages(
				NewMsgRequestMessages([][]byte{id}),
			))
			// The application has answered the first request.
			client.pendingMsgRequest = nil

			err := client.handleRequestMessages(
				NewMsgRequestMessages([][]byte{otherID, id}),
			)
			require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
			require.Equal(t, 1, calls)
			require.Nil(t, client.pendingMsgRequest)
		})
	}
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
