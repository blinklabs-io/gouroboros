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
	Slot uint64
	Hash []byte
}

func NewMsgBlockOffer(slot uint64, hash []byte) *MsgBlockOffer {
	m := &MsgBlockOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockOffer,
		},
		Slot: slot,
		Hash: hash,
	}
	return m
}

type MsgBlockTxsOffer struct {
	protocol.MessageBase
	Slot uint64
	Hash []byte
}

func NewMsgBlockTxsOffer(slot uint64, hash []byte) *MsgBlockTxsOffer {
	m := &MsgBlockTxsOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsOffer,
		},
		Slot: slot,
		Hash: hash,
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
