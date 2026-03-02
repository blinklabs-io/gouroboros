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

package messagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
)

// TestMsgInitEncoding tests MsgInit message encoding
func TestMsgInitEncoding(t *testing.T) {
	msg := &MsgInit{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeInit,
		},
	}

	assert.Equal(t, uint8(MessageTypeInit), msg.Type())
	// CBOR encode/decode round-trip
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeInit), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeInit), parsed.Type())
}

// TestMsgRequestMessageIdsEncoding tests MsgRequestMessageIds encoding
func TestMsgRequestMessageIdsEncoding(t *testing.T) {
	msg := &MsgRequestMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessageIds,
		},
		IsBlocking:   true,
		AckCount:     5,
		RequestCount: 10,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessageIds), msg.Type())
	assert.Equal(t, true, msg.IsBlocking)
	assert.Equal(t, uint16(5), msg.AckCount)
	assert.Equal(t, uint16(10), msg.RequestCount)
	// CBOR encode/decode round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessageIds), data)
	assert.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessageIds)
	assert.True(t, ok)
	assert.Equal(t, msg.IsBlocking, decoded.IsBlocking)
	assert.Equal(t, msg.AckCount, decoded.AckCount)
	assert.Equal(t, msg.RequestCount, decoded.RequestCount)
	// Re-encode and assert deterministic equality
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessageIdsEncoding tests MsgReplyMessageIds encoding
func TestMsgReplyMessageIdsEncoding(t *testing.T) {
	messages := []pcommon.MessageIDAndSize{
		{
			MessageID:   []byte("id1"),
			SizeInBytes: 100,
		},
		{
			MessageID:   []byte("id2"),
			SizeInBytes: 200,
		},
	}

	msg := &MsgReplyMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessageIds,
		},
		Messages: messages,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessageIds), msg.Type())
	assert.Equal(t, 2, len(msg.Messages))
	assert.Equal(t, []byte("id1"), msg.Messages[0].MessageID)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeReplyMessageIds), data)
	assert.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessageIds)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	assert.Equal(t, msg.Messages[0].MessageID, decoded.Messages[0].MessageID)
	// Re-encode deterministic equality
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgRequestMessagesEncoding tests MsgRequestMessages encoding
func TestMsgRequestMessagesEncoding(t *testing.T) {
	messageIDs := [][]byte{
		[]byte("id1"),
		[]byte("id2"),
		[]byte("id3"),
	}

	msg := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		MessageIDs: messageIDs,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessages), msg.Type())
	assert.Equal(t, 3, len(msg.MessageIDs))
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	assert.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessages)
	assert.True(t, ok)
	assert.Equal(t, len(msg.MessageIDs), len(decoded.MessageIDs))
	for i := range msg.MessageIDs {
		assert.Equal(t, msg.MessageIDs[i], decoded.MessageIDs[i])
	}
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesEncoding tests MsgReplyMessages encoding
func TestMsgReplyMessagesEncoding(t *testing.T) {
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

	msg := &MsgReplyMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessages,
		},
		Messages: messages,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessages), msg.Type())
	assert.Equal(t, 1, len(msg.Messages))
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeReplyMessages), data)
	assert.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessages)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	assert.Equal(
		t,
		msg.Messages[0].Payload.MessageID,
		decoded.Messages[0].Payload.MessageID,
	)
	// Re-encode deterministic equality
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgDoneEncoding tests MsgDone encoding
func TestMsgDoneEncoding(t *testing.T) {
	msg := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}

	assert.Equal(t, uint8(MessageTypeDone), msg.Type())
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeDone), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeDone), parsed.Type())
}

// TestNewMsgFromCborInit tests parsing MsgInit from CBOR
func TestNewMsgFromCborInit(t *testing.T) {
	msg := &MsgInit{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeInit,
		},
	}
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeInit), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeInit), parsed.Type())
}

// TestNewMsgFromCborUnknownType tests handling of unknown message type
func TestNewMsgFromCborUnknownType(t *testing.T) {
	_, err := NewMsgFromCbor(99, []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}

// Positive CBOR round-trip tests for message types
// Note: This comprehensive round-trip test intentionally overlaps with individual
// encoding tests to provide broader regression coverage across types. Keeping
// redundancy here ensures future changes don't break CBOR parsing semantics.
func TestNewMsgFromCborRoundTrip(t *testing.T) {
	// MsgInit
	initMsg := &MsgInit{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeInit},
	}
	data, err := cbor.Encode(initMsg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeInit), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeInit), parsed.Type())

	// MsgRequestMessageIds
	req := &MsgRequestMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessageIds,
		},
		IsBlocking:   true,
		AckCount:     1,
		RequestCount: 2,
	}
	data, err = cbor.Encode(req)
	assert.NoError(t, err)
	parsed, err = NewMsgFromCbor(uint(MessageTypeRequestMessageIds), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeRequestMessageIds), parsed.Type())
	// Verify decoded values match original
	if decoded, ok := parsed.(*MsgRequestMessageIds); ok {
		assert.Equal(t, req.IsBlocking, decoded.IsBlocking)
		assert.Equal(t, req.AckCount, decoded.AckCount)
		assert.Equal(t, req.RequestCount, decoded.RequestCount)
		// Re-encode and assert deterministic equality
		reencoded, err := cbor.Encode(decoded)
		assert.NoError(t, err)
		assert.Equal(t, data, reencoded)
	} else {
		t.Fatalf("decoded message is not *MsgRequestMessageIds: %T", parsed)
	}

	// MsgReplyMessageIds
	msgs := []pcommon.MessageIDAndSize{
		{MessageID: []byte("id1"), SizeInBytes: 10},
	}
	reply := &MsgReplyMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessageIds,
		},
		Messages: msgs,
	}
	data, err = cbor.Encode(reply)
	assert.NoError(t, err)
	parsed, err = NewMsgFromCbor(uint(MessageTypeReplyMessageIds), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeReplyMessageIds), parsed.Type())

	// MsgRequestMessages
	reqMsgs := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		MessageIDs: [][]byte{[]byte("id1")},
	}
	data, err = cbor.Encode(reqMsgs)
	assert.NoError(t, err)
	parsed, err = NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeRequestMessages), parsed.Type())

	// MsgReplyMessages
	payload := pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   []byte("id1"),
			MessageBody: []byte("body"),
			KESPeriod:   1,
			ExpiresAt:   2000000000,
		},
		KESSignature: make([]byte, 448),
		OperationalCertificate: pcommon.OperationalCertificate{
			KESVerificationKey: make([]byte, 32),
			IssueNumber:        1,
			KESPeriod:          1,
			ColdSignature:      make([]byte, 64),
		},
		ColdVerificationKey: make([]byte, 32),
	}
	replyMsgs := &MsgReplyMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessages,
		},
		Messages: []pcommon.DmqMessage{payload},
	}
	data, err = cbor.Encode(replyMsgs)
	assert.NoError(t, err)
	parsed, err = NewMsgFromCbor(uint(MessageTypeReplyMessages), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeReplyMessages), parsed.Type())

	// MsgDone
	done := &MsgDone{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeDone},
	}
	data, err = cbor.Encode(done)
	assert.NoError(t, err)
	parsed, err = NewMsgFromCbor(uint(MessageTypeDone), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeDone), parsed.Type())
}
