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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalStateQuery server
type Server struct {
	*protocol.Protocol
	config                        *Config
	callbackContext               CallbackContext
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
	enableGetLedgerPeerSnapshot   bool
	enableLedgerPeerKind          bool
	version                       uint16
}

// NewServer returns a new LocalStateQuery server object
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
		StateMap:            StateMap,
		InitialState:        stateIdle,
		MaxReadBufferSize:   cfg.MaxReadBufferSize,
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
	s.Protocol = protocol.New(protoConfig)
	return s
}

// ledgerPeerSnapshotQuery returns the GetLedgerPeerSnapshot leaf of a
// decoded query wrapper, or nil. The query lives under BlockQuery ->
// ShelleyQuery -> ShelleyGetLedgerPeerSnapshotQuery, so we walk that chain
// rather than re-decoding the raw CBOR.
func ledgerPeerSnapshotQuery(
	w QueryWrapper,
) *ShelleyGetLedgerPeerSnapshotQuery {
	bq, ok := w.Query.(*BlockQuery)
	if !ok {
		return nil
	}
	sq, ok := bq.Query.(*ShelleyQuery)
	if !ok {
		return nil
	}
	q, _ := sq.Query.(*ShelleyGetLedgerPeerSnapshotQuery)
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
	s.Protocol.Logger().
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
			if err := s.SendMessage(respMsg); err != nil {
				return err
			}
		} else if errors.Is(err, ErrAcquireFailurePointNotOnChain) {
			respMsg := NewMsgFailure(AcquireFailurePointNotOnChain)
			if err := s.SendMessage(respMsg); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	respMsg := NewMsgAcquired()
	if err := s.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleQuery(msg protocol.Message) error {
	s.Protocol.Logger().
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
	}
	// Encode query result
	resultCbor, err := cbor.Encode(&result)
	if err != nil {
		return err
	}
	respMsg := NewMsgResult(resultCbor)
	if err := s.SendMessage(respMsg); err != nil {
		return err
	}
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
	if s.config.ReleaseFunc == nil {
		return errors.New(
			"received local-state-query Release message but no callback function is defined",
		)
	}
	// Call the user callback function
	return s.config.ReleaseFunc(s.callbackContext)
}

func (s *Server) handleReAcquire(msg protocol.Message) error {
	s.Protocol.Logger().
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
			if err := s.SendMessage(respMsg); err != nil {
				return err
			}
		} else if errors.Is(err, ErrAcquireFailurePointNotOnChain) {
			respMsg := NewMsgFailure(AcquireFailurePointNotOnChain)
			if err := s.SendMessage(respMsg); err != nil {
				return err
			}
		} else {
			return err
		}
		return nil
	}
	respMsg := NewMsgAcquired()
	if err := s.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
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
