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
	"bytes"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockTxsRejectsEnvelopeCountBeforeAllocation(t *testing.T) {
	wire := append(
		[]byte{0x99, 0x13, 0x88},
		bytes.Repeat([]byte{0}, 5000)...,
	)
	_, err := NewMsgFromCbor(MessageTypeBlockTxs, wire)
	require.Error(t, err)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			msg, err := NewMsgFromCbor(MessageTypeBlockTxs, wire)
			if err == nil || msg != nil {
				b.Fatal("invalid envelope accepted")
			}
		}
	})
	require.Positive(t, result.N)
	require.LessOrEqual(t, result.AllocedBytesPerOp(), int64(64<<10))
}

func TestBlockTxsRejectsTransactionCountBeforeAllocation(t *testing.T) {
	// [3, array(5000)] contains no transaction items. The declared inner count
	// must be rejected before []RawMessage allocation.
	wire := []byte{0x82, MessageTypeBlockTxs, 0x99, 0x13, 0x88}
	// Warm lazy decoder initialization before measuring the rejected path.
	_, err := NewMsgFromCbor(MessageTypeBlockTxs, wire)
	require.Error(t, err)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			if _, err := NewMsgFromCbor(MessageTypeBlockTxs, wire); err == nil {
				b.Fatal("truncated transaction list accepted")
			}
		}
	})
	require.Positive(t, result.N)
	require.Less(t, result.AllocedBytesPerOp(), int64(32<<10))
}

// blockTxsWire builds [4, txs] by hand so the tests do not depend on the
// encoder under test. Each entry of txs is already a complete wire item.
func blockTxsWire(txs ...[]byte) []byte {
	wire := []byte{0x82, MessageTypeBlockTxs, 0x80 + byte(len(txs))}
	for _, tx := range txs {
		wire = append(wire, tx...)
	}
	return wire
}

// wrappedTx returns #6.24(bytes .cbor inner) for an inner of under 24 bytes.
func wrappedTx(inner []byte) []byte {
	return append([]byte{0xd8, 0x18, 0x40 + byte(len(inner))}, inner...)
}

// TestBlockTxsWireVector pins the encoding to the blueprint's
// msgLeiosBlockTxs = [4, [* #6.24(bytes .cbor block_transaction)]].
func TestBlockTxsWireVector(t *testing.T) {
	tx1 := []byte{0x82, 0x01, 0x02}
	tx2 := []byte{0x82, 0x03, 0x04}
	want := []byte{
		0x82, 0x04, 0x82,
		0xd8, 0x18, 0x43, 0x82, 0x01, 0x02,
		0xd8, 0x18, 0x43, 0x82, 0x03, 0x04,
	}
	encoded, err := cbor.Encode(
		NewMsgBlockTxs([]cbor.RawMessage{tx1, tx2}),
	)
	require.NoError(t, err)
	require.Equal(t, want, encoded)

	msg, err := NewMsgFromCbor(MessageTypeBlockTxs, want)
	require.NoError(t, err)
	decoded := msg.(*MsgBlockTxs)
	require.Equal(
		t,
		[]cbor.RawMessage{tx1, tx2},
		decoded.TxsRaw,
	)
	reencoded, err := cbor.Encode(decoded)
	require.NoError(t, err)
	require.Equal(t, want, reencoded)
}

func TestBlockTxsEmptyList(t *testing.T) {
	encoded, err := cbor.Encode(NewMsgBlockTxs(nil))
	require.NoError(t, err)
	require.Equal(t, []byte{0x82, 0x04, 0x80}, encoded)
	msg, err := NewMsgFromCbor(MessageTypeBlockTxs, encoded)
	require.NoError(t, err)
	require.Empty(t, msg.(*MsgBlockTxs).TxsRaw)
}

func TestBlockTxsAcceptsIndefiniteList(t *testing.T) {
	wire := []byte{0x82, 0x04, 0x9f}
	wire = append(wire, wrappedTx([]byte{0x80})...)
	wire = append(wire, 0xff)
	msg, err := NewMsgFromCbor(MessageTypeBlockTxs, wire)
	require.NoError(t, err)
	require.Equal(
		t,
		[]cbor.RawMessage{{0x80}},
		msg.(*MsgBlockTxs).TxsRaw,
	)
}

