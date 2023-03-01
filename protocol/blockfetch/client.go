package blockfetch

import (
	"fmt"
	"sync"

	"github.com/cloudstruct/go-ouroboros-network/cbor"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"

	"github.com/cloudstruct/go-ouroboros-network/ledger"
)

type Client struct {
	*protocol.Protocol
	config               *Config
	blockChan            chan ledger.Block
	startBatchResultChan chan error
	busyMutex            sync.Mutex
	blockUseCallback     bool
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:               cfg,
		blockChan:            make(chan ledger.Block),
		startBatchResultChan: make(chan error),
	}
	// Update state map with timeouts
	stateMap := StateMap.Copy()
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
		StateMap:            stateMap,
		InitialState:        STATE_IDLE,
	}
	c.Protocol = protocol.New(protoConfig)
	// Start goroutine to cleanup resources on protocol shutdown
	go func() {
		<-c.Protocol.DoneChan()
		close(c.blockChan)
	}()
	return c
}

func (c *Client) Stop() error {
	msg := NewMsgClientDone()
	return c.SendMessage(msg)
}

// GetBlockRange starts an async process to fetch all blocks in the specified range (inclusive)
func (c *Client) GetBlockRange(start common.Point, end common.Point) error {
	c.busyMutex.Lock()
	c.blockUseCallback = true
	msg := NewMsgRequestRange(start, end)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return err
	}
	err := <-c.startBatchResultChan
	if err != nil {
		c.busyMutex.Unlock()
		return err
	}
	return nil
}

// GetBlock requests and returns a single block specified by the provided point
func (c *Client) GetBlock(point common.Point) (ledger.Block, error) {
	c.busyMutex.Lock()
	c.blockUseCallback = false
	msg := NewMsgRequestRange(point, point)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	err := <-c.startBatchResultChan
	if err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	block, ok := <-c.blockChan
	if !ok {
		return nil, protocol.ProtocolShuttingDownError
	}
	return block, nil
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
	c.startBatchResultChan <- nil
	return nil
}

func (c *Client) handleNoBlocks() error {
	err := fmt.Errorf("block(s) not found")
	c.startBatchResultChan <- err
	return nil
}

func (c *Client) handleBlock(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgBlock)
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := cbor.Decode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	blk, err := ledger.NewBlockFromCbor(wrappedBlock.Type, wrappedBlock.RawBlock)
	if err != nil {
		return err
	}
	// We use the callback when requesting ranges and the internal channel for a single block
	if c.blockUseCallback {
		if err := c.config.BlockFunc(blk); err != nil {
			return err
		}
	} else {
		c.blockChan <- blk
	}
	return nil
}

func (c *Client) handleBatchDone() error {
	c.busyMutex.Unlock()
	return nil
}
