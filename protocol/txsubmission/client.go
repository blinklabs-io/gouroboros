package txsubmission

import (
	"fmt"
	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	config *Config
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[STATE_IDLE]; ok {
		entry.Timeout = c.config.IdleTimeout
		stateMap[STATE_IDLE] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        STATE_INIT,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_REQUEST_TX_IDS:
		err = c.handleRequestTxIds(msg)
	case MESSAGE_TYPE_REQUEST_TXS:
		err = c.handleRequestTxs(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) handleRequestTxIds(msg protocol.Message) error {
	if c.config.RequestTxIdsFunc == nil {
		return fmt.Errorf("received tx-submission RequestTxIds message but no callback function is defined")
	}
	msgRequestTxIds := msg.(*MsgRequestTxIds)
	// Call the user callback function
	return c.config.RequestTxIdsFunc(msgRequestTxIds.Blocking, msgRequestTxIds.Ack, msgRequestTxIds.Req)
}

func (c *Client) handleRequestTxs(msg protocol.Message) error {
	if c.config.RequestTxsFunc == nil {
		return fmt.Errorf("received tx-submission RequestTxs message but no callback function is defined")
	}
	msgRequestTxs := msg.(*MsgRequestTxs)
	// Call the user callback function
	return c.config.RequestTxsFunc(msgRequestTxs.TxIds)
}
