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

package txsubmission

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the TxSubmission server
type Server struct {
	*protocol.Protocol
	protocolMu             sync.RWMutex
	config                 *Config
	callbackContext        CallbackContext
	protoOptions           protocol.ProtocolOptions
	requestMu              sync.Mutex
	stateMu                sync.Mutex
	stopping               bool
	ackCount               int
	outstandingTxIds       []TxIdAndSize
	pendingTxIds           *pendingTxIdsRequest
	pendingTxs             *pendingTxsRequest
	requestTxIdsResultChan chan requestTxIdsResult
	requestTxsResultChan   chan requestTxsResult
}

type requestTxIdsResult struct {
	txIds []TxIdAndSize
	err   error
}

type requestTxsResult struct {
	txs []TxBody
	err error
}

type pendingTxIdsRequest struct {
	blocking  bool
	requested int
	ack       int
}

type pendingTxsRequest struct {
	requested       []TxId
	advertisedSizes map[TxId]uint32
}

// NewServer returns a new TxSubmission server object
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
	// Copy the global StateMap to avoid mutating shared state.
	stateMap := StateMap.Copy()
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
		StateMap:            stateMap,
		InitialState:        stateInit,
		IngressLimit:        MaxPendingMessageBytes,
	}
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	s.stateMu.Lock()
	s.Protocol = p
	s.callbackContext.DoneChan = p.DoneChan()
	s.requestTxIdsResultChan = make(chan requestTxIdsResult, 1)
	s.requestTxsResultChan = make(chan requestTxsResult, 1)
	s.ackCount = 0
	s.outstandingTxIds = nil
	s.pendingTxIds = nil
	s.pendingTxs = nil
	s.stopping = false
	s.stateMu.Unlock()
	s.protocolMu.Unlock()
}

func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
}