func TestBlockTxsRejectsMalformedTransactions(t *testing.T) {
	tests := []struct {
		name string
		tx   []byte
	}{
		{"plain byte string", []byte{0x41, 0x80}},
		{"inline array", []byte{0x80}},
		{"other tag", []byte{0xd8, 0x19, 0x41, 0x80}},
		{"tag 24 around an array", []byte{0xd8, 0x18, 0x80}},
		{
			"indefinite byte string",
			[]byte{0xd8, 0x18, 0x5f, 0x41, 0x80, 0xff},
		},
		{"inner bytes truncated", wrappedTx([]byte{0x82, 0x01})},
		{"inner bytes with trailing item", wrappedTx([]byte{0x80, 0x80})},
		{"empty inner bytes", wrappedTx(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := NewMsgFromCbor(
				MessageTypeBlockTxs,
				blockTxsWire(tc.tx),
			)
			require.Error(t, err)
			require.Nil(t, msg)
		})
	}
}

func TestBlockTxsRejectsEchoedForm(t *testing.T) {
	// [4, point, bitmaps, txs] was the form before the transactions-only
	// message.
	wire := []byte{0x84, MessageTypeBlockTxs, 0x80, 0xa0, 0x80}
	msg, err := NewMsgFromCbor(MessageTypeBlockTxs, wire)
	require.ErrorContains(t, err, "block transactions envelope")
	require.Nil(t, msg)
}

func TestBlockTxsClientRequiresRequestedCount(t *testing.T) {
	wire := blockTxsWire(
		wrappedTx([]byte{0x80}),
		wrappedTx([]byte{0x80}),
		wrappedTx([]byte{0x80}),
	)
	client := &Client{}
	for _, requested := range []int{2, 4} {
		client.blockRequestSlot.maxBlockTxs = requested
		msg, err := client.messageFromCbor(MessageTypeBlockTxs, wire)
		require.Error(t, err, "requested %d", requested)
		require.Nil(t, msg)
	}
	client.blockRequestSlot.maxBlockTxs = 3
	msg, err := client.messageFromCbor(MessageTypeBlockTxs, wire)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgBlockTxs).TxsRaw, 3)
}

func TestBlockTxsPreservesDeepRawTransaction(t *testing.T) {
	tx := append(bytes.Repeat([]byte{0x81}, 64), 0)
	encoded, err := cbor.Encode(NewMsgBlockTxs([]cbor.RawMessage{tx}))
	require.NoError(t, err)
	client := &Client{}
	client.blockRequestSlot.maxBlockTxs = 1

	msg, err := client.messageFromCbor(MessageTypeBlockTxs, encoded)
	require.NoError(t, err)
	require.Equal(t, cbor.RawMessage(tx), msg.(*MsgBlockTxs).TxsRaw[0])
}

func TestBlockTxsRequestRejectsDuplicateBitmapBeforeTypedDecode(t *testing.T) {
	// [3, origin, {_ 0: 1, 0: 2}]
	wire := []byte{
		0x83, MessageTypeBlockTxsRequest, 0x80,
		0xbf, 0x00, 0x01, 0x00, 0x02, 0xff,
	}
	msg, err := NewMsgFromCbor(MessageTypeBlockTxsRequest, wire)
	require.ErrorContains(t, err, "not strictly ascending")
	require.Nil(t, msg)
}

