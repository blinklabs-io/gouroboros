package localtxsubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeSubmitTx = 0
	MessageTypeAcceptTx = 1
	MessageTypeRejectTx = 2
	MessageTypeDone     = 3
)

// NewMsgFromCbor parses a LocalTxSubmission message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeSubmitTx:
		ret = &MsgSubmitTx{}
	case MessageTypeAcceptTx:
		ret = &MsgAcceptTx{}
	case MessageTypeRejectTx:
		ret = &MsgRejectTx{}
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
			MessageType: MessageTypeSubmitTx,
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

func NewMsgAcceptTx() *MsgAcceptTx {
	m := &MsgAcceptTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptTx,
		},
	}
	return m
}

type MsgRejectTx struct {
	protocol.MessageBase
	// We use RawMessage here because the failure reason can be numerous different
	// structures, and we'll need to do further processing
	Reason cbor.RawMessage
}

func NewMsgRejectTx(reasonCbor []byte) *MsgRejectTx {
	m := &MsgRejectTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRejectTx,
		},
		Reason: cbor.RawMessage(reasonCbor),
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
