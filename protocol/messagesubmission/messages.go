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

// Message submission V1 is tx-submission with DMQ messages in place of
// transactions, so its wire tags are tx-submission's (ouroboros-network
// TxSubmission2/Codec.hs, encodeTxSubmission2), not the ones CIP-0137 lists.
const (
	MessageTypeV1RequestMessageIds = 0
	MessageTypeV1ReplyMessageIds   = 1
	MessageTypeV1RequestMessages   = 2
	MessageTypeV1ReplyMessages     = 3
	MessageTypeV1Done              = 4
	MessageTypeV1Init              = 6
)

// Message submission V2 wire tags, as dmq-node's SigSubmissionV2 codec
// (DMQ/Protocol/SigSubmissionV2/Codec.hs) writes them. V2 has no MsgInit and
// adds MsgReplyNoMessageIds, which CIP-0137 does not list.
const (
	MessageTypeV2RequestMessageIds = 1
	MessageTypeV2ReplyMessageIds   = 2
	MessageTypeV2ReplyNoMessageIds = 3
	MessageTypeV2RequestMessages   = 4
	MessageTypeV2ReplyMessages     = 5
	MessageTypeV2Done              = 6
)

// msgKind names a message independently of the version's wire tag.
type msgKind int

const (
	kindInit msgKind = iota
	kindRequestMessageIds
	kindReplyMessageIds
	kindReplyNoMessageIds
	kindRequestMessages
	kindReplyMessages
	kindDone
)

var (
	wireTagsV1 = map[msgKind]uint8{
		kindInit:              MessageTypeV1Init,
		kindRequestMessageIds: MessageTypeV1RequestMessageIds,
		kindReplyMessageIds:   MessageTypeV1ReplyMessageIds,
		kindRequestMessages:   MessageTypeV1RequestMessages,
		kindReplyMessages:     MessageTypeV1ReplyMessages,
		kindDone:              MessageTypeV1Done,
	}
	wireTagsV2 = map[msgKind]uint8{
		kindRequestMessageIds: MessageTypeV2RequestMessageIds,
		kindReplyMessageIds:   MessageTypeV2ReplyMessageIds,
		kindReplyNoMessageIds: MessageTypeV2ReplyNoMessageIds,
		kindRequestMessages:   MessageTypeV2RequestMessages,
		kindReplyMessages:     MessageTypeV2ReplyMessages,
		kindDone:              MessageTypeV2Done,
	}
	kindsV1 = invertWireTags(wireTagsV1)
	kindsV2 = invertWireTags(wireTagsV2)
)

func invertWireTags(tags map[msgKind]uint8) map[uint]msgKind {
	ret := make(map[uint]msgKind, len(tags))
	for kind, tag := range tags {
		ret[uint(tag)] = kind
	}
	return ret
}

func wireTagsFor(version uint16) map[msgKind]uint8 {
	if isMessageSubmissionV2(version) {
		return wireTagsV2
	}
	return wireTagsV1
}

// wireTag returns the tag of kind in the given protocol version. A kind the
// version lacks gets a tag no peer accepts, so the state map refuses to send
// it.
func wireTag(version uint16, kind msgKind) uint8 {
	if tag, ok := wireTagsFor(version)[kind]; ok {
		return tag
	}
	return 0xff
}

func newMessageBase(version uint16, kind msgKind) protocol.MessageBase {
	return protocol.MessageBase{MessageType: wireTag(version, kind)}
}

// encodeIndefinite encodes [tag, [_ items...]]. The reference decoders read
// each message collection with decodeListLenIndef, which refuses a
// definite-length list.
func encodeIndefinite[T any](tag uint8, items []T) ([]byte, error) {
	list := make(cbor.IndefLengthList, 0, len(items))
	for _, item := range items {
		list = append(list, item)
	}
	return cbor.Encode([]any{tag, list})
}

// MsgInit opens a V1 session. It is sent by the side that has messages.
type MsgInit struct {
	protocol.MessageBase
}

// NewMsgInit creates a new MsgInit. MsgInit exists only in V1.
func NewMsgInit() *MsgInit {
	return &MsgInit{
		MessageBase: newMessageBase(protocol.ProtocolVersionDMQNtN1, kindInit),
	}
}

// MsgRequestMessageIds represents a request for message IDs (blocking or non-blocking)
type MsgRequestMessageIds struct {
	protocol.MessageBase
	IsBlocking   bool
	AckCount     uint16
	RequestCount uint16
}

