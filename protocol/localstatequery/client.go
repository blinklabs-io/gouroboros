package localstatequery

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Client struct {
	*protocol.Protocol
	config                        *Config
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
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
	// Enable version-dependent features
	if protoOptions.Version >= 10 {
		c.enableGetChainBlockNo = true
		c.enableGetChainPoint = true
	}
	if protoOptions.Version >= 11 {
		c.enableGetRewardInfoPoolsBlock = true
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACQUIRED:
		err = c.handleAcquired()
	case MESSAGE_TYPE_FAILURE:
		err = c.handleFailure(msg)
	case MESSAGE_TYPE_RESULT:
		err = c.handleResult(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) handleAcquired() error {
	if c.config.AcquiredFunc == nil {
		return fmt.Errorf("received local-state-query Acquired message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.AcquiredFunc()
}

func (c *Client) handleFailure(msg protocol.Message) error {
	if c.config.FailureFunc == nil {
		return fmt.Errorf("received local-state-query Failure message but no callback function is defined")
	}
	msgFailure := msg.(*MsgFailure)
	// Call the user callback function
	return c.config.FailureFunc(msgFailure.Failure)
}

func (c *Client) handleResult(msg protocol.Message) error {
	if c.config.ResultFunc == nil {
		return fmt.Errorf("received local-state-query Result message but no callback function is defined")
	}
	msgResult := msg.(*MsgResult)
	// Call the user callback function
	return c.config.ResultFunc(msgResult.Result)
}
