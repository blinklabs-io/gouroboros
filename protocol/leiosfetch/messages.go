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
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// Message IDs 0 to 4 follow the LeiosFetch messages CDDL in Cardano Blueprint
// as implemented by the node of Leios release candidate 2026w42. That node
// speaks no other message.
const (
	MessageTypeDone            = 0
	MessageTypeBlockRequest    = 1
	MessageTypeBlock           = 2
	MessageTypeBlockTxsRequest = 3
	MessageTypeBlockTxs        = 4
)

// Vote and block-range messages are not part of the reference protocol. They
// are numbered clear of it so a reference-node message can never decode as
// one, and a peer that does not implement them rejects them as unknown.
const (
	MessageTypeVotesRequest           = 10
	MessageTypeVotes                  = 11
	MessageTypeBlockRangeRequest      = 12
	MessageTypeNextBlockAndTxsInRange = 13
	MessageTypeLastBlockAndTxsInRange = 14
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
	default:
		return nil, fmt.Errorf(
			"%s: unknown message type: %d",
			ProtocolName,
			msgType,
		)
	}
	var decodeErr error
	switch msg := ret.(type) {
	case *MsgBlockTxsRequest:
		if decodeErr = validateBlockTxsRequest(data); decodeErr == nil {
			_, decodeErr = cbor.Decode(data, msg)
		}
	case *MsgVotesRequest:
		decodeErr = msg.UnmarshalCBOR(data)
	case *MsgBlockTxs:
		decodeErr = msg.UnmarshalCBOR(data)
	case *MsgVotes:
		decodeErr = msg.UnmarshalCBOR(data)
	case *MsgNextBlockAndTxsInRange:
		decodeErr = msg.UnmarshalCBOR(data)
	case *MsgLastBlockAndTxsInRange:
		decodeErr = msg.UnmarshalCBOR(data)
	default:
		_, decodeErr = cbor.Decode(data, ret)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, decodeErr)
	}
	if uint(ret.Type()) != msgType {
		return nil, fmt.Errorf(
			"%s: message type mismatch: parser received %d, payload contains %d",
			ProtocolName,
			msgType,
			ret.Type(),
		)
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

// MsgBlockTxs carries the requested transactions of an endorser block and
// nothing else: it does not echo the request. Each entry of TxsRaw is the
// block_transaction CBOR of one transaction, in the order of the set bits of
// the request's bitmaps. On the wire each one is wrapped as
// #6.24(bytes .cbor block_transaction).
type MsgBlockTxs struct {
	protocol.MessageBase
	TxsRaw []cbor.RawMessage
}

// NewMsgBlockTxs builds the response to a block transactions request from the
// block_transaction CBOR of each transaction. The tag 24 wrapping is added when
// the message is encoded.
func NewMsgBlockTxs(txs []cbor.RawMessage) *MsgBlockTxs {
	m := &MsgBlockTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxs,
		},
		TxsRaw: slices.Clone(txs),
	}
	return m
}

func (m *MsgBlockTxs) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	wrapped := make([]cbor.RawMessage, len(m.TxsRaw))
	for idx, tx := range m.TxsRaw {
		if len(tx) == 0 {
			return nil, fmt.Errorf(
				"%s: block txs: transaction %d is empty",
				ProtocolName,
				idx,
			)
		}
		inner, err := cbor.Encode([]byte(tx))
		if err != nil {
			return nil, err
		}
		wrapped[idx] = append(
			cbor.RawMessage{0xd8, cbor.CborTagCbor},
			inner...,
		)
	}
	return cbor.Encode([]any{m.MessageType, wrapped})
}

func (m *MsgBlockTxs) UnmarshalCBOR(data []byte) error {
	return m.unmarshalCBORWithLimit(data, maxWireBlockTxs)
}

func (m *MsgBlockTxs) unmarshalCBORWithLimit(
	data []byte,
	maxTxCount int,
) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		2,
		"block transactions envelope",
	)
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"%s: block txs: unexpected element count %d",
			ProtocolName,
			len(fields),
		)
	}
	var messageType uint8
	if err := cborpreflight.ValidateItemDepth(fields[0], 0, "block transactions message type"); err != nil {
		return err
	}
	if _, err := cbor.Decode(fields[0], &messageType); err != nil {
		return err
	}
	txs, err := cborpreflight.ArrayItems(fields[1], maxTxCount, "block transactions")
	if err != nil {
		return err
	}
	txsRaw := make([]cbor.RawMessage, len(txs))
	for idx, tx := range txs {
		inner, err := unwrapBlockTx(tx)
		if err != nil {
			return fmt.Errorf(
				"%s: block txs: transaction %d: %w",
				ProtocolName,
				idx,
				err,
			)
		}
		txsRaw[idx] = append(cbor.RawMessage(nil), inner...)
	}
	m.MessageType = messageType
	m.TxsRaw = txsRaw
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

func (m *MsgVotes) UnmarshalCBOR(data []byte) error {
	return m.unmarshalCBORWithLimits(
		data,
		MaxVotesPerMessage,
		MaxVotesMessageBytes,
	)
}

func NewMsgVotes(votes []cbor.RawMessage) *MsgVotes {
	if votes == nil {
		votes = []cbor.RawMessage{}
	}
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

func (m *MsgNextBlockAndTxsInRange) UnmarshalCBOR(data []byte) error {
	msg, err := decodeBlockAndTxsInRange(
		MessageTypeNextBlockAndTxsInRange,
		data,
		maxWireRangeTxs,
		DefaultMaxBlockRangeResponseBytes,
	)
	if err != nil {
		return err
	}
	decoded := msg.(*MsgNextBlockAndTxsInRange)
	*m = *decoded
	return nil
}

func NewMsgNextBlockAndTxsInRange(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgNextBlockAndTxsInRange {
	return newMsgNextBlockAndTxsInRangeOwned(block, slices.Clone(txs))
}

func newMsgNextBlockAndTxsInRangeOwned(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgNextBlockAndTxsInRange {
	return &MsgNextBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNextBlockAndTxsInRange,
		},
		BlockRaw: block,
		TxsRaw:   txs,
	}
}

type MsgLastBlockAndTxsInRange struct {
	protocol.MessageBase
	BlockRaw cbor.RawMessage
	TxsRaw   []cbor.RawMessage
}

func (m *MsgLastBlockAndTxsInRange) UnmarshalCBOR(data []byte) error {
	msg, err := decodeBlockAndTxsInRange(
		MessageTypeLastBlockAndTxsInRange,
		data,
		maxWireRangeTxs,
		DefaultMaxBlockRangeResponseBytes,
	)
	if err != nil {
		return err
	}
	decoded := msg.(*MsgLastBlockAndTxsInRange)
	*m = *decoded
	return nil
}

func NewMsgLastBlockAndTxsInRange(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgLastBlockAndTxsInRange {
	return newMsgLastBlockAndTxsInRangeOwned(block, slices.Clone(txs))
}

func newMsgLastBlockAndTxsInRangeOwned(
	block cbor.RawMessage,
	txs []cbor.RawMessage,
) *MsgLastBlockAndTxsInRange {
	return &MsgLastBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeLastBlockAndTxsInRange,
		},
		BlockRaw: block,
		TxsRaw:   txs,
	}
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
