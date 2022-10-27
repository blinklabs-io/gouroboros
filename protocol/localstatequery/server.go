package localstatequery

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Server struct {
	*protocol.Protocol
	config                        *Config
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	// Enable version-dependent features
	if protoOptions.Version >= 10 {
		s.enableGetChainBlockNo = true
		s.enableGetChainPoint = true
	}
	if protoOptions.Version >= 11 {
		s.enableGetRewardInfoPoolsBlock = true
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACQUIRE:
		err = s.handleAcquire(msg)
	case MESSAGE_TYPE_QUERY:
		err = s.handleQuery(msg)
	case MESSAGE_TYPE_RELEASE:
		err = s.handleRelease()
	case MESSAGE_TYPE_REACQUIRE:
		err = s.handleReAcquire(msg)
	case MESSAGE_TYPE_ACQUIRE_NO_POINT:
		err = s.handleAcquire(msg)
	case MESSAGE_TYPE_REACQUIRE_NO_POINT:
		err = s.handleReAcquire(msg)
	case MESSAGE_TYPE_DONE:
		err = s.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (s *Server) handleAcquire(msg protocol.Message) error {
	if s.config.AcquireFunc == nil {
		return fmt.Errorf("received local-state-query Acquire message but no callback function is defined")
	}
	switch msgAcquire := msg.(type) {
	case *MsgAcquire:
		// Call the user callback function
		return s.config.AcquireFunc(msgAcquire.Point)
	case *MsgAcquireNoPoint:
		// Call the user callback function
		return s.config.AcquireFunc(nil)
	}
	return nil
}

func (s *Server) handleQuery(msg protocol.Message) error {
	if s.config.QueryFunc == nil {
		return fmt.Errorf("received local-state-query Query message but no callback function is defined")
	}
	msgQuery := msg.(*MsgQuery)
	// Call the user callback function
	return s.config.QueryFunc(msgQuery.Query)
}

func (s *Server) handleRelease() error {
	if s.config.ReleaseFunc == nil {
		return fmt.Errorf("received local-state-query Release message but no callback function is defined")
	}
	// Call the user callback function
	return s.config.ReleaseFunc()
}

func (s *Server) handleReAcquire(msg protocol.Message) error {
	if s.config.ReAcquireFunc == nil {
		return fmt.Errorf("received local-state-query ReAcquire message but no callback function is defined")
	}
	switch msgReAcquire := msg.(type) {
	case *MsgReAcquire:
		// Call the user callback function
		return s.config.ReAcquireFunc(msgReAcquire.Point)
	case *MsgReAcquireNoPoint:
		// Call the user callback function
		return s.config.ReAcquireFunc(nil)
	}
	return nil
}

func (s *Server) handleDone() error {
	if s.config.DoneFunc == nil {
		return fmt.Errorf("received local-state-query Done message but no callback function is defined")
	}
	// Call the user callback function
	return s.config.DoneFunc()
}
