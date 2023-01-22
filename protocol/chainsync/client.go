package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
	"sync"
)

type Client struct {
	*protocol.Protocol
	config                *Config
	busyMutex             sync.Mutex
	intersectResultChan   chan error
	readyForNextBlockChan chan bool
	wantCurrentTip        bool
	currentTipChan        chan Tip
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:                cfg,
		intersectResultChan:   make(chan error),
		readyForNextBlockChan: make(chan bool),
		currentTipChan:        make(chan Tip),
	}
	// Update state map with timeouts
	stateMap := StateMap
	if entry, ok := stateMap[STATE_INTERSECT]; ok {
		entry.Timeout = c.config.IntersectTimeout
		stateMap[STATE_INTERSECT] = entry
	}
	for _, state := range []protocol.State{STATE_CAN_AWAIT, STATE_MUST_REPLY} {
		if entry, ok := stateMap[state]; ok {
			entry.Timeout = c.config.BlockTimeout
			stateMap[state] = entry
		}
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_AWAIT_REPLY:
		err = c.handleAwaitReply()
	case MESSAGE_TYPE_ROLL_FORWARD:
		err = c.handleRollForward(msg)
	case MESSAGE_TYPE_ROLL_BACKWARD:
		err = c.handleRollBackward(msg)
	case MESSAGE_TYPE_INTERSECT_FOUND:
		err = c.handleIntersectFound(msg)
	case MESSAGE_TYPE_INTERSECT_NOT_FOUND:
		err = c.handleIntersectNotFound(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
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

func (c *Client) GetCurrentTip() (*Tip, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	c.wantCurrentTip = true
	msg := NewMsgFindIntersect([]common.Point{})
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	tip := <-c.currentTipChan
	c.wantCurrentTip = false
	return &tip, nil
}

func (c *Client) Sync(intersectPoints []common.Point) error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	if err := <-c.intersectResultChan; err != nil {
		return err
	}
	// Pipeline the initial block requests to speed things up a bit
	// Using a value higher than 10 seems to cause problems with NtN
	for i := 0; i < 10; i++ {
		msg := NewMsgRequestNext()
		if err := c.SendMessage(msg); err != nil {
			return err
		}
	}
	go c.syncLoop()
	return nil
}

func (c *Client) syncLoop() {
	for {
		// Wait for a block to be received
		<-c.readyForNextBlockChan
		c.busyMutex.Lock()
		// Request the next block
		// In practice we already have multiple block requests pipelined
		// and this just adds another one to the pile
		msg := NewMsgRequestNext()
		if err := c.SendMessage(msg); err != nil {
			c.SendError(err)
			return
		}
		c.busyMutex.Unlock()
	}
}

func (c *Client) handleAwaitReply() error {
	return nil
}

func (c *Client) handleRollForward(msgGeneric protocol.Message) error {
	if c.config.RollForwardFunc == nil {
		return fmt.Errorf("received chain-sync RollForward message but no callback function is defined")
	}
	// Signal that we're ready for the next block after we finish handling this one
	defer func() {
		c.readyForNextBlockChan <- true
	}()
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		msg := msgGeneric.(*MsgRollForwardNtN)
		var blockHeader interface{}
		var blockType uint
		blockEra := msg.WrappedHeader.Era
		switch blockEra {
		case ledger.BLOCK_HEADER_TYPE_BYRON:
			blockType = msg.WrappedHeader.ByronType()
			var err error
			blockHeader, err = ledger.NewBlockHeaderFromCbor(blockType, msg.WrappedHeader.HeaderCbor())
			if err != nil {
				return err
			}
		default:
			// Map block header types to block types
			blockTypeMap := map[uint]uint{
				ledger.BLOCK_HEADER_TYPE_SHELLEY: ledger.BLOCK_TYPE_SHELLEY,
				ledger.BLOCK_HEADER_TYPE_ALLEGRA: ledger.BLOCK_TYPE_ALLEGRA,
				ledger.BLOCK_HEADER_TYPE_MARY:    ledger.BLOCK_TYPE_MARY,
				ledger.BLOCK_HEADER_TYPE_ALONZO:  ledger.BLOCK_TYPE_ALONZO,
				ledger.BLOCK_HEADER_TYPE_BABBAGE: ledger.BLOCK_TYPE_BABBAGE,
			}
			blockType = blockTypeMap[blockEra]
			var err error
			blockHeader, err = ledger.NewBlockHeaderFromCbor(blockType, msg.WrappedHeader.HeaderCbor())
			if err != nil {
				return err
			}
		}
		// Call the user callback function
		return c.config.RollForwardFunc(blockType, blockHeader, msg.Tip)
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		blk, err := ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
		if err != nil {
			return err
		}
		// Call the user callback function
		return c.config.RollForwardFunc(msg.BlockType(), blk, msg.Tip)
	}
}

func (c *Client) handleRollBackward(msgGeneric protocol.Message) error {
	if c.config.RollBackwardFunc == nil {
		return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgRollBackward)
	// Call the user callback function
	return c.config.RollBackwardFunc(msg.Point, msg.Tip)
}

func (c *Client) handleIntersectFound(msgGeneric protocol.Message) error {
	if c.wantCurrentTip {
		msgIntersectFound := msgGeneric.(*MsgIntersectFound)
		c.currentTipChan <- msgIntersectFound.Tip
	} else {
		c.intersectResultChan <- nil
	}
	return nil
}

func (c *Client) handleIntersectNotFound(msgGeneric protocol.Message) error {
	if c.wantCurrentTip {
		msgIntersectNotFound := msgGeneric.(*MsgIntersectNotFound)
		c.currentTipChan <- msgIntersectNotFound.Tip
	} else {
		c.intersectResultChan <- IntersectNotFoundError{}
	}
	return nil
}
