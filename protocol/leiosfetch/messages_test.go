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
					{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
					{Slot: 200, VoteIssuerId: []byte{0x05, 0x06, 0x07, 0x08}},
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

func TestMsgVotesRequest(t *testing.T) {
	voteIds := []MsgVotesRequestVoteId{
		{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
		{Slot: 200, VoteIssuerId: []byte{0x05, 0x06, 0x07, 0x08}},
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

func TestNewMsgFromCborUnknownType(t *testing.T) {
	// Test with unknown message type - the NewMsgFromCbor function tries to decode
	// into a nil pointer, which results in an error
	data := []byte{0x80} // empty array
	msg, err := NewMsgFromCbor(999, data)
	// When the message type is unknown, ret is nil, and cbor.Decode(data, nil) returns an error
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
