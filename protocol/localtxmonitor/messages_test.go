// Copyright 2023 Blink Labs, LLC.
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

package localtxmonitor

import (
	"encoding/hex"
	"fmt"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"reflect"
	"testing"
)

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
}

var tests = []testDefinition{
	{
		CborHex:     "8100",
		MessageType: MessageTypeDone,
		Message:     NewMsgDone(),
	},
	{
		CborHex:     "8101",
		MessageType: MessageTypeAcquire,
		Message:     NewMsgAcquire(),
	},
	{
		CborHex:     "82021904d2",
		MessageType: MessageTypeAcquired,
		Message:     NewMsgAcquired(1234),
	},
	{
		CborHex:     "8103",
		Message:     NewMsgRelease(),
		MessageType: MessageTypeRelease,
	},
	{
		CborHex:     "8105",
		MessageType: MessageTypeNextTx,
		Message:     NewMsgNextTx(),
	},
	{
		CborHex:     "8106",
		MessageType: MessageTypeReplyNextTx,
		Message:     NewMsgReplyNextTx(0, nil),
	},
	{
		CborHex:     fmt.Sprintf("82068205d81844%x", 0xDEADBEEF),
		MessageType: MessageTypeReplyNextTx,
		Message:     NewMsgReplyNextTx(5, []byte{0xDE, 0xAD, 0xBE, 0xEF}),
	},
	{
		CborHex:     fmt.Sprintf("820744%x", 0xDEADBEEF),
		MessageType: MessageTypeHasTx,
		Message:     NewMsgHasTx([]byte{0xDE, 0xAD, 0xBE, 0xEF}),
	},
	{
		CborHex:     "8208f5",
		MessageType: MessageTypeReplyHasTx,
		Message:     NewMsgReplyHasTx(true),
	},
	{
		CborHex:     "8109",
		MessageType: MessageTypeGetSizes,
		Message:     NewMsgGetSizes(),
	},
	{
		// [10, [1234, 2345, 3456]]
		CborHex:     "820a831904d2190929190d80",
		MessageType: MessageTypeReplyGetSizes,
		Message:     NewMsgReplyGetSizes(1234, 2345, 3456),
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
			t.Fatalf("CBOR did not decode to expected message object\n  got: %#v\n  wanted: %#v", msg, test.Message)
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
			t.Fatalf("message did not encode to expected CBOR\n  got: %s\n  wanted: %s", cborHex, test.CborHex)
		}
	}
}