// The reference node reads bitmaps in wire order and rejects a key that does
// not exceed its predecessor, a zero bitmap, and a key past the last bitmap an
// endorser block can need. Both map encodings are accepted.
func TestBlockTxsRequestBitmapRules(t *testing.T) {
	point := []byte{0x80}
	request := func(bitmaps ...byte) []byte {
		return append(
			append([]byte{0x83, MessageTypeBlockTxsRequest}, point...),
			bitmaps...,
		)
	}
	tests := []struct {
		name    string
		wire    []byte
		wantErr string
	}{
		{
			name: "definite map",
			wire: request(0xa2, 0x00, 0x01, 0x01, 0x02),
		},
		{
			name: "indefinite map",
			wire: request(0xbf, 0x00, 0x01, 0x01, 0x02, 0xff),
		},
		{
			name: "last index",
			wire: request(0xa1, 0x19, 0x01, 0x0b, 0x01),
		},
		{
			name:    "index past the last bitmap",
			wire:    request(0xa1, 0x19, 0x01, 0x0c, 0x01),
			wantErr: "must be an index below 268",
		},
		{
			name:    "descending keys",
			wire:    request(0xa2, 0x01, 0x01, 0x00, 0x01),
			wantErr: "not strictly ascending",
		},
		{
			name:    "zero bitmap",
			wire:    request(0xa1, 0x00, 0x00),
			wantErr: "selects no transaction",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg, err := NewMsgFromCbor(MessageTypeBlockTxsRequest, test.wire)
			if test.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, msg)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
			require.Nil(t, msg)
		})
	}
}

func TestWireTagsMatchReferenceNode(t *testing.T) {
	assert.Equal(t, 0, MessageTypeDone)
	assert.Equal(t, 1, MessageTypeBlockRequest)
	assert.Equal(t, 2, MessageTypeBlock)
	assert.Equal(t, 3, MessageTypeBlockTxsRequest)
	assert.Equal(t, 4, MessageTypeBlockTxs)
	// The extension messages must not reuse a reference tag
	for _, tag := range []int{
		MessageTypeVotesRequest,
		MessageTypeVotes,
		MessageTypeBlockRangeRequest,
		MessageTypeNextBlockAndTxsInRange,
		MessageTypeLastBlockAndTxsInRange,
	} {
		assert.Greater(t, tag, MessageTypeBlockTxs)
	}
}

func TestLeiosFetchRolesRejectWrongDirectionBeforeDecode(t *testing.T) {
	declared := []byte{0x82, MessageTypeBlockTxs, 0x9a, 0, 0x40, 0}
	msg, err := serverMessageFromCbor(MessageTypeBlockTxs, declared)
	require.ErrorContains(t, err, "not a client request")
	require.Nil(t, msg)

	client := &Client{}
	msg, err = client.messageFromCbor(MessageTypeBlockTxsRequest, declared)
	require.ErrorContains(t, err, "not a server response")
	require.Nil(t, msg)
}

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
					testPointHash(0x01),
				),
			),
			MessageType: 1,
		},
		{
			Name: "MsgBlock",
			Message: NewMsgBlock(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
			),
			MessageType: 2,
		},
		{
			Name: "MsgBlockTxsRequest",
			Message: NewMsgBlockTxsRequest(
				pcommon.NewPoint(
					12345,
					testPointHash(0x01),
				),
				map[uint16]uint64{
					0:  0xff00000000000000,
					64: 0x00ff000000000000,
				},
			),
			MessageType: 3,
		},
		{
			Name: "MsgBlockTxs",
			Message: NewMsgBlockTxs(
				[]cbor.RawMessage{
					[]byte{0x82, 0x01, 0x02},
					[]byte{0x82, 0x03, 0x04},
					[]byte{0x82, 0x05, 0x06},
				},
			),
			MessageType: 4,
		},
		{
			Name: "MsgVotesRequest",
			Message: NewMsgVotesRequest(
				[]MsgVotesRequestVoteId{
					{SlotNo: 100, VoterId: 1},
					{SlotNo: 200, VoterId: 2},
				},
			),
			MessageType: 10,
		},
		{
			Name: "MsgVotes",
			Message: NewMsgVotes(
				[]cbor.RawMessage{
					[]byte{0x82, 0x05, 0x06},
					[]byte{0x82, 0x07, 0x08},
				},
			),
			MessageType: 11,
		},
		{
			Name: "MsgBlockRangeRequest",
			Message: NewMsgBlockRangeRequest(
				pcommon.NewPoint(
					123,
					testPointHash(0x01),
				),
				pcommon.NewPoint(
					456,
					testPointHash(0x09),
				),
			),
			MessageType: 12,
		},
		{
			Name: "MsgNextBlockAndTxsInRange",
			Message: NewMsgNextBlockAndTxsInRange(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
				[]cbor.RawMessage{
					[]byte{0x82, 0x03, 0x04},
				},
			),
			MessageType: 13,
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
			MessageType: 14,
		},
		{
			Name:        "MsgDone",
			Message:     NewMsgDone(),
			MessageType: 0,
		},
	}
}

