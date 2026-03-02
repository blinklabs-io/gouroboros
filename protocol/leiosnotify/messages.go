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

package leiosnotify

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// NOTE: these are dummy message IDs and will probably need to be changed
const (
	MessageTypeNotificationRequestNext = 0
	MessageTypeBlockAnnouncement       = 1
	MessageTypeBlockOffer              = 2
	MessageTypeBlockTxsOffer           = 3
	MessageTypeVotesOffer              = 4
	MessageTypeDone                    = 5
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeNotificationRequestNext:
		ret = &MsgNotificationRequestNext{}
	case MessageTypeBlockAnnouncement:
		ret = &MsgBlockAnnouncement{}
	case MessageTypeBlockOffer:
		ret = &MsgBlockOffer{}
	case MessageTypeBlockTxsOffer:
		ret = &MsgBlockTxsOffer{}
	case MessageTypeVotesOffer:
		ret = &MsgVotesOffer{}
	case MessageTypeDone:
		ret = &MsgDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgNotificationRequestNext struct {
	protocol.MessageBase
}

func NewMsgNotificationRequestNext() *MsgNotificationRequestNext {
	m := &MsgNotificationRequestNext{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNotificationRequestNext,
		},
	}
	return m
}

type MsgBlockAnnouncement struct {
	protocol.MessageBase
	BlockHeaderRaw cbor.RawMessage
}

func NewMsgBlockAnnouncement(
	blockHeader cbor.RawMessage,
) *MsgBlockAnnouncement {
	m := &MsgBlockAnnouncement{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockAnnouncement,
		},
		BlockHeaderRaw: blockHeader,
	}
	return m
}

type MsgBlockOffer struct {
	protocol.MessageBase
	Point pcommon.Point
	Size  uint64
}

func NewMsgBlockOffer(point pcommon.Point, size uint64) *MsgBlockOffer {
	m := &MsgBlockOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockOffer,
		},
		Point: point,
		Size:  size,
	}
	return m
}

type MsgBlockTxsOffer struct {
	protocol.MessageBase
	Point pcommon.Point
}

func NewMsgBlockTxsOffer(point pcommon.Point) *MsgBlockTxsOffer {
	m := &MsgBlockTxsOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsOffer,
		},
		Point: point,
	}
	return m
}

type MsgVotesOffer struct {
	protocol.MessageBase
	Votes []MsgVotesOfferVote
}

type MsgVotesOfferVote struct {
	cbor.StructAsArray
	Slot         uint64
	VoteIssuerId []byte
}

func NewMsgVotesOffer(votes []MsgVotesOfferVote) *MsgVotesOffer {
	m := &MsgVotesOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesOffer,
		},
		Votes: votes,
	}
	return m
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
	return m
}
