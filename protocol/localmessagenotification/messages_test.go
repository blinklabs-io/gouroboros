// Copyright 2025 Blink Labs Software
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

package localmessagenotification

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMsgRequestMessagesNonBlocking tests MsgRequestMessages non-blocking variant
func TestMsgRequestMessagesNonBlocking(t *testing.T) {
	msg := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		IsBlocking: false,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessages), msg.Type())
	assert.False(t, msg.IsBlocking)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessages)
	assert.True(t, ok)
	assert.Equal(t, msg.IsBlocking, decoded.IsBlocking)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgRequestMessagesBlocking tests MsgRequestMessages blocking variant
func TestMsgRequestMessagesBlocking(t *testing.T) {
	msg := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		IsBlocking: true,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessages), msg.Type())
	assert.True(t, msg.IsBlocking)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessages)
	assert.True(t, ok)
	assert.Equal(t, msg.IsBlocking, decoded.IsBlocking)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesNonBlocking tests MsgReplyMessagesNonBlocking struct creation
func TestMsgReplyMessagesNonBlocking(t *testing.T) {
	messages := []pcommon.DmqMessage{
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id1"),
				MessageBody: []byte("body1"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
	}

	msg := &MsgReplyMessagesNonBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesNonBlocking,
		},
		Messages: messages,
		HasMore:  true,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesNonBlocking), msg.Type())
	assert.Equal(t, 1, len(msg.Messages))
	assert.True(t, msg.HasMore)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesNonBlocking),
		data,
	)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesNonBlocking)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	assert.Equal(t, msg.HasMore, decoded.HasMore)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesNonBlockingEmpty tests empty reply struct with HasMore flag
func TestMsgReplyMessagesNonBlockingEmpty(t *testing.T) {
	msg := &MsgReplyMessagesNonBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesNonBlocking,
		},
		Messages: []pcommon.DmqMessage{},
		HasMore:  false,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesNonBlocking), msg.Type())
	assert.Equal(t, 0, len(msg.Messages))
	assert.False(t, msg.HasMore)
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesNonBlocking),
		data,
	)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesNonBlocking)
	assert.True(t, ok)
	assert.Equal(t, msg.HasMore, decoded.HasMore)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesBlocking tests MsgReplyMessagesBlocking struct creation
func TestMsgReplyMessagesBlocking(t *testing.T) {
	messages := []pcommon.DmqMessage{
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id1"),
				MessageBody: []byte("body1"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id2"),
				MessageBody: []byte("body2"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
	}

	msg := &MsgReplyMessagesBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesBlocking,
		},
		Messages: messages,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesBlocking), msg.Type())
	assert.Equal(t, 2, len(msg.Messages))
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeReplyMessagesBlocking), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesBlocking)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgClientDoneEncoding tests MsgClientDone encoding
func TestMsgClientDoneEncoding(t *testing.T) {
	msg := &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeClientDone,
		},
	}

	assert.Equal(t, uint8(MessageTypeClientDone), msg.Type())
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeClientDone), data)
	require.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeClientDone), parsed.Type())
}

// TestNewMsgFromCborUnknownType tests handling of unknown message type
func TestNewMsgFromCborUnknownType(t *testing.T) {
	_, err := NewMsgFromCbor(99, []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}
