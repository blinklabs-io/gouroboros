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
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDefinition struct {
	Name        string
	Message     protocol.Message
	MessageType uint
}

func testVote() Vote {
	return Vote{
		SlotNo:            12345,
		EndorserBlockHash: lcommon.NewBlake2b256([]byte{0x01, 0x02, 0x03, 0x04}),
		VoterId:           42,
		VoteSignature:     testSignature(0xaa),
	}
}

func testSignature(fill byte) []byte {
	ret := make([]byte, lcommon.LeiosBlsSignatureSize)
	for i := range ret {
		ret[i] = fill
	}
	return ret
}

func getTestDefinitions() []testDefinition {
	return []testDefinition{
		{
			Name:        "MsgVotesRequestNext",
			Message:     NewMsgVotesRequestNext(10),
			MessageType: MessageTypeVotesRequestNext,
		},
		{
			Name:        "MsgVote",
			Message:     NewMsgVote(testVote()),
			MessageType: MessageTypeVote,
		},
		{
			Name:        "MsgDone",
			Message:     NewMsgDone(),
			MessageType: MessageTypeDone,
		},
	}
}

func TestCborRoundTrip(t *testing.T) {
	for _, test := range getTestDefinitions() {
		t.Run(test.Name, func(t *testing.T) {
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err)

			decoded, err := NewMsgFromCbor(test.MessageType, encoded)
			require.NoError(t, err)

			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err)
			assert.Equal(t, encoded, reencoded)
		})
	}
}

func TestDecode(t *testing.T) {
	for _, test := range getTestDefinitions() {
		t.Run(test.Name, func(t *testing.T) {
			encoded, err := cbor.Encode(test.Message)
			require.NoError(t, err)

			decoded, err := NewMsgFromCbor(test.MessageType, encoded)
			require.NoError(t, err)
			test.Message.SetCbor(encoded)

			if test.MessageType == MessageTypeVote {
				expected := test.Message.(*MsgVote)
				actual := decoded.(*MsgVote)
				assert.Equal(t, expected.Type(), actual.Type())
				assert.Equal(t, expected.Cbor(), actual.Cbor())
				assert.Equal(t, expected.Vote.SlotNo, actual.Vote.SlotNo)
				assert.Equal(
					t,
					expected.Vote.EndorserBlockHash,
					actual.Vote.EndorserBlockHash,
				)
				assert.Equal(t, expected.Vote.VoterId, actual.Vote.VoterId)
				assert.Equal(
					t,
					expected.Vote.VoteSignature,
					actual.Vote.VoteSignature,
				)
				require.NotEmpty(t, actual.Vote.Cbor())
				return
			}
			assert.True(t, reflect.DeepEqual(decoded, test.Message))
		})
	}
}

func TestMsgVotesRequestNext(t *testing.T) {
	msg := NewMsgVotesRequestNext(100)

	assert.Equal(t, uint8(MessageTypeVotesRequestNext), msg.Type())
	assert.Equal(t, uint64(100), msg.Count)
}

func TestMsgVote(t *testing.T) {
	vote := testVote()
	msg := NewMsgVote(vote)

	assert.Equal(t, uint8(MessageTypeVote), msg.Type())
	assert.Equal(t, vote, msg.Vote)
}

func TestMsgDone(t *testing.T) {
	msg := NewMsgDone()

	assert.Equal(t, uint8(MessageTypeDone), msg.Type())
}

func TestNewMsgFromCborUnknownType(t *testing.T) {
	msg, err := NewMsgFromCbor(999, []byte{0x80})

	assert.Error(t, err)
	assert.Nil(t, msg)
}
