package localtxsubmission

import (
	"encoding/hex"
	"fmt"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"reflect"
	"strings"
	"testing"
)

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
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

// Valid CBOR that serves as a placeholder for real TX content in the tests
// [h'DEADBEEF']
var placeholderTx = hexDecode("8144DEADBEEF")

// Valid CBOR that serves as a placeholder for TX rejection errors
// [2, 4]
var placeholderRejectError = hexDecode("820204")

var tests = []testDefinition{
	{
		CborHex:     fmt.Sprintf("82008204d81846%x", placeholderTx),
		MessageType: MessageTypeSubmitTx,
		Message:     NewMsgSubmitTx(ledger.TX_TYPE_ALONZO, placeholderTx),
	},
	{
		CborHex:     "8101",
		Message:     NewMsgAcceptTx(),
		MessageType: MessageTypeAcceptTx,
	},
	{
		CborHex:     fmt.Sprintf("8202%x", placeholderRejectError),
		MessageType: MessageTypeRejectTx,
		Message:     NewMsgRejectTx(placeholderRejectError),
	},
	{
		CborHex:     "8103",
		Message:     NewMsgDone(),
		MessageType: MessageTypeDone,
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
