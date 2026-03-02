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

package localmessagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
)

// TestMsgSubmitMessageType tests MsgSubmitMessage type and field access
func TestMsgSubmitMessageType(t *testing.T) {
	msg := &MsgSubmitMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSubmitMessage,
		},
		Message: pcommon.DmqMessage{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("test-id"),
				MessageBody: []byte("test-body"),
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

	assert.Equal(t, uint8(MessageTypeSubmitMessage), msg.Type())
	assert.Equal(t, []byte("test-id"), msg.Message.Payload.MessageID)
}

// TestMsgSubmitMessageCBORRoundTrip tests encode/decode round-trip for MsgSubmitMessage
func TestMsgSubmitMessageCBORRoundTrip(t *testing.T) {
	msg := &MsgSubmitMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSubmitMessage,
		},
		Message: pcommon.DmqMessage{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("test-id"),
				MessageBody: []byte("test-body"),
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

	data, err := cbor.Encode(msg)
	assert.NoError(t, err)

	parsed, err := NewMsgFromCbor(uint(MessageTypeSubmitMessage), data)
	assert.NoError(t, err)

	// Type assertion and deep field equality checks
	parsedMsg, ok := parsed.(*MsgSubmitMessage)
	if !ok {
		t.Fatalf("expected *MsgSubmitMessage, got %T", parsed)
	}
	assert.Equal(t, msg.Type(), parsedMsg.Type())
	assert.Equal(
		t,
		msg.Message.Payload.MessageID,
		parsedMsg.Message.Payload.MessageID,
	)
	assert.Equal(
		t,
		msg.Message.Payload.MessageBody,
		parsedMsg.Message.Payload.MessageBody,
	)
	assert.Equal(
		t,
		msg.Message.Payload.KESPeriod,
		parsedMsg.Message.Payload.KESPeriod,
	)
	assert.Equal(
		t,
		msg.Message.Payload.ExpiresAt,
		parsedMsg.Message.Payload.ExpiresAt,
	)
	assert.Equal(
		t,
		len(msg.Message.KESSignature),
		len(parsedMsg.Message.KESSignature),
	)
	assert.Equal(
		t,
		msg.Message.OperationalCertificate.IssueNumber,
		parsedMsg.Message.OperationalCertificate.IssueNumber,
	)
	assert.Equal(
		t,
		msg.Message.OperationalCertificate.KESPeriod,
		parsedMsg.Message.OperationalCertificate.KESPeriod,
	)
	assert.Equal(
		t,
		len(msg.Message.OperationalCertificate.KESVerificationKey),
		len(parsedMsg.Message.OperationalCertificate.KESVerificationKey),
	)
	assert.Equal(
		t,
		len(msg.Message.OperationalCertificate.ColdSignature),
		len(parsedMsg.Message.OperationalCertificate.ColdSignature),
	)
	assert.Equal(
		t,
		len(msg.Message.ColdVerificationKey),
		len(parsedMsg.Message.ColdVerificationKey),
	)

	// Deterministic re-encode
	reencoded, err := cbor.Encode(parsedMsg)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgAcceptMessageEncoding tests MsgAcceptMessage encoding
func TestMsgAcceptMessageEncoding(t *testing.T) {
	msg := &MsgAcceptMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptMessage,
		},
	}

	assert.Equal(t, uint8(MessageTypeAcceptMessage), msg.Type())
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeAcceptMessage), data)
	assert.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeAcceptMessage), parsed.Type())
}

// TestMsgRejectMessageEncoding tests MsgRejectMessage encoding with different reasons
func TestMsgRejectMessageEncoding(t *testing.T) {
	invalidReason := pcommon.InvalidReason{
		Message: "Invalid signature",
	}

	msg := NewMsgRejectMessage(invalidReason)

	assert.Equal(t, uint8(MessageTypeRejectMessage), msg.Type())
	assert.NotNil(t, msg.Reason)
	assert.Equal(t, uint8(0), msg.Reason.RejectReasonType())

	// Verify deterministic encoding: two equivalent messages should encode identically.
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	// Recreate the same message and re-encode
	msg2 := NewMsgRejectMessage(invalidReason)
	data2, err := cbor.Encode(msg2)
	assert.NoError(t, err)
	assert.Equal(t, data, data2)

	// Verify decode round-trip preserves Reason
	parsed, err := NewMsgFromCbor(uint(MessageTypeRejectMessage), data)
	assert.NoError(t, err)
	parsedMsg, ok := parsed.(*MsgRejectMessage)
	if !ok {
		t.Fatalf("expected *MsgRejectMessage, got %T", parsed)
	}
	// Convert back to public API type and check fields
	rr := pcommon.FromRejectReasonData(parsedMsg.Reason)
	inv, ok := rr.(pcommon.InvalidReason)
	if !ok {
		t.Fatalf("expected InvalidReason, got %T", rr)
	}
	assert.Equal(t, invalidReason.Message, inv.Message)
}

// TestMsgRejectMessageAlreadyReceived tests rejection with AlreadyReceivedReason
func TestMsgRejectMessageAlreadyReceived(t *testing.T) {
	reason := pcommon.AlreadyReceivedReason{}

	msg := NewMsgRejectMessage(reason)

	assert.Equal(t, uint8(1), msg.Reason.RejectReasonType())
}

// TestMsgRejectMessageExpired tests rejection with ExpiredReason
func TestMsgRejectMessageExpired(t *testing.T) {
	reason := pcommon.ExpiredReason{}

	msg := NewMsgRejectMessage(reason)

	assert.Equal(t, uint8(2), msg.Reason.RejectReasonType())
}

// TestMsgRejectMessageOther tests rejection with OtherReason
func TestMsgRejectMessageOther(t *testing.T) {
	reason := pcommon.OtherReason{
		Message: "Some other reason",
	}

	msg := NewMsgRejectMessage(reason)

	assert.Equal(t, uint8(3), msg.Reason.RejectReasonType())
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

// TestNewMsgFromCborUnknownType tests handling of unknown message type
func TestNewMsgFromCborUnknownType(t *testing.T) {
	_, err := NewMsgFromCbor(99, []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}
