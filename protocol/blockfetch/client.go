package blockfetch

import (
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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
	// Update state map with timeouts
	stateMap := StateMap
	if entry, ok := stateMap[STATE_BUSY]; ok {
		entry.Timeout = c.config.BatchStartTimeout
		stateMap[STATE_BUSY] = entry
	}
	if entry, ok := stateMap[STATE_STREAMING]; ok {
		entry.Timeout = c.config.BlockTimeout
		stateMap[STATE_STREAMING] = entry
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
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) RequestRange(start []interface{}, end []interface{}) error {
	msg := NewMsgRequestRange(start, end)
	return c.SendMessage(msg)
}

func (c *Client) ClientDone() error {
	msg := NewMsgClientDone()
	return c.SendMessage(msg)
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_START_BATCH:
		err = c.handleStartBatch()
	case MESSAGE_TYPE_NO_BLOCKS:
		err = c.handleNoBlocks()
	case MESSAGE_TYPE_BLOCK:
		err = c.handleBlock(msg)
	case MESSAGE_TYPE_BATCH_DONE:
		err = c.handleBatchDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) handleStartBatch() error {
	if c.config.StartBatchFunc == nil {
		return fmt.Errorf("received block-fetch StartBatch message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.StartBatchFunc()
}

func (c *Client) handleNoBlocks() error {
	if c.config.NoBlocksFunc == nil {
		return fmt.Errorf("received block-fetch NoBlocks message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.NoBlocksFunc()
}

func (c *Client) handleBlock(msgGeneric protocol.Message) error {
	if c.config.BlockFunc == nil {
		return fmt.Errorf("received block-fetch Block message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgBlock)
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := utils.CborDecode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	blk, err := ledger.NewBlockFromCbor(wrappedBlock.Type, wrappedBlock.RawBlock)
	if err != nil {
		return err
	}
	// Call the user callback function
	return c.config.BlockFunc(wrappedBlock.Type, blk)
}

func (c *Client) handleBatchDone() error {
	if c.config.BatchDoneFunc == nil {
		return fmt.Errorf("received block-fetch BatchDone message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.BatchDoneFunc()
}
