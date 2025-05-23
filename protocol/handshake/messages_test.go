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

package handshake

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
}

var tests = []testDefinition{
	{
		CborHex:     "8200a4078202f4088202f4098202f40a8202f4",
		MessageType: MessageTypeProposeVersions,
		Message: NewMsgProposeVersions(
			map[uint16]protocol.VersionData{
				7: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				8: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				9: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				10: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
			},
		),
	},
	{
		CborHex:     "83010a8202f4",
		MessageType: MessageTypeAcceptVersion,
		Message: NewMsgAcceptVersion(
			10,
			protocol.VersionDataNtN7to10{
				CborNetworkMagic:                       2,
				CborInitiatorAndResponderDiffusionMode: false,
			},
		),
	},
	{
		CborHex:     "82028200840708090a",
		MessageType: MessageTypeRefuse,
		Message: NewMsgRefuse(
			[]any{
				uint64(RefuseReasonVersionMismatch),
				[]any{
					uint64(7),
					uint64(8),
					uint64(9),
					uint64(10),
				},
			},
		),
	},
	// TODO: add more tests for other refusal types (#854)
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
