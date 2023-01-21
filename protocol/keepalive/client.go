package keepalive

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"time"
)

type Client struct {
	*protocol.Protocol
	config *Config
	timer  *time.Timer
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
	}
	// Update state map with timeout
	stateMap := StateMap
	if entry, ok := stateMap[STATE_SERVER]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[STATE_SERVER] = entry
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
		InitialState:        STATE_CLIENT,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.Protocol.Start()
	c.startTimer()
}

func (c *Client) startTimer() {
	c.timer = time.AfterFunc(c.config.Period, func() {
		msg := NewMsgKeepAlive(0)
		if err := c.SendMessage(msg); err != nil {
			c.SendError(err)
		}
	})
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_KEEP_ALIVE_RESPONSE:
		err = c.handleKeepAliveResponse(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) handleKeepAliveResponse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgKeepAliveResponse)
	// Start the timer again if we had one previously
	if c.timer != nil {
		defer c.startTimer()
	}
	if c.config != nil && c.config.KeepAliveResponseFunc != nil {
		// Call the user callback function
		return c.config.KeepAliveResponseFunc(msg.Cookie)
	}
	return nil
}
