package chainsync

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"io"
)

const (
	PROTOCOL_ID_NTN uint16 = 2
	PROTOCOL_ID_NTC uint16 = 5

	STATE_IDLE = iota
	STATE_CAN_AWAIT
	STATE_MUST_REPLY
	STATE_INTERSECT
	STATE_DONE
)

// TODO: add locking around outbound calls
type ChainSync struct {
	errorChan      chan error
	sendChan       chan *muxer.Message
	recvChan       chan *muxer.Message
	state          uint8
	nodeToNode     bool
	protocolId     uint16
	recvBuffer     bytes.Buffer
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

// TODO: flesh out func args
// Callback function types
type ChainSyncAwaitReplyFunc func() error
type ChainSyncRollBackwardFunc func(interface{}, interface{}) error
type ChainSyncRollForwardFunc func(uint, interface{}) error
type ChainSyncIntersectFoundFunc func(interface{}, interface{}) error
type ChainSyncIntersectNotFoundFunc func() error
type ChainSyncDoneFunc func() error

func New(m *muxer.Muxer, errorChan chan error, nodeToNode bool, callbackConfig *ChainSyncCallbackConfig) *ChainSync {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	if nodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
	}
	sendChan, recvChan := m.RegisterProtocol(protocolId)
	c := &ChainSync{
		errorChan:      errorChan,
		sendChan:       sendChan,
		recvChan:       recvChan,
		state:          STATE_IDLE,
		nodeToNode:     nodeToNode,
		protocolId:     protocolId,
		callbackConfig: callbackConfig,
	}
	go c.recvLoop()
	return c
}

func (c *ChainSync) recvLoop() {
	for {
		var err error
		// Wait for message
		msg := <-c.recvChan
		// Add message payload to buffer
		c.recvBuffer.Write(msg.Payload)
		// Decode response into generic list until we can determine what type of response it is
		var resp []interface{}
		if _, err := utils.CborDecode(c.recvBuffer.Bytes(), &resp); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				continue
			}
			c.errorChan <- fmt.Errorf("chain-sync: decode error: %s", err)
		}
		switch resp[0].(uint64) {
		case MESSAGE_TYPE_AWAIT_REPLY:
			err = c.handleAwaitReply()
		case MESSAGE_TYPE_ROLL_FORWARD:
			err = c.handleRollForward(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_ROLL_BACKWARD:
			err = c.handleRollBackward(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_INTERSECT_FOUND:
			err = c.handleIntersectFound(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_INTERSECT_NOT_FOUND:
			err = c.handleIntersectNotFound(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_DONE:
			err = c.handleDone()
		default:
			err = fmt.Errorf("chain-sync: received unexpected message: %#v", resp)
		}
		if err != nil {
			c.errorChan <- err
		}
		// Empty out our buffer since we successfully processed the message
		c.recvBuffer.Reset()
	}
}

func (c *ChainSync) RequestNext() error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("chain-sync: RequestNext: protocol not in expected state")
	}
	// Create our request
	data := newMsgRequestNext()
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(c.protocolId, dataBytes, false)
	c.state = STATE_CAN_AWAIT
	// Send request
	c.sendChan <- msg
	return nil
}

func (c *ChainSync) FindIntersect(points []interface{}) error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("chain-sync: FindIntersect: protocol not in expected state")
	}
	data := newMsgFindIntersect(points)
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(c.protocolId, dataBytes, false)
	c.state = STATE_INTERSECT
	// Send request
	c.sendChan <- msg
	return nil
}

func (c *ChainSync) handleAwaitReply() error {
	if c.state != STATE_CAN_AWAIT {
		return fmt.Errorf("received chain-sync AwaitReply message when protocol not in expected state")
	}
	if c.callbackConfig.AwaitReplyFunc == nil {
		return fmt.Errorf("received chain-sync AwaitReply message but no callback function is defined")
	}
	c.state = STATE_MUST_REPLY
	return c.callbackConfig.AwaitReplyFunc()
}

func (c *ChainSync) handleRollForward(data []byte) error {
	if c.state != STATE_CAN_AWAIT && c.state != STATE_MUST_REPLY {
		return fmt.Errorf("received chain-sync RollForward message when protocol not in expected state")
	}
	if c.callbackConfig.RollForwardFunc == nil {
		return fmt.Errorf("received chain-sync RollForward message but no callback function is defined")
	}
	if c.nodeToNode {
		var msg msgRollForwardNtN
		if _, err := utils.CborDecode(data, &msg); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
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
		c.state = STATE_IDLE
		return c.callbackConfig.RollForwardFunc(blockType, blockHeader)
	} else {
		var msg msgRollForwardNtC
		if _, err := utils.CborDecode(data, &msg); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		// Decode only enough to get the block type value
		var wrapBlock wrappedBlock
		if _, err := utils.CborDecode(msg.WrappedData, &wrapBlock); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		blk, err := block.NewBlockFromCbor(wrapBlock.Type, wrapBlock.RawBlock)
		if err != nil {
			return err
		}
		c.state = STATE_IDLE
		return c.callbackConfig.RollForwardFunc(wrapBlock.Type, blk)
	}
}

func (c *ChainSync) handleRollBackward(data []byte) error {
	if c.state != STATE_CAN_AWAIT && c.state != STATE_MUST_REPLY {
		return fmt.Errorf("received chain-sync RollBackward message when protocol not in expected state")
	}
	if c.callbackConfig.RollBackwardFunc == nil {
		return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
	}
	var msg msgRollBackward
	if _, err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	c.state = STATE_IDLE
	return c.callbackConfig.RollBackwardFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectFound(data []byte) error {
	if c.state != STATE_INTERSECT {
		return fmt.Errorf("received chain-sync IntersectFound message when protocol not in expected state")
	}
	if c.callbackConfig.IntersectFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectFound message but no callback function is defined")
	}
	var msg msgIntersectFound
	if _, err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	c.state = STATE_IDLE
	return c.callbackConfig.IntersectFoundFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectNotFound(data []byte) error {
	if c.state != STATE_INTERSECT {
		return fmt.Errorf("received chain-sync IntersectNotFound message when protocol not in expected state")
	}
	if c.callbackConfig.IntersectNotFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectNotFound message but no callback function is defined")
	}
	var msg msgIntersectNotFound
	if _, err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	c.state = STATE_IDLE
	return c.callbackConfig.IntersectNotFoundFunc()
}

func (c *ChainSync) handleDone() error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("received chain-sync Done message when protocol not in expected state")
	}
	if c.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received chain-sync Done message but no callback function is defined")
	}
	c.state = STATE_DONE
	return c.callbackConfig.DoneFunc()
}
