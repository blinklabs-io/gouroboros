package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Client struct {
	*protocol.Protocol
	config *Config
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
	c := &Client{
		config: cfg,
	}
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
	case MESSAGE_TYPE_DONE:
		err = c.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) RequestNext() error {
	msg := NewMsgRequestNext()
	return c.SendMessage(msg)
}

func (c *Client) FindIntersect(points []Point) error {
	msg := NewMsgFindIntersect(points)
	return c.SendMessage(msg)
}

func (c *Client) handleAwaitReply() error {
	if c.config.AwaitReplyFunc == nil {
		return fmt.Errorf("received chain-sync AwaitReply message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.AwaitReplyFunc()
}

func (c *Client) handleRollForward(msgGeneric protocol.Message) error {
	if c.config.RollForwardFunc == nil {
		return fmt.Errorf("received chain-sync RollForward message but no callback function is defined")
	}
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
		return c.config.RollForwardFunc(blockType, blockHeader)
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		blk, err := ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
		if err != nil {
			return err
		}
		// Call the user callback function
		return c.config.RollForwardFunc(msg.BlockType(), blk)
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
	if c.config.IntersectFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectFound message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgIntersectFound)
	// Call the user callback function
	return c.config.IntersectFoundFunc(msg.Point, msg.Tip)
}

func (c *Client) handleIntersectNotFound(msgGeneric protocol.Message) error {
	if c.config.IntersectNotFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectNotFound message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgIntersectNotFound)
	// Call the user callback function
	return c.config.IntersectNotFoundFunc(msg.Tip)
}

func (c *Client) handleDone() error {
	if c.config.DoneFunc == nil {
		return fmt.Errorf("received chain-sync Done message but no callback function is defined")
	}
	// Call the user callback function
	return c.config.DoneFunc()
}
