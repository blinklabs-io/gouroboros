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
	"fmt"
	"math/big"
	"os"
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
	Result      any
	Optional    bool
}

var tests = []testDefinition{
	{
		CborHex:     "820080",
		Message:     NewMsgAcquire(pcommon.Point{}),
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
			&BlockQuery{
				Query: &HardForkQuery{
					Query: &HardForkCurrentEraQuery{
						simpleQueryBase{
							Type: QueryTypeHardForkCurrentEra,
						},
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
		Message:     NewMsgReAcquire(pcommon.Point{}),
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
	{
		CborHex: string(
			readFileString(
				"../../internal/test/cardano-blueprint/src/client/node-to-client/state-query/examples/getSystemStart/query.cbor",
			),
		),
		Message: NewMsgQuery(
			&SystemStartQuery{simpleQueryBase{Type: QueryTypeSystemStart}},
		),
		MessageType: MessageTypeQuery,
		Optional:    true,
	},
	{
		CborHex: string(
			readFileString(
				"../../internal/test/cardano-blueprint/src/client/node-to-client/state-query/examples/getSystemStart/result.cbor",
			),
		),
		Message: NewMsgResult(unsafeCbor(
			SystemStartResult{
				Year: unsafeBigInt(
					[]byte(
						"703941703872597091335551638723343370661404331303175992839224705786473148",
					),
				),
				Day: -4205646576720553090,
				Picoseconds: unsafeBigInt(
					[]byte(
						"-554918151390414980540174869115975093799476848534297657333456993160799627",
					),
				),
			})),
		MessageType: MessageTypeResult,
		Result: SystemStartResult{
			Year: unsafeBigInt(
				[]byte(
					"703941703872597091335551638723343370661404331303175992839224705786473148",
				),
			),
			Day: -4205646576720553090,
			Picoseconds: unsafeBigInt(
				[]byte(
					"-554918151390414980540174869115975093799476848534297657333456993160799627",
				),
			),
		},
		Optional: true,
	},
}

func TestDecode(t *testing.T) {
	for _, test := range tests {
		if test.Optional && test.CborHex == "" {
			// Optional tests rely on external fixtures (e.g. cardano-blueprint) which may not
			// be present in all environments.
			continue
		}
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		msg, err := NewMsgFromCbor(test.MessageType, cborData)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		// cast msg to MsgResult and further try to decode cbor
		if m, ok := msg.(*MsgResult); ok && test.Result != nil {
			var decoded = reflect.New(reflect.TypeOf(test.Result))
			_, err := cbor.Decode(m.Result, decoded.Interface())
			if err != nil {
				t.Fatalf("failed to decode result: %s", err)
			}
			var actual = reflect.Indirect(decoded).Interface()
			if !reflect.DeepEqual(actual, test.Result) {
				t.Fatalf(
					"MsgResult content did not decode to expected Result object\n  got:    %#v\n  wanted: %#v",
					actual,
					test.Result,
				)
			}
		}

		// Set the raw CBOR so the comparison should succeed
		test.Message.SetCbor(cborData)
		if m, ok := msg.(*MsgQuery); ok {
			m.Query.SetCbor(nil)
		}
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
		if test.Optional && test.CborHex == "" {
			continue
		}
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

// Helper function to encode to cbor or panic
func unsafeCbor(data any) []byte {
	cborData, err := cbor.Encode(data)
	if err != nil {
		panic(fmt.Sprintf("error encoding to CBOR: %s", err))
	}
	return cborData
}

func unsafeBigInt(text []byte) big.Int {
	var i big.Int
	err := i.UnmarshalText(text)
	if err != nil {
		panic(fmt.Sprintf("error unmarshalling text to big.Int: %s", err))
	}
	return i
}

// Helper function to allow inline reading of a file without capturing the error
func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
