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
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDefinition struct {
	Name        string
	Message     protocol.Message
	MessageType uint
}

func testLeiosSignature(fill byte) []byte {
	ret := make([]byte, lcommon.LeiosBlsSignatureSize)
	for i := range ret {
		ret[i] = fill
	}
	return ret
}

func getTestDefinitions() []testDefinition {
	return []testDefinition{
		{
			Name: "MsgBlockRequest",
			Message: NewMsgBlockRequest(
				pcommon.NewPoint(
					12345,
					[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				),
			),
			MessageType: MessageTypeBlockRequest,
		},
		{
			Name: "MsgBlock",
			Message: NewMsgBlock(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
			),
			MessageType: MessageTypeBlock,
		},
		{
			Name: "MsgBlockTxsRequest",
			Message: NewMsgBlockTxsRequest(
				pcommon.NewPoint(
					12345,
					[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				),
				map[uint16]uint64{
					0:  0xff00000000000000,
					64: 0x00ff000000000000,
				},
			),
			MessageType: MessageTypeBlockTxsRequest,
		},
		{
			Name: "MsgBlockTxs",
			Message: NewMsgBlockTxs(
				[]cbor.RawMessage{
					[]byte{0x82, 0x01, 0x02},
					[]byte{0x82, 0x03, 0x04},
				},
			),
			MessageType: MessageTypeBlockTxs,
		},
		{
			Name: "MsgVotesRequest",
			Message: NewMsgVotesRequest(
				[]MsgVotesRequestVoteId{
					{SlotNo: 100, VoterId: 1},
					{SlotNo: 200, VoterId: 2},
				},
			),
			MessageType: MessageTypeVotesRequest,
		},
		{
			Name: "MsgVotes",
			Message: NewMsgVotes(
				[]cbor.RawMessage{
					[]byte{0x82, 0x05, 0x06},
					[]byte{0x82, 0x07, 0x08},
				},
			),
			MessageType: MessageTypeVotes,
		},
		{
			Name: "MsgBlockRangeRequest",
			Message: NewMsgBlockRangeRequest(
				pcommon.NewPoint(
					123,
					[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				),
				pcommon.NewPoint(
					456,
					[]byte{0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				),
			),
			MessageType: MessageTypeBlockRangeRequest,
		},
		{
			Name: "MsgNextBlockAndTxsInRange",
			Message: NewMsgNextBlockAndTxsInRange(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
				[]cbor.RawMessage{
					[]byte{0x82, 0x03, 0x04},
				},
			),
			MessageType: MessageTypeNextBlockAndTxsInRange,
		},
		{
			Name: "MsgLastBlockAndTxsInRange",
			Message: NewMsgLastBlockAndTxsInRange(
				cbor.RawMessage([]byte{0x82, 0x09, 0x0a}),
				[]cbor.RawMessage{
					[]byte{0x82, 0x0b, 0x0c},
					[]byte{0x82, 0x0d, 0x0e},
				},
			),
			MessageType: MessageTypeLastBlockAndTxsInRange,
		},
		{
			Name:        "MsgDone",
			Message:     NewMsgDone(),
			MessageType: MessageTypeDone,
		},
		{
			Name:        "MsgNoBlock",
			Message:     NewMsgNoBlock(),
			MessageType: MessageTypeNoBlock,
		},
		{
			Name:        "MsgNoBlockTxs",
			Message:     NewMsgNoBlockTxs(),
			MessageType: MessageTypeNoBlockTxs,
		},
	}
}

func TestCborRoundTrip(t *testing.T) {
	tests := getTestDefinitions()
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			// Encode the message
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err, "failed to encode message to CBOR")

			// Decode the message
			decoded, err := NewMsgFromCbor(test.MessageType, encoded)
			require.NoError(t, err, "failed to decode CBOR")

			// Re-encode and compare
			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err, "failed to re-encode message")

			assert.Equal(t, encoded, reencoded, "CBOR round-trip failed")
		})
	}
}

func TestDecode(t *testing.T) {
	tests := getTestDefinitions()
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			// Encode the message first
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err, "failed to encode message to CBOR")

			// Decode it back
			decoded, err := NewMsgFromCbor(test.MessageType, encoded)
			require.NoError(t, err, "failed to decode CBOR")

			// Set the raw CBOR so the comparison should succeed
			test.Message.SetCbor(encoded)

			assert.True(t, reflect.DeepEqual(decoded, test.Message),
				"CBOR did not decode to expected message object\n  got: %#v\n  wanted: %#v",
				decoded, test.Message)
		})
	}
}

func TestEncode(t *testing.T) {
	tests := getTestDefinitions()
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			// Encode the message
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err, "failed to encode message to CBOR")

			// Verify it can be decoded back without error
			_, err = NewMsgFromCbor(test.MessageType, encoded)
			require.NoError(t, err, "failed to decode encoded message")
		})
	}
}

