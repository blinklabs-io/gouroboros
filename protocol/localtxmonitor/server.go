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
	"context"
	"errors"
	"fmt"
	"hash/maphash"
	"math"
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalTxMonitor server
type Server struct {
	proto        *protocol.Protocol // current instance; replaced on restart, guarded by protocolMu
	protocolMu   sync.RWMutex
	protoOptions protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped          bool
	config           *Config
	callbackContext  CallbackContext
	mempoolCapacity  uint32
	mempoolTxs       []TxAndEraId
	mempoolNextTxIdx int
	// txHashes holds the IDs computed by HasTx, by era and the maphash of the
	// transaction CBOR. Each entry points at the snapshot's own bytes rather
	// than a copy, and acquireMempool re-points carried entries at the new
	// snapshot, so the cache never pins bytes from an older one. Entries
	// survive an Acquire while their transaction is still in the snapshot, so
	// a client polling Acquire and HasTx parses each transaction once rather
	// than the whole mempool on every poll.
	txHashes          map[uint]map[uint64]*txHashEntry
	snapshotId        uint64
	enableGetMeasures bool
	// acquired is true while the client holds a snapshot, so an acquire
	// received then is MsgAwaitAcquire. acquiredSlot is that snapshot's slot.
	acquired           bool
	acquiredSlot       uint64
	connectionDoneChan <-chan any
}

// txHashSeed keys txHashes. Entries are confirmed with bytes.Equal, so a
// hash collision costs a reparse and never a wrong answer.
var txHashSeed = maphash.MakeSeed()

func txHashKey(txCbor []byte) uint64 {
	return maphash.Bytes(txHashSeed, txCbor)
}

type txHashEntry struct {
	tx         []byte
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
		config:             cfg,
		protoOptions:       protoOptions,
		enableGetMeasures:  supportsGetMeasures(protoOptions.Version),
		connectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.initProtocol()
	return s
}

// initProtocol installs a new protocol instance for the next run, unless the
// server was stopped. It reports whether it installed one.
func (s *Server) initProtocol() bool {
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
		StateContext:        &stateContext{},
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	defer s.protocolMu.Unlock()
	if s.stopped {
		return false
	}
	s.proto = p
	return true
}

// ProtocolInstance returns the protocol instance of the current run. The
// server replaces it after each MsgDone.
func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.proto
}

// Stop stops the server. A restart that is in progress when Stop is called
// does not start a new protocol instance.
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
	p := s.proto
	s.protocolMu.Unlock()
	return p.Stop(ctx)
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
	case MessageTypeGetMeasures:
		err = s.handleGetMeasures()
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
	s.ProtocolInstance().Logger().
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
	if s.acquired && s.config.AwaitMempoolFunc != nil {
		stopped, err := s.awaitMempoolChange()
		if err != nil || stopped {
			return err
		}
	}
	mempoolSlotNumber, err := s.acquireMempool()
	if err != nil {
		return err
	}
	newMsg := NewMsgAcquired(mempoolSlotNumber)
	if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
		return err
	}
	s.acquired = true
	s.acquiredSlot = mempoolSlotNumber
	return nil
}

// awaitMempoolChange blocks in the message handler until AwaitMempoolFunc
// reports that the mempool differs from the held snapshot. Blocking the
// receive loop is safe here: in Acquiring the client has no agency, so it
// has nothing to send. It reports stopped when the server or connection
// shut down during the wait, in which case nothing is to be sent.
func (s *Server) awaitMempoolChange() (bool, error) {
	stopChan := s.ProtocolInstance().StopChan()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stopChan:
		case <-s.connectionDoneChan:
		case <-ctx.Done():
		}
		cancel()
	}()
	held := make([]TxAndEraId, len(s.mempoolTxs))
	copy(held, s.mempoolTxs)
	err := s.config.AwaitMempoolFunc(
		ctx,
		s.callbackContext.withOwner(s.ProtocolInstance()),
		s.acquiredSlot,
		held,
	)
	select {
	case <-stopChan:
		return true, nil
	case <-s.connectionDoneChan:
		return true, nil
	default:
	}
	if err != nil {
		return false, fmt.Errorf("%s: await mempool change: %w", ProtocolName, err)
	}
	return false, nil
}

