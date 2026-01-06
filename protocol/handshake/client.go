// Copyright 2025 Blink Labs Software
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
	"errors"
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
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
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
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
		// Send our ProposeVersions message
		msg := NewMsgProposeVersions(c.config.ProtocolVersionMap)
		_ = c.SendMessage(msg)
	})
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptVersion:
		err = c.handleAcceptVersion(msg)
	case MessageTypeRefuse:
		err = c.handleRefuse(msg)
	case MessageTypeQueryReply:
		err = c.handleQueryReply(msg)
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
	c.Protocol.Logger().
		Debug("accepted version negotiation",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if c.config.FinishedFunc == nil {
		return errors.New(
			"received handshake AcceptVersion message but no callback function is defined",
		)
	}
	msgAcceptVersion := msg.(*MsgAcceptVersion)
	protoVersion := protocol.GetProtocolVersion(msgAcceptVersion.Version)
	versionData, err := protoVersion.NewVersionDataFromCborFunc(
		msgAcceptVersion.VersionData,
	)
	if err != nil {
		return err
	}
	return c.config.FinishedFunc(
		c.callbackContext,
		msgAcceptVersion.Version,
		versionData,
	)
}

func (c *Client) handleRefuse(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("refused handshake",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Use checked type assertion
	msg, ok := msgGeneric.(*MsgRefuse)
	if !ok {
		return fmt.Errorf(
			"%s: malformed refuse message: expected *MsgRefuse, got %T",
			ProtocolName,
			msgGeneric,
		)
	}

	// Check for empty reason array
	if len(msg.Reason) == 0 {
		return fmt.Errorf(
			"%s: malformed refuse message: empty reason",
			ProtocolName,
		)
	}

	// Extract and validate reason code
	reasonCode, ok := msg.Reason[0].(uint64)
	if !ok {
		return fmt.Errorf(
			"%s: malformed refuse message: reason code must be uint64, got %T",
			ProtocolName,
			msg.Reason[0],
		)
	}

	// Parse based on reason code
	switch reasonCode {
	case RefuseReasonVersionMismatch:
		// Format: [0, [*anyVersionNumber]]
		if len(msg.Reason) < 2 {
			return fmt.Errorf(
				"%s: malformed version mismatch refusal: missing version numbers array",
				ProtocolName,
			)
		}
		var supportedVersions []uint16
		switch v := msg.Reason[1].(type) {
		case []uint16:
			// Direct []uint16 array (from server encoding)
			supportedVersions = v
		case []any:
			// []any array (from CBOR decoding)
			supportedVersions = make([]uint16, 0, len(v))
			for i, versionVal := range v {
				var versionNum uint64
				switch vv := versionVal.(type) {
				case uint64:
					versionNum = vv
				case uint16:
					versionNum = uint64(vv)
				case uint32:
					versionNum = uint64(vv)
				case uint8:
					versionNum = uint64(vv)
				default:
					return fmt.Errorf(
						"%s: malformed version mismatch refusal: version number at index %d must be numeric, got %T",
						ProtocolName,
						i,
						versionVal,
					)
				}
				if versionNum > 65535 {
					return fmt.Errorf(
						"%s: malformed version mismatch refusal: version number at index %d exceeds uint16 max value",
						ProtocolName,
						i,
					)
				}
				supportedVersions = append(supportedVersions, uint16(versionNum))
			}
		default:
			return fmt.Errorf(
				"%s: malformed version mismatch refusal: expected array of version numbers, got %T",
				ProtocolName,
				msg.Reason[1],
			)
		}
		return &VersionMismatchError{
			SupportedVersions: supportedVersions,
		}

	case RefuseReasonDecodeError:
		// Format: [1, anyVersionNumber, tstr]
		if len(msg.Reason) < 3 {
			return fmt.Errorf(
				"%s: malformed decode error refusal: expected 3 elements, got %d",
				ProtocolName,
				len(msg.Reason),
			)
		}
		var versionNum uint64
		switch v := msg.Reason[1].(type) {
		case uint64:
			versionNum = v
		case uint16:
			versionNum = uint64(v)
		case uint32:
			versionNum = uint64(v)
		case uint8:
			versionNum = uint64(v)
		default:
			return fmt.Errorf(
				"%s: malformed decode error refusal: version number must be numeric, got %T",
				ProtocolName,
				msg.Reason[1],
			)
		}
		if versionNum > 65535 {
			return fmt.Errorf(
				"%s: malformed decode error refusal: version number exceeds uint16 max value",
				ProtocolName,
			)
		}
		errMsg, ok := msg.Reason[2].(string)
		if !ok {
			return fmt.Errorf(
				"%s: malformed decode error refusal: error message must be string, got %T",
				ProtocolName,
				msg.Reason[2],
			)
		}
		return &DecodeError{
			Version: uint16(versionNum),
			Message: errMsg,
		}

	case RefuseReasonRefused:
		// Format: [2, anyVersionNumber, tstr]
		if len(msg.Reason) < 3 {
			return fmt.Errorf(
				"%s: malformed refused error: expected 3 elements, got %d",
				ProtocolName,
				len(msg.Reason),
			)
		}
		var versionNum uint64
		switch v := msg.Reason[1].(type) {
		case uint64:
			versionNum = v
		case uint16:
			versionNum = uint64(v)
		case uint32:
			versionNum = uint64(v)
		case uint8:
			versionNum = uint64(v)
		default:
			return fmt.Errorf(
				"%s: malformed refused error: version number must be numeric, got %T",
				ProtocolName,
				msg.Reason[1],
			)
		}
		if versionNum > 65535 {
			return fmt.Errorf(
				"%s: malformed refused error: version number exceeds uint16 max value",
				ProtocolName,
			)
		}
		errMsg, ok := msg.Reason[2].(string)
		if !ok {
			return fmt.Errorf(
				"%s: malformed refused error: reason message must be string, got %T",
				ProtocolName,
				msg.Reason[2],
			)
		}
		return &RefusedError{
			Version: uint16(versionNum),
			Message: errMsg,
		}

	default:
		return fmt.Errorf(
			"%s: unknown refuse reason: %d",
			ProtocolName,
			reasonCode,
		)
	}
}

func (c *Client) handleQueryReply(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("received query reply",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Use checked type assertion
	msg, ok := msgGeneric.(*MsgQueryReply)
	if !ok {
		return fmt.Errorf(
			"%s: malformed query reply message: expected *MsgQueryReply, got %T",
			ProtocolName,
			msgGeneric,
		)
	}
	if c.config.FinishedFunc == nil {
		return errors.New(
			"received handshake QueryReply message but no callback function is defined",
		)
	}
	versionMap := protocol.ProtocolVersionMap{}
	for version, versionDataCbor := range msg.VersionMap {
		versionInfo := protocol.GetProtocolVersion(version)
		if versionInfo.NewVersionDataFromCborFunc != nil {
			versionData, err := versionInfo.NewVersionDataFromCborFunc(
				versionDataCbor,
			)
			if err == nil && versionData != nil {
				versionMap[version] = versionData
			}
		}
	}
	if c.config.QueryReplyFunc != nil {
		if err := c.config.QueryReplyFunc(c.callbackContext, versionMap); err != nil {
			return err
		}
	}
	return c.config.FinishedFunc(
		c.callbackContext,
		0,
		nil,
	)
}
