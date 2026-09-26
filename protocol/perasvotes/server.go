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

package perasvotes

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server runs the outbound/server side of ObjectDiffusion.
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
}

// NewServer creates a server endpoint with the supplied protocol options.
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	cfg = normalizeConfig(cfg)
	s := &Server{config: cfg}
	s.callbackContext = CallbackContext{
		Server: s, ConnectionId: protoOptions.ConnectionId,
		ConnectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	s.Protocol = protocol.New(protocol.ProtocolConfig{
		Name: ProtocolName, ProtocolId: ProtocolId,
		Muxer: protoOptions.Muxer, Logger: protoOptions.Logger,
		ErrorChan: protoOptions.ErrorChan, Mode: protoOptions.Mode,
		Role: protocol.ProtocolRoleServer, MessageHandlerFunc: s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateContext:        newStateContext(cfg.MaxObjectsUnacknowledged),
		StateMap:            clientStateMap(cfg), InitialState: StateInit,
	})
	return s
}

// Start starts the server endpoint.
func (s *Server) Start() { s.Protocol.Start() }

// Stop interrupts the server endpoint.
func (s *Server) Stop() { s.Protocol.Stop() }

func (s *Server) messageHandler(msg protocol.Message) error {
	switch msg.Type() {
	case MessageTypeInit, MessageTypeDone:
		return nil
	case MessageTypeRequestObjectIDs:
		return s.handleRequestObjectIDs(msg.(*MsgRequestObjectIDs))
	case MessageTypeRequestObjects:
		return s.handleRequestObjects(msg.(*MsgRequestObjects))
	default:
		return fmt.Errorf(
			"%s: server received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
}

func (s *Server) handleRequestObjectIDs(msg *MsgRequestObjectIDs) error {
	if s.config.ObjectIDsFunc == nil {
		return errors.New("peras object IDs function must be configured on the server")
	}
	ids, err := s.config.ObjectIDsFunc(
		s.callbackContext,
		msg.AckCount,
		msg.RequestCount,
	)
	if err != nil {
		return err
	}
	if len(ids) > int(msg.RequestCount) {
		return fmt.Errorf(
			"peras object IDs function returned %d IDs for a request of %d",
			len(ids),
			msg.RequestCount,
		)
	}
	if msg.Blocking && len(ids) == 0 {
		return errors.New(
			"blocking Peras object ID request requires a non-empty reply",
		)
	}
	return s.SendMessage(NewMsgReplyObjectIDs(ids))
}

func (s *Server) handleRequestObjects(msg *MsgRequestObjects) error {
	if s.config.ObjectsFunc == nil {
		return errors.New("peras objects function must be configured on the server")
	}
	objects, err := s.config.ObjectsFunc(s.callbackContext, msg.ObjectIDs)
	if err != nil {
		return err
	}
	if len(objects) > len(msg.ObjectIDs) {
		return fmt.Errorf(
			"peras objects function returned %d objects for a request of %d",
			len(objects),
			len(msg.ObjectIDs),
		)
	}
	requested := make(map[VoteID]struct{}, len(msg.ObjectIDs))
	for _, id := range msg.ObjectIDs {
		requested[id] = struct{}{}
	}
	seen := make(map[VoteID]struct{}, len(objects))
	for _, object := range objects {
		id, err := object.VoteID()
		if err != nil {
			return err
		}
		if _, ok := requested[id]; !ok {
			return fmt.Errorf("peras objects function returned unrequested vote %v", id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("peras objects function returned duplicate vote %v", id)
		}
		seen[id] = struct{}{}
	}
	return s.SendMessage(NewMsgReplyObjects(objects))
}
