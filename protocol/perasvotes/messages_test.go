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

package perasvotes

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func testVoteObject(t *testing.T, id VoteID) VoteObject {
	t.Helper()
	data, err := cbor.Encode([]any{
		id.RoundNo,
		[]byte{0xaa},
		id.SeatIndex,
		[]byte{0xbb},
		[]byte{0xcc},
	})
	require.NoError(t, err)
	return VoteObject(data)
}

func TestObjectDiffusionMessageWireFixtures(t *testing.T) {
	id := VoteID{RoundNo: 1, SeatIndex: 2}
	object := testVoteObject(t, id)
	for _, test := range []struct {
		name string
		msg  any
		want []byte
	}{
		{name: "init", msg: NewMsgInit(), want: []byte{0x81, 0x00}},
		{
			name: "request IDs",
			msg:  NewMsgRequestObjectIDs(true, 1, 3),
			want: []byte{0x84, 0x01, 0xf5, 0x01, 0x03},
		},
		{
			name: "reply IDs",
			msg:  NewMsgReplyObjectIDs([]VoteID{id}),
			want: []byte{0x82, 0x02, 0x9f, 0x82, 0x01, 0x02, 0xff},
		},
		{
			name: "request objects",
			msg:  NewMsgRequestObjects([]VoteID{id}),
			want: []byte{0x82, 0x03, 0x9f, 0x82, 0x01, 0x02, 0xff},
		},
		{
			name: "reply objects",
			msg:  NewMsgReplyObjects([]VoteObject{object}),
			want: []byte{
				0x82, 0x04, 0x9f, 0x85, 0x01, 0x41, 0xaa,
				0x02, 0x41, 0xbb, 0x41, 0xcc, 0xff,
			},
		},
		{name: "done", msg: NewMsgDone(), want: []byte{0x81, 0x05}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := cbor.Encode(test.msg)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestMessagesDecodeObjectDiffusionWireShape(t *testing.T) {
	id := VoteID{RoundNo: 12, SeatIndex: 99}
	object := testVoteObject(t, id)
	for _, test := range []struct {
		typeID uint8
		msg    any
	}{
		{MessageTypeInit, NewMsgInit()},
		{MessageTypeRequestObjectIDs, NewMsgRequestObjectIDs(false, 1, 2)},
		{MessageTypeReplyObjectIDs, NewMsgReplyObjectIDs([]VoteID{id})},
		{MessageTypeRequestObjects, NewMsgRequestObjects([]VoteID{id})},
		{MessageTypeReplyObjects, NewMsgReplyObjects([]VoteObject{object})},
		{MessageTypeDone, NewMsgDone()},
	} {
		encoded, err := cbor.Encode(test.msg)
		require.NoError(t, err)
		decoded, err := NewMsgFromCbor(uint(test.typeID), encoded)
		require.NoError(t, err)
		require.Equal(t, test.typeID, decoded.Type())
	}
}

func TestMalformedAndUnknownMessagesRejected(t *testing.T) {
	for _, test := range []struct {
		name   string
		typeID uint
		data   []byte
	}{
		{"unknown ID 6", 6, []byte{0x81, 0x06}},
		{"unknown ID 10", 10, []byte{0x81, 0x0a}},
		{"mux type mismatch", uint(MessageTypeDone), []byte{0x81, MessageTypeInit}},
		{"wrong field count", uint(MessageTypeInit), []byte{0x82, 0x00, 0x00}},
		{"trailing bytes", uint(MessageTypeDone), []byte{0x81, 0x05, 0x00}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewMsgFromCbor(test.typeID, test.data)
			require.Error(t, err)
		})
	}
}

func TestVoteIDAndObjectRoundTrip(t *testing.T) {
	id := VoteID{RoundNo: 42, SeatIndex: 17}
	encoded, err := cbor.Encode(id)
	require.NoError(t, err)
	var decoded VoteID
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.Equal(t, id, decoded)
	object := testVoteObject(t, id)
	got, err := object.VoteID()
	require.NoError(t, err)
	require.Equal(t, id, got)
	require.True(t, bytes.Equal(object, testVoteObject(t, got)))
}
