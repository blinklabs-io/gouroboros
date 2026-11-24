// Copyright 2024 Blink Labs Software
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

package localstatequery

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalStateQuery server
type Server struct {
	*protocol.Protocol
	protocolMu                    sync.RWMutex
	protoOptions                  protocol.ProtocolOptions
	config                        *Config
	callbackContext               CallbackContext
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
	enableGetLedgerPeerSnapshot   bool
	enableLedgerPeerKind          bool
	version                       uint16
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
}

// NewServer returns a new LocalStateQuery server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config:       cfg,
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.version = protoOptions.Version
	// Enable version-dependent features
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 10 {
		s.enableGetChainBlockNo = true
		s.enableGetChainPoint = true
	}
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 11 {
		s.enableGetRewardInfoPoolsBlock = true
	}
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 19 {
		s.enableGetLedgerPeerSnapshot = true
	}
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 23 {
		s.enableLedgerPeerKind = true
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
		StateMap:            StateMap,
		InitialState:        stateIdle,
		MaxReadBufferSize:   s.config.MaxReadBufferSize,
	}
	p := protocol.New(protoConfig)
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

// Stop stops the server. A restart that is in progress when Stop is called
// does not start a new protocol instance.
func (s *Server) Stop() {
	s.protocolMu.Lock()
	s.stopped = true
	p := s.Protocol
	s.protocolMu.Unlock()
	p.Stop()
}

// ledgerPeerSnapshotQuery returns the GetLedgerPeerSnapshot leaf of a
// decoded query wrapper, or nil.
func ledgerPeerSnapshotQuery(
	w QueryWrapper,
) *ShelleyGetLedgerPeerSnapshotQuery {
	q, _ := shelleyLeafQuery(w).(*ShelleyGetLedgerPeerSnapshotQuery)
	return q
}

// snapshotInResult returns the LedgerPeerSnapshotResult a query callback
// answered with, bare or inside the single-element era result wrapper, or
// nil when the result has neither shape.
func snapshotInResult(result any) *LedgerPeerSnapshotResult {
	switch r := result.(type) {
	case LedgerPeerSnapshotResult:
		return &r
	case *LedgerPeerSnapshotResult:
		return r
	case []any:
		if len(r) == 1 {
			return snapshotInResult(r[0])
		}
	}
	return nil
}

// checkLedgerPeerSnapshotResult rejects a snapshot whose wire version the
// request form does not accept: the client decodes the reply by the form it
// sent, so any other layout fails there instead of here.
func checkLedgerPeerSnapshotResult(
	q *ShelleyGetLedgerPeerSnapshotQuery,
	result any,
) error {
	snap := snapshotInResult(result)
	if snap == nil {
		return nil
	}
	want := uint64(ledgerPeerSnapshotWireVersion)
	if q.WithPeerKind {
		want = ledgerPeerSnapshotWireVersionAll
		if q.PeerKind == LedgerPeerKindBig {
			want = ledgerPeerSnapshotWireVersionBig
		}
	}
	got := snap.Version
	if got == 0 {
		got = ledgerPeerSnapshotWireVersion
	}
	if got != want {
		return fmt.Errorf(
			"%w: result version %d for a request that takes version %d",
			ErrLedgerPeerSnapshotUnsupportedWireVersion,
			got,
			want,
		)
	}
	return nil
}

// withoutSRVRelays drops SRV relays from a version 1 snapshot answering a
// node-to-client version before 22, which cannot decode them, and then any
// pool left with no relay, as ouroboros-network's encoder does. The callback's
// snapshot is left unchanged.
func withoutSRVRelays(result any, version uint16) any {
	if version-protocol.ProtocolVersionNtCOffset >= 22 {
		return result
	}
	snap := snapshotInResult(result)
	if snap == nil || snap.Version > ledgerPeerSnapshotWireVersion {
		return result
	}
	filtered := *snap
	filtered.Pools = make([]PoolLedgerPeers, 0, len(snap.Pools))
	for _, pool := range snap.Pools {
		relays := make([]RelayAccessPoint, 0, len(pool.Detail.Relays))
		for _, relay := range pool.Detail.Relays {
			if relay.Kind != RelayKindSRV {
				relays = append(relays, relay)
			}
		}
		if len(relays) == 0 {
			continue
		}
		pool.Detail.Relays = relays
		filtered.Pools = append(filtered.Pools, pool)
	}
	if _, ok := result.([]any); ok {
		return []any{filtered}
	}
	return filtered
}

