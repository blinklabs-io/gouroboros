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

func (c *ChainSync) NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_REQUEST_NEXT:
		ret = &msgRequestNext{}
	case MESSAGE_TYPE_AWAIT_REPLY:
		ret = &msgAwaitReply{}
	case MESSAGE_TYPE_ROLL_FORWARD:
		if c.nodeToNode {
			ret = &msgRollForwardNtN{}
		} else {
			ret = &msgRollForwardNtC{}
		}
	case MESSAGE_TYPE_ROLL_BACKWARD:
		ret = &msgRollBackward{}
	case MESSAGE_TYPE_FIND_INTERSECT:
		ret = &msgFindIntersect{}
	case MESSAGE_TYPE_INTERSECT_FOUND:
		ret = &msgIntersectFound{}
	case MESSAGE_TYPE_INTERSECT_NOT_FOUND:
		ret = &msgIntersectNotFound{}
	case MESSAGE_TYPE_DONE:
		ret = &msgDone{}
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

type msgRequestNext struct {
	protocol.MessageBase
}

func newMsgRequestNext() *msgRequestNext {
	r := &msgRequestNext{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_NEXT,
		},
	}
	return r
}

type msgAwaitReply struct {
	protocol.MessageBase
}

type msgRollForwardNtC struct {
	protocol.MessageBase
	WrappedData []byte
	Tip         tip
}

type msgRollForwardNtN struct {
	protocol.MessageBase
	WrappedHeader wrappedHeader
	Tip           tip
}

type msgRollBackward struct {
	protocol.MessageBase
	Point point
	Tip   tip
}

type msgFindIntersect struct {
	protocol.MessageBase
	Points []interface{}
}

func newMsgFindIntersect(points []interface{}) *msgFindIntersect {
	m := &msgFindIntersect{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_FIND_INTERSECT,
		},
		Points: points,
	}
	return m
}

type msgIntersectFound struct {
	protocol.MessageBase
	Point point
	Tip   tip
}

type msgIntersectNotFound struct {
	protocol.MessageBase
	Tip tip
}

type msgDone struct {
	protocol.MessageBase
}

type tip struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Point       point
	BlockNumber uint64
}

type point struct {
	Slot uint64
	Hash []byte
}

// A "point" can sometimes be empty, but the CBOR library gets grumpy about this
// when doing automatic decoding from an array, so we have to handle this case specially
func (p *point) UnmarshalCBOR(data []byte) error {
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

type wrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Type     uint
	RawBlock cbor.RawMessage
}

type wrappedHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_       struct{} `cbor:",toarray"`
	Type    uint
	RawData cbor.RawMessage
}

type wrappedHeaderByron struct {
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
