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
		ret = &msgSubmitTx{}
	case MESSAGE_TYPE_ACCEPT_TX:
		ret = &msgAcceptTx{}
	case MESSAGE_TYPE_REJECT_TX:
		ret = &msgRejectTx{}
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

type msgSubmitTx struct {
	protocol.MessageBase
	Transaction msgSubmitTxTransaction
}

type msgSubmitTxTransaction struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_     struct{} `cbor:",toarray"`
	EraId uint16
	Raw   cbor.Tag
}

func newMsgSubmitTx(eraId uint16, tx []byte) *msgSubmitTx {
	m := &msgSubmitTx{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_SUBMIT_TX,
		},
		Transaction: msgSubmitTxTransaction{
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

type msgAcceptTx struct {
	protocol.MessageBase
}

type msgRejectTx struct {
	protocol.MessageBase
	Reason interface{}
}

type msgDone struct {
	protocol.MessageBase
}

func newMsgDone() *msgDone {
	m := &msgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_DONE,
		},
	}
	return m
}
