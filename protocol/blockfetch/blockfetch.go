package blockfetch

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
	PROTOCOL_ID uint16 = 3

	STATE_IDLE = iota
	STATE_BUSY
	STATE_STREAMING
	STATE_DONE
)

type BlockFetch struct {
	errorChan      chan error
	sendChan       chan *muxer.Message
	recvChan       chan *muxer.Message
	state          uint8
	recvBuffer     *bytes.Buffer
	callbackConfig *BlockFetchCallbackConfig
}

type BlockFetchCallbackConfig struct {
	StartBatchFunc BlockFetchStartBatchFunc
	NoBlocksFunc   BlockFetchNoBlocksFunc
	BlockFunc      BlockFetchBlockFunc
	BatchDoneFunc  BlockFetchBatchDoneFunc
}

// Callback function types
type BlockFetchStartBatchFunc func() error
type BlockFetchNoBlocksFunc func() error
type BlockFetchBlockFunc func(uint, interface{}) error
type BlockFetchBatchDoneFunc func() error

func New(m *muxer.Muxer, errorChan chan error, callbackConfig *BlockFetchCallbackConfig) *BlockFetch {
	sendChan, recvChan := m.RegisterProtocol(PROTOCOL_ID)
	b := &BlockFetch{
		errorChan:      errorChan,
		sendChan:       sendChan,
		recvChan:       recvChan,
		state:          STATE_IDLE,
		recvBuffer:     bytes.NewBuffer(nil),
		callbackConfig: callbackConfig,
	}
	go b.recvLoop()
	return b
}

func (b *BlockFetch) recvLoop() {
	leftoverData := false
	for {
		var err error
		// Don't grab the next message from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for message
			msg := <-b.recvChan
			// Add message payload to buffer
			b.recvBuffer.Write(msg.Payload)
		}
		leftoverData = false
		// Decode response into generic list until we can determine what type of response it is
		var resp []interface{}
		numBytesRead, err := utils.CborDecode(b.recvBuffer.Bytes(), &resp)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				continue
			}
			if err == io.EOF && b.recvBuffer.Len() > 0 {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				continue
			}
			b.errorChan <- fmt.Errorf("block-fetch: decode error: %s", err)
		}
		switch resp[0].(uint64) {
		case MESSAGE_TYPE_START_BATCH:
			err = b.handleStartBatch()
		case MESSAGE_TYPE_NO_BLOCKS:
			err = b.handleNoBlocks()
		case MESSAGE_TYPE_BLOCK:
			err = b.handleBlock(b.recvBuffer.Bytes())
		case MESSAGE_TYPE_BATCH_DONE:
			err = b.handleBatchDone()
		default:
			err = fmt.Errorf("block-fetch: received unexpected message: %#v", resp)
		}
		if err != nil {
			b.errorChan <- err
		}
		if numBytesRead < b.recvBuffer.Len() {
			// There is another message in the same muxer segment, so we reset the buffer with just
			// the remaining data
			b.recvBuffer = bytes.NewBuffer(b.recvBuffer.Bytes()[numBytesRead:])
			leftoverData = true
		} else {
			// Empty out our buffer since we successfully processed the message
			b.recvBuffer.Reset()
		}
	}
}

func (c *BlockFetch) RequestRange(start []interface{}, end []interface{}) error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("block-fetch: RequestRange: protocol not in expected state")
	}
	data := newMsgRequestRange(start, end)
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(PROTOCOL_ID, dataBytes, false)
	c.state = STATE_BUSY
	// Send request
	c.sendChan <- msg
	return nil
}

func (c *BlockFetch) ClientDone() error {
	if c.state != STATE_IDLE {
		return fmt.Errorf("block-fetch: ClientDone: protocol not in expected state")
	}
	data := newMsgClientDone()
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(PROTOCOL_ID, dataBytes, false)
	c.state = STATE_BUSY
	// Send request
	c.sendChan <- msg
	return nil
}

func (b *BlockFetch) handleStartBatch() error {
	if b.state != STATE_BUSY {
		return fmt.Errorf("received block-fetch StartBatch message when protocol not in expected state")
	}
	if b.callbackConfig.StartBatchFunc == nil {
		return fmt.Errorf("received block-fetch StartBatch message but no callback function is defined")
	}
	b.state = STATE_STREAMING
	return b.callbackConfig.StartBatchFunc()
}

func (b *BlockFetch) handleNoBlocks() error {
	if b.state != STATE_BUSY {
		return fmt.Errorf("received block-fetch NoBlocks message when protocol not in expected state")
	}
	if b.callbackConfig.NoBlocksFunc == nil {
		return fmt.Errorf("received block-fetch NoBlocks message but no callback function is defined")
	}
	b.state = STATE_IDLE
	return b.callbackConfig.NoBlocksFunc()
}

func (b *BlockFetch) handleBlock(data []byte) error {
	if b.state != STATE_STREAMING {
		return fmt.Errorf("received block-fetch Block message when protocol not in expected state")
	}
	if b.callbackConfig.BlockFunc == nil {
		return fmt.Errorf("received block-fetch Block message but no callback function is defined")
	}
	var msg msgBlock
	if _, err := utils.CborDecode(data, &msg); err != nil {
		return fmt.Errorf("block-fetch: decode error: %s", err)
	}
	// Decode only enough to get the block type value
	var wrapBlock wrappedBlock
	if _, err := utils.CborDecode(msg.WrappedBlock, &wrapBlock); err != nil {
		return fmt.Errorf("block-fetch: decode error: %s", err)
	}
	blk, err := block.NewBlockFromCbor(wrapBlock.Type, wrapBlock.RawBlock)
	if err != nil {
		return err
	}
	// We don't actually need this since it's the state we started in, but it's good to be explicit
	b.state = STATE_STREAMING
	return b.callbackConfig.BlockFunc(wrapBlock.Type, blk)
}

func (b *BlockFetch) handleBatchDone() error {
	if b.state != STATE_STREAMING {
		return fmt.Errorf("received block-fetch BatchDone message when protocol not in expected state")
	}
	if b.callbackConfig.BatchDoneFunc == nil {
		return fmt.Errorf("received block-fetch BatchDone message but no callback function is defined")
	}
	b.state = STATE_IDLE
	return b.callbackConfig.BatchDoneFunc()
}
