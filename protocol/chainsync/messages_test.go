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

package chainsync

import (
	"encoding/hex"
	"fmt"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
	"os"
	"reflect"
	"strings"
	"testing"
)

type testDefinition struct {
	CborHex      string
	Message      protocol.Message
	MessageType  uint
	ProtocolMode protocol.ProtocolMode
}

// Helper function to allow inline hex decoding without capturing the error
func hexDecode(data string) []byte {
	// Strip off any leading/trailing whitespace in hex string
	data = strings.TrimSpace(data)
	decoded, err := hex.DecodeString(data)
	if err != nil {
		panic(fmt.Sprintf("error decoding hex: %s", err))
	}
	return decoded
}

// Helper function to allow inline reading of a file without capturing the error
func readFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("error reading file: %s", err))
	}
	return data
}

// Decode from CBOR and compare to object
func testDecode(test testDefinition, t *testing.T) {
	cborData, err := hex.DecodeString(test.CborHex)
	if err != nil {
		t.Fatalf("failed to decode CBOR hex: %s", err)
	}
	msg, err := NewMsgFromCbor(test.ProtocolMode, test.MessageType, cborData)
	if err != nil {
		t.Fatalf("failed to decode CBOR: %s", err)
	}
	// Set the raw CBOR so the comparison should succeed
	if test.Message != nil {
		test.Message.SetCbor(cborData)
	}
	if !reflect.DeepEqual(msg, test.Message) {
		t.Fatalf(
			"CBOR did not decode to expected message object\n  got: %#v\n  wanted: %#v",
			msg,
			test.Message,
		)
	}
}

// Encode object to CBOR and compare to expected CBOR
func testEncode(test testDefinition, t *testing.T) {
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

// Run the decode/encode tests for a set of test definitions
func runTests(tests []testDefinition, t *testing.T) {
	for _, test := range tests {
		// Strip off any leading/trailing whitespace in CBOR hex string
		test.CborHex = strings.TrimSpace(test.CborHex)
		testDecode(test, t)
		testEncode(test, t)
	}
}

func TestMsgRequestNext(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex:     "8100",
			Message:     NewMsgRequestNext(),
			MessageType: MessageTypeRequestNext,
		},
	}
	runTests(tests, t)
}

func TestMsgAwaitReply(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex:     "8101",
			Message:     NewMsgAwaitReply(),
			MessageType: MessageTypeAwaitReply,
		},
	}
	runTests(tests, t)
}

func TestMsgRollForwardNodeToNode(t *testing.T) {
	tests := []testDefinition{
		// Byron EBB (NtN)
		{
			CborHex: string(
				readFile(
					"testdata/rollforward_ntn_byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
				),
			),
			Message: NewMsgRollForwardNtN(
				ledger.BlockHeaderTypeByron,
				0,
				hexDecode(
					string(
						readFile(
							"testdata/byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
						),
					),
				),
				Tip{
					Point: common.Point{
						Slot: 55740899,
						Hash: hexDecode(
							"C89E652408EC269379751C8B2BF0137297BF9F5D0FB2E76E19ACF63D783C3A66",
						),
					},
					BlockNumber: 3479284,
				},
			),
			MessageType:  MessageTypeRollForward,
			ProtocolMode: protocol.ProtocolModeNodeToNode,
		},
		// TODO: fetch full block content and enable test
		/*
			// Byron main block (NtN)
			{
				CborHex: string(readFile("testdata/rollforward_ntn_byron_main_block_testnet_388a82f053603f3552717d61644a353188f2d5500f4c6354cc1ad27a36a7ea91.hex")),
				Message: NewMsgRollForwardNtN(
					ledger.BlockHeaderTypeByron,
					1,
					hexDecode(string(readFile("testdata/byron_main_block_testnet_xxxx.hex"))),
					Tip{
						Point: common.Point{
							Slot: 55740899,
							Hash: hexDecode("C89E652408EC269379751C8B2BF0137297BF9F5D0FB2E76E19ACF63D783C3A66"),
						},
						BlockNumber: 3479284,
					},
				),
				MessageType:  MessageTypeRollForward,
				ProtocolMode: protocol.ProtocolModeNodeToNode,
			},
		*/
		// Shelley block (NtN)
		{
			CborHex: string(
				readFile(
					"testdata/rollforward_ntn_shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
				),
			),
			Message: NewMsgRollForwardNtN(
				ledger.BlockHeaderTypeShelley,
				0,
				hexDecode(
					string(
						readFile(
							"testdata/shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
						),
					),
				),
				Tip{
					Point: common.Point{
						Slot: 55770176,
						Hash: hexDecode(
							"EA90218C8606AAD58B90C2AD51E37FC35ED6D4C40D8944DF0BC60D22F1E6DD65",
						),
					},
					BlockNumber: 3480174,
				},
			),
			MessageType:  MessageTypeRollForward,
			ProtocolMode: protocol.ProtocolModeNodeToNode,
		},
	}
	runTests(tests, t)
}

