// Copyright 2026 Blink Labs Software
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

package leiosvotes

import (
	"bytes"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// NOTE: Leios is still experimental and these message IDs may change as
// CIP-0164 stabilizes.
const (
	MessageTypeVotesRequestNext = 0
	MessageTypeVote             = 1
	MessageTypeDone             = 2
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeVotesRequestNext:
		ret = &MsgVotesRequestNext{}
	case MessageTypeVote:
		return newMsgVoteFromCbor(data)
	case MessageTypeDone:
		ret = &MsgDone{}
	default:
		return nil, fmt.Errorf("%s: unknown message type %d", ProtocolName, msgType)
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	ret.SetCbor(data)
	return ret, nil
}

type MsgVotesRequestNext struct {
	protocol.MessageBase
	Count uint64
}

func NewMsgVotesRequestNext(count uint64) *MsgVotesRequestNext {
	m := &MsgVotesRequestNext{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesRequestNext,
		},
		Count: count,
	}
	return m
}

type Vote = lcommon.LeiosVote

type MsgVote struct {
	protocol.MessageBase
	Vote Vote
}

func NewMsgVote(vote Vote) *MsgVote {
	m := &MsgVote{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVote,
		},
		Vote: vote,
	}
	return m
}

// newMsgVoteFromCbor admits a Vote message of at most MaxVoteMessageBytes
// before decoding it, and leaves the message and its vote sharing one copy of
// the encoding rather than each retaining its own.
func newMsgVoteFromCbor(data []byte) (*MsgVote, error) {
	if len(data) > MaxVoteMessageBytes {
		return nil, fmt.Errorf(
			"%s: vote message is %d bytes, maximum %d",
			ProtocolName,
			len(data),
			MaxVoteMessageBytes,
		)
	}
	owned := bytes.Clone(data)
	msg := &MsgVote{}
	if _, err := cbor.Decode(owned, msg); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	msg.SetCborReference(owned)
	// The vote is the last element of [type, vote], ahead of the break byte
	// when the array is indefinite-length.
	end := len(owned)
	if owned[0] == cbor.CborTypeArray|31 {
		end--
	}
	voteRaw := msg.Vote.Cbor()
	if start := end - len(voteRaw); start > 0 &&
		bytes.Equal(owned[start:end], voteRaw) {
		msg.Vote.SetCborReference(owned[start:end])
	}
	return msg, nil
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
