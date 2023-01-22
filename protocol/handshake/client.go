package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

type Client struct {
	*protocol.Protocol
	config *Config
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
	if entry, ok := stateMap[STATE_CONFIRM]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[STATE_CONFIRM] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_PROPOSE,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.Protocol.Start()
	// Send our ProposeVersions message
	versionMap := make(map[uint16]interface{})
	diffusionMode := DIFFUSION_MODE_INITIATOR_ONLY
	if c.config.ClientFullDuplex {
		diffusionMode = DIFFUSION_MODE_INITIATOR_AND_RESPONDER
	}
	for _, version := range c.config.ProtocolVersions {
		if c.Mode() == protocol.ProtocolModeNodeToNode {
			versionMap[version] = []interface{}{c.config.NetworkMagic, diffusionMode}
		} else {
			versionMap[version] = c.config.NetworkMagic
		}
	}
	msg := NewMsgProposeVersions(versionMap)
	_ = c.SendMessage(msg)
}

func (c *Client) handleMessage(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACCEPT_VERSION:
		err = c.handleAcceptVersion(msg)
	case MESSAGE_TYPE_REFUSE:
		err = c.handleRefuse(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (c *Client) handleAcceptVersion(msgGeneric protocol.Message) error {
	if c.config.FinishedFunc == nil {
		return fmt.Errorf("received handshake AcceptVersion message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgAcceptVersion)
	fullDuplex := false
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		versionData := msg.VersionData.([]interface{})
		//nolint:gosimple
		if versionData[1].(bool) == DIFFUSION_MODE_INITIATOR_AND_RESPONDER {
			fullDuplex = true
		}
	}
	return c.config.FinishedFunc(msg.Version, fullDuplex)
}

func (c *Client) handleRefuse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgRefuse)
	var err error
	switch msg.Reason[0].(uint64) {
	case REFUSE_REASON_VERSION_MISMATCH:
		err = fmt.Errorf("%s: version mismatch", PROTOCOL_NAME)
	case REFUSE_REASON_DECODE_ERROR:
		err = fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, msg.Reason[2].(string))
	case REFUSE_REASON_REFUSED:
		err = fmt.Errorf("%s: refused: %s", PROTOCOL_NAME, msg.Reason[2].(string))
	}
	return err
}