func TestMsgRollForwardNodeToClient(t *testing.T) {
	tests := []testDefinition{
		// Byron EBB (NtC)
		{
			CborHex: string(
				readFile(
					"testdata/rollforward_ntc_byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
				),
			),
			Message: NewMsgRollForwardNtC(
				0,
				hexDecode(
					string(
						readFile(
							"testdata/byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
						),
					),
				),
				Tip{
					Point: common.Point{
						Slot: 49055,
						Hash: hexDecode(
							"7C288E72BB8C10439308901F379C2821945ED58BD1058578E8376F959078B321",
						),
					},
					BlockNumber: 48025,
				},
			),
			MessageType:  MessageTypeRollForward,
			ProtocolMode: protocol.ProtocolModeNodeToClient,
		},
		// Byron main block (NtC)
		{
			CborHex: string(
				readFile(
					"testdata/rollforward_ntc_byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
				),
			),
			Message: NewMsgRollForwardNtC(
				1,
				hexDecode(
					string(
						readFile(
							"testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
						),
					),
				),
				Tip{
					Point: common.Point{
						Slot: 49055,
						Hash: hexDecode(
							"7C288E72BB8C10439308901F379C2821945ED58BD1058578E8376F959078B321",
						),
					},
					BlockNumber: 48025,
				},
			),
			MessageType:  MessageTypeRollForward,
			ProtocolMode: protocol.ProtocolModeNodeToClient,
		},
		// Shelley block (NtC)
		{
			CborHex: string(
				readFile(
					"testdata/rollforward_ntc_shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
				),
			),
			Message: NewMsgRollForwardNtC(
				2,
				hexDecode(
					string(
						readFile(
							"testdata/shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
						),
					),
				),
				Tip{
					Point: common.Point{
						Slot: 55829927,
						Hash: hexDecode(
							"2809888408DD6F499ECDC868E10F635FA550AF3EBC3B5165C9DACC023D1F52C5",
						),
					},
					BlockNumber: 3481987,
				},
			),
			MessageType:  MessageTypeRollForward,
			ProtocolMode: protocol.ProtocolModeNodeToClient,
		},
	}
	runTests(tests, t)
}

func TestMsgRollBackward(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex: "83038082821a03520ff458201979d7dd2c7211cb7ce393c83aceca09675ec7786741620676e16c3ad3ac81031a00351333",
			Message: NewMsgRollBackward(
				common.Point{},
				Tip{
					Point: common.Point{
						Slot: 55709684,
						Hash: hexDecode(
							"1979D7DD2C7211CB7CE393C83ACECA09675EC7786741620676E16C3AD3AC8103",
						),
					},
					BlockNumber: 3478323,
				},
			),
			MessageType: MessageTypeRollBackward,
		},
	}
	runTests(tests, t)
}

func TestMsgFindIntersect(t *testing.T) {
	tests := []testDefinition{
		// "origin"
		{
			CborHex: "82048180",
			Message: NewMsgFindIntersect(
				[]common.Point{
					common.Point{},
				},
			),
			MessageType: MessageTypeFindIntersect,
		},
		// Beginning of Shelley era
		{
			CborHex: "820481821a001863bf58207e16781b40ebf8b6da18f7b5e8ade855d6738095ef2f1c58c77e88b6e45997a4",
			Message: NewMsgFindIntersect(
				[]common.Point{
					common.Point{
						Slot: 1598399,
						Hash: hexDecode(
							"7E16781B40EBF8B6DA18F7B5E8ADE855D6738095EF2F1C58C77E88B6E45997A4",
						),
					},
				},
			),
			MessageType: MessageTypeFindIntersect,
		},
	}
	runTests(tests, t)
}

func TestMsgIntersectFound(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex: "83058082821a03520ff458201979d7dd2c7211cb7ce393c83aceca09675ec7786741620676e16c3ad3ac81031a00351333",
			Message: NewMsgIntersectFound(
				common.Point{},
				Tip{
					Point: common.Point{
						Slot: 55709684,
						Hash: hexDecode(
							"1979D7DD2C7211CB7CE393C83ACECA09675EC7786741620676E16C3AD3AC8103",
						),
					},
					BlockNumber: 3478323,
				},
			),
			MessageType: MessageTypeIntersectFound,
		},
	}
	runTests(tests, t)
}

func TestMsgIntersectNotFound(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex: "820682821a03520ff458201979d7dd2c7211cb7ce393c83aceca09675ec7786741620676e16c3ad3ac81031a00351333",
			Message: NewMsgIntersectNotFound(
				Tip{
					Point: common.Point{
						Slot: 55709684,
						Hash: hexDecode(
							"1979D7DD2C7211CB7CE393C83ACECA09675EC7786741620676E16C3AD3AC8103",
						),
					},
					BlockNumber: 3478323,
				},
			),
			MessageType: MessageTypeIntersectNotFound,
		},
	}
	runTests(tests, t)
}

func TestMsgDone(t *testing.T) {
	tests := []testDefinition{
		{
			CborHex:     "8107",
			Message:     NewMsgDone(),
			MessageType: MessageTypeDone,
		},
	}
	runTests(tests, t)
}
