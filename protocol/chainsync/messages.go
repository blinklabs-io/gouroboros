package chainsync

import (
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
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
	Tip         interface{}
}

type msgRollBackward struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Point       common.Point
	Tip         []interface{}
}

type msgFindIntersect struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	// points
}

type msgIntersectFound struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	// point
	// tip
}

type msgIntersectNotFound struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	// tip
}

type msgDone struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type Tip struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_       struct{} `cbor:",toarray"`
	Point   common.Point
	Unknown uint64
}

// tip = [ point, uint ]

type wrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint8
	RawBlock cbor.RawMessage
}
