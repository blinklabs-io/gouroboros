package blockfetch

import (
	"encoding/hex"
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

// TODO: implement tests for more messages
var tests = []testDefinition{
	{
		CborHex:     "8105",
		Message:     NewMsgBatchDone(),
		MessageType: MESSAGE_TYPE_BATCH_DONE,
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
