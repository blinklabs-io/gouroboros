package handshake

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the Handshake client
type Client struct {
	*protocol.Protocol
	config *Config
}

// NewClient returns a new Handshake client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateConfirm]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[stateConfirm] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                protocolName,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        statePropose,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

// Start begins the handshake process
func (c *Client) Start() {
	c.Protocol.Start()
	// Send our ProposeVersions message
	versionMap := make(map[uint16]interface{})
	diffusionMode := DiffusionModeInitiatorOnly
	if c.config.ClientFullDuplex {
		diffusionMode = DiffusionModeInitiatorAndResponder
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
	case MessageTypeAcceptVersion:
		err = c.handleAcceptVersion(msg)
	case MessageTypeRefuse:
		err = c.handleRefuse(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
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
		if versionData[1].(bool) == DiffusionModeInitiatorAndResponder {
			fullDuplex = true
		}
	}
	return c.config.FinishedFunc(msg.Version, fullDuplex)
}

func (c *Client) handleRefuse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgRefuse)
	var err error
	switch msg.Reason[0].(uint64) {
	case RefuseReasonVersionMismatch:
		err = fmt.Errorf("%s: version mismatch", protocolName)
	case RefuseReasonDecodeError:
		err = fmt.Errorf("%s: decode error: %s", protocolName, msg.Reason[2].(string))
	case RefuseReasonRefused:
		err = fmt.Errorf("%s: refused: %s", protocolName, msg.Reason[2].(string))
	}
	return err
}
