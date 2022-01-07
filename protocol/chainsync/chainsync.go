package chainsync

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
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

	BLOCK_TYPE_BYRON_EBB  = 0
	BLOCK_TYPE_BYRON_MAIN = 1
	// TODO: more block types
)

type ChainSync struct {
	errorChan       chan error
	sendChan        chan *muxer.Message
	recvChan        chan *muxer.Message
	state           uint8
	nodeToNode      bool
	protocolId      uint16
	requestNextChan chan *RequestNextResult
	recvBuffer      bytes.Buffer
}

func New(m *muxer.Muxer, errorChan chan error, nodeToNode bool) *ChainSync {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	if nodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
	}
	sendChan, recvChan := m.RegisterProtocol(protocolId)
	c := &ChainSync{
		errorChan:       errorChan,
		sendChan:        sendChan,
		recvChan:        recvChan,
		state:           STATE_IDLE,
		nodeToNode:      nodeToNode,
		protocolId:      protocolId,
		requestNextChan: make(chan *RequestNextResult, 10),
	}
	go c.recvLoop()
	return c
}

func (c *ChainSync) recvLoop() {
	for {
		var err error
		// Wait for response
		msg := <-c.recvChan
		c.recvBuffer.Write(msg.Payload)
		// Decode response into generic list until we can determine what type of response it is
		var resp []interface{}
		if err := utils.CborDecode(c.recvBuffer.Bytes(), &resp); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				continue
			}
			c.errorChan <- fmt.Errorf("chain-sync: decode error: %s", err)
		}
		switch resp[0].(uint64) {
		case MESSAGE_TYPE_AWAIT_REPLY:
			//err = c.handleRequest(msg)
			fmt.Printf("MESSAGE_TYPE_AWAIT_REPLY\n")
		case MESSAGE_TYPE_ROLL_FORWARD:
			err = c.handleRollForward(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_ROLL_BACKWARD:
			err = c.handleRollBackward(c.recvBuffer.Bytes())
		case MESSAGE_TYPE_INTERSECT_FOUND:
			// TODO
			fmt.Printf("MESSAGE_TYPE_INTERSECT_FOUND\n")
		case MESSAGE_TYPE_INTERSECT_NOT_FOUND:
			// TODO
			fmt.Printf("MESSAGE_TYPE_INTERSECT_NOT_FOUND\n")
		case MESSAGE_TYPE_DONE:
			// TODO
			fmt.Printf("MESSAGE_TYPE_DONE\n")
		default:
			err = fmt.Errorf("chain-sync: received unexpected message: %#v", resp)
		}
		c.recvBuffer.Reset()
		if err != nil {
			c.errorChan <- err
		}
	}
}

func (c *ChainSync) RequestNext() (*RequestNextResult, error) {
	// TODO
	// if c.state != STATE_IDLE {
	// 	return nil, fmt.Errorf("protocol not in expected state")
	// }
	// Create our request
	data := newMsgRequestNext()
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return nil, err
	}
	msg := muxer.NewMessage(c.protocolId, dataBytes, false)
	// Send request
	c.sendChan <- msg
	// Set the new state
	c.state = STATE_CAN_AWAIT
	resp := <-c.requestNextChan
	return resp, nil
}

func (c *ChainSync) FindIntersect(points []string) {
	return
}

func (c *ChainSync) handleAwaitReply(msg *muxer.Message) error {
	return nil
}

func (c *ChainSync) handleRollForward(data []byte) error {
	// TODO
	// if c.state != STATE_CONFIRM {
	// 	return fmt.Errorf("received handshake accept response when protocol is in wrong state")
	// }
	var msg msgRollForward
	if err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	/*
		if len(msg.WrappedData) < 8000 {
			for _, x := range msg.WrappedData {
				fmt.Printf("%02x ", x)
			}
			fmt.Printf("\n")
		}
	*/
	var block wrappedBlock
	if err := utils.CborDecode(msg.WrappedData, &block); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
		//fmt.Printf("ignoring wrapped data decode error for now...%s\n", err)
	}
	resp := &RequestNextResult{
		BlockType: block.Type,
	}
	switch block.Type {
	case BLOCK_TYPE_BYRON_EBB:
		var block2 common.ByronEpochBoundaryBlock
		if err := utils.CborDecode(block.RawBlock, &block2); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		resp.Block = block2
	case BLOCK_TYPE_BYRON_MAIN:
		var block2 common.ByronMainBlock
		if err := utils.CborDecode(block.RawBlock, &block2); err != nil {
			return fmt.Errorf("chain-sync: decode error: %s", err)
		}
		//fmt.Printf("epoch = %d, slot = %d, prevBlock = %s\n", block2.Header.ConsensusData.SlotId.Epoch, block2.Header.ConsensusData.SlotId.Slot, block2.Header.PrevBlock)
		//fmt.Printf("block2 = %#v\n", block2)
		resp.Block = block2
	// TODO: support more block types
	default:
		var payload interface{}
		if err := utils.CborDecode(msg.WrappedData, &payload); err != nil {
			//return fmt.Errorf("chain-sync: decode error: %s", err)
			//fmt.Printf("ignoring generic payload decode error for now...%s\n", err)
		}
		//fmt.Printf("payload = %s\n", utils.DumpCborStructure(payload, ""))
		resp.Block = payload
	}
	c.requestNextChan <- resp
	// TODO
	// c.state = STATE_DONE
	return nil
}

func (c *ChainSync) handleRollBackward(data []byte) error {
	// TODO
	// if c.state != STATE_CONFIRM {
	// 	return fmt.Errorf("received handshake accept response when protocol is in wrong state")
	// }
	var msg msgRollBackward
	if err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("chain-sync: decode error: %s", err)
	}
	fmt.Printf("handleRollBackward: msg = %#v\n", msg)
	resp := &RequestNextResult{
		Rollback: true,
	}
	c.requestNextChan <- resp
	// TODO
	// c.state = STATE_DONE
	return nil
}

func (c *ChainSync) handleIntersectFound(msg *muxer.Message) error {
	return nil
}

func (c *ChainSync) handleIntersectNotFound(msg *muxer.Message) error {
	return nil
}

func (c *ChainSync) handleDone(msg *muxer.Message) error {
	return nil
}
