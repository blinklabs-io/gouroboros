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
	"slices"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the Handshake server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
}

// NewServer returns a new Handshake server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[statePropose]; ok {
		entry.Timeout = s.config.Timeout
		stateMap[statePropose] = entry
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        statePropose,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) handleMessage(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeProposeVersions:
		err = s.handleProposeVersions(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleProposeVersions(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("propose versions",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.FinishedFunc == nil {
		return errors.New(
			"received handshake ProposeVersions message but no callback function is defined",
		)
	}
	msgProposeVersions := msg.(*MsgProposeVersions)

	for proposedVersion, versionDataCbor := range msgProposeVersions.VersionMap {
		versionInfo := protocol.GetProtocolVersion(proposedVersion)
		if versionInfo.NewVersionDataFromCborFunc != nil {
			proposedVersionData, err := versionInfo.NewVersionDataFromCborFunc(
				versionDataCbor,
			)
			if err == nil && proposedVersionData != nil && proposedVersionData.Query() {
				msgQueryReply := NewMsgQueryReply(s.config.ProtocolVersionMap)
				if err := s.SendMessage(msgQueryReply); err != nil {
					return err
				}
				return errors.New("handshake query mode: connection terminated after query reply")
			}
		}
	}

	// Compute intersection of supported and proposed protocol versions
	var versionIntersect []uint16
	for proposedVersion := range msgProposeVersions.VersionMap {
		if _, ok := s.config.ProtocolVersionMap[proposedVersion]; ok {
			versionIntersect = append(versionIntersect, proposedVersion)
		}
	}
	// Send refusal if there are no matching versions
	if len(versionIntersect) == 0 {
		var supportedVersions []uint16
		for supportedVersion := range s.config.ProtocolVersionMap {
			supportedVersions = append(supportedVersions, supportedVersion)
		}

		// sort asending - iterating over map is not deterministic
		slices.Sort(supportedVersions)

		msgRefuse := NewMsgRefuse(
			[]any{
				RefuseReasonVersionMismatch,
				supportedVersions,
			},
		)
		if err := s.SendMessage(msgRefuse); err != nil {
			return err
		}
		return errors.New("handshake failed: refused due to version mismatch")
	}
	// Compute highest version from intersection
	var proposedVersion uint16
	for _, version := range versionIntersect {
		if version > proposedVersion {
			proposedVersion = version
		}
	}
	// Decode protocol parameters for selected version
	versionInfo := protocol.GetProtocolVersion(proposedVersion)
	versionData := s.config.ProtocolVersionMap[proposedVersion]
	if versionData == nil {
		msgRefuse := NewMsgRefuse(
			[]any{
				RefuseReasonDecodeError,
				proposedVersion,
				errors.New(
					"handshake failed: refused due to empty version data",
				),
			},
		)
		if err := s.SendMessage(msgRefuse); err != nil {
			return err
		}
		return errors.New("handshake failed: refused due to empty version data")
	}
	proposedVersionData, err := versionInfo.NewVersionDataFromCborFunc(
		msgProposeVersions.VersionMap[proposedVersion],
	)
	if err != nil {
		msgRefuse := NewMsgRefuse(
			[]any{
				RefuseReasonDecodeError,
				proposedVersion,
				err.Error(),
			},
		)
		if err := s.SendMessage(msgRefuse); err != nil {
			return err
		}
		return fmt.Errorf(
			"handshake failed: refused due to protocol parameters decode failure: %w",
			err,
		)
	}
	if proposedVersionData == nil {
		msgRefuse := NewMsgRefuse(
			[]any{
				RefuseReasonDecodeError,
				proposedVersion,
				errors.New(
					"handshake failed: refused due to empty version map",
				),
			},
		)
		if err := s.SendMessage(msgRefuse); err != nil {
			return err
		}
		return errors.New("handshake failed: refused due to empty version map")
	}

	// Check network magic
	if proposedVersionData.NetworkMagic() != versionData.NetworkMagic() {
		errMsg := fmt.Sprintf(
			"network magic mismatch: %#v /= %#v",
			versionData,
			proposedVersionData,
		)
		msgRefuse := NewMsgRefuse(
			[]any{
				RefuseReasonRefused,
				proposedVersion,
				errMsg,
			},
		)
		if err := s.SendMessage(msgRefuse); err != nil {
			return err
		}
		return fmt.Errorf(
			"handshake failed: refused due to protocol parameters mismatch: %s",
			errMsg,
		)
	}
	// Accept the proposed version
	// We send our version data in the response and the proposed version data in the callback
	msgAcceptVersion := NewMsgAcceptVersion(proposedVersion, versionData)
	if err := s.SendMessage(msgAcceptVersion); err != nil {
		return err
	}
	return s.config.FinishedFunc(
		s.callbackContext,
		proposedVersion,
		proposedVersionData,
	)
}
