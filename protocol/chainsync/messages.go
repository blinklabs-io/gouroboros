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
	WrappedBlock cbor.Tag
	Tip          Tip
	blockType    uint
	blockCbor    []byte
}

func NewMsgRollForwardNtC(blockType uint, blockCbor []byte, tip Tip) *MsgRollForwardNtC {
	m := &MsgRollForwardNtC{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ROLL_FORWARD,
		},
		Tip: tip,
	}
	wb := NewWrappedBlock(blockType, blockCbor)
	content, err := cbor.Marshal(wb)
	// TODO: figure out better way to handle error
	if err != nil {
		return nil
	}
	m.WrappedBlock = cbor.Tag{Number: 24, Content: content}
	return m
}

func (m *MsgRollForwardNtC) BlockType() uint {
	return m.blockType
}

func (m *MsgRollForwardNtC) BlockCbor() []byte {
	return m.blockCbor
}

type MsgRollForwardNtN struct {
	protocol.MessageBase
	WrappedHeader WrappedHeader
	Tip           Tip
}

func NewMsgRollForwardNtN(era uint, byronType uint, blockCbor []byte, tip Tip) *MsgRollForwardNtN {
	m := &MsgRollForwardNtN{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ROLL_FORWARD,
		},
		Tip: tip,
	}
	wrappedHeader := NewWrappedHeader(era, byronType, blockCbor)
	m.WrappedHeader = *wrappedHeader
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
	Points []Point
}

func NewMsgFindIntersect(points []Point) *MsgFindIntersect {
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
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Slot uint64
	Hash []byte
}

func NewPoint(slot uint64, blockHash []byte) Point {
	return Point{
		Slot: slot,
		Hash: blockHash,
	}
}

func NewPointOrigin() Point {
	return Point{}
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

func (p *Point) MarshalCBOR() ([]byte, error) {
	var data []interface{}
	if p.Slot == 0 && p.Hash == nil {
		// Return an empty list if values are zero
		data = make([]interface{}, 0)
	} else {
		data = []interface{}{p.Slot, p.Hash}
	}
	return utils.CborEncode(data)
}
