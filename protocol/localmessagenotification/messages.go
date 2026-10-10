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
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
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
	return newMsgFromCborWithLimit(msgType, data, 0)
}

func newMsgFromCborWithLimit(
	msgType uint,
	data []byte,
	maxReplyMessages int,
) (protocol.Message, error) {
	if maxReplyMessages > 0 &&
		(msgType == MessageTypeReplyMessagesNonBlocking ||
			msgType == MessageTypeReplyMessagesBlocking) {
		if err := validateReplyMessageCount(data, maxReplyMessages); err != nil {
			return nil, err
		}
	}
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

func validateReplyMessageCount(data []byte, maxCount int) error {
	if maxCount <= 0 {
		return fmt.Errorf("%s: invalid reply message limit %d", ProtocolName, maxCount)
	}
	maxCountUint := uint64(maxCount) // #nosec G115 -- maxCount is positive
	head, ok, err := cborwalk.ReadHead(data, 0)
	if err != nil {
		return fmt.Errorf("%s: decode reply envelope: %w", ProtocolName, err)
	}
	if !ok {
		return fmt.Errorf("%s: truncated reply envelope", ProtocolName)
	}
	if head.Major != 4 {
		return fmt.Errorf("%s: reply is not an array", ProtocolName)
	}
	pos := head.EncodedSize
	messageTypeLength, err := cborwalk.ItemLength(data[pos:])
	if err != nil {
		return fmt.Errorf("%s: decode reply message type: %w", ProtocolName, err)
	}
	pos += messageTypeLength
	messagesHead, ok, err := cborwalk.ReadHead(data, pos)
	if err != nil {
		return fmt.Errorf("%s: decode reply messages: %w", ProtocolName, err)
	}
	if !ok {
		return fmt.Errorf("%s: truncated reply messages", ProtocolName)
	}
	if messagesHead.Major != 4 {
		return fmt.Errorf("%s: reply messages is not an array", ProtocolName)
	}
	if !messagesHead.Indefinite {
		if messagesHead.Argument > maxCountUint {
			return fmt.Errorf(
				"%s: reply has %d messages, maximum is %d",
				ProtocolName,
				messagesHead.Argument,
				maxCount,
			)
		}
		return validateReplyShape(data)
	}
	pos += messagesHead.EncodedSize
	for count := 0; ; count++ {
		if pos >= len(data) {
			return fmt.Errorf("%s: unterminated reply messages", ProtocolName)
		}
		if data[pos] == 0xff {
			return validateReplyShape(data)
		}
		if count >= maxCount {
			return fmt.Errorf(
				"%s: reply has more than %d messages",
				ProtocolName,
				maxCount,
			)
		}
		itemLength, err := cborwalk.ItemLength(data[pos:])
		if err != nil {
			return fmt.Errorf(
				"%s: decode reply message %d: %w",
				ProtocolName,
				count,
				err,
			)
		}
		pos += itemLength
	}
}

func validateReplyShape(data []byte) error {
	const maxReplyNesting = 4
	itemLength, err := cborwalk.ItemLengthWithin(data, maxReplyNesting)
	if err != nil {
		return fmt.Errorf("%s: invalid reply shape: %w", ProtocolName, err)
	}
	if itemLength != len(data) {
		return fmt.Errorf("%s: trailing data after reply", ProtocolName)
	}
	return nil
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
