package blockfetch

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	MESSAGE_TYPE_REQUEST_RANGE = 0
	MESSAGE_TYPE_CLIENT_DONE   = 1
	MESSAGE_TYPE_START_BATCH   = 2
	MESSAGE_TYPE_NO_BLOCKS     = 3
	MESSAGE_TYPE_BLOCK         = 4
	MESSAGE_TYPE_BATCH_DONE    = 5
)

type msgRequestRange struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
	Start       interface{} //point
	End         interface{} //point
}

func newMsgRequestRange(start interface{}, end interface{}) *msgRequestRange {
	m := &msgRequestRange{
		MessageType: MESSAGE_TYPE_REQUEST_RANGE,
		Start:       start,
		End:         end,
	}
	return m
}

type msgClientDone struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

func newMsgClientDone() *msgClientDone {
	m := &msgClientDone{
		MessageType: MESSAGE_TYPE_CLIENT_DONE,
	}
	return m
}

// TODO: uncomment these when adding support for sending them
/*
type msgStartBatch struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type msgNoBlocks struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}
*/

type msgBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_            struct{} `cbor:",toarray"`
	MessageType  uint8
	WrappedBlock []byte
}

// TODO: uncomment this when adding support for sending it
/*
type msgBatchDone struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}
*/

// TODO: use this above and expose it, or just remove it
/*
type point struct {
	Slot uint64
	Hash []byte
}
*/

type wrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint
	RawBlock cbor.RawMessage
}
