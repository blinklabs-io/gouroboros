package keepalive

import (
	"fmt"
	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	*protocol.Protocol
	config *Config
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
		InitialState:        STATE_CLIENT,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_KEEP_ALIVE:
		err = s.handleKeepAlive(msg)
	case MESSAGE_TYPE_DONE:
		err = s.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (s *Server) handleKeepAlive(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgKeepAlive)
	if s.config != nil && s.config.KeepAliveFunc != nil {
		// Call the user callback function
		return s.config.KeepAliveFunc(msg.Cookie)
	} else {
		// Send the keep-alive response
		resp := NewMsgKeepAliveResponse(msg.Cookie)
		return s.SendMessage(resp)
	}
}

func (s *Server) handleDone() error {
	if s.config != nil && s.config.DoneFunc != nil {
		// Call the user callback function
		return s.config.DoneFunc()
	}
	return nil
}
