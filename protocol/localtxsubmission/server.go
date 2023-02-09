package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

// Server implements the LocalTxSubmission server
type Server struct {
	*protocol.Protocol
	config *Config
}

// NewServer returns a new Server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
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
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MessageTypeSubmitTx:
		err = s.handleSubmitTx(msg)
	case MessageTypeDone:
		err = s.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
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
