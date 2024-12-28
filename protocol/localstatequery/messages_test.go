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

package localstatequery

import (
	"encoding/hex"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
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
		CborHex:     "820080",
		Message:     NewMsgAcquire(common.Point{}),
		MessageType: MessageTypeAcquire,
	},
	{
		CborHex:     "8101",
		Message:     NewMsgAcquired(),
		MessageType: MessageTypeAcquired,
	},
	{
		CborHex:     "820201",
		Message:     NewMsgFailure(AcquireFailurePointNotOnChain),
		MessageType: MessageTypeFailure,
	},
	{
		CborHex: "8203820082028101",
		Message: NewMsgQuery(
			// Current era hard-fork query
			[]interface{}{
				uint64(0),
				[]interface{}{
					uint64(2),
					[]interface{}{
						uint64(1),
					},
				},
			},
		),
		MessageType: MessageTypeQuery,
	},
	{
		CborHex:     "820405",
		Message:     NewMsgResult([]byte{5}),
		MessageType: MessageTypeResult,
	},
	{
		CborHex:     "8105",
		Message:     NewMsgRelease(),
		MessageType: MessageTypeRelease,
	},
	{
		CborHex:     "820680",
		Message:     NewMsgReAcquire(common.Point{}),
		MessageType: MessageTypeReacquire,
	},
	{
		CborHex:     "8107",
		Message:     NewMsgDone(),
		MessageType: MessageTypeDone,
	},
	{
		CborHex:     "8108",
		Message:     NewMsgAcquireVolatileTip(),
		MessageType: MessageTypeAcquireVolatileTip,
	},
	{
		CborHex:     "8109",
		Message:     NewMsgReAcquireVolatileTip(),
		MessageType: MessageTypeReacquireVolatileTip,
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
				"CBOR did not decode to expected message object\n  got:    %#v\n  wanted: %#v",
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
				"message did not encode to expected CBOR\n  got:    %s\n  wanted: %s",
				cborHex,
				test.CborHex,
			)
		}
	}
}
