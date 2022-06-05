package txsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "tx-submission"
	PROTOCOL_ID   uint16 = 4
)

var (
	STATE_HELLO              = protocol.NewState(1, "Hello")
	STATE_IDLE               = protocol.NewState(2, "Idle")
	STATE_TX_IDS_BLOCKING    = protocol.NewState(3, "TxIdsBlocking")
	STATE_TX_IDS_NONBLOCKING = protocol.NewState(4, "TxIdsNonBlocking")
	STATE_TXS                = protocol.NewState(5, "Txs")
	STATE_DONE               = protocol.NewState(6, "Done")
)

var StateMap = protocol.StateMap{
	STATE_HELLO: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_HELLO,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TX_IDS,
				NewState: STATE_TX_IDS_BLOCKING,
				// Match if blocking
				MatchFunc: func(msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TX_IDS,
				NewState: STATE_TX_IDS_NONBLOCKING,
				// Metch if non-blocking
				MatchFunc: func(msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return !msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TXS,
				NewState: STATE_TXS,
			},
		},
	},
	STATE_TX_IDS_BLOCKING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TX_IDS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_TX_IDS_NONBLOCKING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TX_IDS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_TXS: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TXS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type TxSubmission struct {
	*protocol.Protocol
	callbackConfig *CallbackConfig
}

type CallbackConfig struct {
	RequestTxIdsFunc RequestTxIdsFunc
	ReplyTxIdsFunc   ReplyTxIdsFunc
	RequestTxsFunc   RequestTxsFunc
	ReplyTxsFunc     ReplyTxsFunc
	DoneFunc         DoneFunc
	HelloFunc        HelloFunc
}

// Callback function types
type RequestTxIdsFunc func(bool, uint16, uint16) error
type ReplyTxIdsFunc func(interface{}) error
type RequestTxsFunc func(interface{}) error
type ReplyTxsFunc func(interface{}) error
type DoneFunc func() error
type HelloFunc func() error

func New(options protocol.ProtocolOptions) *TxSubmission {
	t := &TxSubmission{}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               options.Muxer,
		ErrorChan:           options.ErrorChan,
		Mode:                options.Mode,
		Role:                options.Role,
		MessageHandlerFunc:  t.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_HELLO,
	}
	t.Protocol = protocol.New(protoConfig)
	return t
}

func (t *TxSubmission) Start(callbackConfig *CallbackConfig) {
	t.callbackConfig = callbackConfig
	t.Protocol.Start()
}

func (t *TxSubmission) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_REQUEST_TX_IDS:
		err = t.handleRequestTxIds(msg)
	case MESSAGE_TYPE_REPLY_TX_IDS:
		err = t.handleReplyTxIds(msg)
	case MESSAGE_TYPE_REQUEST_TXS:
		err = t.handleRequestTxs(msg)
	case MESSAGE_TYPE_REPLY_TXS:
		err = t.handleReplyTxs(msg)
	case MESSAGE_TYPE_DONE:
		err = t.handleDone()
	case MESSAGE_TYPE_HELLO:
		err = t.handleHello()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (t *TxSubmission) handleRequestTxIds(msg protocol.Message) error {
	if t.callbackConfig.RequestTxIdsFunc == nil {
		return fmt.Errorf("received tx-submission RequestTxIds message but no callback function is defined")
	}
	msgRequestTxIds := msg.(*MsgRequestTxIds)
	// Call the user callback function
	return t.callbackConfig.RequestTxIdsFunc(msgRequestTxIds.Blocking, msgRequestTxIds.Ack, msgRequestTxIds.Req)
}

func (t *TxSubmission) handleReplyTxIds(msg protocol.Message) error {
	if t.callbackConfig.ReplyTxIdsFunc == nil {
		return fmt.Errorf("received tx-submission ReplyTxIds message but no callback function is defined")
	}
	msgReplyTxIds := msg.(*MsgReplyTxIds)
	// Call the user callback function
	return t.callbackConfig.ReplyTxIdsFunc(msgReplyTxIds.TxIds)
}

func (t *TxSubmission) handleRequestTxs(msg protocol.Message) error {
	if t.callbackConfig.RequestTxsFunc == nil {
		return fmt.Errorf("received tx-submission RequestTxs message but no callback function is defined")
	}
	msgRequestTxs := msg.(*MsgRequestTxs)
	// Call the user callback function
	return t.callbackConfig.RequestTxsFunc(msgRequestTxs.TxIds)
}

func (t *TxSubmission) handleReplyTxs(msg protocol.Message) error {
	if t.callbackConfig.ReplyTxsFunc == nil {
		return fmt.Errorf("received tx-submission ReplyTxs message but no callback function is defined")
	}
	msgReplyTxs := msg.(*MsgReplyTxs)
	// Call the user callback function
	return t.callbackConfig.ReplyTxsFunc(msgReplyTxs.Txs)
}

func (t *TxSubmission) handleDone() error {
	if t.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received tx-submission Done message but no callback function is defined")
	}
	// Call the user callback function
	return t.callbackConfig.DoneFunc()
}

func (t *TxSubmission) handleHello() error {
	if t.callbackConfig.HelloFunc == nil {
		return fmt.Errorf("received tx-submission Hello message but no callback function is defined")
	}
	// Call the user callback function
	return t.callbackConfig.HelloFunc()
}
