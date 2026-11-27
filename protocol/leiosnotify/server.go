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

package leiosnotify

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// serverRequestQueueDepth is the number of received messages the server queues
// ahead of its handlers. The reference client pipelines up to 1000 requests
// before it can send MsgQuit, and the quit must be read behind all of them
// without the read loop stalling.
const serverRequestQueueDepth = 1002

// quitSignal is closed once, when the client's MsgQuit is read.
type quitSignal struct {
	once sync.Once
	ch   chan struct{}
}

func newQuitSignal() *quitSignal {
	return &quitSignal{ch: make(chan struct{})}
}

func (q *quitSignal) fire() {
	q.once.Do(func() { close(q.ch) })
}

func (q *quitSignal) fired() bool {
	select {
	case <-q.ch:
		return true
	default:
		return false
	}
}

type Server struct {
	proto           *protocol.Protocol // current instance; replaced on restart, guarded by protocolMu
	quit            *quitSignal
	protocolMu      sync.RWMutex
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
	// retired holds the instances replaced by a restart whose goroutines may
	// still be running, guarded by protocolMu. A request source that ignores
	// QuitChan outlives its run and hands its notification to
	// ResponseSentFunc on the retired instance, so Stop waits for these too.
	retired []*protocol.Protocol
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
		// Save this for re-use later
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:             s,
		ConnectionId:       protoOptions.ConnectionId,
		ConnectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	s.initProtocol()
	return s
}