// NewMsgRequestMessageIds creates a new MsgRequestMessageIds with the wire
// tag of the negotiated protocol version.
func NewMsgRequestMessageIds(
	version uint16,
	isBlocking bool,
	ackCount, requestCount uint16,
) *MsgRequestMessageIds {
	return &MsgRequestMessageIds{
		MessageBase:  newMessageBase(version, kindRequestMessageIds),
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

// NewMsgReplyMessageIds creates a new MsgReplyMessageIds with the wire tag of
// the negotiated protocol version.
func NewMsgReplyMessageIds(
	version uint16,
	messages []pcommon.MessageIDAndSize,
) *MsgReplyMessageIds {
	return &MsgReplyMessageIds{
		MessageBase: newMessageBase(version, kindReplyMessageIds),
		Messages:    messages,
	}
}

// MarshalCBOR encodes MsgReplyMessageIds with an indefinite-length list.
func (m MsgReplyMessageIds) MarshalCBOR() ([]byte, error) {
	return encodeIndefinite(m.MessageType, m.Messages)
}

// MsgReplyNoMessageIds answers a V2 blocking request for message IDs with
// none, handing agency back to the requesting side. The acknowledgement in
// the request still applies.
type MsgReplyNoMessageIds struct {
	protocol.MessageBase
}

// NewMsgReplyNoMessageIds creates a new MsgReplyNoMessageIds. It exists only
// in V2.
func NewMsgReplyNoMessageIds() *MsgReplyNoMessageIds {
	return &MsgReplyNoMessageIds{
		MessageBase: newMessageBase(
			protocol.ProtocolVersionDMQNtN2,
			kindReplyNoMessageIds,
		),
	}
}

// MsgRequestMessages represents a request for specific messages
type MsgRequestMessages struct {
	protocol.MessageBase
	MessageIDs [][]byte
}

// NewMsgRequestMessages creates a new MsgRequestMessages with the wire tag of
// the negotiated protocol version.
func NewMsgRequestMessages(
	version uint16,
	messageIDs [][]byte,
) *MsgRequestMessages {
	return &MsgRequestMessages{
		MessageBase: newMessageBase(version, kindRequestMessages),
		MessageIDs:  messageIDs,
	}
}

// MarshalCBOR encodes MsgRequestMessages with an indefinite-length list.
func (m MsgRequestMessages) MarshalCBOR() ([]byte, error) {
	return encodeIndefinite(m.MessageType, m.MessageIDs)
}

// MsgReplyMessages represents the reply with full messages
type MsgReplyMessages struct {
	protocol.MessageBase
	Messages              []pcommon.DmqMessage
	legacyMessageEncoding bool
}

// NewMsgReplyMessages creates a new MsgReplyMessages with the wire tag of the
// negotiated protocol version.
func NewMsgReplyMessages(
	version uint16,
	messages []pcommon.DmqMessage,
) *MsgReplyMessages {
	return &MsgReplyMessages{
		MessageBase: newMessageBase(version, kindReplyMessages),
		Messages:    messages,
	}
}

// MarshalCBOR encodes MsgReplyMessages with an indefinite-length list. V1
// peers using the pre-CIP-0137-v4 message shape can be supported by setting
// legacyMessageEncoding.
func (m MsgReplyMessages) MarshalCBOR() ([]byte, error) {
	if !m.legacyMessageEncoding {
		return encodeIndefinite(m.MessageType, m.Messages)
	}

	rawMessages := make([]cbor.RawMessage, 0, len(m.Messages))
	for _, msg := range m.Messages {
		msgCbor, err := pcommon.MarshalDmqMessageLegacyCBOR(msg)
		if err != nil {
			return nil, err
		}
		rawMessages = append(rawMessages, cbor.RawMessage(msgCbor))
	}
	return encodeIndefinite(m.MessageType, rawMessages)
}

// MsgDone represents the protocol completion message
type MsgDone struct {
	protocol.MessageBase
}

// NewMsgDone creates a new MsgDone with the wire tag of the negotiated
// protocol version.
func NewMsgDone(version uint16) *MsgDone {
	return &MsgDone{
		MessageBase: newMessageBase(version, kindDone),
	}
}

// NewMsgFromCbor parses a V1 Message Submission message from CBOR.
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	return decodeMsgFromCborWithLimit(
		protocol.ProtocolVersionDMQNtN1,
		msgType,
		data,
		maxWireMessageIDs,
	)
}

// NewMsgFromCborV2 parses a V2 Message Submission message from CBOR.
func NewMsgFromCborV2(msgType uint, data []byte) (protocol.Message, error) {
	return decodeMsgFromCborWithLimit(
		protocol.ProtocolVersionDMQNtN2,
		msgType,
		data,
		maxWireMessageIDs,
	)
}

func kindOf(version uint16, msgType uint) (msgKind, bool) {
	kinds := kindsV1
	if isMessageSubmissionV2(version) {
		kinds = kindsV2
	}
	kind, ok := kinds[msgType]
	return kind, ok
}

func decodeMsgFromCborWithLimit(
	version uint16,
	msgType uint,
	data []byte,
	maxCount int,
) (protocol.Message, error) {
	kind, ok := kindOf(version, msgType)
	if !ok {
		return nil, fmt.Errorf(
			"%s: unknown message type: %d",
			ProtocolName,
			msgType,
		)
	}
	var ret protocol.Message
	switch kind {
	case kindInit:
		ret = &MsgInit{}
	case kindRequestMessageIds:
		ret = &MsgRequestMessageIds{}
	case kindReplyMessageIds:
		ret = &MsgReplyMessageIds{}
	case kindReplyNoMessageIds:
		ret = &MsgReplyNoMessageIds{}
	case kindRequestMessages:
		ret = &MsgRequestMessages{}
	case kindReplyMessages:
		ret = &MsgReplyMessages{}
	case kindDone:
		ret = &MsgDone{}
	}
	if kind == kindReplyMessageIds ||
		kind == kindRequestMessages ||
		kind == kindReplyMessages {
		if err := validateMessageSubmissionCollection(kind, data, maxCount); err != nil {
			return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
		}
	}
	if _, err := cbor.DecodeUntrustedExact(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	// Store the raw message CBOR (ret is always non-nil for handled types)
	ret.SetCbor(data)
	return ret, nil
}
