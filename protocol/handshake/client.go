// Copyright 2023 Blink Labs, LLC.
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
	config    *Config
	onceStart sync.Once
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
		versionMap := make(map[uint16]interface{})
		diffusionMode := DiffusionModeInitiatorOnly
		if c.config.ClientFullDuplex {
			diffusionMode = DiffusionModeInitiatorAndResponder
		}
		for _, version := range c.config.ProtocolVersions {
			if c.Mode() == protocol.ProtocolModeNodeToNode {
				if version >= 11 {
					// TODO: make peer sharing mode configurable once it actually works
					versionMap[version] = NtNVersionDataPeerSharingQuery{
						NetworkMagic:                       c.config.NetworkMagic,
						InitiatorAndResponderDiffusionMode: diffusionMode,
						PeerSharing:                        PeerSharingModeNoPeerSharing,
						Query:                              QueryModeDisabled,
					}
				} else {
					versionMap[version] = NtNVersionDataLegacy{
						NetworkMagic:                       c.config.NetworkMagic,
						InitiatorAndResponderDiffusionMode: diffusionMode,
					}
				}
			} else {
				if (version - NodeToClientVersionOffset) >= 15 {
					versionMap[version] = NtCVersionData{
						NetworkMagic: c.config.NetworkMagic,
						Query:        QueryModeDisabled,
					}
				} else {
					versionMap[version] = c.config.NetworkMagic
				}
			}
		}
		msg := NewMsgProposeVersions(versionMap)
		_ = c.SendMessage(msg)
	})
}

func (c *Client) handleMessage(msg protocol.Message, isResponse bool) error {
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

func (c *Client) handleAcceptVersion(msgGeneric protocol.Message) error {
	if c.config.FinishedFunc == nil {
		return fmt.Errorf(
			"received handshake AcceptVersion message but no callback function is defined",
		)
	}
	msg := msgGeneric.(*MsgAcceptVersion)
	fullDuplex := false
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		// TODO: switch to using the VersionData types
		// this is more annoying than it would seem until we fix some other things
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
