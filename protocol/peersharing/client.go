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

package peersharing

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the PeerSharing client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	sharePeersChan  chan []PeerAddress
}

// NewClient returns a new PeerSharing client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:         cfg,
		sharePeersChan: make(chan []PeerAddress),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateBusy]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[stateBusy] = entry
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
		InitialState:        stateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) GetPeers(amount uint8) ([]PeerAddress, error) {
	msg := NewMsgShareRequest(amount)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	peers, ok := <-c.sharePeersChan
	if !ok {
		return nil, protocol.ProtocolShuttingDownError
	}
	return peers, nil
}

func (c *Client) handleMessage(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeSharePeers:
		err = c.handleSharePeers(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleSharePeers(msg protocol.Message) error {
	msgSharePeers := msg.(*MsgSharePeers)
	c.sharePeersChan <- msgSharePeers.PeerAddresses
	return nil
}
