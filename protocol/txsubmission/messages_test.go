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

package txsubmission

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

type testDefinition struct {
	Name        string
	CborHex     string
	Message     func() protocol.Message
	MessageType uint
}

var tests = []testDefinition{
	{
		Name:    "RequestTxIds",
		CborHex: "8400f50a14",
		Message: func() protocol.Message {
			return NewMsgRequestTxIds(true, 10, 20)
		},
		MessageType: MessageTypeRequestTxIds,
	},
	{
		Name: "ReplyTxIds",
		CborHex: "82019f" +
			"8282015820" + testTxIdHex(0x01, 0x02) + "1864" +
			"8282025820" + testTxIdHex(0x03, 0x04) + "18c8" +
			"ff",
		Message: func() protocol.Message {
			return NewMsgReplyTxIds([]TxIdAndSize{
				{TxId: testTxId(1, 0x01, 0x02), Size: 100},
				{TxId: testTxId(2, 0x03, 0x04), Size: 200},
			})
		},
		MessageType: MessageTypeReplyTxIds,
	},
	{
		Name: "RequestTxs",
		CborHex: "82029f" +
			"82015820" + testTxIdHex(0x01) +
			"82025820" + testTxIdHex(0x02) +
			"ff",
		Message: func() protocol.Message {
			return NewMsgRequestTxs([]TxId{
				testTxId(1, 0x01),
				testTxId(2, 0x02),
			})
		},
		MessageType: MessageTypeRequestTxs,
	},
	{
		Name: "ReplyTxs",
		CborHex: "82039f" +
			"8201d81843010203" +
			"8202d81843040506" +
			"ff",
		Message: func() protocol.Message {
			return NewMsgReplyTxs([]TxBody{
				{EraId: 1, TxBody: []byte{0x01, 0x02, 0x03}},
				{EraId: 2, TxBody: []byte{0x04, 0x05, 0x06}},
			})
		},
		MessageType: MessageTypeReplyTxs,
	},
	{
		Name:        "Done",
		CborHex:     "8104",
		Message:     func() protocol.Message { return NewMsgDone() },
		MessageType: MessageTypeDone,
	},
	{
		Name:        "Init",
		CborHex:     "8106",
		Message:     func() protocol.Message { return NewMsgInit() },
		MessageType: MessageTypeInit,
	},
}

func TestDecode(t *testing.T) {
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			cborData, err := hex.DecodeString(test.CborHex)
			require.NoError(t, err)
			msg, err := NewMsgFromCbor(test.MessageType, cborData)
			require.NoError(t, err)
			expected := test.Message()
			expected.SetCbor(cborData)
			require.Equal(t, expected, msg)
		})
	}
}

func TestEncode(t *testing.T) {
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			cborData, err := cbor.Encode(test.Message())
			require.NoError(t, err)
			require.Equal(t, test.CborHex, hex.EncodeToString(cborData))
		})
	}
}

func testTxId(eraId uint16, txIdPrefix ...byte) TxId {
	var txId [32]byte
	copy(txId[:], txIdPrefix)
	return TxId{
		EraId: eraId,
		TxId:  txId,
	}
}

func testTxIdHex(txIdPrefix ...byte) string {
	txId := testTxId(0, txIdPrefix...).TxId
	return hex.EncodeToString(txId[:])
}
