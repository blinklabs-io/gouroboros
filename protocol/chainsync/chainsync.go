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
		if err := utils.CborDecode(c.recvBuffer.Bytes(), &resp); err != nil {
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
		return fmt.Errorf("protocol not in expected state")
	}
	// Create our request
	data := newMsgRequestNext()
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(c.protocolId, dataBytes, false)
	// Send request
	c.sendChan <- msg
	c.state = STATE_CAN_AWAIT
	return nil
}

func (c *ChainSync) FindIntersect(points []interface{}) error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("protocol not in expected state")
	}
	data := newMsgFindIntersect(points)
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(c.protocolId, dataBytes, false)
	// Send request
	c.sendChan <- msg
	c.state = STATE_INTERSECT
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
	var msg msgRollForward
	if err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	// Decode only enough to get the block type value
	var wrapBlock wrappedBlock
	if err := utils.CborDecode(msg.WrappedData, &wrapBlock); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	var respBlock interface{}
	switch wrapBlock.Type {
	case block.BLOCK_TYPE_BYRON_EBB:
		var byronEbbBlock block.ByronEpochBoundaryBlock
		if err := utils.CborDecode(wrapBlock.RawBlock, &byronEbbBlock); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		respBlock = byronEbbBlock
	case block.BLOCK_TYPE_BYRON_MAIN:
		var byronMainBlock block.ByronMainBlock
		if err := utils.CborDecode(wrapBlock.RawBlock, &byronMainBlock); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		respBlock = byronMainBlock
	case block.BLOCK_TYPE_SHELLEY:
		var shelleyBlock block.ShelleyBlock
		if err := utils.CborDecode(wrapBlock.RawBlock, &shelleyBlock); err != nil {
			/*
				var payload interface{}
				if err := utils.CborDecode(msg.WrappedData, &payload); err != nil {
					return fmt.Errorf("chain-sync: decode error: %s", err)
				}
				fmt.Printf("payload = %s\n", utils.DumpCborStructure(payload, ""))
			*/
			if len(msg.WrappedData) < 8000 {
				for _, x := range msg.WrappedData {
					fmt.Printf("%02x ", x)
				}
				fmt.Printf("\n")
			}
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		respBlock = shelleyBlock
	// TODO: support more block types
	default:
		var payload interface{}
		if err := utils.CborDecode(msg.WrappedData, &payload); err != nil {
			//return fmt.Errorf("chain-sync: decode error: %s", err)
			//fmt.Printf("ignoring generic payload decode error for now...%s\n", err)
		}
		fmt.Printf("payload = %s\n", utils.DumpCborStructure(payload, ""))
		return fmt.Errorf("unsupported block type\n")
		respBlock = payload
	}
	// Set new state
	c.state = STATE_IDLE
	return c.callbackConfig.RollForwardFunc(wrapBlock.Type, respBlock)
}

func (c *ChainSync) handleRollBackward(data []byte) error {
	if c.state != STATE_CAN_AWAIT && c.state != STATE_MUST_REPLY {
		return fmt.Errorf("received chain-sync RollBackward message when protocol not in expected state")
	}
	if c.callbackConfig.RollBackwardFunc == nil {
		return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
	}
	var msg msgRollBackward
	if err := utils.CborDecode(data, &msg); err != nil {
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
	if err := utils.CborDecode(data, &msg); err != nil {
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
	if err := utils.CborDecode(data, &msg); err != nil {
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
