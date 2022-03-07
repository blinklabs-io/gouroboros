package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"github.com/fxamacker/cbor/v2"
)

const (
	MESSAGE_TYPE_SUBMIT_TX = 0
	MESSAGE_TYPE_ACCEPT_TX = 1
	MESSAGE_TYPE_REJECT_TX = 2
	MESSAGE_TYPE_DONE      = 3
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_SUBMIT_TX:
		ret = &MsgSubmitTx{}
	case MESSAGE_TYPE_ACCEPT_TX:
		ret = &MsgAcceptTx{}
	case MESSAGE_TYPE_REJECT_TX:
		ret = &MsgRejectTx{}
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

type MsgSubmitTx struct {
	protocol.MessageBase
	Transaction MsgSubmitTxTransaction
}

type MsgSubmitTxTransaction struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_     struct{} `cbor:",toarray"`
	EraId uint16
	Raw   cbor.Tag
}

func NewMsgSubmitTx(eraId uint16, tx []byte) *MsgSubmitTx {
	m := &MsgSubmitTx{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_SUBMIT_TX,
		},
		Transaction: MsgSubmitTxTransaction{
			EraId: eraId,
			Raw: cbor.Tag{
				// Wrapped CBOR
				Number:  24,
				Content: tx,
			},
		},
	}
	return m
}

type MsgAcceptTx struct {
	protocol.MessageBase
}

type MsgRejectTx struct {
	protocol.MessageBase
	Reason interface{}
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
