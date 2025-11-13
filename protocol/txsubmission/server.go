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
	"sync"
	"sync/atomic"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the TxSubmission server
type Server struct {
	*protocol.Protocol
	config                 *Config
	callbackContext        CallbackContext
	protoOptions           protocol.ProtocolOptions
	ackCount               int32
	requestTxIdsResultChan chan requestTxIdsResult
	requestTxsResultChan   chan []TxBody
	done                   chan struct{}
	doneMutex              sync.Mutex
	onceStop               sync.Once
	restartMutex           sync.Mutex
	stopped                bool // indicates permanent stop has been requested
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
		requestTxIdsResultChan: make(chan requestTxIdsResult, 1),
		requestTxsResultChan:   make(chan []TxBody, 1),
		done:                   make(chan struct{}),
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
}

// Stop stops the server protocol.
func (s *Server) Stop() error {
	s.onceStop.Do(func() {
		s.restartMutex.Lock()
		defer s.restartMutex.Unlock()
		s.Protocol.Logger().
			Debug("stopping server protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", s.callbackContext.ConnectionId.String(),
			)
		s.stopped = true
		s.doneMutex.Lock()
		select {
		case <-s.done:
			// Already closed
		default:
			close(s.done)
		}
		s.doneMutex.Unlock()
		_ = s.Protocol.Stop() // Error ignored - method returns nil by design
	})
	return nil
}

// doneChan returns the current done channel, safely accessed under mutex
func (s *Server) doneChan() <-chan struct{} {
	s.doneMutex.Lock()
	defer s.doneMutex.Unlock()
	return s.done
}

// IsStopped returns true if the server has been permanently stopped
func (s *Server) IsStopped() bool {
	s.restartMutex.Lock()
	defer s.restartMutex.Unlock()
	return s.stopped
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
	// Validate request counts
	if reqCount < 0 {
		s.Protocol.Logger().
			Error("TxSubmission request count must be non-negative", "requested", reqCount)
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	if reqCount > MaxRequestCount {
		s.Protocol.Logger().
			Error("TxSubmission request count exceeded", "requested", reqCount, "limit", MaxRequestCount)
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	ackCount := atomic.LoadInt32(&s.ackCount)
	if ackCount < 0 {
		s.Protocol.Logger().
			Error("TxSubmission ack count must be non-negative", "ack_count", ackCount)
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	if ackCount > MaxAckCount {
		s.Protocol.Logger().
			Error("TxSubmission ack count exceeded", "ack_count", ackCount, "limit", MaxAckCount)
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}

	// Safe conversions after validation
	//nolint:gosec // Already validated above to be non-negative and within uint16 range
	ack := uint16(ackCount)
	//nolint:gosec // Already validated above to be non-negative and within uint16 range
	req := uint16(reqCount)
	msg := NewMsgRequestTxIds(blocking, ack, req)
	if err := s.SendMessage(msg); err != nil {
		return nil, err
	}
	// Wait for result
	s.restartMutex.Lock()
	resultChan := s.requestTxIdsResultChan
	s.restartMutex.Unlock()
	select {
	case result, ok := <-resultChan:
		if !ok {
			return nil, protocol.ErrProtocolShuttingDown
		}
		if result.err != nil {
			return nil, result.err
		}
		// Update ack count for next call
		// #nosec G115 - len(result.txIds) is bounded by MaxRequestCount (65535) which fits in int32
		atomic.StoreInt32(&s.ackCount, int32(len(result.txIds)))
		return result.txIds, nil
	case <-s.doneChan():
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// RequestTxs requests the content of the requested TX identifiers from the remote node's mempool
func (s *Server) RequestTxs(txIds []TxId) ([]TxBody, error) {
	// Pre-allocate slice to avoid repeated allocations
	txString := make([]string, 0, len(txIds))
	for _, t := range txIds {
		// Convert TxId directly to Blake2b256 without intermediate slice
		txString = append(txString, common.NewBlake2b256(t.TxId[:]).String())
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
	s.restartMutex.Lock()
	resultChan := s.requestTxsResultChan
	s.restartMutex.Unlock()
	select {
	case <-s.doneChan():
		return nil, protocol.ErrProtocolShuttingDown
	case txs, ok := <-resultChan:
		if !ok {
			return nil, protocol.ErrProtocolShuttingDown
		}
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
	s.restartMutex.Lock()
	s.requestTxIdsResultChan <- requestTxIdsResult{
		txIds: msgReplyTxIds.TxIds,
	}
	s.restartMutex.Unlock()
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
	s.restartMutex.Lock()
	s.requestTxsResultChan <- msgReplyTxs.Txs
	s.restartMutex.Unlock()
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
	// Signal the RequestTxIds function to stop waiting (non-blocking)
	s.restartMutex.Lock()
	resultChan := s.requestTxIdsResultChan
	s.restartMutex.Unlock()
	select {
	case resultChan <- requestTxIdsResult{
		err: ErrStopServerProcess,
	}:
	default:
		// No one is waiting, which is fine
	}
	// Call the user callback function
	if s.config != nil && s.config.DoneFunc != nil {
		if err := s.config.DoneFunc(s.callbackContext); err != nil {
			return err
		}
	}
	// Restart protocol
	s.restartMutex.Lock()
	// Check if permanent stop has been requested
	if s.stopped {
		s.restartMutex.Unlock()
		return nil
	}
	// Stop current protocol (without using onceStop since we're restarting)
	s.Protocol.Logger().
		Debug("stopping server protocol for restart",
			"component", "network",
			"protocol", ProtocolName,
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.doneMutex.Lock()
	select {
	case <-s.done:
		// Already closed by Stop()
	default:
		close(s.done)
	}
	s.doneMutex.Unlock()
	stopErr := s.Protocol.Stop()
	s.initProtocol()
	s.requestTxIdsResultChan = make(chan requestTxIdsResult, 1)
	s.requestTxsResultChan = make(chan []TxBody, 1)
	s.doneMutex.Lock()
	s.done = make(chan struct{})
	s.doneMutex.Unlock()
	atomic.StoreInt32(&s.ackCount, 0)
	s.restartMutex.Unlock()
	// Check again if permanent stop has been requested (TOCTOU protection)
	if s.IsStopped() {
		return nil
	}
	// Start the new protocol outside the lock for better responsiveness
	s.Start()
	return stopErr
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