func TestMessageTagFixtures(t *testing.T) {
	for _, test := range getTestDefinitions() {
		t.Run(test.Name, func(t *testing.T) {
			require.Equal(t, uint8(test.MessageType), test.Message.Type())
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err)
			var fields []cbor.RawMessage
			_, err = cbor.Decode(encoded, &fields)
			require.NoError(t, err)
			require.NotEmpty(t, fields)
			var got uint
			_, err = cbor.Decode(fields[0], &got)
			require.NoError(t, err)
			require.Equal(t, test.MessageType, got)
		})
	}
}

func TestUnknownLeiosFetchMessageIDsRejected(t *testing.T) {
	for _, id := range []uint{10, 11} {
		_, err := NewMsgFromCbor(id, []byte{0x81, byte(id)})
		require.Error(t, err)
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
	hash := testPointHash(0x01)

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
	hash := testPointHash(0x01)
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
	start := pcommon.NewPoint(100, testPointHash(0x01))
	end := pcommon.NewPoint(200, testPointHash(0x05))

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

func TestBlockRangeConstructorsCloneTransactionHeaders(t *testing.T) {
	for _, construct := range []func(
		cbor.RawMessage,
		[]cbor.RawMessage,
	) []cbor.RawMessage{
		func(block cbor.RawMessage, txs []cbor.RawMessage) []cbor.RawMessage {
			return NewMsgNextBlockAndTxsInRange(block, txs).TxsRaw
		},
		func(block cbor.RawMessage, txs []cbor.RawMessage) []cbor.RawMessage {
			return NewMsgLastBlockAndTxsInRange(block, txs).TxsRaw
		},
	} {
		txs := []cbor.RawMessage{{0x80}}
		retained := construct(nil, txs)
		txs[0] = cbor.RawMessage{0x81, 0x00}
		require.Equal(t, cbor.RawMessage{0x80}, retained[0])
	}
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
	data := []byte{0x80} // empty array
	msg, err := NewMsgFromCbor(999, data)
	require.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), ProtocolName)
	assert.Contains(t, err.Error(), "999")
}

func TestNewMsgFromCborRejectsMismatchedType(t *testing.T) {
	_, err := NewMsgFromCbor(MessageTypeDone, []byte{0x81, MessageTypeVotes})
	require.Error(t, err)
	assert.ErrorContains(t, err, "message type mismatch")
}

func TestMsgBlockTxsRequestEmptyBitmaps(t *testing.T) {
	slot := uint64(123)
	hash := testPointHash(0x01)
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

func TestMsgVotesEmpty(t *testing.T) {
	msg := NewMsgVotes([]cbor.RawMessage{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeVotes, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgVotes)
	assert.Equal(t, 0, len(decodedMsg.VotesRaw))
}

func TestMsgVotesNilUsesEmptyArray(t *testing.T) {
	encoded, err := cbor.Encode(NewMsgVotes(nil))
	require.NoError(t, err)
	require.Equal(t, []byte{0x82, MessageTypeVotes, 0x80}, encoded)
}

func TestVotesResponseUsesResponseCountBudget(t *testing.T) {
	valid := append(
		[]byte{0x82, MessageTypeVotes, 0x99, 0x03, 0xe8},
		bytes.Repeat([]byte{0x80}, 1_000)...,
	)
	msg, err := NewMsgFromCbor(MessageTypeVotes, valid)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgVotes).VotesRaw, 1_000)

	oversized := append(
		[]byte{0x82, MessageTypeVotes, 0x99, 0x03, 0xe9},
		bytes.Repeat([]byte{0x80}, 1_001)...,
	)
	msg, err = NewMsgFromCbor(MessageTypeVotes, oversized)
	require.ErrorContains(t, err, "maximum is 1000")
	require.Nil(t, msg)
}

