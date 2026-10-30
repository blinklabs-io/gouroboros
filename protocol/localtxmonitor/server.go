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
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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
	// txHashes holds the IDs computed by HasTx, by era and transaction CBOR.
	// Entries survive an Acquire while their transaction is still in the
	// snapshot, so a client polling Acquire and HasTx parses each
	// transaction once rather than the whole mempool on every poll.
	txHashes   map[uint]map[string]*txHashEntry
	snapshotId uint64
}

type txHashEntry struct {
	hash       lcommon.Blake2b256
	snapshotId uint64
}

// NewServer returns a new Server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
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
		StateContext:        &stateContext{},
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
		s.handleDone()
	case MessageTypeRelease:
		s.handleRelease()
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
	mempoolSlotNumber, err := s.acquireMempool()
	if err != nil {
		return err
	}
	newMsg := NewMsgAcquired(mempoolSlotNumber)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}

// acquireMempool takes a new snapshot from the GetMempool callback. The
// transactions are not parsed here: only HasTx needs their IDs, and it
// computes them on demand.
func (s *Server) acquireMempool() (uint64, error) {
	mempoolSlotNumber, mempoolCapacity, mempoolTxs, err := s.config.GetMempoolFunc(
		s.callbackContext,
	)
	if err != nil {
		return 0, err
	}
	s.mempoolCapacity = mempoolCapacity
	s.mempoolNextTxIdx = 0
	s.mempoolTxs = make([]TxAndEraId, len(mempoolTxs))
	copy(s.mempoolTxs, mempoolTxs)
	// Keep the IDs of transactions still in the snapshot and drop the rest,
	// so the cache never outgrows the current mempool.
	s.snapshotId++
	for _, tx := range s.mempoolTxs {
		if entry, ok := s.txHashes[tx.EraId][string(tx.Tx)]; ok {
			entry.snapshotId = s.snapshotId
		}
	}
	for eraId, eraHashes := range s.txHashes {
		for key, entry := range eraHashes {
			if entry.snapshotId != s.snapshotId {
				delete(eraHashes, key)
			}
		}
		if len(eraHashes) == 0 {
			delete(s.txHashes, eraId)
		}
	}
	return mempoolSlotNumber, nil
}

func (s *Server) handleDone() {
	s.Protocol.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
}

func (s *Server) handleRelease() {
	s.Protocol.Logger().
		Debug("release",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.mempoolCapacity = 0
	s.mempoolTxs = nil
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
	hasTx, err := s.hasTx(msgHasTx)
	if err != nil {
		return err
	}
	newMsg := NewMsgReplyHasTx(hasTx)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) hasTx(msg *MsgHasTx) (bool, error) {
	for idx, tx := range s.mempoolTxs {
		if tx.EraId != uint(msg.EraId) {
			continue
		}
		hash, err := s.txHash(tx)
		if err != nil {
			return false, fmt.Errorf(
				"%s: parse mempool transaction %d (era %d): %w",
				ProtocolName,
				idx,
				tx.EraId,
				err,
			)
		}
		if bytes.Equal(hash[:], msg.TxId) {
			return true, nil
		}
	}
	return false, nil
}

// txHash returns the ID of a snapshot transaction, parsing it only the first
// time it is seen.
func (s *Server) txHash(tx TxAndEraId) (lcommon.Blake2b256, error) {
	if entry, ok := s.txHashes[tx.EraId][string(tx.Tx)]; ok {
		return entry.hash, nil
	}
	txObj, err := ledger.NewTransactionFromCbor(tx.EraId, tx.Tx)
	if err != nil {
		return lcommon.Blake2b256{}, err
	}
	hash := txObj.Hash()
	if s.txHashes == nil {
		s.txHashes = make(map[uint]map[string]*txHashEntry)
	}
	eraHashes := s.txHashes[tx.EraId]
	if eraHashes == nil {
		eraHashes = make(map[string]*txHashEntry)
		s.txHashes[tx.EraId] = eraHashes
	}
	eraHashes[string(tx.Tx)] = &txHashEntry{
		hash:       hash,
		snapshotId: s.snapshotId,
	}
	return hash, nil
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
	const maxUint32 = uint64(math.MaxUint32)
	totalTxSize := uint64(0)
	for _, tx := range s.mempoolTxs {
		txSize := uint64(len(tx.Tx))
		if txSize > maxUint32 || totalTxSize > maxUint32-txSize {
			return errors.New("integrer overflow in total tx size")
		}
		totalTxSize += txSize
	}
	numTxs := uint64(len(s.mempoolTxs))
	if numTxs > maxUint32 {
		return errors.New("integrer overflow in tx count")
	}
	newMsg := NewMsgReplyGetSizes(
		s.mempoolCapacity,
		uint32(totalTxSize),
		uint32(numTxs),
	)
	if err := s.SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}
