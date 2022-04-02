package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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

func NewMsgFromCborNtN(msgType uint, data []byte) (protocol.Message, error) {
	return NewMsgFromCbor(protocol.ProtocolModeNodeToNode, msgType, data)
}

func NewMsgFromCborNtC(msgType uint, data []byte) (protocol.Message, error) {
	return NewMsgFromCbor(protocol.ProtocolModeNodeToClient, msgType, data)
}

func NewMsgFromCbor(protoMode protocol.ProtocolMode, msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_REQUEST_NEXT:
		ret = &MsgRequestNext{}
	case MESSAGE_TYPE_AWAIT_REPLY:
		ret = &MsgAwaitReply{}
	case MESSAGE_TYPE_ROLL_FORWARD:
		if protoMode == protocol.ProtocolModeNodeToNode {
			ret = &MsgRollForwardNtN{}
		} else {
			ret = &MsgRollForwardNtC{}
		}
	case MESSAGE_TYPE_ROLL_BACKWARD:
		ret = &MsgRollBackward{}
	case MESSAGE_TYPE_FIND_INTERSECT:
		ret = &MsgFindIntersect{}
	case MESSAGE_TYPE_INTERSECT_FOUND:
		ret = &MsgIntersectFound{}
	case MESSAGE_TYPE_INTERSECT_NOT_FOUND:
		ret = &MsgIntersectNotFound{}
	case MESSAGE_TYPE_DONE:
		ret = &MsgDone{}
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

type MsgRequestNext struct {
	protocol.MessageBase
}

func NewMsgRequestNext() *MsgRequestNext {
	m := &MsgRequestNext{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_NEXT,
		},
	}
	return m
}

type MsgAwaitReply struct {
	protocol.MessageBase
}

func NewMsgAwaitReply() *MsgAwaitReply {
	m := &MsgAwaitReply{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_AWAIT_REPLY,
		},
	}
	return m
}

type MsgRollForwardNtC struct {
	protocol.MessageBase
	WrappedData []byte
	Tip         Tip
}

func NewMsgRollForwardNtC(wrappedData []byte, tip Tip) *MsgRollForwardNtC {
	m := &MsgRollForwardNtC{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ROLL_FORWARD,
		},
		WrappedData: wrappedData,
		Tip:         tip,
	}
	return m
}

type MsgRollForwardNtN struct {
	protocol.MessageBase
	WrappedHeader WrappedHeader
	Tip           Tip
}

func NewMsgRollForwardNtN(wrappedHeader WrappedHeader, tip Tip) *MsgRollForwardNtN {
	m := &MsgRollForwardNtN{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ROLL_FORWARD,
		},
		WrappedHeader: wrappedHeader,
		Tip:           tip,
	}
	return m
}

type MsgRollBackward struct {
	protocol.MessageBase
	Point Point
	Tip   Tip
}

func NewMsgRollBackward(point Point, tip Tip) *MsgRollBackward {
	m := &MsgRollBackward{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ROLL_BACKWARD,
		},
		Point: point,
		Tip:   tip,
	}
	return m
}

type MsgFindIntersect struct {
	protocol.MessageBase
	Points []interface{}
}

func NewMsgFindIntersect(points []interface{}) *MsgFindIntersect {
	m := &MsgFindIntersect{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_FIND_INTERSECT,
		},
		Points: points,
	}
	return m
}

type MsgIntersectFound struct {
	protocol.MessageBase
	Point Point
	Tip   Tip
}

func NewMsgIntersectFound(point Point, tip Tip) *MsgIntersectFound {
	m := &MsgIntersectFound{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_INTERSECT_FOUND,
		},
		Point: point,
		Tip:   tip,
	}
	return m
}

type MsgIntersectNotFound struct {
	protocol.MessageBase
	Tip Tip
}

func NewMsgIntersectNotFound(tip Tip) *MsgIntersectNotFound {
	m := &MsgIntersectNotFound{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_INTERSECT_NOT_FOUND,
		},
		Tip: tip,
	}
	return m
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_DONE,
		},
	}
	return m
}

type Tip struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Point       Point
	BlockNumber uint64
}

type Point struct {
	Slot uint64
	Hash []byte
}

// A "point" can sometimes be empty, but the CBOR library gets grumpy about this
// when doing automatic decoding from an array, so we have to handle this case specially
func (p *Point) UnmarshalCBOR(data []byte) error {
	var tmp []interface{}
	if err := cbor.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if len(tmp) > 0 {
		p.Slot = tmp[0].(uint64)
		p.Hash = tmp[1].([]byte)
	}
	return nil
}

type WrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint
	RawBlock cbor.RawMessage
}

type WrappedHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_       struct{} `cbor:",toarray"`
	Type    uint
	RawData cbor.RawMessage
}

type WrappedHeaderByron struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_       struct{} `cbor:",toarray"`
	Unknown struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_       struct{} `cbor:",toarray"`
		Type    uint
		Unknown uint64
	}
	RawHeader []byte
}
