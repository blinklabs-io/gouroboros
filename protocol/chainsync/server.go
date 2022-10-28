package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Server struct {
	*protocol.Protocol
	config *Config
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	// TODO: add cases for messages from client
	switch msg.Type() {
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}
