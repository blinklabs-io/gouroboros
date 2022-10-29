package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Client struct {
	*protocol.Protocol
	config *Config
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	c := &Client{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACCEPT_TX:
		err = c.handleAcceptTx()
	case MESSAGE_TYPE_REJECT_TX:
		err = c.handleRejectTx(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) SubmitTx(eraId uint16, tx []byte) error {
	msg := NewMsgSubmitTx(eraId, tx)
	return c.SendMessage(msg)
}

func (c *Client) Done(tx interface{}) error {
	msg := NewMsgDone()
	return c.SendMessage(msg)
}

func (c *Client) handleAcceptTx() error {
	if c.config.AcceptTxFunc == nil {
		return fmt.Errorf("received local-tx-submission AcceptTx message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.AcceptTxFunc()
}

func (c *Client) handleRejectTx(msgGeneric protocol.Message) error {
	if c.config.RejectTxFunc == nil {
		return fmt.Errorf("received local-tx-submission RejectTx message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgRejectTx)
	// Call the user callback function
	return c.config.RejectTxFunc([]byte(msg.Reason))
}
