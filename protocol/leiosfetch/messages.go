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

package leiosfetch

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// NOTE: these are dummy message IDs and will probably need to be changed
const (
	MessageTypeBlockRequest           = 0
	MessageTypeBlock                  = 1
	MessageTypeBlockTxsRequest        = 2
	MessageTypeBlockTxs               = 3
	MessageTypeVotesRequest           = 4
	MessageTypeVotes                  = 5
	MessageTypeBlockRangeRequest      = 6
	MessageTypeLastBlockAndTxsInRange = 7
	MessageTypeNextBlockAndTxsInRange = 8
	MessageTypeDone                   = 9
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeBlockRequest:
		ret = &MsgBlockRequest{}
	case MessageTypeBlock:
		ret = &MsgBlock{}
	case MessageTypeBlockTxsRequest:
		ret = &MsgBlockTxsRequest{}
	case MessageTypeBlockTxs:
		ret = &MsgBlockTxs{}
	case MessageTypeVotesRequest:
		ret = &MsgVotesRequest{}
	case MessageTypeVotes:
		ret = &MsgVotes{}
	case MessageTypeBlockRangeRequest:
		ret = &MsgBlockRangeRequest{}
	case MessageTypeLastBlockAndTxsInRange:
		ret = &MsgLastBlockAndTxsInRange{}
	case MessageTypeNextBlockAndTxsInRange:
		ret = &MsgNextBlockAndTxsInRange{}
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

type MsgBlockRequest struct {
	protocol.MessageBase
	Slot uint64
	Hash []byte
}

func NewMsgBlockRequest(slot uint64, hash []byte) *MsgBlockRequest {
	m := &MsgBlockRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockRequest,
		},
		Slot: slot,
		Hash: bytes.Clone(hash),
	}
	return m
}

type MsgBlock struct {
	protocol.MessageBase
	BlockRaw cbor.RawMessage
}

func NewMsgBlock(block cbor.RawMessage) *MsgBlock {
	m := &MsgBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlock,
		},
		BlockRaw: block,
	}
	return m
}

type MsgBlockTxsRequest struct {
	protocol.MessageBase
	Slot     uint64
	Hash     []byte
	TxBitmap [8]byte
}

func NewMsgBlockTxsRequest(slot uint64, hash []byte, txBitmap [8]byte) *MsgBlockTxsRequest {
	m := &MsgBlockTxsRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsRequest,
		},
		Slot:     slot,
		Hash:     bytes.Clone(hash),
		TxBitmap: txBitmap,
	}
	return m
}

type MsgBlockTxs struct {
	protocol.MessageBase
	TxsRaw []cbor.RawMessage
}

func NewMsgBlockTxs(txs []cbor.RawMessage) *MsgBlockTxs {
	m := &MsgBlockTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxs,
		},
		TxsRaw: slices.Clone(txs),
	}
	return m
}

type MsgVotesRequest struct {
	protocol.MessageBase
	VoteIds []MsgVotesRequestVoteId
}

type MsgVotesRequestVoteId struct {
	cbor.StructAsArray
	Slot         uint64
	VoteIssuerId []byte
}

func NewMsgVotesRequest(voteIds []MsgVotesRequestVoteId) *MsgVotesRequest {
	m := &MsgVotesRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesRequest,
		},
		VoteIds: slices.Clone(voteIds),
	}
	return m
}

type MsgVotes struct {
	protocol.MessageBase
	VotesRaw []cbor.RawMessage
}

func NewMsgVotes(votes []cbor.RawMessage) *MsgVotes {
	m := &MsgVotes{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotes,
		},
		VotesRaw: slices.Clone(votes),
	}
	return m
}

type MsgBlockRangeRequest struct {
	protocol.MessageBase
	Start pcommon.Point
	End   pcommon.Point
}

func NewMsgBlockRangeRequest(start pcommon.Point, end pcommon.Point) *MsgBlockRangeRequest {
	m := &MsgBlockRangeRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockRangeRequest,
		},
		Start: start,
		End:   end,
	}
	return m
}

type MsgNextBlockAndTxsInRange struct {
	protocol.MessageBase
	BlockRaw cbor.RawMessage
	TxsRaw   []cbor.RawMessage
}

func NewMsgNextBlockAndTxsInRange(block cbor.RawMessage, txs []cbor.RawMessage) *MsgNextBlockAndTxsInRange {
	m := &MsgNextBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNextBlockAndTxsInRange,
		},
		BlockRaw: block,
		TxsRaw:   slices.Clone(txs),
	}
	return m
}

type MsgLastBlockAndTxsInRange struct {
	protocol.MessageBase
	BlockRaw cbor.RawMessage
	TxsRaw   []cbor.RawMessage
}

func NewMsgLastBlockAndTxsInRange(block cbor.RawMessage, txs []cbor.RawMessage) *MsgLastBlockAndTxsInRange {
	m := &MsgLastBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeLastBlockAndTxsInRange,
		},
		BlockRaw: block,
		TxsRaw:   slices.Clone(txs),
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
