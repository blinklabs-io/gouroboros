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

package blockfetch

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
}

var tests = []testDefinition{
	{
		CborHex:     "830082187b480102030405060708821901c848090a0b0c0d0e0f10",
		Message:     NewMsgRequestRange(pcommon.NewPoint(123, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}), pcommon.NewPoint(456, []byte{0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})),
		MessageType: MessageTypeRequestRange,
	},
	{
		CborHex:     "8101",
		Message:     NewMsgClientDone(),
		MessageType: MessageTypeClientDone,
	},
	{
		CborHex:     "8102",
		Message:     NewMsgStartBatch(),
		MessageType: MessageTypeStartBatch,
	},
	{
		CborHex:     "8103",
		Message:     NewMsgNoBlocks(),
		MessageType: MessageTypeNoBlocks,
	},
	{
		CborHex: "8204d818458206820102",
		Message: func() *MsgBlock {
			wrappedBlock := WrappedBlock{
				Type:     6,
				RawBlock: cbor.RawMessage([]byte{0x82, 0x01, 0x02}),
			}
			wrappedBlockCbor, _ := cbor.Encode(&wrappedBlock)
			return NewMsgBlock(wrappedBlockCbor)
		}(),
		MessageType: MessageTypeBlock,
	},
	{
		CborHex:     "8105",
		Message:     NewMsgBatchDone(),
		MessageType: MessageTypeBatchDone,
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
