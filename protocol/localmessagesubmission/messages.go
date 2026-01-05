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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Message type constants following CIP-0137 CDDL specification
const (
	MessageTypeSubmitMessage = 0
	MessageTypeAcceptMessage = 1
	MessageTypeRejectMessage = 2
	MessageTypeDone          = 3
)

// MsgSubmitMessage represents a local message submission
type MsgSubmitMessage struct {
	protocol.MessageBase
	Message pcommon.DmqMessage
}

// NewMsgSubmitMessage creates a new MsgSubmitMessage
func NewMsgSubmitMessage(msg pcommon.DmqMessage) *MsgSubmitMessage {
	return &MsgSubmitMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSubmitMessage,
		},
		Message: msg,
	}
}

// MsgAcceptMessage represents acceptance of a submitted message
type MsgAcceptMessage struct {
	protocol.MessageBase
}

// NewMsgAcceptMessage creates a new MsgAcceptMessage
func NewMsgAcceptMessage() *MsgAcceptMessage {
	return &MsgAcceptMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptMessage,
		},
	}
}

// MsgRejectMessage represents rejection of a submitted message
type MsgRejectMessage struct {
	protocol.MessageBase
	Reason pcommon.RejectReasonData
}

// NewMsgRejectMessage creates a new MsgRejectMessage
func NewMsgRejectMessage(reason pcommon.RejectReason) *MsgRejectMessage {
	return &MsgRejectMessage{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRejectMessage,
		},
		Reason: pcommon.ToRejectReasonData(reason),
	}
}

// MsgDone represents the protocol completion message
type MsgDone struct {
	protocol.MessageBase
}

// NewMsgDone creates a new MsgDone message
func NewMsgDone() *MsgDone {
	return &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
}

// NewMsgFromCbor parses a Local Message Submission message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeSubmitMessage:
		ret = &MsgSubmitMessage{}
	case MessageTypeAcceptMessage:
		ret = &MsgAcceptMessage{}
	case MessageTypeRejectMessage:
		ret = &MsgRejectMessage{}
	case MessageTypeDone:
		ret = &MsgDone{}
	default:
		return nil, fmt.Errorf(
			"%s: unknown message type: %d",
			ProtocolName,
			msgType,
		)
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	// Store the raw message CBOR (ret is always non-nil for handled types)
	ret.SetCbor(data)
	return ret, nil
}

// Type returns the message type
func (m *MsgSubmitMessage) Type() uint8 {
	return MessageTypeSubmitMessage
}

// Type returns the message type
func (m *MsgAcceptMessage) Type() uint8 {
	return MessageTypeAcceptMessage
}

// Type returns the message type
func (m *MsgRejectMessage) Type() uint8 {
	return MessageTypeRejectMessage
}

// Type returns the message type
func (m *MsgDone) Type() uint8 {
	return MessageTypeDone
}
