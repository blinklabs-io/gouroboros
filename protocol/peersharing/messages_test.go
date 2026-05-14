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

package peersharing

import (
	"encoding/hex"
	"net"
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
		CborHex:     "820007",
		MessageType: MessageTypeShareRequest,
		Message:     NewMsgShareRequest(7),
	},
	{
		CborHex:     "82018183001a04030201190bb9",
		MessageType: MessageTypeSharePeers,
		Message: NewMsgSharePeers(
			[]PeerAddress{
				{
					IP:   net.IP{1, 2, 3, 4},
					Port: 3001,
				},
			},
		),
	},
	{
		CborHex:     "8102",
		MessageType: MessageTypeDone,
		Message:     NewMsgDone(),
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

func TestPeerAddressEncodeDecodeIPv6(t *testing.T) {
	expectedHex := "86011ab80d012000001a01000000190bb9"
	expectedPeer := PeerAddress{
		IP:   net.ParseIP("2001:db8::1"),
		Port: 3001,
	}
	cborData, err := cbor.Encode(expectedPeer)
	if err != nil {
		t.Fatalf("failed to encode peer address to CBOR: %s", err)
	}
	if got := hex.EncodeToString(cborData); got != expectedHex {
		t.Fatalf(
			"peer address did not encode to expected CBOR\n  got:    %s\n  wanted: %s",
			got,
			expectedHex,
		)
	}

	var decoded PeerAddress
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		t.Fatalf("failed to decode peer address from CBOR: %s", err)
	}
	if !decoded.IP.Equal(expectedPeer.IP) || decoded.Port != expectedPeer.Port {
		t.Fatalf(
			"peer address did not decode to expected value\n  got:    %#v\n  wanted: %#v",
			decoded,
			expectedPeer,
		)
	}
}

// TestPeerAddressDecodeIPv6V11 verifies that the legacy 8-element peer-address
// shape used by handshake versions 11 and 12 (FlowInfo and ScopeId fields
// present, then ignored) round-trips through Decode.
func TestPeerAddressDecodeIPv6V11(t *testing.T) {
	// CBOR array of 8 elements: [1, addr1..addr4 (LE u32), flowInfo, scopeId, port].
	// Same address as the v13 test above so we can compare results.
	v11Hex := "88011ab80d012000001a010000000000190bb9"
	cborData, err := hex.DecodeString(v11Hex)
	if err != nil {
		t.Fatalf("failed to decode hex: %s", err)
	}
	var decoded PeerAddress
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		t.Fatalf("failed to decode v11 peer address: %s", err)
	}
	expected := net.ParseIP("2001:db8::1")
	if !decoded.IP.Equal(expected) {
		t.Fatalf("v11 peer address IP mismatch: got %s, wanted %s",
			decoded.IP, expected)
	}
	if decoded.Port != 3001 {
		t.Fatalf("v11 peer address port mismatch: got %d, wanted 3001",
			decoded.Port)
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
