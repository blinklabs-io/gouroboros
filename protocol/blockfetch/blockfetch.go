package blockfetch

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	PROTOCOL_NAME        = "block-fetch"
	PROTOCOL_ID   uint16 = 3
)

var (
	STATE_IDLE      = protocol.NewState(1, "Idle")
	STATE_BUSY      = protocol.NewState(2, "Busy")
	STATE_STREAMING = protocol.NewState(3, "Streaming")
	STATE_DONE      = protocol.NewState(4, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_RANGE,
				NewState: STATE_BUSY,
			},
			{
				MsgType:  MESSAGE_TYPE_CLIENT_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_BUSY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_START_BATCH,
				NewState: STATE_STREAMING,
			},
			{
				MsgType:  MESSAGE_TYPE_NO_BLOCKS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_STREAMING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_BLOCK,
				NewState: STATE_STREAMING,
			},
			{
				MsgType:  MESSAGE_TYPE_BATCH_DONE,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type BlockFetch struct {
	*protocol.Protocol
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

func New(options protocol.ProtocolOptions, callbackConfig *BlockFetchCallbackConfig) *BlockFetch {
	b := &BlockFetch{
		callbackConfig: callbackConfig,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               options.Muxer,
		ErrorChan:           options.ErrorChan,
		Mode:                options.Mode,
		Role:                options.Role,
		MessageHandlerFunc:  b.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	b.Protocol = protocol.New(protoConfig)
	return b
}

func (b *BlockFetch) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_START_BATCH:
		err = b.handleStartBatch()
	case MESSAGE_TYPE_NO_BLOCKS:
		err = b.handleNoBlocks()
	case MESSAGE_TYPE_BLOCK:
		err = b.handleBlock(msg)
	case MESSAGE_TYPE_BATCH_DONE:
		err = b.handleBatchDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (b *BlockFetch) RequestRange(start []interface{}, end []interface{}) error {
	msg := NewMsgRequestRange(start, end)
	return b.SendMessage(msg)
}

func (b *BlockFetch) ClientDone() error {
	msg := NewMsgClientDone()
	return b.SendMessage(msg)
}

func (b *BlockFetch) handleStartBatch() error {
	if b.callbackConfig.StartBatchFunc == nil {
		return fmt.Errorf("received block-fetch StartBatch message but no callback function is defined")
	}
	// Call the user callback function
	return b.callbackConfig.StartBatchFunc()
}

func (b *BlockFetch) handleNoBlocks() error {
	if b.callbackConfig.NoBlocksFunc == nil {
		return fmt.Errorf("received block-fetch NoBlocks message but no callback function is defined")
	}
	// Call the user callback function
	return b.callbackConfig.NoBlocksFunc()
}

func (b *BlockFetch) handleBlock(msgGeneric protocol.Message) error {
	if b.callbackConfig.BlockFunc == nil {
		return fmt.Errorf("received block-fetch Block message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgBlock)
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := utils.CborDecode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	blk, err := block.NewBlockFromCbor(wrappedBlock.Type, wrappedBlock.RawBlock)
	if err != nil {
		return err
	}
	// Call the user callback function
	return b.callbackConfig.BlockFunc(wrappedBlock.Type, blk)
}

func (b *BlockFetch) handleBatchDone() error {
	if b.callbackConfig.BatchDoneFunc == nil {
		return fmt.Errorf("received block-fetch BatchDone message but no callback function is defined")
	}
	// Call the user callback function
	return b.callbackConfig.BatchDoneFunc()
}
