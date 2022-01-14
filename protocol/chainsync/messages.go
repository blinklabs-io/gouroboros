package chainsync

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	MESSAGE_TYPE_REQUEST_NEXT        = 0
	MESSAGE_TYPE_AWAIT_REPLY         = 1
	MESSAGE_TYPE_ROLL_FORWARD        = 2
	MESSAGE_TYPE_ROLL_BACKWARD       = 3
	MESSAGE_TYPE_FIND_INTERSECT      = 4
	MESSAGE_TYPE_INTERSECT_FOUND     = 5
	MESSAGE_TYPE_INTERSECT_NOT_FOUND = 6
	MESSAGE_TYPE_DONE                = 7
)

type msgRequestNext struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

func newMsgRequestNext() *msgRequestNext {
	r := &msgRequestNext{
		MessageType: MESSAGE_TYPE_REQUEST_NEXT,
	}
	return r
}

func newMsgFindIntersect(points []interface{}) *msgFindIntersect {
	m := &msgFindIntersect{
		MessageType: MESSAGE_TYPE_FIND_INTERSECT,
		Points:      points,
	}
	return m
}

type msgAwaitReply struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type msgRollForward struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	WrappedData []byte
	Tip         tip
}

type msgRollBackward struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Point       point
	Tip         tip
}

type msgFindIntersect struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Points      []interface{}
}

type msgIntersectFound struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Point       point
	Tip         tip
}

type msgIntersectNotFound struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Tip         tip
}

type msgDone struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type tip struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Point       point
	BlockNumber uint64
}

type point struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Slot uint64
	Hash []byte
}

type wrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint
	RawBlock cbor.RawMessage
}
