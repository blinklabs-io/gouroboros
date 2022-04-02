package txsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	MESSAGE_TYPE_REQUEST_TX_IDS = 0
	MESSAGE_TYPE_REPLY_TX_IDS   = 1
	MESSAGE_TYPE_REQUEST_TXS    = 2
	MESSAGE_TYPE_REPLY_TXS      = 3
	MESSAGE_TYPE_DONE           = 4
	MESSAGE_TYPE_HELLO          = 6
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_REQUEST_TX_IDS:
		ret = &MsgRequestTxIds{}
	case MESSAGE_TYPE_REPLY_TX_IDS:
		ret = &MsgReplyTxIds{}
	case MESSAGE_TYPE_REQUEST_TXS:
		ret = &MsgRequestTxs{}
	case MESSAGE_TYPE_REPLY_TXS:
		ret = &MsgReplyTxs{}
	case MESSAGE_TYPE_DONE:
		ret = &MsgDone{}
	case MESSAGE_TYPE_HELLO:
		ret = &MsgHello{}
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

type MsgRequestTxIds struct {
	protocol.MessageBase
	Blocking bool
	Ack      uint16
	Req      uint16
}

func NewMsgRequestTxIds(blocking bool, ack uint16, req uint16) *MsgRequestTxIds {
	m := &MsgRequestTxIds{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_TX_IDS,
		},
		Blocking: blocking,
		Ack:      ack,
		Req:      req,
	}
	return m
}

type MsgReplyTxIds struct {
	protocol.MessageBase
	TxIds []TxIdAndSize
}

func NewMsgReplyTxIds(txIds []TxIdAndSize) *MsgReplyTxIds {
	m := &MsgReplyTxIds{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REPLY_TX_IDS,
		},
		TxIds: txIds,
	}
	return m
}

type MsgRequestTxs struct {
	protocol.MessageBase
	TxIds []TxId
}

func NewMsgRequestTxs(txIds []TxId) *MsgRequestTxs {
	m := &MsgRequestTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REQUEST_TXS,
		},
		TxIds: txIds,
	}
	return m
}

type MsgReplyTxs struct {
	protocol.MessageBase
	Txs []TxBody
}

func NewMsgReplyTxs(txs []TxBody) *MsgReplyTxs {
	m := &MsgReplyTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_REPLY_TXS,
		},
		Txs: txs,
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

type MsgHello struct {
	protocol.MessageBase
}

func NewMsgHello() *MsgHello {
	m := &MsgHello{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_HELLO,
		},
	}
	return m
}

type TxId struct {
	EraId uint16
	TxId  [32]byte
}

type TxBody struct {
	EraId  uint16
	TxBody []byte
}

type TxIdAndSize struct {
	TxId TxId
	Size uint32
}
