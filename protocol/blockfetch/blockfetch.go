package blockfetch

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
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

type BlockFetch struct {
	proto          *protocol.Protocol
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
	b := &BlockFetch{
		callbackConfig: callbackConfig,
	}
	b.proto = protocol.New(PROTOCOL_NAME, PROTOCOL_ID, m, errorChan, b.messageHandler, NewMsgFromCbor)
	// Set initial state
	b.proto.SetState(STATE_IDLE)
	return b
}

func (b *BlockFetch) messageHandler(msg protocol.Message) error {
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
	if err := b.proto.LockState([]protocol.State{STATE_IDLE}); err != nil {
		return fmt.Errorf("%s: RequestRange: protocol not in expected state", PROTOCOL_NAME)
	}
	msg := newMsgRequestRange(start, end)
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_BUSY)
	// Send request
	return b.proto.SendMessage(msg, false)
}

func (b *BlockFetch) ClientDone() error {
	if err := b.proto.LockState([]protocol.State{STATE_IDLE}); err != nil {
		return fmt.Errorf("%s: ClientDone: protocol not in expected state", PROTOCOL_NAME)
	}
	msg := newMsgClientDone()
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_BUSY)
	// Send request
	return b.proto.SendMessage(msg, false)
}

func (b *BlockFetch) handleStartBatch() error {
	if err := b.proto.LockState([]protocol.State{STATE_BUSY}); err != nil {
		return fmt.Errorf("received block-fetch StartBatch message when protocol not in expected state")
	}
	if b.callbackConfig.StartBatchFunc == nil {
		return fmt.Errorf("received block-fetch StartBatch message but no callback function is defined")
	}
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_STREAMING)
	// Call the user callback function
	return b.callbackConfig.StartBatchFunc()
}

func (b *BlockFetch) handleNoBlocks() error {
	if err := b.proto.LockState([]protocol.State{STATE_BUSY}); err != nil {
		return fmt.Errorf("received block-fetch NoBlocks message when protocol not in expected state")
	}
	if b.callbackConfig.NoBlocksFunc == nil {
		return fmt.Errorf("received block-fetch NoBlocks message but no callback function is defined")
	}
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_IDLE)
	// Call the user callback function
	return b.callbackConfig.NoBlocksFunc()
}

func (b *BlockFetch) handleBlock(msgGeneric protocol.Message) error {
	if err := b.proto.LockState([]protocol.State{STATE_STREAMING}); err != nil {
		return fmt.Errorf("received block-fetch Block message when protocol not in expected state")
	}
	if b.callbackConfig.BlockFunc == nil {
		return fmt.Errorf("received block-fetch Block message but no callback function is defined")
	}
	msg := msgGeneric.(*msgBlock)
	// Decode only enough to get the block type value
	var wrapBlock wrappedBlock
	if _, err := utils.CborDecode(msg.WrappedBlock, &wrapBlock); err != nil {
		return fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	blk, err := block.NewBlockFromCbor(wrapBlock.Type, wrapBlock.RawBlock)
	if err != nil {
		return err
	}
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_STREAMING)
	// Call the user callback function
	return b.callbackConfig.BlockFunc(wrapBlock.Type, blk)
}

func (b *BlockFetch) handleBatchDone() error {
	if err := b.proto.LockState([]protocol.State{STATE_STREAMING}); err != nil {
		return fmt.Errorf("received block-fetch BatchDone message when protocol not in expected state")
	}
	if b.callbackConfig.BatchDoneFunc == nil {
		return fmt.Errorf("received block-fetch BatchDone message but no callback function is defined")
	}
	// Unlock and change state when we're done
	defer b.proto.UnlockState(STATE_IDLE)
	// Call the user callback function
	return b.callbackConfig.BatchDoneFunc()
}
