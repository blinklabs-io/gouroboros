package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
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
		InitialState:        STATE_IDLE,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_SUBMIT_TX:
		err = s.handleSubmitTx(msg)
	case MESSAGE_TYPE_DONE:
		err = s.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (s *Server) handleSubmitTx(msgGeneric protocol.Message) error {
	if s.config.SubmitTxFunc == nil {
		return fmt.Errorf("received local-tx-submission SubmitTx message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgSubmitTx)
	// Call the user callback function
	return s.config.SubmitTxFunc(msg.Transaction)
}

func (s *Server) handleDone() error {
	return nil
}
