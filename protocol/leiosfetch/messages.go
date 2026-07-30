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

package leiosfetch

import (
	"fmt"
	"maps"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// NOTE: these are dummy message IDs and will probably need to be changed
//
// NOTE: MessageTypeNoBlock and MessageTypeNoBlockTxs (10 and 11) are
// placeholder IDs. They must be confirmed against the Leios protocol spec
// (CIP-0164 or equivalent) before they can be relied on for wire interop with
// the IOG prototype relay.
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
	MessageTypeNoBlock                = 10
	MessageTypeNoBlockTxs             = 11
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
	case MessageTypeNoBlock:
		ret = &MsgNoBlock{}
	case MessageTypeNoBlockTxs:
		ret = &MsgNoBlockTxs{}
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
	Point pcommon.Point
}

func NewMsgBlockRequest(point pcommon.Point) *MsgBlockRequest {
	m := &MsgBlockRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockRequest,
		},
		Point: point,
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
	Point pcommon.Point
	// Bitmaps identifies which transactions to fetch using a map
	// from 16-bit index to a 64-bit bitmap (8 bytes) per CIP-0164.
	// The offset of the first transaction bit is 64*index.
	Bitmaps map[uint16]uint64
}

func NewMsgBlockTxsRequest(
	point pcommon.Point,
	bitmaps map[uint16]uint64,
) *MsgBlockTxsRequest {
	m := &MsgBlockTxsRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsRequest,
		},
		Point:   point,
		Bitmaps: maps.Clone(bitmaps),
	}
	return m
}

// MarshalCBOR encodes the request as [msgType, point, bitmaps], where bitmaps
// is an indefinite-length CBOR map (0xbf ... 0xff). The IOG Leios prototype
// emits and expects the bitmaps map in indefinite-length form; encoding it as
// a definite-length map causes the prototype relay to reject the request and
// reset the connection.
func (m *MsgBlockTxsRequest) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	bitmaps, err := encodeBlockTxBitmaps(m.Bitmaps)
	if err != nil {
		return nil, err
	}
	return cbor.Encode(
		[]any{m.MessageType, m.Point, cbor.RawMessage(bitmaps)},
	)
}

func encodeBlockTxBitmaps(bitmaps map[uint16]uint64) ([]byte, error) {
	keys := make([]uint16, 0, len(bitmaps))
	for k := range bitmaps {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	ret := []byte{0xbf} // indefinite-length map header
	for _, k := range keys {
		kc, err := cbor.Encode(k)
		if err != nil {
			return nil, err
		}
		vc, err := cbor.Encode(bitmaps[k])
		if err != nil {
			return nil, err
		}
		ret = append(ret, kc...)
		ret = append(ret, vc...)
	}
	ret = append(ret, 0xff) // break
	return ret, nil
}

// MsgBlockTxs carries the transactions of an endorser block. Two wire shapes
// are accepted for prototype interop:
//
//   - dingo: [tx_list] — just the transactions (2-element message).
//   - IOG Leios prototype: [point, bitmaps, tx_list] — echoes the request's
//     point and bitmaps alongside the transactions (4-element message).
//
// Point/Bitmaps are populated only when decoding the prototype form; encoding
// emits the prototype form when Bitmaps is non-nil, otherwise the dingo form.
type MsgBlockTxs struct {
	protocol.MessageBase
	Point   pcommon.Point
	Bitmaps map[uint16]uint64
	TxsRaw  []cbor.RawMessage
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

// NewMsgBlockTxsFull builds the prototype's 4-element block-txs response,
// echoing the requested point and bitmaps alongside the transactions.
func NewMsgBlockTxsFull(
	point pcommon.Point,
	bitmaps map[uint16]uint64,
	txs []cbor.RawMessage,
) *MsgBlockTxs {
	m := &MsgBlockTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxs,
		},
		Point:   point,
		Bitmaps: maps.Clone(bitmaps),
		TxsRaw:  slices.Clone(txs),
	}
	return m
}

