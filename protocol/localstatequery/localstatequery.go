package localstatequery

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "local-state-query"
	PROTOCOL_ID   uint16 = 7
)

var (
	STATE_IDLE      = protocol.NewState(1, "Idle")
	STATE_ACQUIRING = protocol.NewState(2, "Acquiring")
	STATE_ACQUIRED  = protocol.NewState(3, "Acquired")
	STATE_QUERYING  = protocol.NewState(4, "Querying")
	STATE_DONE      = protocol.NewState(5, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACQUIRE,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_ACQUIRE_NO_POINT,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_ACQUIRING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_FAILURE,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ACQUIRED,
				NewState: STATE_ACQUIRED,
			},
		},
	},
	STATE_ACQUIRED: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_QUERY,
				NewState: STATE_QUERYING,
			},
			{
				MsgType:  MESSAGE_TYPE_REACQUIRE,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_REACQUIRE_NO_POINT,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_RELEASE,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_QUERYING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_RESULT,
				NewState: STATE_ACQUIRED,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type LocalStateQuery struct {
	*protocol.Protocol
	callbackConfig                *CallbackConfig
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
}

type CallbackConfig struct {
	AcquireFunc   AcquireFunc
	AcquiredFunc  AcquiredFunc
	FailureFunc   FailureFunc
	QueryFunc     QueryFunc
	ResultFunc    ResultFunc
	ReleaseFunc   ReleaseFunc
	ReAcquireFunc ReAcquireFunc
	DoneFunc      DoneFunc
}

// Callback function types
// TODO: update callbacks
type AcquireFunc func(interface{}) error
type AcquiredFunc func() error
type FailureFunc func(interface{}) error
type QueryFunc func(interface{}) error
type ResultFunc func(interface{}) error
type ReleaseFunc func() error
type ReAcquireFunc func(interface{}) error
type DoneFunc func() error

func New(options protocol.ProtocolOptions, callbackConfig *CallbackConfig) *LocalStateQuery {
	l := &LocalStateQuery{
		callbackConfig: callbackConfig,
	}
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
	// Enable version-dependent features
	if options.Version >= 10 {
		l.enableGetChainBlockNo = true
		l.enableGetChainPoint = true
	}
	if options.Version >= 11 {
		l.enableGetRewardInfoPoolsBlock = true
	}
	l.Protocol = protocol.New(protoConfig)
	return l
}

func (l *LocalStateQuery) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACQUIRE:
		err = l.handleAcquire(msg)
	case MESSAGE_TYPE_ACQUIRED:
		err = l.handleAcquired()
	case MESSAGE_TYPE_FAILURE:
		err = l.handleFailure(msg)
	case MESSAGE_TYPE_QUERY:
		err = l.handleQuery(msg)
	case MESSAGE_TYPE_RESULT:
		err = l.handleResult(msg)
	case MESSAGE_TYPE_RELEASE:
		err = l.handleRelease()
	case MESSAGE_TYPE_REACQUIRE:
		err = l.handleReAcquire(msg)
	case MESSAGE_TYPE_ACQUIRE_NO_POINT:
		err = l.handleAcquire(msg)
	case MESSAGE_TYPE_REACQUIRE_NO_POINT:
		err = l.handleReAcquire(msg)
	case MESSAGE_TYPE_DONE:
		err = l.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (l *LocalStateQuery) handleAcquire(msg protocol.Message) error {
	if l.callbackConfig.AcquireFunc == nil {
		return fmt.Errorf("received local-state-query Acquire message but no callback function is defined")
	}
	switch msgAcquire := msg.(type) {
	case *MsgAcquire:
		// Call the user callback function
		return l.callbackConfig.AcquireFunc(msgAcquire.Point)
	case *MsgAcquireNoPoint:
		// Call the user callback function
		return l.callbackConfig.AcquireFunc(nil)
	}
	return nil
}

func (l *LocalStateQuery) handleAcquired() error {
	if l.callbackConfig.AcquiredFunc == nil {
		return fmt.Errorf("received local-state-query Acquired message but no callback function is defined")
	}
	// Call the user callback function
	return l.callbackConfig.AcquiredFunc()
}

func (l *LocalStateQuery) handleFailure(msg protocol.Message) error {
	if l.callbackConfig.FailureFunc == nil {
		return fmt.Errorf("received local-state-query Failure message but no callback function is defined")
	}
	msgFailure := msg.(*MsgFailure)
	// Call the user callback function
	return l.callbackConfig.FailureFunc(msgFailure.Failure)
}

func (l *LocalStateQuery) handleQuery(msg protocol.Message) error {
	if l.callbackConfig.QueryFunc == nil {
		return fmt.Errorf("received local-state-query Query message but no callback function is defined")
	}
	msgQuery := msg.(*MsgQuery)
	// Call the user callback function
	return l.callbackConfig.QueryFunc(msgQuery.Query)
}

func (l *LocalStateQuery) handleResult(msg protocol.Message) error {
	if l.callbackConfig.ResultFunc == nil {
		return fmt.Errorf("received local-state-query Result message but no callback function is defined")
	}
	msgResult := msg.(*MsgResult)
	// Call the user callback function
	return l.callbackConfig.ResultFunc(msgResult.Result)
}

func (l *LocalStateQuery) handleRelease() error {
	if l.callbackConfig.ReleaseFunc == nil {
		return fmt.Errorf("received local-state-query Release message but no callback function is defined")
	}
	// Call the user callback function
	return l.callbackConfig.ReleaseFunc()
}

func (l *LocalStateQuery) handleReAcquire(msg protocol.Message) error {
	if l.callbackConfig.ReAcquireFunc == nil {
		return fmt.Errorf("received local-state-query ReAcquire message but no callback function is defined")
	}
	switch msgReAcquire := msg.(type) {
	case *MsgReAcquire:
		// Call the user callback function
		return l.callbackConfig.ReAcquireFunc(msgReAcquire.Point)
	case *MsgReAcquireNoPoint:
		// Call the user callback function
		return l.callbackConfig.ReAcquireFunc(nil)
	}
	return nil
}

func (l *LocalStateQuery) handleDone() error {
	if l.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received local-state-query Done message but no callback function is defined")
	}
	// Call the user callback function
	return l.callbackConfig.DoneFunc()
}
