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

package blockfetch

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the Block Fetch protocol server, which serves blocks to clients.
type Server struct {
	*protocol.Protocol
	config          *Config                  // Protocol configuration
	callbackContext CallbackContext          // Callback context for server
	protoOptions    protocol.ProtocolOptions // Protocol options
}

// NewServer creates a new Block Fetch protocol server with the given options and configuration.
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
		// Save this for re-use later
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.initProtocol()
	return s
}

func (s *Server) initProtocol() {
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        StateIdle,
	}
	if s.config != nil {
		protoConfig.RecvQueueSize = s.config.RecvQueueSize
	}
	s.Protocol = protocol.New(protoConfig)
}

// NoBlocks sends a NoBlocks message to the client, indicating no blocks are available.
func (s *Server) NoBlocks() error {
	s.Protocol.Logger().
		Debug("calling NoBlocks()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := NewMsgNoBlocks()
	return s.SendMessage(msg)
}

// StartBatch sends a StartBatch message to the client, indicating the start of a batch.
func (s *Server) StartBatch() error {
	s.Protocol.Logger().
		Debug("calling StartBatch()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := NewMsgStartBatch()
	return s.SendMessage(msg)
}

// Block sends a Block message to the client with the given block type and data.
func (s *Server) Block(blockType uint, blockData []byte) error {
	s.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling Block(blockType: %+x, blockData: %x)", blockType, blockData),
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	wrappedBlock := WrappedBlock{
		Type:     blockType,
		RawBlock: blockData,
	}
	wrappedBlockData, err := cbor.Encode(&wrappedBlock)
	if err != nil {
		return err
	}
	msg := NewMsgBlock(wrappedBlockData)
	return s.SendMessage(msg)
}

// BatchDone sends a BatchDone message to the client, indicating the end of a batch.
func (s *Server) BatchDone() error {
	s.Protocol.Logger().
		Debug("calling BatchDone()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := NewMsgBatchDone()
	return s.SendMessage(msg)
}

// messageHandler handles incoming protocol messages for the server.
func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestRange:
		err = s.handleRequestRange(msg)
	case MessageTypeClientDone:
		err = s.handleClientDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

// handleRequestRange handles the RequestRange message from the client.
func (s *Server) handleRequestRange(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("request range",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.RequestRangeFunc == nil {
		return errors.New(
			"received block-fetch RequestRange message but no callback function is defined",
		)
	}
	msgRequestRange := msg.(*MsgRequestRange)
	return s.config.RequestRangeFunc(
		s.callbackContext,
		msgRequestRange.Start,
		msgRequestRange.End,
	)
}

// handleClientDone handles the ClientDone message from the client and restarts the protocol.
func (s *Server) handleClientDone() error {
	s.Protocol.Logger().
		Debug("client done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Restart protocol
	if err := s.Stop(); err != nil {
		return err
	}
	s.initProtocol()
	s.Start()
	return nil
}
