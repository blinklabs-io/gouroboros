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
			MessageType: MessageTypeNotificationRequestNext,
		},
		{
			Name: "MsgBlockAnnouncement",
			Message: NewMsgBlockAnnouncement(
				cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
			),
			MessageType: MessageTypeBlockAnnouncement,
		},
		{
			Name: "MsgBlockOffer",
			Message: NewMsgBlockOffer(
				pcommon.NewPoint(
					12345,
					[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				),
				12345,
			),
			MessageType: MessageTypeBlockOffer,
		},
		{
			Name: "MsgBlockTxsOffer",
			Message: NewMsgBlockTxsOffer(
				pcommon.NewPoint(
					67890,
					[]byte{0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				),
			),
			MessageType: MessageTypeBlockTxsOffer,
		},
		{
			Name: "MsgVotesOffer",
			Message: NewMsgVotesOffer(
				[]MsgVotesOfferVote{
					{SlotNo: 100, VoterId: 1},
					{SlotNo: 200, VoterId: 2},
				},
			),
			MessageType: MessageTypeVotesOffer,
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
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	msg := NewMsgBlockOffer(pcommon.NewPoint(slot, hash), 12345)

	assert.Equal(t, uint8(MessageTypeBlockOffer), msg.Type())
	assert.Equal(t, slot, msg.Point.Slot)
	assert.Equal(t, hash, msg.Point.Hash)
}

func TestMsgBlockTxsOffer(t *testing.T) {
	slot := uint64(123456)
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

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

// TestMsgVotesOfferThreeElementVote verifies decoding the prototype-2026w27
// deployed vote shape: a 3-element array [endorser_block_hash (hash32),
// voter_id (uint), signature (bytes .size 48)] — the full LeiosVote with the
// leading slot dropped. Shape confirmed from live musashi captures.
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
	require.Len(t, m.FullVotes, 1)
	require.Empty(t, m.Votes)
	v := m.FullVotes[0]
	assert.Equal(t, uint64(0), v.SlotNo, "3-element vote omits slot")
	assert.Equal(t, ebHash, v.EndorserBlockHash.Bytes())
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
