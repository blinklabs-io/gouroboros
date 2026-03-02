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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Message type constants following CIP-0137 CDDL specification
const (
	MessageTypeInit              = 0
	MessageTypeRequestMessageIds = 1
	MessageTypeReplyMessageIds   = 2
	MessageTypeRequestMessages   = 3
	MessageTypeReplyMessages     = 4
	MessageTypeDone              = 5
)

// MsgInit represents the initialization message
type MsgInit struct {
	protocol.MessageBase
}

// NewMsgInit creates a new MsgInit
func NewMsgInit() *MsgInit {
	return &MsgInit{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeInit,
		},
	}
}

// MsgRequestMessageIds represents a request for message IDs (blocking or non-blocking)
type MsgRequestMessageIds struct {
	protocol.MessageBase
	IsBlocking   bool
	AckCount     uint16
	RequestCount uint16
}

// NewMsgRequestMessageIds creates a new MsgRequestMessageIds
func NewMsgRequestMessageIds(
	isBlocking bool,
	ackCount, requestCount uint16,
) *MsgRequestMessageIds {
	return &MsgRequestMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessageIds,
		},
		IsBlocking:   isBlocking,
		AckCount:     ackCount,
		RequestCount: requestCount,
	}
}

// MsgReplyMessageIds represents the reply with message IDs and sizes
type MsgReplyMessageIds struct {
	protocol.MessageBase
	Messages []pcommon.MessageIDAndSize
}

// NewMsgReplyMessageIds creates a new MsgReplyMessageIds
func NewMsgReplyMessageIds(
	messages []pcommon.MessageIDAndSize,
) *MsgReplyMessageIds {
	return &MsgReplyMessageIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessageIds,
		},
		Messages: messages,
	}
}

// MsgRequestMessages represents a request for specific messages
type MsgRequestMessages struct {
	protocol.MessageBase
	MessageIDs [][]byte
}

// NewMsgRequestMessages creates a new MsgRequestMessages
func NewMsgRequestMessages(messageIDs [][]byte) *MsgRequestMessages {
	return &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		MessageIDs: messageIDs,
	}
}

// MsgReplyMessages represents the reply with full messages
type MsgReplyMessages struct {
	protocol.MessageBase
	Messages []pcommon.DmqMessage
}

// NewMsgReplyMessages creates a new MsgReplyMessages
func NewMsgReplyMessages(messages []pcommon.DmqMessage) *MsgReplyMessages {
	return &MsgReplyMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessages,
		},
		Messages: messages,
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

// NewMsgFromCbor parses a Message Submission message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeInit:
		ret = &MsgInit{}
	case MessageTypeRequestMessageIds:
		ret = &MsgRequestMessageIds{}
	case MessageTypeReplyMessageIds:
		ret = &MsgReplyMessageIds{}
	case MessageTypeRequestMessages:
		ret = &MsgRequestMessages{}
	case MessageTypeReplyMessages:
		ret = &MsgReplyMessages{}
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
func (m *MsgInit) Type() uint8 {
	return MessageTypeInit
}

// Type returns the message type
func (m *MsgRequestMessageIds) Type() uint8 {
	return MessageTypeRequestMessageIds
}

// Type returns the message type
func (m *MsgReplyMessageIds) Type() uint8 {
	return MessageTypeReplyMessageIds
}

// Type returns the message type
func (m *MsgRequestMessages) Type() uint8 {
	return MessageTypeRequestMessages
}

// Type returns the message type
func (m *MsgReplyMessages) Type() uint8 {
	return MessageTypeReplyMessages
}

// Type returns the message type
func (m *MsgDone) Type() uint8 {
	return MessageTypeDone
}