// acquireMempool takes a new snapshot from the GetMempool callback. The
// transactions are not parsed here: only HasTx needs their IDs, and it
// computes them on demand.
func (s *Server) acquireMempool() (uint64, error) {
	mempoolSlotNumber, mempoolCapacity, mempoolTxs, err := s.config.GetMempoolFunc(
		s.callbackContext.withOwner(s.ProtocolInstance()),
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
	if len(s.txHashes) == 0 {
		return mempoolSlotNumber, nil
	}
	s.snapshotId++
	for _, tx := range s.mempoolTxs {
		entry, ok := s.txHashes[tx.EraId][txHashKey(tx.Tx)]
		if ok && bytes.Equal(entry.tx, tx.Tx) {
			entry.tx = tx.Tx
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

// handleDone restarts the server in Idle, as the reference's inbound
// governor restarts a responder that ended cleanly, so the client can start
// another run on the same connection.
func (s *Server) handleDone() {
	p := s.ProtocolInstance()
	p.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.releaseMempool()
	s.mempoolNextTxIdx = 0
	_ = p.StopForRestart(p.CallbackContext())
	if !s.initProtocol() {
		p.CancelRestart()
		return
	}
	s.ProtocolInstance().Start()
}

func (s *Server) handleRelease() {
	s.ProtocolInstance().Logger().
		Debug("release",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.releaseMempool()
	s.acquired = false
}

// releaseMempool drops the snapshot along with its cached IDs. A client that
// polls re-acquires without releasing, so it keeps the cache; one that
// releases may stay idle and should not hold a copy of the mempool.
func (s *Server) releaseMempool() {
	s.mempoolCapacity = 0
	s.mempoolTxs = nil
	s.txHashes = nil
}

func (s *Server) handleHasTx(msg protocol.Message) error {
	s.ProtocolInstance().Logger().
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
	if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
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
	key := txHashKey(tx.Tx)
	existing, found := s.txHashes[tx.EraId][key]
	if found && bytes.Equal(existing.tx, tx.Tx) {
		return existing.hash, nil
	}
	txObj, err := ledger.NewTransactionFromCbor(tx.EraId, tx.Tx)
	if err != nil {
		return lcommon.Blake2b256{}, err
	}
	hash := txObj.Hash()
	if found {
		// Another transaction owns this key; leave it and do not cache.
		return hash, nil
	}
	if s.txHashes == nil {
		s.txHashes = make(map[uint]map[uint64]*txHashEntry)
	}
	eraHashes := s.txHashes[tx.EraId]
	if eraHashes == nil {
		eraHashes = make(map[uint64]*txHashEntry)
		s.txHashes[tx.EraId] = eraHashes
	}
	eraHashes[key] = &txHashEntry{
		tx:         tx.Tx,
		hash:       hash,
		snapshotId: s.snapshotId,
	}
	return hash, nil
}

func (s *Server) handleNextTx() error {
	s.ProtocolInstance().Logger().
		Debug("next tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.mempoolNextTxIdx >= len(s.mempoolTxs) {
		newMsg := NewMsgReplyNextTx(0, nil)
		if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
			return err
		}
		return nil
	}
	mempoolTx := s.mempoolTxs[s.mempoolNextTxIdx]
	if mempoolTx.EraId > math.MaxUint8 {
		return errors.New("integer overflow in era id")
	}
	newMsg := NewMsgReplyNextTx(uint8(mempoolTx.EraId), mempoolTx.Tx)
	if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
		return err
	}
	s.mempoolNextTxIdx++
	return nil
}

func (s *Server) handleGetSizes() error {
	s.ProtocolInstance().Logger().
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
	if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
		return err
	}
	return nil
}

// handleGetMeasures answers with the transactions NextTx has not yet
// returned from the snapshot, as the reference server does: its count and
// MeasureTransactionBytes are taken over the remaining transactions, while
// GetSizes covers the whole snapshot. The reference encoder refuses
// MsgReplyGetMeasures before node-to-client version 20, so this does too.
func (s *Server) handleGetMeasures() error {
	s.ProtocolInstance().Logger().
		Debug("get measures",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if !s.enableGetMeasures {
		return fmt.Errorf(
			"%s: received GetMeasures before node-to-client version %d",
			ProtocolName,
			getMeasuresMinVersion,
		)
	}
	remaining := s.mempoolTxs[s.mempoolNextTxIdx:]
	numTxs := uint64(len(remaining))
	if numTxs > math.MaxUint32 {
		return errors.New("integer overflow in tx count")
	}
	var totalTxSize uint64
	for _, tx := range remaining {
		totalTxSize += uint64(len(tx.Tx))
	}
	newMsg := NewMsgReplyGetMeasures(
		uint32(numTxs),
		map[string]MeasureSizeAndCapacity{
			MeasureTransactionBytes: {
				Size:     totalTxSize,
				Capacity: uint64(s.mempoolCapacity),
			},
		},
	)
	return s.ProtocolInstance().SendMessage(newMsg)
}
