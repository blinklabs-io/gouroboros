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

package localtxmonitor

import (
	"fmt"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalTxMonitor server
type Server struct {
	*protocol.Protocol
	config *Config
}

// NewServer returns a new Server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                protocolName,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquire:
		err = s.handleAcquire()
	case MessageTypeDone:
		err = s.handleDone()
	case MessageTypeRelease:
		err = s.handleRelease()
	case MessageTypeHasTx:
		err = s.handleHasTx(msg)
	case MessageTypeNextTx:
		err = s.handleNextTx()
	case MessageTypeGetSizes:
		err = s.handleGetSizes()
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
	}
	return err
}

func (s *Server) handleAcquire() error {
	return nil
}

func (s *Server) handleDone() error {
	return nil
}

func (s *Server) handleRelease() error {
	return nil
}

func (s *Server) handleHasTx(msg protocol.Message) error {
	return nil
}

func (s *Server) handleNextTx() error {
	return nil
}

func (s *Server) handleGetSizes() error {
	return nil
}