func TestMsgBlockRequest(t *testing.T) {
	slot := uint64(123456)
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	msg := NewMsgBlockRequest(pcommon.NewPoint(slot, hash))

	assert.Equal(t, uint8(MessageTypeBlockRequest), msg.Type())
	assert.Equal(t, slot, msg.Point.Slot)
	assert.Equal(t, hash, msg.Point.Hash)
}

func TestMsgBlock(t *testing.T) {
	blockRaw := cbor.RawMessage([]byte{0x82, 0x01, 0x02})

	msg := NewMsgBlock(blockRaw)

	assert.Equal(t, uint8(MessageTypeBlock), msg.Type())
	assert.Equal(t, blockRaw, msg.BlockRaw)
}

func TestMsgBlockTxsRequest(t *testing.T) {
	slot := uint64(123456)
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	bitmaps := map[uint16]uint64{
		0: 0xff00000000000000,
	}

	msg := NewMsgBlockTxsRequest(pcommon.NewPoint(slot, hash), bitmaps)

	assert.Equal(t, uint8(MessageTypeBlockTxsRequest), msg.Type())
	assert.Equal(t, slot, msg.Point.Slot)
	assert.Equal(t, hash, msg.Point.Hash)
	assert.Equal(t, bitmaps, msg.Bitmaps)
}

func TestMsgBlockTxs(t *testing.T) {
	txs := []cbor.RawMessage{
		[]byte{0x82, 0x01, 0x02},
		[]byte{0x82, 0x03, 0x04},
	}

	msg := NewMsgBlockTxs(txs)

	assert.Equal(t, uint8(MessageTypeBlockTxs), msg.Type())
	assert.Equal(t, txs, msg.TxsRaw)
}

func TestMsgBlockTxsFullUsesRequestBitmapEncoding(t *testing.T) {
	point := pcommon.NewPoint(123, []byte{0x01, 0x02})
	bitmaps := map[uint16]uint64{
		0:  1,
		64: 0x00ff000000000000,
	}

	requestEncoded, err := cbor.Encode(
		NewMsgBlockTxsRequest(point, bitmaps),
	)
	require.NoError(t, err)

	responseEncoded, err := cbor.Encode(
		NewMsgBlockTxsFull(
			point,
			bitmaps,
			[]cbor.RawMessage{[]byte{0x82, 0x01, 0x02}},
		),
	)
	require.NoError(t, err)

	var requestElems []cbor.RawMessage
	_, err = cbor.Decode(requestEncoded, &requestElems)
	require.NoError(t, err)

	var responseElems []cbor.RawMessage
	_, err = cbor.Decode(responseEncoded, &responseElems)
	require.NoError(t, err)

	require.Len(t, requestElems, 3)
	require.Len(t, responseElems, 4)
	require.NotEmpty(t, responseElems[2])
	assert.Equal(t, requestElems[2], responseElems[2])
	assert.Equal(t, byte(0xbf), responseElems[2][0])
	assert.Equal(t, byte(0xff), responseElems[2][len(responseElems[2])-1])

	decoded, err := NewMsgFromCbor(MessageTypeBlockTxs, responseEncoded)
	require.NoError(t, err)
	assert.Equal(t, bitmaps, decoded.(*MsgBlockTxs).Bitmaps)
}

func TestMsgVotesRequest(t *testing.T) {
	voteIds := []MsgVotesRequestVoteId{
		{SlotNo: 100, VoterId: 1},
		{SlotNo: 200, VoterId: 2},
	}

	msg := NewMsgVotesRequest(voteIds)

	assert.Equal(t, uint8(MessageTypeVotesRequest), msg.Type())
	assert.Equal(t, voteIds, msg.VoteIds)
}

func TestMsgVotes(t *testing.T) {
	votes := []cbor.RawMessage{
		[]byte{0x82, 0x05, 0x06},
	}

	msg := NewMsgVotes(votes)

	assert.Equal(t, uint8(MessageTypeVotes), msg.Type())
	assert.Equal(t, votes, msg.VotesRaw)
}

func TestMsgVotesTypedHelpers(t *testing.T) {
	votes := []lcommon.LeiosVote{
		{
			SlotNo:            100,
			EndorserBlockHash: lcommon.NewBlake2b256([]byte{0x01}),
			VoterId:           7,
			VoteSignature:     testLeiosSignature(0xaa),
		},
	}

	msg, err := NewMsgVotesFromVotes(votes)
	require.NoError(t, err)

	decodedVotes, err := msg.DecodeVotes()
	require.NoError(t, err)
	require.Len(t, decodedVotes, len(votes))
	assert.Equal(t, votes[0].SlotNo, decodedVotes[0].SlotNo)
	assert.Equal(t, votes[0].EndorserBlockHash, decodedVotes[0].EndorserBlockHash)
	assert.Equal(t, votes[0].VoterId, decodedVotes[0].VoterId)
	assert.Equal(t, votes[0].VoteSignature, decodedVotes[0].VoteSignature)
	assert.Equal(t, []byte(msg.VotesRaw[0]), decodedVotes[0].Cbor())
}

