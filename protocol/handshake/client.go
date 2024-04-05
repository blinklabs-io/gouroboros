// Copyright 2024 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handshake

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the Handshake client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
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
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateConfirm]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[stateConfirm] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
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
	c.onceStart.Do(func() {
		c.Protocol.Start()
		// Send our ProposeVersions message
		msg := NewMsgProposeVersions(c.config.ProtocolVersionMap)
		_ = c.SendMessage(msg)
	})
}

func (c *Client) handleMessage(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptVersion:
		err = c.handleAcceptVersion(msg)
	case MessageTypeRefuse:
		err = c.handleRefuse(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleAcceptVersion(msg protocol.Message) error {
	if c.config.FinishedFunc == nil {
		return fmt.Errorf(
			"received handshake AcceptVersion message but no callback function is defined",
		)
	}
	msgAcceptVersion := msg.(*MsgAcceptVersion)
	protoVersion := protocol.GetProtocolVersion(msgAcceptVersion.Version)
	versionData, err := protoVersion.NewVersionDataFromCborFunc(msgAcceptVersion.VersionData)
	if err != nil {
		return err
	}
	return c.config.FinishedFunc(c.callbackContext, msgAcceptVersion.Version, versionData)
}

func (c *Client) handleRefuse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgRefuse)
	var err error
	switch msg.Reason[0].(uint64) {
	case RefuseReasonVersionMismatch:
		err = fmt.Errorf("%s: version mismatch", ProtocolName)
	case RefuseReasonDecodeError:
		err = fmt.Errorf(
			"%s: decode error: %s",
			ProtocolName,
			msg.Reason[2].(string),
		)
	case RefuseReasonRefused:
		err = fmt.Errorf(
			"%s: refused: %s",
			ProtocolName,
			msg.Reason[2].(string),
		)
	}
	return err
}
