package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "local-tx-submission"
	PROTOCOL_ID   uint16 = 6
)

var (
	STATE_IDLE = protocol.NewState(1, "Idle")
	STATE_BUSY = protocol.NewState(2, "Busy")
	STATE_DONE = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_SUBMIT_TX,
				NewState: STATE_BUSY,
			},
		},
	},
	STATE_BUSY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACCEPT_TX,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_REJECT_TX,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type LocalTxSubmission struct {
	*protocol.Protocol
	callbackConfig *CallbackConfig
}

type CallbackConfig struct {
	SubmitTxFunc SubmitTxFunc
	AcceptTxFunc AcceptTxFunc
	RejectTxFunc RejectTxFunc
	DoneFunc     DoneFunc
}

// Callback function types
type SubmitTxFunc func(interface{}) error
type AcceptTxFunc func() error
type RejectTxFunc func(interface{}) error
type DoneFunc func() error

func New(options protocol.ProtocolOptions) *LocalTxSubmission {
	l := &LocalTxSubmission{}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               options.Muxer,
		ErrorChan:           options.ErrorChan,
		Mode:                options.Mode,
		Role:                options.Role,
		MessageHandlerFunc:  l.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	l.Protocol = protocol.New(protoConfig)
	return l
}

func (l *LocalTxSubmission) Start(callbackConfig *CallbackConfig) {
	l.callbackConfig = callbackConfig
	l.Protocol.Start()
}

func (l *LocalTxSubmission) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_SUBMIT_TX:
		err = l.handleSubmitTx(msg)
	case MESSAGE_TYPE_ACCEPT_TX:
		err = l.handleAcceptTx()
	case MESSAGE_TYPE_REJECT_TX:
		err = l.handleRejectTx(msg)
	case MESSAGE_TYPE_DONE:
		err = l.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (l *LocalTxSubmission) SubmitTx(eraId uint16, tx []byte) error {
	msg := NewMsgSubmitTx(eraId, tx)
	return l.SendMessage(msg)
}

func (l *LocalTxSubmission) Done(tx interface{}) error {
	msg := NewMsgDone()
	return l.SendMessage(msg)
}

func (l *LocalTxSubmission) handleSubmitTx(msgGeneric protocol.Message) error {
	if l.callbackConfig.SubmitTxFunc == nil {
		return fmt.Errorf("received local-tx-submission SubmitTx message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgSubmitTx)
	// Call the user callback function
	return l.callbackConfig.SubmitTxFunc(msg.Transaction)
}

func (l *LocalTxSubmission) handleAcceptTx() error {
	if l.callbackConfig.AcceptTxFunc == nil {
		return fmt.Errorf("received local-tx-submission AcceptTx message but no callback function is defined")
	}
	// Call the user callback function
	return l.callbackConfig.AcceptTxFunc()
}

func (l *LocalTxSubmission) handleRejectTx(msgGeneric protocol.Message) error {
	if l.callbackConfig.RejectTxFunc == nil {
		return fmt.Errorf("received local-tx-submission RejectTx message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgRejectTx)
	// Call the user callback function
	return l.callbackConfig.RejectTxFunc(msg.Reason)
}

func (l *LocalTxSubmission) handleDone() error {
	if l.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received local-tx-submission Done message but no callback function is defined")
	}
	// Call the user callback function
	return l.callbackConfig.DoneFunc()
}
