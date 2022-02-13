package blockfetch

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_REQUEST_RANGE:
		ret = &msgRequestRange{}
	case MESSAGE_TYPE_CLIENT_DONE:
		ret = &msgClientDone{}
	case MESSAGE_TYPE_START_BATCH:
		ret = &msgStartBatch{}
	case MESSAGE_TYPE_NO_BLOCKS:
		ret = &msgNoBlocks{}
	case MESSAGE_TYPE_BLOCK:
		ret = &msgBlock{}
	case MESSAGE_TYPE_BATCH_DONE:
		ret = &msgBatchDone{}
	}
	if _, err := utils.CborDecode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type msgRequestRange struct {
	protocol.MessageBase
	Start interface{} //point
	End   interface{} //point
}

func newMsgRequestRange(start interface{}, end interface{}) *msgRequestRange {
	m := &msgRequestRange{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_RANGE,
		},
		Start: start,
		End:   end,
	}
	return m
}

type msgClientDone struct {
	protocol.MessageBase
}

func newMsgClientDone() *msgClientDone {
	m := &msgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_CLIENT_DONE,
		},
	}
	return m
}

type msgStartBatch struct {
	protocol.MessageBase
}

type msgNoBlocks struct {
	protocol.MessageBase
}

type msgBlock struct {
	protocol.MessageBase
	WrappedBlock []byte
}

type msgBatchDone struct {
	protocol.MessageBase
}

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
