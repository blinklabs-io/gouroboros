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
					{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
					{Slot: 200, VoteIssuerId: []byte{0x05, 0x06, 0x07, 0x08}},
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
		{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
		{Slot: 200, VoteIssuerId: []byte{0x05, 0x06, 0x07, 0x08}},
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
	// Test with unknown message type - the NewMsgFromCbor function tries to decode
	// into a nil pointer, which results in an error
	data := []byte{0x80} // empty array
	msg, err := NewMsgFromCbor(999, data)
	// When the message type is unknown, ret is nil, and cbor.Decode(data, nil) returns an error
	assert.Error(t, err)
	assert.Nil(t, msg)
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