func (s *Server) initProtocol() bool {
	cfg := s.config
	if cfg == nil {
		defaultConfig := NewConfig()
		cfg = &defaultConfig
	}
	quit := newQuitSignal()
	protoConfig := cfg.protocolConfig(protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		MessageReceivedFunc: func(msg protocol.Message) {
			if msg.Type() == MessageTypeQuit {
				quit.fire()
			}
		},
		InitialState: StateIdle,
	})
	protoConfig.RecvQueueSize = max(
		protoConfig.RecvQueueSize,
		serverRequestQueueDepth,
	)
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	if s.stopped {
		s.protocolMu.Unlock()
		return false
	}
	s.quit = quit
	s.proto = p
	s.protocolMu.Unlock()
	return true
}

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
// for. Otherwise it waits until the protocol's goroutines, and those of the
// runs the client ended with MsgQuit, have returned, or until ctx is done,
// in which case it returns ctx.Err() and shutdown still proceeds.
func (s *Server) Stop(ctx context.Context) error {
	s.protocolMu.Lock()
	s.stopped = true
	p := s.proto
	retired := slices.Clone(s.retired)
	s.protocolMu.Unlock()
	// A callback of a retired instance is one of the goroutines the loop
	// below waits for, and one of the current instance waits for nothing,
	// so a callback's Stop only signals: Stop with a done context does not
	// wait.
	if owner := protocol.CallbackOwner(ctx); owner == p ||
		(owner != nil && slices.Contains(retired, owner)) {
		signal, cancel := context.WithCancel(ctx)
		cancel()
		_ = p.Stop(signal)
		return nil
	}
	if err := p.Stop(ctx); err != nil {
		return err
	}
	// A retired instance was stopped at its restart, so Stop only waits
	// for it.
	for _, old := range retired {
		if err := old.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// retire records old, which a restart has stopped, for Stop to wait for,
// and forgets the retired instances whose goroutines have all returned.
func (s *Server) retire(old *protocol.Protocol) {
	// A done context makes Protocol.Stop report, without waiting, whether
	// the instance's goroutines have already returned.
	done, cancel := context.WithCancel(context.Background())
	cancel()
	s.protocolMu.Lock()
	defer s.protocolMu.Unlock()
	live := s.retired[:0]
	for _, p := range s.retired {
		if p.Stop(done) != nil {
			live = append(live, p)
		}
	}
	clear(s.retired[len(live):])
	s.retired = append(live, old)
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeNotificationRequestNext:
		s.ProtocolInstance().RunLoop("leios-notify request", func() {
			if err := s.handleRequestNext(); err != nil {
				s.ProtocolInstance().SendError(err)
			}
		})
	case MessageTypeQuit:
		err = s.handleQuit()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

// handleRequestNext answers one request. messageHandler runs it off the
// receive loop, which must keep reading so a MsgQuit the client pipelined
// behind the request is seen while the request is still waiting for a
// notification.
func (s *Server) handleRequestNext() error {
	s.ProtocolInstance().Logger().
		Debug("notificiation request next",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.protocolMu.RLock()
	quit := s.quit
	s.protocolMu.RUnlock()
	return s.serveRequestNext(quit)
}

type requestNextResult struct {
	resp protocol.Message
	err  error
}

func (s *Server) serveRequestNext(quit *quitSignal) error {
	if quit.fired() {
		return s.ProtocolInstance().SendMessageAndWait(NewMsgCanceled())
	}
	if s.config == nil || s.config.RequestNextFunc == nil {
		// No notification source is wired. This happens when a node negotiates
		// an N2N version that includes the Leios mini-protocols but does not
		// actively serve Leios notifications (e.g. a node following the chain
		// over chainsync/blockfetch without participating in Leios diffusion).
		// Leave the request pending in the Busy state instead of erroring:
		// leios-notify has no response timeout and no await message, so a
		// server with nothing to announce legitimately holds agency until it
		// has a notification, or until the client quits. Erroring here would
		// tear down the whole connection -- including chainsync/blockfetch --
		// over an unanswered notification request.
		s.ProtocolInstance().RunLoop("leios-notify pending request", func() {
			select {
			case <-quit.ch:
				if err := s.ProtocolInstance().SendMessageAndWait(NewMsgCanceled()); err != nil {
					s.ProtocolInstance().SendError(err)
				}
			case <-s.ProtocolInstance().DoneChan():
			}
		})
		return nil
	}
	ctx := s.callbackContext.withOwner(s.ProtocolInstance())
	ctx.QuitChan = quit.ch
	results := make(chan requestNextResult, 1)
	s.ProtocolInstance().RunLoop("leios-notify request source", func() {
		resp, err := s.config.RequestNextFunc(ctx)
		results <- requestNextResult{resp: resp, err: err}
	})
	// A source that stops waiting because the client quit returns an error or
	// nothing; that answers the request with MsgCanceled, not a failure.
	resolve := func(result requestNextResult) error {
		if quit.fired() && (result.err != nil || result.resp == nil) {
			return s.ProtocolInstance().SendMessageAndWait(NewMsgCanceled())
		}
		return s.deliverNotification(ctx, result)
	}
	select {
	case result := <-results:
		return resolve(result)
	case <-quit.ch:
		// A notification that is already ready is still delivered, as the
		// reference server does
		select {
		case result := <-results:
			return resolve(result)
		default:
		}
		s.releaseLateNotification(ctx, results, ErrRequestCanceled)
		return s.ProtocolInstance().SendMessageAndWait(NewMsgCanceled())
	case <-s.ProtocolInstance().DoneChan():
		s.releaseLateNotification(
			ctx,
			results,
			protocol.ErrProtocolShuttingDown,
		)
		return nil
	}
}

// releaseLateNotification reports a notification the source hands back after
// it can no longer be sent, so that the source can release it.
func (s *Server) releaseLateNotification(
	ctx CallbackContext,
	results <-chan requestNextResult,
	reason error,
) {
	s.ProtocolInstance().RunLoop("leios-notify late notification", func() {
		result := <-results
		if result.err == nil && result.resp != nil &&
			s.config.ResponseSentFunc != nil {
			s.config.ResponseSentFunc(ctx, result.resp, reason)
		}
	})
}

func (s *Server) deliverNotification(
	ctx CallbackContext,
	result requestNextResult,
) error {
	if result.err != nil {
		return result.err
	}
	if result.resp == nil {
		return errors.New(
			"received leios-notify NotificationRequestNext message but callback returned nil",
		)
	}
	sendErr := s.ProtocolInstance().SendMessageAndWait(result.resp)
	if s.config.ResponseSentFunc != nil {
		s.config.ResponseSentFunc(ctx, result.resp, sendErr)
	}
	return sendErr
}

// handleQuit acknowledges the client's MsgQuit with MsgDone. A request that
// was outstanding when the client quit was already answered, because the
// protocol holds MsgQuit back until the server has sent its reply.
func (s *Server) handleQuit() error {
	s.ProtocolInstance().Logger().
		Debug("client quit",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if err := s.ProtocolInstance().SendMessageAndWait(NewMsgDone()); err != nil {
		return err
	}
	// Restart protocol
	old := s.ProtocolInstance()
	_ = old.StopForRestart(old.CallbackContext())
	s.retire(old)
	if !s.initProtocol() {
		old.CancelRestart()
		return nil
	}
	s.Start()
	return nil
}
