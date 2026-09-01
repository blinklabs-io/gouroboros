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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Message type constants following CIP-0137 CDDL specification
const (
	MessageTypeRequestMessages          = 0
	MessageTypeReplyMessagesNonBlocking = 1
	MessageTypeReplyMessagesBlocking    = 2
	MessageTypeClientDone               = 3
)

// MsgRequestMessages represents a request for messages (blocking or non-blocking)
type MsgRequestMessages struct {
	protocol.MessageBase
	IsBlocking bool
}

// NewMsgRequestMessages creates a new MsgRequestMessages
func NewMsgRequestMessages(isBlocking bool) *MsgRequestMessages {
	return &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		IsBlocking: isBlocking,
	}
}

// MsgReplyMessagesNonBlocking represents a reply with available messages (non-blocking)
type MsgReplyMessagesNonBlocking struct {
	protocol.MessageBase
	Messages []pcommon.DmqMessage
	HasMore  bool
}

// NewMsgReplyMessagesNonBlocking creates a new MsgReplyMessagesNonBlocking
func NewMsgReplyMessagesNonBlocking(
	messages []pcommon.DmqMessage,
	hasMore bool,
) *MsgReplyMessagesNonBlocking {
	return &MsgReplyMessagesNonBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesNonBlocking,
		},
		Messages: messages,
		HasMore:  hasMore,
	}
}

// MsgReplyMessagesBlocking represents a reply with available messages (blocking)
type MsgReplyMessagesBlocking struct {
	protocol.MessageBase
	Messages []pcommon.DmqMessage
}

// NewMsgReplyMessagesBlocking creates a new MsgReplyMessagesBlocking
func NewMsgReplyMessagesBlocking(
	messages []pcommon.DmqMessage,
) *MsgReplyMessagesBlocking {
	return &MsgReplyMessagesBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesBlocking,
		},
		Messages: messages,
	}
}

// MsgClientDone represents the protocol completion message from client
type MsgClientDone struct {
	protocol.MessageBase
}

// NewMsgClientDone creates a new MsgClientDone message
func NewMsgClientDone() *MsgClientDone {
	return &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeClientDone,
		},
	}
}

// NewMsgFromCbor parses a Local Message Notification message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeRequestMessages:
		ret = &MsgRequestMessages{}
	case MessageTypeReplyMessagesNonBlocking:
		ret = &MsgReplyMessagesNonBlocking{}
	case MessageTypeReplyMessagesBlocking:
		ret = &MsgReplyMessagesBlocking{}
	case MessageTypeClientDone:
		ret = &MsgClientDone{}
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
func (m *MsgRequestMessages) Type() uint8 {
	return MessageTypeRequestMessages
}

// Type returns the message type
func (m *MsgReplyMessagesNonBlocking) Type() uint8 {
	return MessageTypeReplyMessagesNonBlocking
}

// Type returns the message type
func (m *MsgReplyMessagesBlocking) Type() uint8 {
	return MessageTypeReplyMessagesBlocking
}

// Type returns the message type
func (m *MsgClientDone) Type() uint8 {
	return MessageTypeClientDone
}
