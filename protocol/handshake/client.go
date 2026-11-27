// Copyright 2026 Blink Labs Software
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
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the Handshake client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	// proposalSent is closed once the proposal is written or has failed,
	// with the failure, if any, in proposalErr.
	proposalSent chan struct{}
	proposalErr  error
}

// NewClient returns a new Handshake client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:       cfg,
		proposalSent: make(chan struct{}),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Select state map based on protocol mode
	baseStateMap := StateMapNtN
	if protoOptions.Mode == protocol.ProtocolModeNodeToClient {
		baseStateMap = StateMapNtC
	}
	stateMap := baseStateMap.Copy()
	// Override confirm timeout from config for NtN only (client waits for server response).
	// NtC has no timeouts per spec Table 3.5.
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		if entry, ok := stateMap[stateConfirm]; ok {
			entry.Timeout = c.config.Timeout
			stateMap[stateConfirm] = entry
		}
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

// Start begins the handshake process. It returns once the proposal is queued
// and the client is waiting for the answer, so a muxer started afterwards
// cannot deliver the peer's messages too early.
func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
		msg := NewMsgProposeVersions(c.config.ProtocolVersionMap)
		// The write is awaited in the background: on a connection that
		// cannot buffer it, it completes only once the peer reads.
		go func() {
			err := c.SendMessageAndWait(msg)
			// Publish the outcome before stopping: a handler waiting for it
			// runs on a loop that Stop joins.
			c.proposalErr = err
			close(c.proposalSent)
			if err == nil ||
				errors.Is(err, protocol.ErrProtocolShuttingDown) {
				return
			}
			c.Protocol.Logger().
				Warn("failed to send handshake propose versions",
					"component", "network",
					"protocol", ProtocolName,
					"connection_id", c.callbackContext.ConnectionId.String(),
					"error", err,
				)
			// Start cannot return this error, so log it and still clean up.
			_ = c.Stop(context.Background())
		}()
		// A peer that dials at the same moment sends its own proposal,
		// which is only legal once ours has moved the client to Confirm;
		// arriving in Propose it would be a protocol violation. Returning
		// after that transition lets the caller start reading safely.
		// An error here means the protocol is already shutting down and
		// has reported why.
		_ = c.WaitForStateExit(context.Background(), statePropose)
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
	case MessageTypeProposeVersions:
		err = c.handleReplyVersions(msg)
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
	proposedData, ok := c.config.ProtocolVersionMap[msgAcceptVersion.Version]
	if !ok {
		return fmt.Errorf(
			"unproposed protocol version accepted by peer: %d",
			msgAcceptVersion.Version,
		)
	}
	protoVersion := protocol.GetProtocolVersion(msgAcceptVersion.Version)
	if protoVersion.NewVersionDataFromCborFunc == nil {
		return fmt.Errorf(
			"unsupported protocol version accepted by peer: %d",
			msgAcceptVersion.Version,
		)
	}
	versionData, err := protoVersion.NewVersionDataFromCborFunc(
		msgAcceptVersion.VersionData,
	)
	if err != nil {
		return err
	}
	if versionData.NetworkMagic() != proposedData.NetworkMagic() {
		return fmt.Errorf(
			"network magic mismatch for accepted protocol version %d: got %d, want %d",
			msgAcceptVersion.Version,
			versionData.NetworkMagic(),
			proposedData.NetworkMagic(),
		)
	}
	// End with this side's own agreement of its data with the server's, as
	// the reference client does, so a server that sends anything but the
	// agreed data cannot set a flag this side did not offer.
	return c.config.FinishedFunc(
		c.callbackContext.withOwner(c.Protocol),
		msgAcceptVersion.Version,
		agreedVersionData(msgAcceptVersion.Version, proposedData, versionData),
	)
}

// handleReplyVersions handles the peer's MsgProposeVersions, received in
// Confirm when both sides dialled at once (MsgReplyVersions in the
// specification). As the reference client does, it chooses the version
// itself, with the same rule the server applies, so that both sides reach the
// same choice, and ends with the agreed version data. A refusal is returned
// as the typed error a server would have sent; nothing is sent to the peer,
// which makes its own choice and sees the connection close.
func (c *Client) handleReplyVersions(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("received peer proposal in simultaneous open",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msg, ok := msgGeneric.(*MsgProposeVersions)
	if !ok {
		return fmt.Errorf(
			"%s: malformed reply versions message: expected *MsgProposeVersions, got %T",
			ProtocolName,
			msgGeneric,
		)
	}
	if c.config.FinishedFunc == nil {
		return errors.New(
			"received handshake ReplyVersions message but no callback function is defined",
		)
	}
	// Ending the handshake stops the protocol, and the muxer drops a
	// message it has not yet written, so the peer, which is waiting for
	// this side's proposal to make its own choice, would never get it. The
	// reference cannot end first: its send is a synchronous bearer write
	// that completes before it reads (network-mux Mux/Types.hs 327-330).
	<-c.proposalSent
	if c.proposalErr != nil {
		return fmt.Errorf(
			"%s: sending proposal: %w",
			ProtocolName,
			c.proposalErr,
		)
	}
	var version uint16
	found := false
	for proposed := range msg.VersionMap {
		if _, ok := c.config.ProtocolVersionMap[proposed]; !ok {
			continue
		}
		if !found || proposed > version {
			version = proposed
			found = true
		}
	}
	if !found {
		supported := make([]uint16, 0, len(c.config.ProtocolVersionMap))
		for v := range c.config.ProtocolVersionMap {
			supported = append(supported, v)
		}
		slices.Sort(supported)
		return &VersionMismatchError{SupportedVersions: supported}
	}
	ownData := c.config.ProtocolVersionMap[version]
	decode := protocol.GetProtocolVersion(version).NewVersionDataFromCborFunc
	if ownData == nil || decode == nil {
		return &DecodeError{
			Version: version,
			Message: "no version data for the chosen version",
		}
	}
	peerData, err := decode(msg.VersionMap[version])
	if err != nil {
		return &DecodeError{Version: version, Message: err.Error()}
	}
	if peerData == nil {
		return &DecodeError{
			Version: version,
			Message: "empty version data",
		}
	}
	if peerData.NetworkMagic() != ownData.NetworkMagic() {
		return &RefusedError{
			Version: version,
			Message: "network magic mismatch",
		}
	}
	// Both sides end with the agreed data, as the reference acceptOrRefuse
	// returns it, so their connections derive the same diffusion mode.
	return c.config.FinishedFunc(
		c.callbackContext,
		version,
		agreedVersionData(ownData, peerData),
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
	if !c.proposedQuery() {
		return errors.New(
			"received handshake QueryReply message without proposing query mode",
		)
	}
	if c.config.QueryReplyFunc == nil && c.config.FinishedFunc == nil {
		return errors.New(
			"received handshake QueryReply message but neither QueryReplyFunc nor FinishedFunc is defined",
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
		if err := c.config.QueryReplyFunc(c.callbackContext.withOwner(c.Protocol), versionMap); err != nil {
			return err
		}
	}
	if c.config.FinishedFunc != nil {
		return c.config.FinishedFunc(
			c.callbackContext.withOwner(c.Protocol),
			0,
			nil,
		)
	}
	return nil
}

// proposedQuery reports whether any proposed version requested query mode,
// the only case in which the server may answer with MsgQueryReply.
func (c *Client) proposedQuery() bool {
	for _, versionData := range c.config.ProtocolVersionMap {
		if versionData != nil && versionData.Query() {
			return true
		}
	}
	return false
}
