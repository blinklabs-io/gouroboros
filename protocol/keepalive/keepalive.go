package keepalive

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"time"
)

const (
	PROTOCOL_NAME        = "keep-alive"
	PROTOCOL_ID   uint16 = 8

	// Time between keep-alive probes, in seconds
	KEEP_ALIVE_PERIOD = 60
)

var (
	STATE_CLIENT = protocol.NewState(1, "Client")
	STATE_SERVER = protocol.NewState(2, "Server")
	STATE_DONE   = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_CLIENT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE,
				NewState: STATE_SERVER,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_SERVER: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE_RESPONSE,
				NewState: STATE_CLIENT,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type KeepAlive struct {
	proto          *protocol.Protocol
	callbackConfig *KeepAliveCallbackConfig
	timer          *time.Timer
}

type KeepAliveCallbackConfig struct {
	KeepAliveFunc         KeepAliveFunc
	KeepAliveResponseFunc KeepAliveResponseFunc
	DoneFunc              DoneFunc
}

// Callback function types
type KeepAliveFunc func(uint16) error
type KeepAliveResponseFunc func(uint16) error
type DoneFunc func() error

func New(options protocol.ProtocolOptions, callbackConfig *KeepAliveCallbackConfig) *KeepAlive {
	k := &KeepAlive{
		callbackConfig: callbackConfig,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               options.Muxer,
		ErrorChan:           options.ErrorChan,
		Mode:                options.Mode,
		Role:                options.Role,
		MessageHandlerFunc:  k.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_CLIENT,
	}
	k.proto = protocol.New(protoConfig)
	return k
}

func (k *KeepAlive) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_KEEP_ALIVE:
		err = k.handleKeepAlive(msg)
	case MESSAGE_TYPE_KEEP_ALIVE_RESPONSE:
		err = k.handleKeepAliveResponse(msg)
	case MESSAGE_TYPE_DONE:
		err = k.handleDone()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (k *KeepAlive) Start() {
	k.timer = time.AfterFunc(KEEP_ALIVE_PERIOD*time.Second, func() {
		if err := k.KeepAlive(0); err != nil {
			k.proto.SendError(err)
		}
	})
}

func (k *KeepAlive) Stop() {
	if k.timer != nil {
		k.timer.Stop()
	}
	// Remove timer, since we check for its presence elsewhere
	k.timer = nil
}

func (k *KeepAlive) KeepAlive(cookie uint16) error {
	msg := NewMsgKeepAlive(cookie)
	return k.proto.SendMessage(msg, false)
}

func (k *KeepAlive) handleKeepAlive(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgKeepAlive)
	if k.callbackConfig != nil && k.callbackConfig.KeepAliveFunc != nil {
		// Call the user callback function
		return k.callbackConfig.KeepAliveFunc(msg.Cookie)
	} else {
		// Send the keep-alive response
		resp := NewMsgKeepAliveResponse(msg.Cookie)
		return k.proto.SendMessage(resp, true)
	}
}

func (k *KeepAlive) handleKeepAliveResponse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgKeepAliveResponse)
	// Start the timer again if we had one previously
	if k.timer != nil {
		defer k.Start()
	}
	if k.callbackConfig != nil && k.callbackConfig.KeepAliveResponseFunc != nil {
		// Call the user callback function
		return k.callbackConfig.KeepAliveResponseFunc(msg.Cookie)
	}
	return nil
}

func (k *KeepAlive) handleDone() error {
	if k.callbackConfig != nil && k.callbackConfig.DoneFunc != nil {
		// Call the user callback function
		return k.callbackConfig.DoneFunc()
	}
	return nil
}
