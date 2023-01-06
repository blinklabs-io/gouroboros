package localstatequery

import (
	"encoding/hex"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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
		MessageType: MESSAGE_TYPE_ACQUIRE,
	},
	{
		CborHex:     "8101",
		Message:     NewMsgAcquired(),
		MessageType: MESSAGE_TYPE_ACQUIRED,
	},
	{
		CborHex:     "820201",
		Message:     NewMsgFailure(ACQUIRE_FAILURE_POINT_NOT_ON_CHAIN),
		MessageType: MESSAGE_TYPE_FAILURE,
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
		MessageType: MESSAGE_TYPE_QUERY,
	},
	{
		CborHex:     "820405",
		Message:     NewMsgResult([]byte{5}),
		MessageType: MESSAGE_TYPE_RESULT,
	},
	{
		CborHex:     "8105",
		Message:     NewMsgRelease(),
		MessageType: MESSAGE_TYPE_RELEASE,
	},
	{
		CborHex:     "820680",
		Message:     NewMsgReAcquire(common.Point{}),
		MessageType: MESSAGE_TYPE_REACQUIRE,
	},
	{
		CborHex:     "8107",
		Message:     NewMsgDone(),
		MessageType: MESSAGE_TYPE_DONE,
	},
	{
		CborHex:     "8108",
		Message:     NewMsgAcquireNoPoint(),
		MessageType: MESSAGE_TYPE_ACQUIRE_NO_POINT,
	},
	{
		CborHex:     "8109",
		Message:     NewMsgReAcquireNoPoint(),
		MessageType: MESSAGE_TYPE_REACQUIRE_NO_POINT,
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
