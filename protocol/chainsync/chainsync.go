package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	PROTOCOL_NAME          = "chain-sync"
	PROTOCOL_ID_NTN uint16 = 2
	PROTOCOL_ID_NTC uint16 = 5

	STATE_IDLE = iota
	STATE_CAN_AWAIT
	STATE_MUST_REPLY
	STATE_INTERSECT
	STATE_DONE
)

type ChainSync struct {
	proto          *protocol.Protocol
	nodeToNode     bool
	callbackConfig *ChainSyncCallbackConfig
}

type ChainSyncCallbackConfig struct {
	AwaitReplyFunc        ChainSyncAwaitReplyFunc
	RollBackwardFunc      ChainSyncRollBackwardFunc
	RollForwardFunc       ChainSyncRollForwardFunc
	IntersectFoundFunc    ChainSyncIntersectFoundFunc
	IntersectNotFoundFunc ChainSyncIntersectNotFoundFunc
	DoneFunc              ChainSyncDoneFunc
}

// Callback function types
type ChainSyncAwaitReplyFunc func() error
type ChainSyncRollBackwardFunc func(interface{}, interface{}) error
type ChainSyncRollForwardFunc func(uint, interface{}) error
type ChainSyncIntersectFoundFunc func(interface{}, interface{}) error
type ChainSyncIntersectNotFoundFunc func(interface{}) error
type ChainSyncDoneFunc func() error

func New(m *muxer.Muxer, errorChan chan error, nodeToNode bool, callbackConfig *ChainSyncCallbackConfig) *ChainSync {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	if nodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
	}
	c := &ChainSync{
		nodeToNode:     nodeToNode,
		callbackConfig: callbackConfig,
	}
	c.proto = protocol.New(PROTOCOL_NAME, protocolId, m, errorChan, c.messageHandler, c.NewMsgFromCbor)
	c.proto.SetState(STATE_IDLE)
	return c
}

