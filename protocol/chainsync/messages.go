package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/cbor"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
)

// Message types
const (
	MessageTypeRequestNext       = 0
	MessageTypeAwaitReply        = 1
	MessageTypeRollForward       = 2
	MessageTypeRollBackward      = 3
	MessageTypeFindIntersect     = 4
	MessageTypeIntersectFound    = 5
	MessageTypeIntersectNotFound = 6
	MessageTypeDone              = 7
)

// NewMsgFromCborNtN parses a NtC ChainSync message from CBOR
func NewMsgFromCborNtN(msgType uint, data []byte) (protocol.Message, error) {
	return NewMsgFromCbor(protocol.ProtocolModeNodeToNode, msgType, data)
}

// NewMsgFromCborNtC parses a NtC ChainSync message from CBOR
func NewMsgFromCborNtC(msgType uint, data []byte) (protocol.Message, error) {
	return NewMsgFromCbor(protocol.ProtocolModeNodeToClient, msgType, data)
}

// NewMsgFromCbor parses a ChainSync message from CBOR
func NewMsgFromCbor(protoMode protocol.ProtocolMode, msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeRequestNext:
		ret = &MsgRequestNext{}
	case MessageTypeAwaitReply:
		ret = &MsgAwaitReply{}
	case MessageTypeRollForward:
		if protoMode == protocol.ProtocolModeNodeToNode {
			ret = &MsgRollForwardNtN{}
		} else {
			ret = &MsgRollForwardNtC{}
		}
	case MessageTypeRollBackward:
		ret = &MsgRollBackward{}
	case MessageTypeFindIntersect:
		ret = &MsgFindIntersect{}
	case MessageTypeIntersectFound:
		ret = &MsgIntersectFound{}
	case MessageTypeIntersectNotFound:
		ret = &MsgIntersectNotFound{}
	case MessageTypeDone:
		ret = &MsgDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", protocolName, err)
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
			MessageType: MessageTypeRequestNext,
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
			MessageType: MessageTypeAwaitReply,
		},
	}
	return m
}

// MsgRollForwardNtC is the NtC version of the RollForward message
type MsgRollForwardNtC struct {
	protocol.MessageBase
	WrappedBlock cbor.Tag
	Tip          Tip
	blockType    uint
	blockCbor    []byte
}

// NewMsgRollForwardNtC returns a MsgRollForwardNtC with the provided parameters
func NewMsgRollForwardNtC(blockType uint, blockCbor []byte, tip Tip) *MsgRollForwardNtC {
	m := &MsgRollForwardNtC{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRollForward,
		},
		Tip:       tip,
		blockType: blockType,
		blockCbor: make([]byte, len(blockCbor)),
	}
	copy(m.blockCbor, blockCbor)
	wb := NewWrappedBlock(blockType, blockCbor)
	content, err := cbor.Encode(wb)
	// TODO: figure out better way to handle error
	if err != nil {
		return nil
	}
	m.WrappedBlock = cbor.Tag{Number: 24, Content: content}
	return m
}

func (m *MsgRollForwardNtC) UnmarshalCBOR(data []byte) error {
	if err := cbor.DecodeGeneric(data, m); err != nil {
		return err
	}
	var wb WrappedBlock
	if _, err := cbor.Decode(m.WrappedBlock.Content.([]byte), &wb); err != nil {
		return err
	}
	m.blockType = wb.BlockType
	m.blockCbor = wb.BlockCbor
	return nil
}

// BlockType returns the block type
func (m *MsgRollForwardNtC) BlockType() uint {
	return m.blockType
}

// BlockCbor returns the block CBOR
func (m *MsgRollForwardNtC) BlockCbor() []byte {
	return m.blockCbor
}

// MsgRollForwardNtN is the NtN version of the RollForward message
type MsgRollForwardNtN struct {
	protocol.MessageBase
	WrappedHeader WrappedHeader
	Tip           Tip
}

// NewMsgRollForwardNtN returns a MsgRollForwardNtN with the provided parameters
func NewMsgRollForwardNtN(era uint, byronType uint, blockCbor []byte, tip Tip) *MsgRollForwardNtN {
	m := &MsgRollForwardNtN{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRollForward,
		},
		Tip: tip,
	}
	wrappedHeader := NewWrappedHeader(era, byronType, blockCbor)
	m.WrappedHeader = *wrappedHeader
	return m
}

type MsgRollBackward struct {
	protocol.MessageBase
	Point common.Point
	Tip   Tip
}

func NewMsgRollBackward(point common.Point, tip Tip) *MsgRollBackward {
	m := &MsgRollBackward{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRollBackward,
		},
		Point: point,
		Tip:   tip,
	}
	return m
}

type MsgFindIntersect struct {
	protocol.MessageBase
	Points []common.Point
}

func NewMsgFindIntersect(points []common.Point) *MsgFindIntersect {
	m := &MsgFindIntersect{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeFindIntersect,
		},
		Points: points,
	}
	return m
}

type MsgIntersectFound struct {
	protocol.MessageBase
	Point common.Point
	Tip   Tip
}

func NewMsgIntersectFound(point common.Point, tip Tip) *MsgIntersectFound {
	m := &MsgIntersectFound{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeIntersectFound,
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
			MessageType: MessageTypeIntersectNotFound,
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
			MessageType: MessageTypeDone,
		},
	}
	return m
}

type Tip struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Point       common.Point
	BlockNumber uint64
}