func (s *Server) Start() {
	p := s.ProtocolInstance()
	p.Logger().
		Debug("starting server protocol",
			"component", "network",
			"protocol", ProtocolName,
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	p.Start()
}

// RequestTxIds requests the next set of TX identifiers from the remote node's mempool
func (s *Server) RequestTxIds(
	blocking bool,
	reqCount int,
) ([]TxIdAndSize, error) {
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	s.protocolMu.RLock()
	s.stateMu.Lock()
	p := s.Protocol
	p.Logger().
		Debug(
			fmt.Sprintf("calling RequestTxIds(blocking: %+v, reqCount: %d)", blocking, reqCount),
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.protoOptions.ConnectionId.String(),
		)
	// Validate request counts
	if reqCount < 0 {
		p.Logger().
			Error("TxSubmission request count must be non-negative", "requested", reqCount)
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	// Keep the wire-range check ahead of the window check below: it bounds
	// reqCount before it is added to a counter, and it is what licenses the
	// uint16 conversion.
	if reqCount > MaxRequestCount {
		p.Logger().
			Error("TxSubmission request count exceeded", "requested", reqCount, "limit", MaxRequestCount)
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	if s.stopping {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
	if s.pendingTxIds != nil || s.pendingTxs != nil {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, invalidTxSubmissionMessage("a request is already pending")
	}
	ackCount := s.ackCount
	unackedCount := len(s.outstandingTxIds)
	if ackCount < 0 {
		p.Logger().
			Error("TxSubmission ack count must be non-negative", "ack_count", ackCount)
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	if ackCount > MaxAckCount {
		p.Logger().
			Error("TxSubmission ack count exceeded", "ack_count", ackCount, "limit", MaxAckCount)
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	if ackCount > unackedCount {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	// A request must also leave the peer inside the outstanding window, or a
	// conforming peer refuses it: the reference implementation's outbound side
	// throws ProtocolErrorRequestedTooManyTxids when
	// unackedNo - ackNo + reqNo exceeds maxUnacked
	// (Ouroboros.Network.TxSubmission.Outbound). Our own client applies the
	// same condition, and MaxPendingMessageBytes is derived from that window,
	// so a reply to a larger request could not fit it.
	if unackedCount-ackCount+reqCount > MaxUnackedTxIds {
		p.Logger().
			Error("TxSubmission request count exceeded",
				"requested", reqCount,
				"ack", ackCount,
				"unacknowledged", unackedCount,
				"limit", MaxUnackedTxIds,
			)
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	pending := &pendingTxIdsRequest{
		blocking:  blocking,
		requested: reqCount,
		ack:       ackCount,
	}
	s.pendingTxIds = pending
	resultChan := s.requestTxIdsResultChan
	s.stateMu.Unlock()
	s.protocolMu.RUnlock()

	// Safe conversions after validation
	//nolint:gosec // Already validated above to be non-negative and within uint16 range
	ack := uint16(ackCount)
	//nolint:gosec // Already validated above to be non-negative and within uint16 range
	req := uint16(reqCount)
	msg := NewMsgRequestTxIds(blocking, ack, req)
	if err := p.SendMessage(msg); err != nil {
		s.stateMu.Lock()
		if s.pendingTxIds == pending {
			s.pendingTxIds = nil
		}
		s.stateMu.Unlock()
		return nil, err
	}
	// Wait for result
	select {
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		return result.txIds, nil
	case <-p.DoneChan():
		s.stateMu.Lock()
		if s.pendingTxIds == pending {
			s.pendingTxIds = nil
			s.stopping = true
		}
		s.stateMu.Unlock()
		select {
		case result := <-resultChan:
			if result.err != nil {
				return nil, result.err
			}
			return result.txIds, nil
		default:
		}
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// RequestTxs requests the content of the requested TX identifiers from the remote node's mempool
func (s *Server) RequestTxs(txIds []TxId) ([]TxBody, error) {
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	if len(txIds) > MaxUnackedTxIds {
		p := s.ProtocolInstance()
		p.Logger().
			Error("TxSubmission tx request count exceeded",
				"requested", len(txIds),
				"limit", MaxUnackedTxIds,
			)
		return nil, protocol.ErrProtocolViolationRequestExceeded
	}
	txIds = append([]TxId(nil), txIds...)
	s.protocolMu.RLock()
	s.stateMu.Lock()
	p := s.Protocol
	if s.stopping {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
	if s.pendingTxIds != nil || s.pendingTxs != nil {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, invalidTxSubmissionMessage("a request is already pending")
	}
	if err := requestedTxIdsAreOutstanding(s.outstandingTxIds, txIds); err != nil {
		s.stateMu.Unlock()
		s.protocolMu.RUnlock()
		return nil, err
	}
	advertisedSizes := make(map[TxId]uint32, len(s.outstandingTxIds))
	for _, txIdAndSize := range s.outstandingTxIds {
		advertisedSizes[txIdAndSize.TxId] = txIdAndSize.Size
	}
	pending := &pendingTxsRequest{
		requested:       append([]TxId(nil), txIds...),
		advertisedSizes: advertisedSizes,
	}
	s.pendingTxs = pending
	resultChan := s.requestTxsResultChan
	// Pre-allocate slice to avoid repeated allocations
	txString := make([]string, 0, len(txIds))
	for _, t := range txIds {
		// Convert TxId directly to Blake2b256 without intermediate slice
		txString = append(txString, common.NewBlake2b256(t.TxId[:]).String())
	}
	p.Logger().
		Debug(
			fmt.Sprintf("calling RequestTxs(txIds: %+v)", txString),
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.protoOptions.ConnectionId.String(),
		)
	msg := NewMsgRequestTxs(txIds)
	s.stateMu.Unlock()
	s.protocolMu.RUnlock()
	if err := p.SendMessage(msg); err != nil {
		s.stateMu.Lock()
		if s.pendingTxs == pending {
			s.pendingTxs = nil
		}
		s.stateMu.Unlock()
		return nil, err
	}
	// Wait for result
	select {
	case <-p.DoneChan():
		s.stateMu.Lock()
		if s.pendingTxs == pending {
			s.pendingTxs = nil
			s.stopping = true
		}
		s.stateMu.Unlock()
		select {
		case result := <-resultChan:
			if result.err != nil {
				return nil, result.err
			}
			return result.txs, nil
		default:
		}
		return nil, protocol.ErrProtocolShuttingDown
	case result := <-resultChan:
		return result.txs, result.err
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
	p := s.ProtocolInstance()
	p.Logger().
		Debug("reply tx ids",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.protoOptions.ConnectionId.String(),
		)
	msgReplyTxIds := msg.(*MsgReplyTxIds)
	s.stateMu.Lock()
	pending := s.pendingTxIds
	outstanding := append([]TxIdAndSize(nil), s.outstandingTxIds...)
	resultChan := s.requestTxIdsResultChan
	s.stateMu.Unlock()
	if pending == nil {
		return invalidTxSubmissionMessage("received ReplyTxIds without a pending request")
	}
	nextOutstanding, err := reconcileTxIds(
		outstanding,
		pending.ack,
		pending.requested,
		pending.blocking,
		msgReplyTxIds.TxIds,
	)
	s.stateMu.Lock()
	if s.pendingTxIds != pending {
		s.stateMu.Unlock()
		return protocol.ErrProtocolShuttingDown
	}
	s.pendingTxIds = nil
	if err == nil {
		s.outstandingTxIds = nextOutstanding
		s.ackCount = len(msgReplyTxIds.TxIds)
	}
	s.stateMu.Unlock()
	result := requestTxIdsResult{err: err}
	if err == nil {
		result.txIds = msgReplyTxIds.TxIds
	}
	resultChan <- result
	return err
}

func (s *Server) handleReplyTxs(msg protocol.Message) error {
	p := s.ProtocolInstance()
	p.Logger().
		Debug("reply txs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.protoOptions.ConnectionId.String(),
		)
	msgReplyTxs := msg.(*MsgReplyTxs)
	s.stateMu.Lock()
	pending := s.pendingTxs
	resultChan := s.requestTxsResultChan
	s.stateMu.Unlock()
	if pending == nil {
		return invalidTxSubmissionMessage("received ReplyTxs without a pending request")
	}
	orderedTxs, err := validateAndOrderTxBodies(
		pending.requested,
		msgReplyTxs.Txs,
		pending.advertisedSizes,
	)
	s.stateMu.Lock()
	if s.pendingTxs != pending {
		s.stateMu.Unlock()
		return protocol.ErrProtocolShuttingDown
	}
	s.pendingTxs = nil
	s.stateMu.Unlock()
	result := requestTxsResult{err: err}
	if err == nil {
		result.txs = orderedTxs
	}
	resultChan <- result
	return err
}

func (s *Server) handleDone() error {
	p := s.ProtocolInstance()
	s.stateMu.Lock()
	pending := s.pendingTxIds
	if pending == nil || !pending.blocking {
		s.stateMu.Unlock()
		return invalidTxSubmissionMessage("received Done without a blocking TxIds request")
	}
	s.stopping = true
	s.pendingTxIds = nil
	callbackContext := s.callbackContext
	resultChan := s.requestTxIdsResultChan
	s.stateMu.Unlock()
	p.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", callbackContext.ConnectionId.String(),
		)
	// Signal the RequestTxIds function to stop waiting
	resultChan <- requestTxIdsResult{
		err: ErrStopServerProcess,
	}
	// Call the user callback function
	if s.config != nil && s.config.DoneFunc != nil {
		if err := s.config.DoneFunc(callbackContext); err != nil {
			return err
		}
	}
	// Restart protocol
	p.Stop()
	s.initProtocol()
	s.Start()
	return nil
}

func (s *Server) handleInit() error {
	p := s.ProtocolInstance()
	s.stateMu.Lock()
	callbackContext := s.callbackContext
	s.stateMu.Unlock()
	p.Logger().
		Debug("init",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.InitFunc == nil {
		return errors.New(
			"received tx-submission Init message but no callback function is defined",
		)
	}
	// Call the user callback function
	return s.config.InitFunc(callbackContext)
}
