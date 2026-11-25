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
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server runs the outbound/server side of ObjectDiffusion.
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protocolMu      sync.RWMutex
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
}

// NewServer creates a server endpoint with the supplied protocol options.
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	cfg = normalizeConfig(cfg)
	s := &Server{config: cfg, protoOptions: protoOptions}
	s.callbackContext = CallbackContext{
		Server: s, ConnectionId: protoOptions.ConnectionId,
		ConnectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	s.initProtocol()
	return s
}

// initProtocol installs a new protocol instance for the next run, unless the
// server was stopped. It reports whether it installed one.
func (s *Server) initProtocol() bool {
	p := protocol.New(protocol.ProtocolConfig{
		Name: ProtocolName, ProtocolId: ProtocolId,
		Muxer: s.protoOptions.Muxer, Logger: s.protoOptions.Logger,
		ErrorChan: s.protoOptions.ErrorChan, Mode: s.protoOptions.Mode,
		Role: protocol.ProtocolRoleServer, MessageHandlerFunc: s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateContext:        newStateContext(s.config.MaxObjectsUnacknowledged),
		StateMap:            clientStateMap(s.config), InitialState: StateInit,
	})
	s.protocolMu.Lock()
	defer s.protocolMu.Unlock()
	if s.stopped {
		return false
	}
	s.Protocol = p
	return true
}

// ProtocolInstance returns the protocol instance of the current run. The
// server replaces it after each MsgDone.
func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
}

// Start starts the server endpoint.
func (s *Server) Start() { s.ProtocolInstance().Start() }

// Stop interrupts the server endpoint. A restart that is in progress when
// Stop is called does not start a new protocol instance.
//
// Stop signals shutdown at once. When ctx is the Context of a
// CallbackContext this server passed to a callback, Stop returns without
// waiting, since the goroutine running the callback is one it would wait
// for. Otherwise it waits until the protocol's goroutines have returned, or
// until ctx is done, in which case it returns ctx.Err() and shutdown still
// proceeds.
func (s *Server) Stop(ctx context.Context) error {
	s.protocolMu.Lock()
	s.stopped = true
	p := s.Protocol
	s.protocolMu.Unlock()
	return p.Stop(ctx)
}

func (s *Server) messageHandler(msg protocol.Message) error {
	switch msg.Type() {
	case MessageTypeInit:
		return nil
	case MessageTypeDone:
		s.handleDone()
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
	proto := s.ProtocolInstance()
	if s.config.ObjectIDsFunc == nil {
		return errors.New(
			"peras object IDs function must be configured on the server",
		)
	}
	ids, err := s.config.ObjectIDsFunc(
		s.callbackContext.withOwner(s.Protocol),
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
		// A blocking reply cannot be empty, and the client gives up after
		// CanAwaitTimeout, so say promptly that the answer will come later.
		if err := proto.SendMessage(NewMsgAwaitReply()); err != nil {
			return err
		}
		go s.awaitObjectIDs(proto, msg.RequestCount)
		return nil
	}
	return proto.SendMessage(NewMsgReplyObjectIDs(ids))
}

// awaitObjectIDs finishes a blocking request after MsgAwaitReply: the IDs
// that arrive within AwaitTimeout, or MsgServerIdle. It runs off the receive
// path; the client has no agency until it answers, so nothing else is
// handled meanwhile. It answers on proto, the instance of the run that made
// the request.
func (s *Server) awaitObjectIDs(proto *protocol.Protocol, requestCount uint16) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		s.config.AwaitTimeout,
	)
	defer cancel()
	go func() {
		select {
		case <-proto.DoneChan():
		case <-s.callbackContext.ConnectionDoneChan:
		case <-ctx.Done():
		}
		cancel()
	}()
	var ids []VoteID
	if s.config.AwaitObjectIDsFunc == nil {
		<-ctx.Done()
	} else {
		var err error
		ids, err = s.config.AwaitObjectIDsFunc(
			ctx,
			s.callbackContext.withOwner(s.Protocol),
			requestCount,
		)
		if err != nil && ctx.Err() == nil {
			proto.SendError(err)
			return
		}
	}
	select {
	case <-proto.DoneChan():
		return
	case <-s.callbackContext.ConnectionDoneChan:
		return
	default:
	}
	if len(ids) > int(requestCount) {
		proto.SendError(fmt.Errorf(
			"peras await object IDs function returned %d IDs for a request of %d",
			len(ids),
			requestCount,
		))
		return
	}
	var reply protocol.Message = NewMsgServerIdle()
	if len(ids) > 0 {
		reply = NewMsgReplyObjectIDs(ids)
	}
	if err := proto.SendMessage(reply); err != nil {
		proto.SendError(err)
	}
}

func (s *Server) handleRequestObjects(msg *MsgRequestObjects) error {
	if s.config.ObjectsFunc == nil {
		return errors.New(
			"peras objects function must be configured on the server",
		)
	}
	objects, err := s.config.ObjectsFunc(s.callbackContext.withOwner(s.Protocol), msg.ObjectIDs)
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
			return fmt.Errorf(
				"peras objects function returned unrequested vote %v",
				id,
			)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf(
				"peras objects function returned duplicate vote %v",
				id,
			)
		}
		seen[id] = struct{}{}
	}
	return s.ProtocolInstance().SendMessage(NewMsgReplyObjects(objects))
}

// handleDone restarts the server in its initial state. The reference runs
// this side as a responder, and the inbound governor restarts a responder
// that ended cleanly, so the client may start a new run on the connection.
func (s *Server) handleDone() {
	old := s.ProtocolInstance()
	old.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	_ = old.StopForRestart(old.CallbackContext())
	if !s.initProtocol() {
		old.CancelRestart()
		return
	}
	s.Start()
}
