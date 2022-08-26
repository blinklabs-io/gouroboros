package chainsync

import (
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME          = "chain-sync"
	PROTOCOL_ID_NTN uint16 = 2
	PROTOCOL_ID_NTC uint16 = 5
)

var (
	STATE_IDLE       = protocol.NewState(1, "Idle")
	STATE_CAN_AWAIT  = protocol.NewState(2, "CanAwait")
	STATE_MUST_REPLY = protocol.NewState(3, "MustReply")
	STATE_INTERSECT  = protocol.NewState(4, "Intersect")
	STATE_DONE       = protocol.NewState(5, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_NEXT,
				NewState: STATE_CAN_AWAIT,
			},
			{
				MsgType:  MESSAGE_TYPE_FIND_INTERSECT,
				NewState: STATE_INTERSECT,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_CAN_AWAIT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_AWAIT_REPLY,
				NewState: STATE_MUST_REPLY,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_FORWARD,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_BACKWARD,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_INTERSECT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_INTERSECT_FOUND,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_INTERSECT_NOT_FOUND,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_MUST_REPLY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ROLL_FORWARD,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_BACKWARD,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type ChainSync struct {
	*protocol.Protocol
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

func New(options protocol.ProtocolOptions) *ChainSync {
	// Use node-to-client protocol ID
	protocolId := PROTOCOL_ID_NTC
	msgFromCborFunc := NewMsgFromCborNtC
	if options.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		protocolId = PROTOCOL_ID_NTN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	c := &ChainSync{}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          protocolId,
		Muxer:               options.Muxer,
		ErrorChan:           options.ErrorChan,
		Mode:                options.Mode,
		Role:                options.Role,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *ChainSync) Start(callbackConfig *ChainSyncCallbackConfig) {
	c.callbackConfig = callbackConfig
	c.Protocol.Start()
}

func (c *ChainSync) messageHandler(msg protocol.Message, isResponse bool) error {
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
	msg := NewMsgRequestNext()
	return c.SendMessage(msg)
}

func (c *ChainSync) FindIntersect(points []Point) error {
	msg := NewMsgFindIntersect(points)
	return c.SendMessage(msg)
}

func (c *ChainSync) handleAwaitReply() error {
	if c.callbackConfig.AwaitReplyFunc == nil {
		return fmt.Errorf("received chain-sync AwaitReply message but no callback function is defined")
	}
	// Call the user callback function
	return c.callbackConfig.AwaitReplyFunc()
}

func (c *ChainSync) handleRollForward(msgGeneric protocol.Message) error {
	if c.callbackConfig.RollForwardFunc == nil {
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
		return c.callbackConfig.RollForwardFunc(blockType, blockHeader)
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		blk, err := ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
		if err != nil {
			return err
		}
		// Call the user callback function
		return c.callbackConfig.RollForwardFunc(msg.BlockType(), blk)
	}
}

func (c *ChainSync) handleRollBackward(msgGeneric protocol.Message) error {
	if c.callbackConfig.RollBackwardFunc == nil {
		return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgRollBackward)
	// Call the user callback function
	return c.callbackConfig.RollBackwardFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectFound(msgGeneric protocol.Message) error {
	if c.callbackConfig.IntersectFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectFound message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgIntersectFound)
	// Call the user callback function
	return c.callbackConfig.IntersectFoundFunc(msg.Point, msg.Tip)
}

func (c *ChainSync) handleIntersectNotFound(msgGeneric protocol.Message) error {
	if c.callbackConfig.IntersectNotFoundFunc == nil {
		return fmt.Errorf("received chain-sync IntersectNotFound message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgIntersectNotFound)
	// Call the user callback function
	return c.callbackConfig.IntersectNotFoundFunc(msg.Tip)
}

func (c *ChainSync) handleDone() error {
	if c.callbackConfig.DoneFunc == nil {
		return fmt.Errorf("received chain-sync Done message but no callback function is defined")
	}
	// Call the user callback function
	return c.callbackConfig.DoneFunc()
}
