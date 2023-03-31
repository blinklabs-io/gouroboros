package blockfetch

import (
	"fmt"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
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
		ret = &MsgRequestRange{}
	case MESSAGE_TYPE_CLIENT_DONE:
		ret = &MsgClientDone{}
	case MESSAGE_TYPE_START_BATCH:
		ret = &MsgStartBatch{}
	case MESSAGE_TYPE_NO_BLOCKS:
		ret = &MsgNoBlocks{}
	case MESSAGE_TYPE_BLOCK:
		ret = &MsgBlock{}
	case MESSAGE_TYPE_BATCH_DONE:
		ret = &MsgBatchDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgRequestRange struct {
	protocol.MessageBase
	Start interface{} //point
	End   interface{} //point
}

func NewMsgRequestRange(start interface{}, end interface{}) *MsgRequestRange {
	m := &MsgRequestRange{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_RANGE,
		},
		Start: start,
		End:   end,
	}
	return m
}

type MsgClientDone struct {
	protocol.MessageBase
}

func NewMsgClientDone() *MsgClientDone {
	m := &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_CLIENT_DONE,
		},
	}
	return m
}

type MsgStartBatch struct {
	protocol.MessageBase
}

func NewMsgStartBatch() *MsgStartBatch {
	m := &MsgStartBatch{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_START_BATCH,
		},
	}
	return m
}

type MsgNoBlocks struct {
	protocol.MessageBase
}

func NewMsgNoBlocks() *MsgNoBlocks {
	m := &MsgNoBlocks{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_NO_BLOCKS,
		},
	}
	return m
}

type MsgBlock struct {
	protocol.MessageBase
	WrappedBlock []byte
}

func NewMsgBlock(wrappedBlock []byte) *MsgBlock {
	m := &MsgBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_BLOCK,
		},
		WrappedBlock: wrappedBlock,
	}
	return m
}

type MsgBatchDone struct {
	protocol.MessageBase
}

func NewMsgBatchDone() *MsgBatchDone {
	m := &MsgBatchDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_BATCH_DONE,
		},
	}
	return m
}

// TODO: use this above and expose it, or just remove it
/*
type point struct {
	Slot uint64
	Hash []byte
}
*/

type WrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint
	RawBlock cbor.RawMessage
}