func TestVotesResponseUsesRequestedCount(t *testing.T) {
	wire := []byte{0x82, MessageTypeVotes, 0x83, 0x80, 0x80, 0x80}
	client := &Client{}
	client.blockRequestSlot.maxVotes = 2
	msg, err := client.messageFromCbor(MessageTypeVotes, wire)
	require.ErrorContains(t, err, "maximum is 2")
	require.Nil(t, msg)

	client.blockRequestSlot.maxVotes = 3
	msg, err = client.messageFromCbor(MessageTypeVotes, wire)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgVotes).VotesRaw, 3)
}

func TestVotesResponseRejectsDeepVoteBeforeTypedDecode(t *testing.T) {
	wire := []byte{0x82, MessageTypeVotes, 0x81, 0x81, 0x81, 0x00}
	msg, err := NewMsgFromCbor(MessageTypeVotes, wire)
	require.ErrorContains(t, err, "maximum depth 1")
	require.Nil(t, msg)
}

func TestBlockRangeResponsesBoundTransactionCollections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		msgType uint
		wire    []byte
	}{
		{
			name:    "definite_next",
			msgType: MessageTypeNextBlockAndTxsInRange,
			wire: []byte{0x83, MessageTypeNextBlockAndTxsInRange, 0xf6,
				0x83, 0x80, 0x80, 0x80},
		},
		{
			name:    "indefinite_last",
			msgType: MessageTypeLastBlockAndTxsInRange,
			wire: []byte{0x83, MessageTypeLastBlockAndTxsInRange, 0xf6,
				0x9f, 0x80, 0x80, 0x80, 0xff},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := decodeBlockAndTxsInRange(
				tc.msgType,
				tc.wire,
				maxWireBlockTxs,
				2*rangeTxDecodedBudgetBytes,
			)
			require.ErrorContains(t, err, "maximum is 2")
			require.Nil(t, msg)
		})
	}
}

func TestBlockRangeResponseCountUsesDecodedMemoryBudget(t *testing.T) {
	require.Equal(
		t,
		DefaultMaxBlockRangeResponseBytes/rangeTxDecodedBudgetBytes,
		maxRangeTransactionCount(DefaultMaxBlockRangeResponseBytes),
	)
}

func TestBlockRangeRejectsDecodedCountBeforeAllocation(t *testing.T) {
	// The transaction array contains one more item than the default decoded
	// memory budget allows. Its count must be rejected before item storage is
	// allocated.
	wire := append([]byte{
		0x83, MessageTypeNextBlockAndTxsInRange, 0xf6,
		0x9a, 0x00, 0x10, 0x00, 0x01,
	}, bytes.Repeat([]byte{0}, 1_048_577)...)
	_, err := NewMsgFromCbor(MessageTypeNextBlockAndTxsInRange, wire)
	require.ErrorContains(t, err, "maximum is 1048576")
}

func TestBlockRangeResponseUsesConfiguredByteBudget(t *testing.T) {
	wire := []byte{
		0x83, MessageTypeNextBlockAndTxsInRange, 0xf6, 0x80,
	}
	msg, err := decodeBlockAndTxsInRange(
		MessageTypeNextBlockAndTxsInRange,
		wire,
		maxWireBlockTxs,
		len(wire)-1,
	)
	require.ErrorContains(t, err, "exceeds maximum")
	require.Nil(t, msg)

	msg, err = decodeBlockAndTxsInRange(
		MessageTypeNextBlockAndTxsInRange,
		wire,
		maxWireBlockTxs,
		len(wire),
	)
	require.NoError(t, err)
	require.Empty(t, msg.(*MsgNextBlockAndTxsInRange).TxsRaw)
}

func TestBlockRangeResponseUsesTransactionCountBudget(t *testing.T) {
	wire := []byte{
		0x83, MessageTypeLastBlockAndTxsInRange, 0xf6,
		0x83, 0x80, 0x80, 0x80,
	}
	msg, err := decodeBlockAndTxsInRange(
		MessageTypeLastBlockAndTxsInRange,
		wire,
		2,
		3*rangeTxDecodedBudgetBytes,
	)
	require.ErrorContains(t, err, "maximum is 2")
	require.Nil(t, msg)

	msg, err = decodeBlockAndTxsInRange(
		MessageTypeLastBlockAndTxsInRange,
		wire,
		3,
		3*rangeTxDecodedBudgetBytes,
	)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgLastBlockAndTxsInRange).TxsRaw, 3)
}
