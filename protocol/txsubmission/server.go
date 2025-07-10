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

package txsubmission

import (
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the TxSubmission server
type Server struct {
	*protocol.Protocol
	config                 *Config
	callbackContext        CallbackContext
	protoOptions           protocol.ProtocolOptions
	ackCount               int
	requestTxIdsResultChan chan requestTxIdsResult
	requestTxsResultChan   chan []TxBody
}

type requestTxIdsResult struct {
	txIds []TxIdAndSize
	err   error
}

// NewServer returns a new TxSubmission server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
		// Save this for re-use later
		protoOptions:           protoOptions,
		requestTxIdsResultChan: make(chan requestTxIdsResult),
		requestTxsResultChan:   make(chan []TxBody),
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
		InitialState:        stateInit,
	}
	s.Protocol = protocol.New(protoConfig)
}

func (s *Server) Start() {
	s.Protocol.Logger().
		Debug("starting server protocol",
			"component", "network",
			"protocol", ProtocolName,
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.Protocol.Start()
	// Start goroutine to cleanup resources on protocol shutdown
	go func() {
		// We create our own vars for these channels since they get replaced on restart
		requestTxIdsResultChan := s.requestTxIdsResultChan
		requestTxsResultChan := s.requestTxsResultChan
		<-s.DoneChan()
		close(requestTxIdsResultChan)
		close(requestTxsResultChan)
	}()
}

// RequestTxIds requests the next set of TX identifiers from the remote node's mempool
func (s *Server) RequestTxIds(
	blocking bool,
	reqCount int,
) ([]TxIdAndSize, error) {
	s.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling RequestTxIds(blocking: %+v, reqCount: %d)", blocking, reqCount),
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	var ack, req uint16
	if s.ackCount > 0 && s.ackCount <= math.MaxUint16 {
		ack = uint16(s.ackCount)
	} else if s.ackCount > math.MaxUint16 {
		ack = math.MaxUint16
	}
	if reqCount > 0 && reqCount <= math.MaxUint16 {
		req = uint16(reqCount)
	} else if reqCount > math.MaxUint16 {
		req = math.MaxUint16
	}
	msg := NewMsgRequestTxIds(blocking, ack, req)
	if err := s.SendMessage(msg); err != nil {
		return nil, err
	}
	// Wait for result
	result, ok := <-s.requestTxIdsResultChan
	if !ok {
		return nil, protocol.ErrProtocolShuttingDown
	}
	if result.err != nil {
		return nil, result.err
	}
	// Update ack count for next call
	s.ackCount = len(result.txIds)
	return result.txIds, nil
}

// RequestTxs requests the content of the requested TX identifiers from the remote node's mempool
func (s *Server) RequestTxs(txIds []TxId) ([]TxBody, error) {
	txString := []string{}
	for _, t := range txIds {
		ba := []byte{}
		for _, b := range t.TxId {
			ba = append(ba, b)
		}
		txString = append(txString, common.NewBlake2b256(ba).String())
	}
	s.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling RequestTxs(txIds: %+v)", txString),
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := NewMsgRequestTxs(txIds)
	if err := s.SendMessage(msg); err != nil {
		return nil, err
	}
	// Wait for result
	select {
	case <-s.DoneChan():
		return nil, protocol.ErrProtocolShuttingDown
	case txs := <-s.requestTxsResultChan:
		return txs, nil
	}
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeReplyTxIds:
		err = s.handleReplyTxIds(msg)
	case MessageTypeReplyTxs:
		err = s.handleReplyTxs(msg)
	case MessageTypeDone:
		err = s.handleDone()
	case MessageTypeInit:
		err = s.handleInit()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleReplyTxIds(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("reply tx ids",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msgReplyTxIds := msg.(*MsgReplyTxIds)
	s.requestTxIdsResultChan <- requestTxIdsResult{
		txIds: msgReplyTxIds.TxIds,
	}
	return nil
}

func (s *Server) handleReplyTxs(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("reply txs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msgReplyTxs := msg.(*MsgReplyTxs)
	s.requestTxsResultChan <- msgReplyTxs.Txs
	return nil
}

func (s *Server) handleDone() error {
	s.Protocol.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Signal the RequestTxIds function to stop waiting
	s.requestTxIdsResultChan <- requestTxIdsResult{
		err: ErrStopServerProcess,
	}
	// Call the user callback function
	if s.config != nil && s.config.DoneFunc != nil {
		if err := s.config.DoneFunc(s.callbackContext); err != nil {
			return err
		}
	}
	// Restart protocol
	s.Stop()
	s.initProtocol()
	s.requestTxIdsResultChan = make(chan requestTxIdsResult)
	s.requestTxsResultChan = make(chan []TxBody)
	s.Start()
	return nil
}

func (s *Server) handleInit() error {
	s.Protocol.Logger().
		Debug("init",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.InitFunc == nil {
		return errors.New(
			"received tx-submission Init message but no callback function is defined",
		)
	}
	// Call the user callback function
	return s.config.InitFunc(s.callbackContext)
}
