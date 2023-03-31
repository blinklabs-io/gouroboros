package chainsync

import (
	"fmt"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the ChainSync server
type Server struct {
	*protocol.Protocol
	config *Config
}

// NewServer returns a new ChainSync server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	// Use node-to-client protocol ID
	protocolId := protocolIdNtC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		protocolId = protocolIdNtN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                protocolName,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestNext:
		err = s.handleRequestNext(msg)
	case MessageTypeFindIntersect:
		err = s.handleFindIntersect(msg)
	case MessageTypeDone:
		err = s.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
	}
	return err
}

func (s *Server) handleRequestNext(msg protocol.Message) error {
	// TODO
	return nil
}

func (s *Server) handleFindIntersect(msg protocol.Message) error {
	// TODO
	return nil
}

func (s *Server) handleDone() error {
	return nil
}
