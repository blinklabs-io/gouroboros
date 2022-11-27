package localstatequery

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"github.com/fxamacker/cbor/v2"
)

const (
	MESSAGE_TYPE_ACQUIRE            = 0
	MESSAGE_TYPE_ACQUIRED           = 1
	MESSAGE_TYPE_FAILURE            = 2
	MESSAGE_TYPE_QUERY              = 3
	MESSAGE_TYPE_RESULT             = 4
	MESSAGE_TYPE_RELEASE            = 5
	MESSAGE_TYPE_REACQUIRE          = 6
	MESSAGE_TYPE_DONE               = 7
	MESSAGE_TYPE_ACQUIRE_NO_POINT   = 8
	MESSAGE_TYPE_REACQUIRE_NO_POINT = 9

	ACQUIRE_FAILURE_POINT_TOO_OLD      = 0
	ACQUIRE_FAILURE_POINT_NOT_ON_CHAIN = 1
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_ACQUIRE:
		ret = &MsgAcquire{}
	case MESSAGE_TYPE_ACQUIRED:
		ret = &MsgAcquired{}
	case MESSAGE_TYPE_FAILURE:
		ret = &MsgFailure{}
	case MESSAGE_TYPE_QUERY:
		ret = &MsgQuery{}
	case MESSAGE_TYPE_RESULT:
		ret = &MsgResult{}
	case MESSAGE_TYPE_RELEASE:
		ret = &MsgRelease{}
	case MESSAGE_TYPE_REACQUIRE:
		ret = &MsgReAcquire{}
	case MESSAGE_TYPE_ACQUIRE_NO_POINT:
		ret = &MsgAcquireNoPoint{}
	case MESSAGE_TYPE_REACQUIRE_NO_POINT:
		ret = &MsgReAcquireNoPoint{}
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

type MsgAcquire struct {
	protocol.MessageBase
	Point common.Point
}

func NewMsgAcquire(point common.Point) *MsgAcquire {
	m := &MsgAcquire{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ACQUIRE,
		},
		Point: point,
	}
	return m
}

type MsgAcquireNoPoint struct {
	protocol.MessageBase
}

func NewMsgAcquireNoPoint() *MsgAcquireNoPoint {
	m := &MsgAcquireNoPoint{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ACQUIRE_NO_POINT,
		},
	}
	return m
}

type MsgAcquired struct {
	protocol.MessageBase
}

func NewMsgAcquired() *MsgAcquired {
	m := &MsgAcquired{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ACQUIRED,
		},
	}
	return m
}

type MsgFailure struct {
	protocol.MessageBase
	Failure uint8
}

func NewMsgFailure(failure uint8) *MsgFailure {
	m := &MsgFailure{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_FAILURE,
		},
		Failure: failure,
	}
	return m
}

type MsgQuery struct {
	protocol.MessageBase
	Query interface{}
}

func NewMsgQuery(query interface{}) *MsgQuery {
	m := &MsgQuery{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_QUERY,
		},
		Query: query,
	}
	return m
}

type MsgResult struct {
	protocol.MessageBase
	Result cbor.RawMessage
}

func NewMsgResult(resultCbor []byte) *MsgResult {
	m := &MsgResult{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_RESULT,
		},
		Result: cbor.RawMessage(resultCbor),
	}
	return m
}

type MsgRelease struct {
	protocol.MessageBase
}

func NewMsgRelease() *MsgRelease {
	m := &MsgRelease{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_RELEASE,
		},
	}
	return m
}

type MsgReAcquire struct {
	protocol.MessageBase
	Point common.Point
}

func NewMsgReAcquire(point common.Point) *MsgReAcquire {
	m := &MsgReAcquire{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REACQUIRE,
		},
		Point: point,
	}
	return m
}

type MsgReAcquireNoPoint struct {
	protocol.MessageBase
}

func NewMsgReAcquireNoPoint() *MsgReAcquireNoPoint {
	m := &MsgReAcquireNoPoint{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REACQUIRE_NO_POINT,
		},
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
