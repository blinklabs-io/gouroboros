package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"sync"
)

type Client struct {
	*protocol.Protocol
	config           *Config
	busyMutex        sync.Mutex
	submitResultChan chan error
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	c := &Client{
		config:           cfg,
		submitResultChan: make(chan error),
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
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgSubmitTx(eraId, tx)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	err := <-c.submitResultChan
	return err
}

func (c *Client) Stop() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgDone()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

func (c *Client) handleAcceptTx() error {
	c.submitResultChan <- nil
	return nil
}

func (c *Client) handleRejectTx(msg protocol.Message) error {
	msgRejectTx := msg.(*MsgRejectTx)
	err := TransactionRejectedError{
		ReasonCbor: []byte(msgRejectTx.Reason),
	}
	c.submitResultChan <- err
	return nil
}
