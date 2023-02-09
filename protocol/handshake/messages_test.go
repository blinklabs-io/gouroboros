package handshake

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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
			map[uint16]interface{}{
				7:  []interface{}{uint64(2), false},
				8:  []interface{}{uint64(2), false},
				9:  []interface{}{uint64(2), false},
				10: []interface{}{uint64(2), false},
			},
		),
	},
	{
		CborHex:     "83010a8202f4",
		MessageType: MessageTypeAcceptVersion,
		Message:     NewMsgAcceptVersion(10, []interface{}{uint64(2), false}),
	},
	{
		CborHex:     "82028200840708090a",
		MessageType: MessageTypeRefuse,
		Message: NewMsgRefuse(
			[]interface{}{
				uint64(RefuseReasonVersionMismatch),
				[]interface{}{
					uint64(7),
					uint64(8),
					uint64(9),
					uint64(10),
				},
			},
		),
	},
	// TODO: add more tests for other refusal types
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
			t.Fatalf("CBOR did not decode to expected message object\n  got:    %#v\n  wanted: %#v", msg, test.Message)
		}
	}
}

func TestEncode(t *testing.T) {
	for _, test := range tests {
		cborData, err := utils.CborEncode(test.Message)
		if err != nil {
			t.Fatalf("failed to encode message to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf("message did not encode to expected CBOR\n  got:    %s\n  wanted: %s", cborHex, test.CborHex)
		}
	}
}
