// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package messagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestCollectionLimitsFollowProtocolDirection(t *testing.T) {
	cfg := NewConfig(WithMaxUnacknowledgedMessageIDs(7))
	client := &Client{config: &cfg, pendingMessageIDs: [][]byte{{1}, {2}, {3}}}
	require.Equal(t, 3, client.collectionLimit(MessageTypeRequestMessages))
	require.Equal(t, 7, client.collectionLimit(MessageTypeReplyMessageIds))

	server := &Server{config: &cfg, expectedReplyMessageIDs: 5, expectedReplyMessages: 2}
	require.Equal(t, 5, server.collectionLimit(MessageTypeReplyMessageIds))
	require.Equal(t, 2, server.collectionLimit(MessageTypeReplyMessages))
	require.Equal(t, 7, server.collectionLimit(MessageTypeRequestMessages))
}

func TestClientReplyCountsCannotExceedPeerRequest(t *testing.T) {
	cfg := NewConfig(WithMaxUnacknowledgedMessageIDs(7))
	client := &Client{config: &cfg, requestedReplyMessageIDs: 2, requestedReplyMessages: 1}
	require.ErrorContains(t, client.ReplyMessageIds(make([]pcommon.MessageIDAndSize, 3)), "requested maximum is 2")
	require.ErrorContains(t, client.ReplyMessages(make([]pcommon.DmqMessage, 2)), "requested maximum is 1")
}

func TestMessageSubmissionAcceptsConfiguredCollectionLimit(t *testing.T) {
	ids := make([][]byte, 7)
	for idx := range ids {
		ids[idx] = []byte{byte(idx)}
	}
	wire, err := cbor.Encode(NewMsgRequestMessages(ids))
	require.NoError(t, err)
	msg, err := decodeMsgFromCborWithLimit(MessageTypeRequestMessages, wire, len(ids))
	require.NoError(t, err)
	require.Len(t, msg.(*MsgRequestMessages).MessageIDs, len(ids))
}
