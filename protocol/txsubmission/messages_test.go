// Copyright 2023 Blink Labs Software
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

package txsubmission

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
)

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
}

// TODO: implement tests for more messages
var tests = []testDefinition{
	{
		CborHex:     "8104",
		Message:     NewMsgDone(),
		MessageType: MessageTypeDone,
	},
	{
		CborHex:     "8106",
		Message:     NewMsgInit(),
		MessageType: MessageTypeInit,
	},
}

func TestDecode(t *testing.T) {
	for _, test := range tests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		msg, err := NewMsgFromCbor(test.MessageType, cborData)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		// Set the raw CBOR so the comparison should succeed
		test.Message.SetCbor(cborData)
		if !reflect.DeepEqual(msg, test.Message) {
			t.Fatalf(
				"CBOR did not decode to expected message object\n  got: %#v\n  wanted: %#v",
				msg,
				test.Message,
			)
		}
	}
}

func TestEncode(t *testing.T) {
	for _, test := range tests {
		cborData, err := cbor.Encode(test.Message)
		if err != nil {
			t.Fatalf("failed to encode message to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf(
				"message did not encode to expected CBOR\n  got: %s\n  wanted: %s",
				cborHex,
				test.CborHex,
			)
		}
	}
}

func TestMsgRequestTxIds(t *testing.T) {
	msg := NewMsgRequestTxIds(true, 10, 20) // blocking, ack, req

	// Test encoding
	encoded, err := cbor.Encode(msg)
	assert.NoError(t, err)

	// Test decoding
	decoded, err := NewMsgFromCbor(MessageTypeRequestTxIds, encoded)
	assert.NoError(t, err)
	assert.Equal(t, msg.Blocking, decoded.(*MsgRequestTxIds).Blocking)
	assert.Equal(t, msg.Ack, decoded.(*MsgRequestTxIds).Ack)
	assert.Equal(t, msg.Req, decoded.(*MsgRequestTxIds).Req)
}

func TestMsgReplyTxIds(t *testing.T) {
	txIds := []TxIdAndSize{
		{TxId: TxId{EraId: 1, TxId: [32]byte{0x01, 0x02}}, Size: 100},
		{TxId: TxId{EraId: 2, TxId: [32]byte{0x03, 0x04}}, Size: 200},
	}
	msg := NewMsgReplyTxIds(txIds)

	encoded, err := cbor.Encode(msg)
	assert.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeReplyTxIds, encoded)
	assert.NoError(t, err)
	assert.Len(t, decoded.(*MsgReplyTxIds).TxIds, 2)
	assert.Equal(
		t,
		txIds[0].TxId.EraId,
		decoded.(*MsgReplyTxIds).TxIds[0].TxId.EraId,
	)
	assert.Equal(t, txIds[0].Size, decoded.(*MsgReplyTxIds).TxIds[0].Size)
}

func TestMsgRequestTxs(t *testing.T) {
	txIds := []TxId{
		{EraId: 1, TxId: [32]byte{0x01}},
		{EraId: 2, TxId: [32]byte{0x02}},
	}
	msg := NewMsgRequestTxs(txIds)

	encoded, err := cbor.Encode(msg)
	assert.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeRequestTxs, encoded)
	assert.NoError(t, err)
	assert.Len(t, decoded.(*MsgRequestTxs).TxIds, 2)
	assert.Equal(t, txIds[0].EraId, decoded.(*MsgRequestTxs).TxIds[0].EraId)
}

func TestMsgReplyTxs(t *testing.T) {
	txs := []TxBody{
		{EraId: 1, TxBody: []byte{0x01, 0x02, 0x03}},
		{EraId: 2, TxBody: []byte{0x04, 0x05, 0x06}},
	}
	msg := NewMsgReplyTxs(txs)

	encoded, err := cbor.Encode(msg)
	assert.NoError(t, err)

	decoded, err := NewMsgFromCbor(MessageTypeReplyTxs, encoded)
	assert.NoError(t, err)
	assert.Len(t, decoded.(*MsgReplyTxs).Txs, 2)
	assert.Equal(t, txs[0].EraId, decoded.(*MsgReplyTxs).Txs[0].EraId)
	assert.Equal(t, txs[0].TxBody, decoded.(*MsgReplyTxs).Txs[0].TxBody)
}