func (c *ChainSync) messageHandler(msg protocol.Message) error {
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

func (c *ChainSync) RequestNext() error {
	if c.proto.GetState() != STATE_IDLE {
		return fmt.Errorf("chain-sync: RequestNext: protocol not in expected state")
	}
	// Create our request
	msg := newMsgRequestNext()
	c.proto.SetState(STATE_CAN_AWAIT)
	// Send request
	return c.proto.SendMessage(msg, false)
}

func (c *ChainSync) FindIntersect(points []interface{}) error {
	if c.proto.GetState() != STATE_IDLE {
		return fmt.Errorf("chain-sync: FindIntersect: protocol not in expected state")
	}
	msg := newMsgFindIntersect(points)
	c.proto.SetState(STATE_INTERSECT)
	// Send request
	return c.proto.SendMessage(msg, false)
}

func (c *ChainSync) handleAwaitReply() error {
	if c.proto.GetState() != STATE_CAN_AWAIT {
		return fmt.Errorf("received chain-sync AwaitReply message when protocol not in expected state")
	}
	if c.callbackConfig.AwaitReplyFunc == nil {
		return fmt.Errorf("received chain-sync AwaitReply message but no callback function is defined")
	}
	c.proto.SetState(STATE_MUST_REPLY)
	return c.callbackConfig.AwaitReplyFunc()
}

func (c *ChainSync) handleRollForward(msgGeneric protocol.Message) error {
	if c.proto.GetState() != STATE_CAN_AWAIT && c.proto.GetState() != STATE_MUST_REPLY {
		return fmt.Errorf("received chain-sync RollForward message when protocol not in expected state")
	}
	if c.callbackConfig.RollForwardFunc == nil {
		return fmt.Errorf("received chain-sync RollForward message but no callback function is defined")
	}
	if c.nodeToNode {
		msg := msgGeneric.(*msgRollForwardNtN)
		var blockHeader interface{}
		var blockType uint
		blockHeaderType := msg.WrappedHeader.Type
		switch blockHeaderType {
		case block.BLOCK_HEADER_TYPE_BYRON:
			var wrapHeaderByron wrappedHeaderByron
			if _, err := utils.CborDecode(msg.WrappedHeader.RawData, &wrapHeaderByron); err != nil {
				return fmt.Errorf("chain-sync: decode error: %s", err)
			}
			blockType = wrapHeaderByron.Unknown.Type
			var err error
			blockHeader, err = block.NewBlockHeaderFromCbor(blockType, wrapHeaderByron.RawHeader)
			if err != nil {
				return err
			}
		default:
			// Map block header types to block types
			blockTypeMap := map[uint]uint{
				block.BLOCK_HEADER_TYPE_SHELLEY: block.BLOCK_TYPE_SHELLEY,
				block.BLOCK_HEADER_TYPE_ALLEGRA: block.BLOCK_TYPE_ALLEGRA,
				block.BLOCK_HEADER_TYPE_MARY:    block.BLOCK_TYPE_MARY,
				block.BLOCK_HEADER_TYPE_ALONZO:  block.BLOCK_TYPE_ALONZO,
			}
			blockType = blockTypeMap[blockHeaderType]
			// We decode into a byte array to implicitly unwrap the CBOR tag object
			var payload []byte
			if _, err := utils.CborDecode(msg.WrappedHeader.RawData, &payload); err != nil {
				return fmt.Errorf("failed fallback decode: %s", err)
			}
			var err error
			blockHeader, err = block.NewBlockHeaderFromCbor(blockType, payload)
			if err != nil {
				return err
			}
		}
		c.proto.SetState(STATE_IDLE)
		return c.callbackConfig.RollForwardFunc(blockType, blockHeader)
	} else {
		msg := msgGeneric.(*msgRollForwardNtC)
		// Decode only enough to get the block type value
		var wrapBlock wrappedBlock
		if _, err := utils.CborDecode(msg.WrappedData, &wrapBlock); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		blk, err := block.NewBlockFromCbor(wrapBlock.Type, wrapBlock.RawBlock)
		if err != nil {
			return err
		}
		c.proto.SetState(STATE_IDLE)
		return c.callbackConfig.RollForwardFunc(wrapBlock.Type, blk)
	}
}

func (c *ChainSync) handleRollBackward(msgGeneric protocol.Message) error {
	if c.proto.GetState() != STATE_CAN_AWAIT && c.proto.GetState() != STATE_MUST_REPLY {
		return fmt.Errorf("received chain-sync RollBackward message when protocol not in expected state")
	}
	if c.callbackConfig.RollBackwardFunc == nil {
		return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
	}
	msg := msgGeneric.(*msgRollBackward)
	c.proto.SetState(STATE_IDLE)
	return c.callbackConfig.RollBackwardFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectFound(msgGeneric protocol.Message) error {
	if c.proto.GetState() != STATE_INTERSECT {
		return fmt.Errorf("received chain-sync IntersectFound message when protocol not in expected state")
	}
	if c.callbackConfig.IntersectFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectFound message but no callback function is defined")
	}
	msg := msgGeneric.(*msgIntersectFound)
	c.proto.SetState(STATE_IDLE)
	return c.callbackConfig.IntersectFoundFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectNotFound(msgGeneric protocol.Message) error {
	if c.proto.GetState() != STATE_INTERSECT {
		return fmt.Errorf("received chain-sync IntersectNotFound message when protocol not in expected state")
	}
	if c.callbackConfig.IntersectNotFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectNotFound message but no callback function is defined")
	}
	msg := msgGeneric.(*msgIntersectNotFound)
	c.proto.SetState(STATE_IDLE)
	return c.callbackConfig.IntersectNotFoundFunc(msg.Tip)
}

func (c *ChainSync) handleDone() error {
	if c.proto.GetState() != STATE_IDLE {
		return fmt.Errorf("received chain-sync Done message when protocol not in expected state")
	}
	if c.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received chain-sync Done message but no callback function is defined")
	}
	c.proto.SetState(STATE_DONE)
	return c.callbackConfig.DoneFunc()
}