func TestMsgVotesDecodeVotesRejectsInvalidVote(t *testing.T) {
	invalidVoteCbor, err := cbor.Encode([]any{
		uint64(100),
		lcommon.NewBlake2b256([]byte{0x01}),
		uint64(7),
		[]byte{0xaa},
	})
	require.NoError(t, err)

	msg := NewMsgVotes([]cbor.RawMessage{invalidVoteCbor})
	_, err = msg.DecodeVotes()
	require.Error(t, err)
}

func TestMsgBlockRangeRequest(t *testing.T) {
	start := pcommon.NewPoint(100, []byte{0x01, 0x02, 0x03, 0x04})
	end := pcommon.NewPoint(200, []byte{0x05, 0x06, 0x07, 0x08})

	msg := NewMsgBlockRangeRequest(start, end)

	assert.Equal(t, uint8(MessageTypeBlockRangeRequest), msg.Type())
	assert.Equal(t, start, msg.Start)
	assert.Equal(t, end, msg.End)
}

func TestMsgNextBlockAndTxsInRange(t *testing.T) {
	blockRaw := cbor.RawMessage([]byte{0x82, 0x01, 0x02})
	txs := []cbor.RawMessage{
		[]byte{0x82, 0x03, 0x04},
	}

	msg := NewMsgNextBlockAndTxsInRange(blockRaw, txs)

	assert.Equal(t, uint8(MessageTypeNextBlockAndTxsInRange), msg.Type())
	assert.Equal(t, blockRaw, msg.BlockRaw)
	assert.Equal(t, txs, msg.TxsRaw)
}

func TestMsgLastBlockAndTxsInRange(t *testing.T) {
	blockRaw := cbor.RawMessage([]byte{0x82, 0x01, 0x02})
	txs := []cbor.RawMessage{
		[]byte{0x82, 0x03, 0x04},
	}

	msg := NewMsgLastBlockAndTxsInRange(blockRaw, txs)

	assert.Equal(t, uint8(MessageTypeLastBlockAndTxsInRange), msg.Type())
	assert.Equal(t, blockRaw, msg.BlockRaw)
	assert.Equal(t, txs, msg.TxsRaw)
}

func TestMsgDone(t *testing.T) {
	msg := NewMsgDone()

	assert.Equal(t, uint8(MessageTypeDone), msg.Type())
}

func TestMsgNoBlock(t *testing.T) {
	msg := NewMsgNoBlock()

	assert.Equal(t, uint8(MessageTypeNoBlock), msg.Type())

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeNoBlock, encoded)
	require.NoError(t, err)
	assert.IsType(t, &MsgNoBlock{}, decoded)
	assert.Equal(t, uint8(MessageTypeNoBlock), decoded.Type())
}

func TestMsgNoBlockTxs(t *testing.T) {
	msg := NewMsgNoBlockTxs()

	assert.Equal(t, uint8(MessageTypeNoBlockTxs), msg.Type())

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeNoBlockTxs, encoded)
	require.NoError(t, err)
	assert.IsType(t, &MsgNoBlockTxs{}, decoded)
	assert.Equal(t, uint8(MessageTypeNoBlockTxs), decoded.Type())
}

func TestNewMsgFromCborUnknownType(t *testing.T) {
	// Test with unknown message type - the NewMsgFromCbor function tries to
	// decode into a nil pointer, which results in an error
	data := []byte{0x80} // empty array
	msg, err := NewMsgFromCbor(999, data)
	// When the message type is unknown, ret is nil, and
	// cbor.Decode(data, nil) returns an error
	assert.Error(t, err)
	assert.Nil(t, msg)
}

func TestMsgBlockTxsRequestEmptyBitmaps(t *testing.T) {
	slot := uint64(123)
	hash := []byte{0x01, 0x02}
	bitmaps := map[uint16]uint64{}

	msg := NewMsgBlockTxsRequest(pcommon.NewPoint(slot, hash), bitmaps)

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeBlockTxsRequest, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgBlockTxsRequest)
	assert.Equal(t, slot, decodedMsg.Point.Slot)
	assert.Equal(t, hash, decodedMsg.Point.Hash)
	assert.Equal(t, 0, len(decodedMsg.Bitmaps))
}

func TestMsgVotesRequestEmpty(t *testing.T) {
	msg := NewMsgVotesRequest([]MsgVotesRequestVoteId{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeVotesRequest, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgVotesRequest)
	assert.Equal(t, 0, len(decodedMsg.VoteIds))
}

func TestMsgBlockTxsEmpty(t *testing.T) {
	msg := NewMsgBlockTxs([]cbor.RawMessage{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeBlockTxs, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgBlockTxs)
	assert.Equal(t, 0, len(decodedMsg.TxsRaw))
}

func TestMsgVotesEmpty(t *testing.T) {
	msg := NewMsgVotes([]cbor.RawMessage{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeVotes, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgVotes)
	assert.Equal(t, 0, len(decodedMsg.VotesRaw))
}
