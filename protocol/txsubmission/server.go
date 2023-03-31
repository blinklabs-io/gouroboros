package txsubmission

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
		InitialState:        STATE_INIT,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_REPLY_TX_IDS:
		err = s.handleReplyTxIds(msg)
	case MESSAGE_TYPE_REPLY_TXS:
		err = s.handleReplyTxs(msg)
	case MESSAGE_TYPE_DONE:
		err = s.handleDone()
	case MESSAGE_TYPE_INIT:
		err = s.handleInit()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (s *Server) handleReplyTxIds(msg protocol.Message) error {
	if s.config.ReplyTxIdsFunc == nil {
		return fmt.Errorf("received tx-submission ReplyTxIds message but no callback function is defined")
	}
	msgReplyTxIds := msg.(*MsgReplyTxIds)
	// Call the user callback function
	return s.config.ReplyTxIdsFunc(msgReplyTxIds.TxIds)
}

func (s *Server) handleReplyTxs(msg protocol.Message) error {
	if s.config.ReplyTxsFunc == nil {
		return fmt.Errorf("received tx-submission ReplyTxs message but no callback function is defined")
	}
	msgReplyTxs := msg.(*MsgReplyTxs)
	// Call the user callback function
	return s.config.ReplyTxsFunc(msgReplyTxs.Txs)
}

func (s *Server) handleDone() error {
	if s.config.DoneFunc == nil {
		return fmt.Errorf("received tx-submission Done message but no callback function is defined")
	}
	// Call the user callback function
	return s.config.DoneFunc()
}

func (s *Server) handleInit() error {
	if s.config.InitFunc == nil {
		return fmt.Errorf("received tx-submission Init message but no callback function is defined")
	}
	// Call the user callback function
	return s.config.InitFunc()
}
