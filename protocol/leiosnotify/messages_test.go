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

package leiosnotify

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

type testDefinition struct {
	Name        string
	Message     protocol.Message
	MessageType uint
}

func getTestDefinitions() []testDefinition {
	return []testDefinition{
		{
			Name:        "MsgNotificationRequestNext",
			Message:     NewMsgNotificationRequestNext(),
			MessageType: 0,
		},
		{
			Name: "MsgBlockAnnouncement",
			Message: NewMsgBlockAnnouncement(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
			),
			MessageType: 1,
		},
		{
			Name: "MsgBlockOffer",
			Message: NewMsgBlockOffer(
				pcommon.NewPoint(
					12345,
					testPointHash(0x01),
				),
				12345,
			),
			MessageType: 2,
		},
		{
			Name: "MsgBlockTxsOffer",
			Message: NewMsgBlockTxsOffer(
				pcommon.NewPoint(
					67890,
					testPointHash(0x09),
				),
			),
			MessageType: 3,
		},
		{
			Name: "MsgVotesOffer",
			Message: NewMsgVotesOffer(
				[]MsgVotesOfferVote{
					{SlotNo: 100, VoterId: 1},
					{SlotNo: 200, VoterId: 2},
				},
			),
			MessageType: 4,
		},
		{
			Name:        "MsgDone",
			Message:     NewMsgDone(),
			MessageType: 5,
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

func TestUnknownMessageIDsRejected(t *testing.T) {
	for _, id := range []uint{6, 10, 11} {
		_, err := NewMsgFromCbor(id, []byte{0x81, byte(id)})
		require.Error(t, err)
	}
}

func TestMessageTypeMustMatchPayload(t *testing.T) {
	_, err := NewMsgFromCbor(MessageTypeDone, []byte{0x81, MessageTypeBlockOffer})
	require.Error(t, err)
	assert.ErrorContains(t, err, "message type mismatch")
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

func TestMsgNotificationRequestNext(t *testing.T) {
	msg := NewMsgNotificationRequestNext()

	assert.Equal(t, uint8(MessageTypeNotificationRequestNext), msg.Type())
}

func TestMsgBlockAnnouncement(t *testing.T) {
	blockHeaderRaw := cbor.RawMessage([]byte{0x82, 0x01, 0x02})

	msg := NewMsgBlockAnnouncement(blockHeaderRaw)

	assert.Equal(t, uint8(MessageTypeBlockAnnouncement), msg.Type())
	assert.Equal(t, blockHeaderRaw, msg.BlockHeaderRaw)
}

func TestMsgBlockOffer(t *testing.T) {
	slot := uint64(123456)
	hash := testPointHash(0x01)

	msg := NewMsgBlockOffer(pcommon.NewPoint(slot, hash), 12345)

	assert.Equal(t, uint8(MessageTypeBlockOffer), msg.Type())
	assert.Equal(t, slot, msg.Point.Slot)
	assert.Equal(t, hash, msg.Point.Hash)
}

func TestMsgBlockTxsOffer(t *testing.T) {
	slot := uint64(123456)
	hash := testPointHash(0x01)

	msg := NewMsgBlockTxsOffer(pcommon.NewPoint(slot, hash))

	assert.Equal(t, uint8(MessageTypeBlockTxsOffer), msg.Type())
	assert.Equal(t, slot, msg.Point.Slot)
	assert.Equal(t, hash, msg.Point.Hash)
}

func TestMsgVotesOffer(t *testing.T) {
	votes := []MsgVotesOfferVote{
		{SlotNo: 100, VoterId: 1},
		{SlotNo: 200, VoterId: 2},
	}

	msg := NewMsgVotesOffer(votes)

	assert.Equal(t, uint8(MessageTypeVotesOffer), msg.Type())
	assert.Equal(t, votes, msg.Votes)
}

func TestMsgDone(t *testing.T) {
	msg := NewMsgDone()

	assert.Equal(t, uint8(MessageTypeDone), msg.Type())
}

func TestNewMsgFromCborUnknownType(t *testing.T) {
	data := []byte{0x80} // empty array
	msg, err := NewMsgFromCbor(999, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type 999")
	require.Nil(t, msg)
}

func TestMsgVotesOfferEmpty(t *testing.T) {
	msg := NewMsgVotesOffer([]MsgVotesOfferVote{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeVotesOffer, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgVotesOffer)
	assert.Equal(t, 0, len(decodedMsg.Votes))
}

func TestMsgVotesOfferRejectsOversizedBatchBeforeVoteDecode(t *testing.T) {
	t.Parallel()

	vote := cbor.RawMessage{0x82, 0x01, 0x02}
	votes := make([]cbor.RawMessage, MaxVotesOfferCount+1)
	for idx := range votes {
		votes[idx] = vote
	}
	data, err := cbor.Encode([]any{uint8(MessageTypeVotesOffer), votes})
	require.NoError(t, err)

	var msg MsgVotesOffer
	err = msg.UnmarshalCBOR(data)
	require.ErrorContains(t, err, "maximum")
	assert.Empty(t, msg.Votes)
}

func TestMsgVotesOfferRejectsOversizedMessageBeforeParsing(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte{0xff}, MaxVotesOfferBytes+1)
	msg, err := NewMsgFromCbor(MessageTypeVotesOffer, data)
	require.ErrorContains(t, err, "exceeds maximum")
	require.ErrorContains(t, err, "bytes")
	assert.Nil(t, msg)
}

func TestNewMsgFromCborBoundsVotesOfferBeforeGenericValidation(t *testing.T) {
	t.Parallel()

	// The declared vote count exceeds the limit and the body is intentionally
	// truncated. The votes-offer parser must reject from the array header before
	// a generic well-formedness pass scans the body.
	data := []byte{0x82, MessageTypeVotesOffer, 0x99, 0x03, 0xe9}
	msg, err := NewMsgFromCbor(MessageTypeVotesOffer, data)
	require.ErrorContains(t, err, "maximum")
	assert.Nil(t, msg)
}

func TestMsgVotesOfferMarshalRejectsOversizedBatch(t *testing.T) {
	t.Parallel()

	votes := make([]MsgVotesOfferVote, MaxVotesOfferCount+1)
	_, err := NewMsgVotesOffer(votes).MarshalCBOR()
	require.ErrorContains(t, err, "maximum")
}

func TestMsgVotesOfferMarshalRejectsOversizedMessage(t *testing.T) {
	t.Parallel()

	msg := NewMsgVotesOfferFull([]lcommon.LeiosVote{{
		VoteSignature: make([]byte, MaxVotesOfferBytes),
	}})
	_, err := msg.MarshalCBOR()
	require.ErrorContains(t, err, "exceeds maximum")
}

func TestMsgVotesOfferRejectsOversizedIndefiniteBatch(t *testing.T) {
	t.Parallel()

	var data bytes.Buffer
	data.Write([]byte{0x9f, 0x04, 0x9f})
	for range MaxVotesOfferCount + 1 {
		data.Write([]byte{0x82, 0x01, 0x02})
	}
	data.Write([]byte{0xff, 0xff})

	var msg MsgVotesOffer
	err := msg.UnmarshalCBOR(data.Bytes())
	require.ErrorContains(t, err, "maximum")
	assert.Empty(t, msg.Votes)
}

func TestMsgVotesOfferRejectsOversizedVoteShape(t *testing.T) {
	t.Parallel()

	vote, err := cbor.Encode([]any{
		uint64(1), uint64(2), uint64(3), uint64(4), uint64(5),
	})
	require.NoError(t, err)
	data, err := cbor.Encode([]any{
		uint8(MessageTypeVotesOffer),
		[]cbor.RawMessage{cbor.RawMessage(vote)},
	})
	require.NoError(t, err)

	var msg MsgVotesOffer
	require.Error(t, msg.UnmarshalCBOR(data))
}

func TestMsgVotesOfferAcceptsIndefiniteArrays(t *testing.T) {
	t.Parallel()

	// [4, [[1, 2]]] with indefinite outer and vote arrays.
	data := []byte{0x9f, 0x04, 0x9f, 0x82, 0x01, 0x02, 0xff, 0xff}
	var msg MsgVotesOffer
	require.NoError(t, msg.UnmarshalCBOR(data))
	require.Len(t, msg.Votes, 1)
	assert.Equal(t, uint64(1), msg.Votes[0].SlotNo)
	assert.Equal(t, uint64(2), msg.Votes[0].VoterId)
}

func TestMsgBlockAnnouncementEmpty(t *testing.T) {
	// Create a message with empty block header
	msg := NewMsgBlockAnnouncement(cbor.RawMessage{})

	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeBlockAnnouncement, encoded)
	require.NoError(t, err)

	decodedMsg := decoded.(*MsgBlockAnnouncement)
	// After CBOR round-trip, empty RawMessage may decode differently
	// so we just check it's not nil
	assert.NotNil(t, decodedMsg)
}

// TestMsgVotesOfferThreeElementVote verifies the current prototype vote shape:
// [announcing_rb_hash, voter_id, signature].
func TestMsgVotesOfferThreeElementVote(t *testing.T) {
	ebHash := bytes.Repeat([]byte{0xAB}, lcommon.Blake2b256Size)
	sig := bytes.Repeat([]byte{0xCD}, lcommon.LeiosBlsSignatureSize)
	voteCbor, err := cbor.Encode([]any{ebHash, uint64(7), sig})
	require.NoError(t, err)
	msgCbor, err := cbor.Encode([]any{
		uint(MessageTypeVotesOffer),
		[]cbor.RawMessage{cbor.RawMessage(voteCbor)},
	})
	require.NoError(t, err)

	var m MsgVotesOffer
	require.NoError(t, m.UnmarshalCBOR(msgCbor))
	require.Len(t, m.PrototypeVotes, 1)
	require.Empty(t, m.Votes)
	require.Empty(t, m.FullVotes)
	v := m.PrototypeVotes[0]
	assert.Equal(t, ebHash, v.AnnouncingRbHash.Bytes())
	assert.Equal(t, uint64(7), v.VoterId)
	assert.Equal(t, sig, v.VoteSignature)

	// A 3-element vote with a wrong-length signature is rejected.
	badCbor, err := cbor.Encode([]any{ebHash, uint64(7), []byte{0x01, 0x02}})
	require.NoError(t, err)
	badMsg, err := cbor.Encode([]any{
		uint(MessageTypeVotesOffer),
		[]cbor.RawMessage{cbor.RawMessage(badCbor)},
	})
	require.NoError(t, err)
	var m2 MsgVotesOffer
	require.ErrorContains(t, m2.UnmarshalCBOR(badMsg), "signature is 2 bytes")
}

func TestMsgVotesOfferPrototypeRoundTrip(t *testing.T) {
	vote := PrototypeVote{
		AnnouncingRbHash: lcommon.NewBlake2b256(bytes.Repeat([]byte{0xAB}, 32)),
		VoterId:          7,
		VoteSignature:    bytes.Repeat([]byte{0xCD}, lcommon.LeiosBlsSignatureSize),
	}
	raw, err := NewMsgVotesOfferPrototype([]PrototypeVote{vote}).MarshalCBOR()
	require.NoError(t, err)
	var decoded MsgVotesOffer
	require.NoError(t, decoded.UnmarshalCBOR(raw))
	require.Equal(t, []PrototypeVote{vote}, decoded.PrototypeVotes)
}

func TestMsgVotesOfferUnknownVoteShape(t *testing.T) {
	voteCbor, err := cbor.Encode([]any{uint64(100)})
	require.NoError(t, err)
	msgCbor, err := cbor.Encode([]any{
		uint(MessageTypeVotesOffer),
		[]cbor.RawMessage{cbor.RawMessage(voteCbor)},
	})
	require.NoError(t, err)

	var m MsgVotesOffer
	require.ErrorContains(
		t,
		m.UnmarshalCBOR(msgCbor),
		"votes offer: vote 0 unexpected element count 1",
	)
}