// wrapLedgerPeerSnapshotResult puts a bare snapshot into the single-element
// era result wrapper the client decodes, so a callback may answer with either
// form and the reply always has the wrapped one.
func wrapLedgerPeerSnapshotResult(result any) any {
	if _, wrapped := result.([]any); wrapped {
		return result
	}
	if snap := snapshotInResult(result); snap != nil {
		return []any{*snap}
	}
	return result
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquire:
		err = s.handleAcquire(msg)
	case MessageTypeQuery:
		err = s.handleQuery(msg)
	case MessageTypeRelease:
		err = s.handleRelease()
	case MessageTypeReacquire:
		err = s.handleReAcquire(msg)
	case MessageTypeAcquireVolatileTip:
		err = s.handleAcquire(msg)
	case MessageTypeAcquireImmutableTip:
		err = s.handleAcquire(msg)
	case MessageTypeReacquireVolatileTip:
		err = s.handleReAcquire(msg)
	case MessageTypeReacquireImmutableTip:
		err = s.handleReAcquire(msg)
	case MessageTypeDone:
		s.handleDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleAcquire(msg protocol.Message) error {
	proto := s.ProtocolInstance()
	proto.Logger().
		Debug("acquire",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.AcquireFunc == nil {
		return errors.New(
			"received local-state-query Acquire message but no callback function is defined",
		)
	}
	var acquireTarget AcquireTarget
	switch msgAcquire := msg.(type) {
	case *MsgAcquire:
		acquireTarget = AcquireSpecificPoint{
			Point: msgAcquire.Point,
		}
	case *MsgAcquireVolatileTip:
		acquireTarget = AcquireVolatileTip{}
	case *MsgAcquireImmutableTip:
		acquireTarget = AcquireImmutableTip{}
	}
	// Call the user callback function
	err := s.config.AcquireFunc(s.callbackContext, acquireTarget, false)
	if err != nil {
		if errors.Is(err, ErrAcquireFailurePointTooOld) {
			respMsg := NewMsgFailure(AcquireFailurePointTooOld)
			if err := proto.SendMessage(respMsg); err != nil {
				return err
			}
		} else if errors.Is(err, ErrAcquireFailurePointNotOnChain) {
			respMsg := NewMsgFailure(AcquireFailurePointNotOnChain)
			if err := proto.SendMessage(respMsg); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	respMsg := NewMsgAcquired()
	if err := proto.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleQuery(msg protocol.Message) error {
	proto := s.ProtocolInstance()
	proto.Logger().
		Debug("query",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.QueryFunc == nil {
		return errors.New(
			"received local-state-query Query message but no callback function is defined",
		)
	}
	msgQuery := msg.(*MsgQuery)
	// Reject queries that the negotiated NtC protocol version does not
	// advertise, before the user callback ever sees them. The decoder map
	// in QueryWrapper accepts sub-tag 34 on any connection, so the gate
	// has to live here.
	if q := ledgerPeerSnapshotQuery(msgQuery.Query); q != nil {
		if !s.enableGetLedgerPeerSnapshot {
			return fmt.Errorf(
				"%s: received GetLedgerPeerSnapshot query but negotiated NtC protocol version is older than 19",
				ProtocolName,
			)
		}
		// Version 23 takes only the peer-kind form and earlier versions
		// only the bare one, so the form also tells the callback which
		// result layout to answer with.
		if q.WithPeerKind != s.enableLedgerPeerKind {
			return fmt.Errorf(
				"%s: %w: GetLedgerPeerSnapshot form (peer kind: %v)",
				ProtocolName,
				ErrQueryUnsupportedVersion,
				q.WithPeerKind,
			)
		}
	}
	if queryType, ok := shelleyLeafQueryType(msgQuery.Query); ok &&
		!shelleyQuerySupported(queryType, s.version) {
		return fmt.Errorf("%s: %w", ProtocolName, unsupportedQueryError(
			fmt.Sprintf("Shelley query %d", queryType),
			s.version,
		))
	}
	// Call the user callback function
	result, err := s.config.QueryFunc(s.callbackContext, msgQuery.Query)
	if err != nil {
		return err
	}
	if q := ledgerPeerSnapshotQuery(msgQuery.Query); q != nil {
		if err := checkLedgerPeerSnapshotResult(q, result); err != nil {
			return fmt.Errorf("%s: %w", ProtocolName, err)
		}
		result = withoutSRVRelays(result, s.version)
		result = wrapLedgerPeerSnapshotResult(result)
	}
	// Encode query result
	resultCbor, err := cbor.Encode(&result)
	if err != nil {
		return err
	}
	respMsg := NewMsgResult(resultCbor)
	if err := proto.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleRelease() error {
	proto := s.ProtocolInstance()
	proto.Logger().
		Debug("release",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// MsgRelease is always legal from Acquired; without a callback there is
	// simply nothing for the application to free.
	if s.config.ReleaseFunc == nil {
		return nil
	}
	// Call the user callback function
	return s.config.ReleaseFunc(s.callbackContext)
}

func (s *Server) handleReAcquire(msg protocol.Message) error {
	proto := s.ProtocolInstance()
	proto.Logger().
		Debug("reacquire",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.AcquireFunc == nil {
		return errors.New(
			"received local-state-query ReAcquire message but no callback function is defined",
		)
	}
	var acquireTarget AcquireTarget
	switch msgReAcquire := msg.(type) {
	case *MsgReAcquire:
		acquireTarget = AcquireSpecificPoint{
			Point: msgReAcquire.Point,
		}
	case *MsgReAcquireVolatileTip:
		acquireTarget = AcquireVolatileTip{}
	case *MsgReAcquireImmutableTip:
		acquireTarget = AcquireImmutableTip{}
	}
	// Call the user callback function
	err := s.config.AcquireFunc(s.callbackContext, acquireTarget, true)
	if err != nil {
		if errors.Is(err, ErrAcquireFailurePointTooOld) {
			respMsg := NewMsgFailure(AcquireFailurePointTooOld)
			if err := proto.SendMessage(respMsg); err != nil {
				return err
			}
		} else if errors.Is(err, ErrAcquireFailurePointNotOnChain) {
			respMsg := NewMsgFailure(AcquireFailurePointNotOnChain)
			if err := proto.SendMessage(respMsg); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	respMsg := NewMsgAcquired()
	if err := proto.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleDone() {
	proto := s.ProtocolInstance()
	proto.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// The reference's inbound governor restarts a node-to-client responder
	// that ended cleanly, so the client may start a new run on this
	// connection. StopForRestart keeps the client's next messages for the
	// new instance.
	proto.StopForRestart()
	if !s.initProtocol() {
		proto.CancelRestart()
		return
	}
	s.ProtocolInstance().Start()
}