func (m *MsgBlockTxs) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if m.Bitmaps != nil {
		bitmaps, err := encodeBlockTxBitmaps(m.Bitmaps)
		if err != nil {
			return nil, err
		}
		return cbor.Encode(
			[]any{
				m.MessageType,
				m.Point,
				cbor.RawMessage(bitmaps),
				m.TxsRaw,
			},
		)
	}
	return cbor.Encode([]any{m.MessageType, m.TxsRaw})
}

func (m *MsgBlockTxs) UnmarshalCBOR(data []byte) error {
	var elems []cbor.RawMessage
	if _, err := cbor.Decode(data, &elems); err != nil {
		return err
	}
	switch len(elems) {
	case 2: // [msgType, tx_list] — dingo form
		var env struct {
			cbor.StructAsArray
			MessageType uint8
			TxsRaw      []cbor.RawMessage
		}
		if _, err := cbor.Decode(data, &env); err != nil {
			return err
		}
		m.MessageType = env.MessageType
		m.TxsRaw = env.TxsRaw
	case 4: // [msgType, point, bitmaps, tx_list] — prototype form
		var env struct {
			cbor.StructAsArray
			MessageType uint8
			Point       pcommon.Point
			Bitmaps     map[uint16]uint64
			TxsRaw      []cbor.RawMessage
		}
		if _, err := cbor.Decode(data, &env); err != nil {
			return err
		}
		m.MessageType = env.MessageType
		m.Point = env.Point
		m.Bitmaps = env.Bitmaps
		m.TxsRaw = env.TxsRaw
	default:
		return fmt.Errorf(
			"%s: block txs: unexpected element count %d",
			ProtocolName,
			len(elems),
		)
	}
	m.SetCbor(data)
	return nil
}

type MsgVotesRequest struct {
	protocol.MessageBase
	VoteIds []MsgVotesRequestVoteId
}

type MsgVotesRequestVoteId = lcommon.LeiosVoteId

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

func NewMsgVotesFromVotes(votes []lcommon.LeiosVote) (*MsgVotes, error) {
	votesRaw := make([]cbor.RawMessage, 0, len(votes))
	for _, vote := range votes {
		if err := vote.Validate(); err != nil {
			return nil, err
		}
		voteCbor, err := cbor.Encode(vote)
		if err != nil {
			return nil, err
		}
		votesRaw = append(votesRaw, cbor.RawMessage(voteCbor))
	}
	return NewMsgVotes(votesRaw), nil
}

func (m *MsgVotes) DecodeVotes() ([]lcommon.LeiosVote, error) {
	ret := make([]lcommon.LeiosVote, 0, len(m.VotesRaw))
	for idx, voteRaw := range m.VotesRaw {
		var vote lcommon.LeiosVote
		if _, err := cbor.Decode(voteRaw, &vote); err != nil {
			return nil, fmt.Errorf("decode vote %d: %w", idx, err)
		}
		ret = append(ret, vote)
	}
	return ret, nil
}

type MsgBlockRangeRequest struct {
	protocol.MessageBase
	Start pcommon.Point
	End   pcommon.Point
}

func NewMsgBlockRangeRequest(
	start pcommon.Point,
	end pcommon.Point,
) *MsgBlockRangeRequest {
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

func NewMsgNextBlockAndTxsInRange(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgNextBlockAndTxsInRange {
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

func NewMsgLastBlockAndTxsInRange(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgLastBlockAndTxsInRange {
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

// MsgNoBlock is the server's response to a BlockRequest for an endorser block
// that is not available. It lets the server decline gracefully instead of
// returning an error that would tear down the connection.
type MsgNoBlock struct {
	protocol.MessageBase
}

func NewMsgNoBlock() *MsgNoBlock {
	m := &MsgNoBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNoBlock,
		},
	}
	return m
}

// MsgNoBlockTxs is the server's response to a BlockTxsRequest whose endorser
// block transactions are not available. It lets the server decline gracefully
// instead of returning an error that would tear down the connection.
type MsgNoBlockTxs struct {
	protocol.MessageBase
}

func NewMsgNoBlockTxs() *MsgNoBlockTxs {
	m := &MsgNoBlockTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNoBlockTxs,
		},
	}
	return m
}
