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

package localtxmonitor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalTxMonitor server
type Server struct {
	*protocol.Protocol
	config           *Config
	callbackContext  CallbackContext
	mempoolCapacity  uint32
	mempoolTxs       []TxAndEraId
	mempoolNextTxIdx int
}

// NewServer returns a new Server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
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

func (s *Server) messageHandler(msg protocol.Message) error {
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
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleAcquire() error {
	s.Protocol.Logger().
		Debug("acquire",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.GetMempoolFunc == nil {
		return errors.New(
			"received local-tx-monitor Acquire message but no GetMempool callback function is defined",
		)
	}
	// Call the user callback function to get mempool information
	mempoolSlotNumber, mempoolCapacity, mempoolTxs, err := s.config.GetMempoolFunc(
		s.callbackContext,
	)
	if err != nil {
		return err
	}
	s.mempoolCapacity = mempoolCapacity
	s.mempoolNextTxIdx = 0
	s.mempoolTxs = make([]TxAndEraId, 0)
	for _, mempoolTx := range mempoolTxs {
		newTx := TxAndEraId{
			EraId: mempoolTx.EraId,
			Tx:    mempoolTx.Tx[:],
		}
		// Pre-parse TX for convenience
		tmpTxObj, err := ledger.NewTransactionFromCbor(
			mempoolTx.EraId,
			mempoolTx.Tx,
		)
		if err != nil {
			return err
		}
		newTx.txObj = tmpTxObj
		s.mempoolTxs = append(s.mempoolTxs, newTx)
	}
	newMsg := NewMsgAcquired(mempoolSlotNumber)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
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
	return nil
}

func (s *Server) handleRelease() error {
	s.Protocol.Logger().
		Debug("release",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.mempoolCapacity = 0
	s.mempoolTxs = nil
	return nil
}

func (s *Server) handleHasTx(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("has tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msgHasTx := msg.(*MsgHasTx)
	txId := hex.EncodeToString(msgHasTx.TxId)
	hasTx := false
	for _, tx := range s.mempoolTxs {
		if tx.txObj.Hash() == txId {
			hasTx = true
			break
		}
	}
	newMsg := NewMsgReplyHasTx(hasTx)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleNextTx() error {
	s.Protocol.Logger().
		Debug("next tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.mempoolNextTxIdx >= len(s.mempoolTxs) {
		newMsg := NewMsgReplyNextTx(0, nil)
		if err := s.SendMessage(newMsg); err != nil {
			return err
		}
		return nil
	}
	mempoolTx := s.mempoolTxs[s.mempoolNextTxIdx]
	if mempoolTx.EraId > math.MaxUint8 {
		return errors.New("integer overflow in era id")
	}
	newMsg := NewMsgReplyNextTx(uint8(mempoolTx.EraId), mempoolTx.Tx)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	s.mempoolNextTxIdx++
	return nil
}

func (s *Server) handleGetSizes() error {
	s.Protocol.Logger().
		Debug("get sizes",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	totalTxSize := 0
	for _, tx := range s.mempoolTxs {
		totalTxSize += len(tx.Tx)
	}
	numTxs := len(s.mempoolTxs)
	// check for over/underflows
	if totalTxSize < 0 || totalTxSize > math.MaxUint32 {
		return errors.New("integrer overflow in total tx size")
	}
	if numTxs < 0 || numTxs > math.MaxUint32 {
		return errors.New("integrer overflow in tx count")
	}
	newMsg := NewMsgReplyGetSizes(
		s.mempoolCapacity,
		uint32(totalTxSize), // #nosec G115
		uint32(numTxs),
	)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}
