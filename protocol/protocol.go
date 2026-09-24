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

// Package protocol provides the common functionality for mini-protocols
package protocol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/internal/panics"
	"github.com/blinklabs-io/gouroboros/muxer"
)

// This is completely arbitrary, but the line had to be drawn somewhere
const maxMessagesPerSegment = 20

// maxReadBufferSize is the default upper bound on the read buffer in
// readLoop, used whenever a ProtocolConfig doesn't override it via
// MaxReadBufferSize. This prevents a malicious peer from sending incomplete
// CBOR indefinitely to cause unbounded memory growth (OOM). 16MB
// accommodates most legitimate messages, but not every one: a
// LocalStateQuery reply is bounded only by the size of whatever the query
// asks for, and a whole-UTxO-set dump against a large enough chain can
// exceed 16MB by a wide margin (confirmed live against a real ~3.17M-UTxO
// Preview node, blinklabs-io/dingo#1900's node-parity tool). A caller
// talking to a peer it trusts with that kind of query (a local socket, or a
// bridge it controls) should override this via MaxReadBufferSize rather
// than have a legitimate reply rejected as if it were the DoS this constant
// guards against.
//
// This bound is per ProtocolConfig, so it bounds one mini-protocol in one
// role. The connection-wide total is bounded separately by the muxer's
// reassembly allowance (muxer.Muxer.RaiseReadBufferBudget), which every
// protocol raises to its own cap as it registers; raising MaxReadBufferSize
// therefore raises the connection allowance with it.
const maxReadBufferSize = 16 * 1024 * 1024 // 16MB

// DefaultRecvQueueSize is the default capacity for the recv queue channel
const DefaultRecvQueueSize = 55

// Protocol implements the base functionality of an Ouroboros mini-protocol
type Protocol struct {
	config              ProtocolConfig
	doneChan            chan struct{}
	stopChan            chan struct{}
	muxerSendChan       chan *muxer.Segment
	muxerRecvChan       chan *muxer.Segment
	muxerDoneChan       chan bool
	sendQueueChan       chan outboundMessage
	recvDoneChan        chan struct{}
	recvQueueChan       chan Message
	recvReadyChan       chan bool
	sendDoneChan        chan struct{}
	sendReadyChan       chan bool
	stateTransitionChan chan<- protocolStateTransition
	onceRegister        sync.Once
	onceStart           sync.Once
	onceStop            sync.Once
	doneOnce            sync.Once
	lifecycleMu         sync.Mutex
	started             bool
	stopped             bool
	pendingBytesMu      sync.Mutex
	pendingSendBytes    int
	pendingRecvBytes    int
	pendingRecvSizes    []int // Track sizes of pending received messages for accurate decrement
	currentStateMu      sync.RWMutex
	currentState        State
	// pipelinedDequeueHook, when non-nil, is invoked by sendLoop immediately
	// after dequeuing a message through the pipelined-send path, before the
	// state re-check that decides eligibility or promotion (see
	// resolvePipelinedDequeue). It exists only to let a test deterministically
	// land a concurrent stateLoop transition inside that window, which is
	// otherwise a genuine data race and cannot be forced without it.
	// Production code never sets this.
	pipelinedDequeueHook func()
	// resolvePipelinedDequeuePostFlushHook, when non-nil, is invoked by
	// resolvePipelinedDequeue immediately before its post-flush drain of
	// sendReadyChan and the state read that decides this call's outcome
	// (see observeStateAndDrainSendReady). It exists only to let a test
	// position a concurrent stateLoop transition -- one independent of this
	// call's own flush, e.g. standing in for recvLoop handling a real peer
	// reply -- as close as possible to that decision point; the window is a
	// few adjacent statements wide and a real reproduction only lands it
	// probabilistically. Production code never sets this.
	resolvePipelinedDequeuePostFlushHook func()
	// batchRecheckHook, when non-nil, is invoked by sendLoop's readSendQueueLoop
	// immediately before it re-reads the current state to decide whether a
	// message batched behind an earlier pipelined-dequeue message (one fetched
	// directly from sendQueueChan rather than through resolvePipelinedDequeue)
	// may have its transition applied immediately. It exists only to let a
	// test deterministically land a concurrent stateLoop transition inside
	// that window -- otherwise a genuine data race -- so the batch's own
	// currentState read observes a state that legitimately grants this role
	// agency while earlier messages in the same batch still have their
	// transitions deferred in queuedStateTransitions. Production code never
	// sets this.
	batchRecheckHook func()
}

// ProtocolConfig provides the configuration for Protocol
type ProtocolConfig struct {
	Name                string
	ProtocolId          uint16
	ErrorChan           chan error
	Muxer               *muxer.Muxer
	Logger              *slog.Logger
	Mode                ProtocolMode
	Role                ProtocolRole
	MessageHandlerFunc  MessageHandlerFunc
	MessageFromCborFunc MessageFromCborFunc
	StateMap            StateMap
	StateContext        any
	InitialState        State
	// InitialStateTimeout enables the initial state's configured timeout.
	// It is disabled by default because most protocols begin timing out only
	// after their first state transition.
	InitialStateTimeout bool
	RecvQueueSize       int
	// MaxReadBufferSize overrides maxReadBufferSize's default 16MB cap on
	// how large an incomplete, still-reassembling multi-segment message may
	// grow before readLoop gives up and errors out. Zero means "use the
	// default" -- there is no sensible "unbounded" spelling here (unlike a
	// timeout, where 0 means "wait forever"), since a literal zero-byte
	// buffer could never hold even the smallest message.
	MaxReadBufferSize int
}

// maxReadBufferSize returns the effective read-buffer cap for this config:
// its own MaxReadBufferSize when set, otherwise the package default.
func (c ProtocolConfig) maxReadBufferSize() int {
	if c.MaxReadBufferSize > 0 {
		return c.MaxReadBufferSize
	}
	return maxReadBufferSize
}

// ProtocolMode is an enum of the protocol modes
type ProtocolMode uint

const (
	ProtocolModeNone         ProtocolMode = 0 // Default (invalid) protocol mode
	ProtocolModeNodeToClient ProtocolMode = 1 // Node-to-client protocol mode
	ProtocolModeNodeToNode   ProtocolMode = 2 // Node-to-node protocol mode
)

// ProtocolRole is an enum of the protocol roles
type ProtocolRole uint

// Protocol roles
const (
	ProtocolRoleNone   ProtocolRole = 0 // Default (invalid) protocol role
	ProtocolRoleClient ProtocolRole = 1 // Client protocol role
	ProtocolRoleServer ProtocolRole = 2 // Server protocol role
)

// ProtocolOptions provides common arguments for all mini-protocols
type ProtocolOptions struct {
	ConnectionId connection.ConnectionId
	// ConnectionDoneChan is closed when the owning connection begins
	// shutdown, before protocol goroutines have necessarily returned.
	ConnectionDoneChan <-chan any
	Muxer              *muxer.Muxer
	Logger             *slog.Logger
	ErrorChan          chan error
	Mode               ProtocolMode
	// TODO(cleanup): Role field may be redundant with Mode - evaluate removal
	Role    ProtocolRole
	Version uint16
}

type protocolStateTransition struct {
	msg       Message
	errorChan chan<- error
}

type outboundMessage struct {
	message       Message
	data          []byte
	deliveryChan  chan error
	waitForAgency bool
}

// MessageHandlerFunc represents a function that handles an incoming message
type MessageHandlerFunc func(Message) error

// MessageFromCborFunc represents a function that parses a mini-protocol message
type MessageFromCborFunc func(uint, []byte) (Message, error)

// New returns a new Protocol object
func New(config ProtocolConfig) *Protocol {
	if config.RecvQueueSize == 0 {
		config.RecvQueueSize = DefaultRecvQueueSize
	}
	p := &Protocol{
		config:       config,
		currentState: config.InitialState,
		doneChan:     make(chan struct{}),
		stopChan:     make(chan struct{}),
		recvDoneChan: make(chan struct{}),
		sendDoneChan: make(chan struct{}),
	}
	return p
}

// EnsureRegistered registers the protocol with the muxer if not already registered.
func (p *Protocol) EnsureRegistered() {
	p.onceRegister.Do(func() {
		muxerProtocolRole := muxer.ProtocolRoleInitiator
		if p.config.Role == ProtocolRoleServer {
			muxerProtocolRole = muxer.ProtocolRoleResponder
		}
		// Contribute this protocol's own read-buffer cap to the
		// connection-wide reassembly allowance before any segment can
		// arrive for it.
		p.config.Muxer.RaiseReadBufferBudget(p.config.maxReadBufferSize())
		p.muxerSendChan, p.muxerRecvChan, p.muxerDoneChan = p.config.Muxer.RegisterProtocol(
			p.config.ProtocolId,
			muxerProtocolRole,
		)
	})
}

// Start initializes the mini-protocol
func (p *Protocol) Start() {
	p.onceStart.Do(func() {
		p.lifecycleMu.Lock()
		if p.stopped {
			p.lifecycleMu.Unlock()
			return
		}

		p.EnsureRegistered()

		if p.muxerDoneChan == nil {
			p.lifecycleMu.Unlock()
			p.SendError(errors.New("could not register protocol with muxer"))
			p.closeDone()
			return
		}

		// Create channels
		p.sendQueueChan = make(chan outboundMessage, 80)
		p.recvQueueChan = make(chan Message, p.config.RecvQueueSize)
		p.recvReadyChan = make(chan bool, 1)
		p.sendReadyChan = make(chan bool, 1)

		stateTransitionChan := make(chan protocolStateTransition)
		p.stateTransitionChan = stateTransitionChan

		// Start our send and receive Goroutines
		go func() {
			<-p.recvDoneChan
			<-p.sendDoneChan
			p.closeDone()
		}()

		go p.stateLoop(stateTransitionChan)
		go p.readLoop()
		go p.recvLoop()
		go p.sendLoop()

		p.started = true
		p.lifecycleMu.Unlock()
	})
}

// Stop shuts down the mini-protocol
func (p *Protocol) Stop() {
	p.onceStop.Do(func() {
		p.lifecycleMu.Lock()
		p.stopped = true
		started := p.started
		p.lifecycleMu.Unlock()

		close(p.stopChan)
		if !started {
			p.closeDone()
		}

		// Unregister protocol from muxer
		muxerProtocolRole := muxer.ProtocolRoleInitiator
		if p.config.Role == ProtocolRoleServer {
			muxerProtocolRole = muxer.ProtocolRoleResponder
		}
		if p.config.Muxer == nil {
			return
		}
		p.config.Muxer.UnregisterProtocol(
			p.config.ProtocolId,
			muxerProtocolRole,
		)
	})
}

func (p *Protocol) closeDone() {
	p.doneOnce.Do(func() {
		close(p.doneChan)
	})
}

// Logger returns the protocol logger
func (p *Protocol) Logger() *slog.Logger {
	if p.config.Logger == nil {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return p.config.Logger
}

// Mode returns the protocol mode
func (p *Protocol) Mode() ProtocolMode {
	return p.config.Mode
}

// Role understands the protocol role
func (p *Protocol) Role() ProtocolRole {
	return p.config.Role
}

// DoneChan returns the channel used to signal protocol shutdown
func (p *Protocol) DoneChan() <-chan struct{} {
	return p.doneChan
}

// IsDone returns true if the protocol has finished (done channel is closed or in AgencyNone state).
// Use this to check if the protocol should avoid sending messages (e.g., Done message in Stop()).
func (p *Protocol) IsDone() bool {
	// Check if done channel is closed
	select {
	case <-p.doneChan:
		return true
	default:
	}
	// Check if current state has AgencyNone (terminal state)
	currentState := p.getCurrentState()
	if entry, exists := p.config.StateMap[currentState]; exists {
		if entry.Agency == AgencyNone {
			return true
		}
	}
	return false
}

// IsStopping reports whether shutdown has been requested, even if the
// protocol's worker goroutines have not finished closing DoneChan yet.
func (p *Protocol) IsStopping() bool {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	return p.stopped
}

// IsInTerminalOrIdleState returns true if the protocol is in a state where connection
// close should not be treated as an error. This includes:
// - Protocol stop/done channel is closed
// - Current state has AgencyNone (terminal Done state after receiving Done message)
// - Current state is the initial state (no messages exchanged yet)
func (p *Protocol) IsInTerminalOrIdleState() bool {
	// Check if done channel is closed
	select {
	case <-p.doneChan:
		return true
	default:
	}
	// Check current state
	currentState := p.getCurrentState()
	// Return true if current state has AgencyNone (terminal state)
	if entry, exists := p.config.StateMap[currentState]; exists {
		if entry.Agency == AgencyNone {
			return true
		}
	}
	// Return true if current state is the initial state (no messages sent yet)
	return currentState == p.config.InitialState
}

// WaitSendQueueDrained blocks until the send queue has been drained (best-effort) or the protocol
// begins shutting down, or the timeout expires.
func (p *Protocol) WaitSendQueueDrained(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		// Treat shutdown as "not drained"
		select {
		case <-p.stopChan:
			return false
		case <-p.doneChan:
			return false
		default:
		}

		p.pendingBytesMu.Lock()
		pending := p.pendingSendBytes
		p.pendingBytesMu.Unlock()
		queueLen := 0
		if p.sendQueueChan != nil {
			queueLen = len(p.sendQueueChan)
		}
		if pending == 0 && queueLen == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// getCurrentState returns the current protocol state in a thread-safe manner
func (p *Protocol) getCurrentState() State {
	p.currentStateMu.RLock()
	defer p.currentStateMu.RUnlock()
	return p.currentState
}

// observeStateAndDrainSendReady atomically drains any pending sendReadyChan
// token and reads the current state as a single critical section under
// currentStateMu -- the same lock stateLoop's setState now holds across its
// own state write and token write (see stateLoop). That pairing is what
// makes this call safe against a concurrent transition landing between a
// drain and a state read taken as two separate steps: this call's critical
// section can only run entirely before a given setState call or entirely
// after it, never interleaved with it, so it either sees the old state with
// nothing to drain, or the new state with that state's own token already in
// the channel (if the new state's agency produces one at all) ready to be
// drained here. Used by resolvePipelinedDequeue's post-flush decision,
// where the prior two-step version could leak a token produced by a
// concurrent recvLoop-driven transition -- not only this call's own flush
// -- into a later, unrelated sendLoop iteration (blinklabs-io/gouroboros#2494).
func (p *Protocol) observeStateAndDrainSendReady() State {
	p.currentStateMu.Lock()
	defer p.currentStateMu.Unlock()
	select {
	case <-p.sendReadyChan:
	default:
	}
	return p.currentState
}

// pipelinedSendAllowed reports whether the current state permits our role to
// write queued messages while the peer holds agency. Only the role without
// agency in the state pipelines; the role with agency uses the normal path.
func (p *Protocol) pipelinedSendAllowed() bool {
	return p.roleMayPipeline(p.config.StateMap[p.getCurrentState()])
}

// agencyHolder identifies which side may send in a state.
type agencyHolder int

const (
	agencyNeither agencyHolder = iota
	agencyLocal
	agencyPeer
)

// agencyHolderFor maps a state's agency onto this protocol's role. It is the
// single source of that mapping for stateLoop's ready signals, the pipelining
// check, and the agency checks in sendLoop, so they cannot disagree.
func (p *Protocol) agencyHolderFor(entry StateMapEntry) agencyHolder {
	var local ProtocolStateAgency
	switch p.config.Role {
	case ProtocolRoleClient:
		local = AgencyClient
	case ProtocolRoleServer:
		local = AgencyServer
	case ProtocolRoleNone:
		return agencyNeither
	default:
		return agencyNeither
	}
	switch entry.Agency {
	case AgencyClient, AgencyServer:
		if entry.Agency == local {
			return agencyLocal
		}
		return agencyPeer
	case AgencyNone:
		return agencyNeither
	default:
		return agencyNeither
	}
}

// roleMayPipeline reports whether this role may write while the peer holds
// agency in the state described by entry.
func (p *Protocol) roleMayPipeline(entry StateMapEntry) bool {
	return entry.AllowPipelinedSend && p.agencyHolderFor(entry) == agencyPeer
}

// roleHasAgency reports whether this role holds ordinary send agency in the
// state described by entry.
func (p *Protocol) roleHasAgency(entry StateMapEntry) bool {
	return p.agencyHolderFor(entry) == agencyLocal
}

// messageHasAgencyTransition reports whether msg is legal as an ordinary send
// from any state where this role holds agency. It is only reached when a
// message cannot be sent immediately, never on the steady-state send path.
func (p *Protocol) messageHasAgencyTransition(msg Message) bool {
	for _, entry := range p.config.StateMap {
		if !p.roleHasAgency(entry) {
			continue
		}
		for _, transition := range entry.Transitions {
			if transition.MsgType == msg.Type() {
				return true
			}
		}
	}
	return false
}

// takeAgencyToken reads the current state and, when this role holds agency
// there, consumes the sendReadyChan token setState deposited for it, as one
// critical section under currentStateMu. It is for a caller about to apply a
// transition from that state without waiting on sendReadyChan.
func (p *Protocol) takeAgencyToken() (State, StateMapEntry, bool) {
	p.currentStateMu.Lock()
	defer p.currentStateMu.Unlock()
	state := p.currentState
	entry := p.config.StateMap[state]
	if !p.roleHasAgency(entry) {
		return state, entry, false
	}
	select {
	case <-p.sendReadyChan:
	default:
	}
	return state, entry, true
}

// SendMessage appends a message to the send queue
func (p *Protocol) SendMessage(msg Message) error {
	return p.enqueueMessage(context.Background(), msg, nil)
}

// SendMessageContext appends a message to the send queue, or returns the
// context's error if the message cannot be queued before the context ends.
func (p *Protocol) SendMessageContext(ctx context.Context, msg Message) error {
	return p.enqueueMessage(ctx, msg, nil)
}

// SendMessageAndWait queues a message and waits until its final muxer segment
// is written to the underlying connection. It returns an error if the write
// fails or the protocol shuts down before delivery completes.
func (p *Protocol) SendMessageAndWait(msg Message) error {
	return p.SendMessageContextAndWait(context.Background(), msg)
}

// SendMessageContextAndWait queues a message and waits until its final muxer
// segment is written, or the context is canceled. Cancellation stops waiting;
// an already queued message may still be delivered.
func (p *Protocol) SendMessageContextAndWait(
	ctx context.Context,
	msg Message,
) error {
	deliveryChan := make(chan error, 1)
	if err := p.enqueueMessage(
		ctx,
		msg,
		deliveryChan,
	); err != nil {
		return err
	}
	return p.waitForMessageDeliveryContext(ctx, deliveryChan)
}

func (p *Protocol) waitForMessageDeliveryContext(
	ctx context.Context,
	deliveryChan <-chan error,
) error {
	select {
	case err := <-deliveryChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stopChan:
		return deliveryResultOrShutdown(deliveryChan)
	case <-p.doneChan:
		return deliveryResultOrShutdown(deliveryChan)
	case <-p.muxerDoneChan:
		return deliveryResultOrShutdown(deliveryChan)
	}
}

// deliveryResultOrShutdown preserves a delivery result that was reported
// immediately before its corresponding shutdown signal became observable.
func deliveryResultOrShutdown(deliveryChan <-chan error) error {
	select {
	case err := <-deliveryChan:
		return err
	default:
		return ErrProtocolShuttingDown
	}
}

func (p *Protocol) enqueueMessage(
	ctx context.Context,
	msg Message,
	deliveryChan chan error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Immediately return if we're already shutting down
	select {
	case <-p.stopChan:
		return ErrProtocolShuttingDown
	case <-p.doneChan:
		return ErrProtocolShuttingDown
	case <-p.muxerDoneChan:
		return ErrProtocolShuttingDown
	case <-p.recvDoneChan:
		return ErrProtocolShuttingDown
	case <-p.sendDoneChan:
		return ErrProtocolShuttingDown
	default:
	}

	// Encode once while queueing and keep the bytes in the queue entry. Do not
	// populate the message's DecodeStoreCbor cache: callers may reuse a message
	// instance, and that cache belongs to the caller's message lifecycle.
	var data []byte
	if msg.Cbor() != nil {
		data = msg.Cbor()
	} else {
		var err error
		data, err = cbor.Encode(msg)
		if err != nil {
			return err
		}
	}
	msgLen := len(data)
	// Check pending send bytes limit for current state
	currentState := p.getCurrentState()
	limit := 0
	if entry, ok := p.config.StateMap[currentState]; ok {
		limit = entry.PendingMessageByteLimit
	}
	p.pendingBytesMu.Lock()
	if limit > 0 && p.pendingSendBytes+msgLen > limit {
		p.pendingBytesMu.Unlock()
		p.SendError(ErrProtocolViolationQueueExceeded)
		return ErrProtocolViolationQueueExceeded
	}
	p.pendingSendBytes += msgLen
	p.pendingBytesMu.Unlock()
	outbound := outboundMessage{
		message:      msg,
		data:         data,
		deliveryChan: deliveryChan,
	}
	var enqueueErr error
	select {
	case p.sendQueueChan <- outbound:
		return nil
	case <-ctx.Done():
		enqueueErr = ctx.Err()
	case <-p.stopChan:
	case <-p.doneChan:
	case <-p.muxerDoneChan:
	case <-p.recvDoneChan:
	case <-p.sendDoneChan:
	}

	// The message was accounted for above but never reached the queue.
	p.pendingBytesMu.Lock()
	p.pendingSendBytes -= msgLen
	p.pendingBytesMu.Unlock()
	if enqueueErr != nil {
		return enqueueErr
	}
	return ErrProtocolShuttingDown
}

// SendError sends an error to the handler in the Ouroboros object and stops the protocol.
// This ensures that protocol errors immediately terminate the connection and prevent
// further errors from being generated.
func (p *Protocol) SendError(err error) {
	// Immediately return if we're already shutting down
	select {
	case <-p.stopChan:
		return
	case <-p.doneChan:
		return
	default:
	}
	// Send error to consumer
	select {
	case p.config.ErrorChan <- err:
	default:
		// Discard error if the buffer is full
		// The connection will get closed on the first error, so any
		// additional errors are unnecessary
	}
	// Stop the protocol on any error to prevent further errors from being generated
	// and to ensure the connection is properly terminated
	p.Stop()
}

// flushQueuedStateTransitions applies each deferred pipelined-send state
// transition in send order, stopping at the first error. The messages were
// already written to the wire earlier (while the peer held agency), so their
// transitions must be applied before any later message's own transition.
func (p *Protocol) flushQueuedStateTransitions(
	queuedStateTransitions []Message,
) ([]Message, error) {
	var err error
	applied := 0
	for _, msg := range queuedStateTransitions {
		if err = p.transitionState(msg); err != nil {
			break
		}
		applied++
	}
	return slices.Delete(queuedStateTransitions, 0, applied), err
}

// pipelinedMessageFits reports whether msg is still eligible for the
// pipelined-send path in the state described by entry.
func pipelinedMessageFits(entry StateMapEntry, msg Message) bool {
	return slices.Contains(entry.PipelinedMessageTypes, msg.Type())
}

// errPipelinedMessageNotAllowed reports that msg fits neither the pipelined
// path nor an ordinary transition in state.
func (p *Protocol) errPipelinedMessageNotAllowed(
	state State,
	msg Message,
) error {
	return fmt.Errorf(
		"%s: message type %d is not allowed while pipelined in state %s",
		p.config.Name,
		msg.Type(),
		state,
	)
}

// resolvePipelinedDequeue decides how to handle a message dequeued through
// the pipelined-send path once its type is checked against the *current*
// protocol state, which can legitimately have advanced since
// pipelinedSendAllowed() was last checked at the top of the loop -- e.g. the
// peer's own terminal event (a block-fetch BatchDone) moving
// Busy/Streaming to Idle while a message pipelined earlier is still queued.
//
// If the message is still valid for the pipelined path in the current
// state, it is returned unchanged for the pipelined send below. If this
// role has no ordinary send agency in the current state either, the
// message is a genuine caller ordering violation and is rejected.
//
// Otherwise a real stateLoop transition landed concurrently and granted
// this role ordinary send agency: setState put a token on sendReadyChan for
// it (see setState's AgencyClient/AgencyServer cases), and this dequeue --
// having taken the pipelined path instead of waking via that channel -- has
// not consumed it. The message is held here rather than promoted in place,
// so its fate is decided against the state the protocol actually settles in
// once any deferred work finishes, not the state this dequeue started in.
//
// Any transitions deferred by earlier pipelined sends are flushed as far as
// the current real state allows before that decision is made. Each
// deferred transition needs its own real round trip with the peer, so a
// backlog more than one message deep can only be partly flushed by the
// single concurrent transition that landed here -- flushQueuedStateTransitions
// stopping partway through and returning the remainder is an expected
// outcome, not a fatal error; only a shutdown signal from the flush is
// fatal here. The flush's own transition into a peer-agency state releases
// recvLoop, so the peer's reply to a just-flushed message can return this
// role to agency before the decision below; when that happens and the
// pass applied at least one entry, the flush is repeated rather than the
// message rejected. A backlog that makes no progress in a state where this
// role holds agency is a genuine ordering violation.
//
// A flushed transition applies through the same setState this function's
// own entry token came from, so it can grant this role a fresh token of its
// own for any state the flush passes through, not only the one it finally
// settles in -- sendReadyChan holds at most one token regardless of how
// many transitions produce one, so a drain after the flush clears whichever
// is pending, if any. Left undrained, it would outlive this function and be
// misread as a grant for an unrelated state once the loop runs again.
//
// That drain and the state read that follows it are taken together via
// observeStateAndDrainSendReady, as a single critical section under
// currentStateMu, rather than as two independent steps. A concurrent
// transition is not only this function's own flush: recvLoop can process a
// genuine, independent peer reply at any point while this function runs,
// including in the gap between a drain and a state read taken separately.
// Reading the two under the same lock setState uses for its own state-and-token
// write means such a transition can never leave its token visible without
// its state also being visible here, or the reverse -- this call's critical
// section is entirely before that setState call or entirely after it, never
// interleaved with it.
//
// The message is then re-checked against the state the flush actually
// reached: pipelined again there, promoted to an ordinary transition there,
// held for agency when the peer holds it there and the message is legal from
// some state where this role holds agency (the same rule as before the
// flush), or otherwise rejected, naming the settled state rather than the
// stale one this dequeue started with. Promotion additionally requires the
// backlog to be fully drained: sendLoop applies any remaining queued
// transition before it ever sends a promoted message and loops without
// sending it, so promoting here while a remainder is still queued would
// silently drop the message.
func (p *Protocol) resolvePipelinedDequeue(
	outbound *outboundMessage,
	haveAgency bool,
	queuedStateTransitions *[]Message,
) (*outboundMessage, bool, error) {
	currentState := p.getCurrentState()
	currentEntry := p.config.StateMap[currentState]
	if pipelinedMessageFits(currentEntry, outbound.message) {
		return outbound, haveAgency, nil
	}
	if !p.roleHasAgency(currentEntry) {
		if p.messageHasAgencyTransition(outbound.message) {
			outbound.waitForAgency = true
			return outbound, haveAgency, nil
		}
		return nil, haveAgency, p.errPipelinedMessageNotAllowed(
			currentState,
			outbound.message,
		)
	}

	// Drain the agency token the concurrent transition produced before
	// doing anything else with this message.
	select {
	case <-p.stopChan:
		return nil, haveAgency, ErrProtocolShuttingDown
	case <-p.recvDoneChan:
		return nil, haveAgency, ErrProtocolShuttingDown
	case <-p.sendReadyChan:
	}

	// Each pass that ends with this role holding agency and a backlog still
	// queued must have applied at least one entry, so the loop is bounded by
	// the backlog length. Continuing is safe only because this role holds
	// agency in the observed state: the peer cannot move it before the next
	// pass's flush.
	for {
		before := len(*queuedStateTransitions)
		remaining, flushErr := p.flushQueuedStateTransitions(
			*queuedStateTransitions,
		)
		*queuedStateTransitions = remaining
		if errors.Is(flushErr, ErrProtocolShuttingDown) {
			return nil, haveAgency, flushErr
		}
		progressed := len(remaining) < before

		if p.resolvePipelinedDequeuePostFlushHook != nil {
			p.resolvePipelinedDequeuePostFlushHook()
		}
		// Drain any post-flush token and read the resulting state as a
		// single atomic step (see observeStateAndDrainSendReady above): a
		// token here could come from this function's own flush, or from an
		// entirely independent, concurrent stateLoop transition -- e.g.
		// recvLoop handling a real peer reply -- and either way it must not
		// be observable separately from the state it belongs to.
		postFlushState := p.observeStateAndDrainSendReady()
		postFlushEntry := p.config.StateMap[postFlushState]
		if pipelinedMessageFits(postFlushEntry, outbound.message) {
			return outbound, haveAgency, nil
		}
		if !p.roleHasAgency(postFlushEntry) {
			// Same outcome as the pre-flush check above: the flush can hand
			// agency back to the peer, and a message legal once agency
			// returns is held rather than rejected.
			if p.messageHasAgencyTransition(outbound.message) {
				outbound.waitForAgency = true
				return outbound, haveAgency, nil
			}
		} else if len(*queuedStateTransitions) == 0 {
			if _, err := p.nextState(
				postFlushState,
				outbound.message,
			); err == nil {
				return outbound, true, nil
			}
		} else if progressed {
			continue
		}
		return nil, haveAgency, p.errPipelinedMessageNotAllowed(
			postFlushState,
			outbound.message,
		)
	}
}

// recoverLoop is the panic backstop for the goroutines this protocol runs. It
// must be deferred directly so that recover sees the panic.
//
// Each of those goroutines is started by this package, so nothing above it on
// the stack belongs to the consumer and an escaping panic takes the process
// down -- every other connection with it -- over one peer's message. Reporting
// the panic with SendError gives it the disposition a decode error or a
// protocol violation already has: the consumer reads it from the protocol's
// error channel on a best-effort basis and the connection is torn down. If the
// channel is full, the panic is logged so its stack is not lost.
//
// Continuing is not on offer here. A panic can leave a half-applied state
// transition, an un-decremented byte count or a partly consumed read buffer
// behind it, and a mini-protocol that resumes from any of those has silently
// desynchronised from its peer rather than failed. Failing the one connection
// is the honest containment; the process surviving is the point.
//
// Panics raised by this library's own decoding and panics raised by a callback
// the consumer registered are treated identically, because both run in this
// goroutine and neither is distinguishable from here. The error carries the
// panic value and the stack, which does name the responsible frame, so a
// consumer's programming error stays diagnosable rather than being absorbed.
func (p *Protocol) recoverLoop(where string) {
	err := panics.New(ErrHandlerPanic, p.config.Name+": "+where, recover())
	if err != nil {
		// Report directly instead of using SendError: a protocol-owned
		// shutdown watcher can execute a consumer callback after DoneChan is
		// closed, and SendError deliberately ignores ordinary errors once
		// shutdown has begun. A panic still needs to remain diagnosable.
		select {
		case p.config.ErrorChan <- err:
		default:
			p.Logger().Error("contained panic with a full error channel", "error", err)
		}
		p.Stop()
	}
}

// RunLoop runs a mini-protocol-owned loop in the current goroutine with the
// same panic containment as Protocol's common loops. Callers normally start it
// with go and must supply a stable, diagnostic loop name.
func (p *Protocol) RunLoop(where string, loop func()) {
	defer p.recoverLoop(where)
	loop()
}

func (p *Protocol) sendLoop() {
	defer func() {
		// Close muxer send channel
		// We are responsible for closing this channel as the sender, even through it
		// was created by the muxer
		close(p.muxerSendChan)
		close(p.sendDoneChan)
	}()
	// Registered after the cleanup above so that it runs first and the
	// cleanup still runs on the way out, leaving the muxer's accounting
	// correct whether this loop ends normally or in a panic.
	defer p.recoverLoop("send loop")

	var queuedStateTransitions []Message
	var pipelinedOutbound *outboundMessage
waitSendReadyChan:
	for {
		// haveAgency records that we were woken because the state map granted
		// our role agency, as opposed to being woken by a pipelined message
		// that may be written while the peer holds agency.
		haveAgency := false
		// pipelinedOutbound holds a message dequeued through the pipelined
		// send path. Its state transition, and those of any messages batched
		// behind it, are deferred until agency returns.
		//
		// A held message stays held on pipelinedOutbound != nil alone, not on
		// waitForAgency: once the select below consumes a sendReadyChan token
		// for it, waitForAgency is cleared, but a non-empty
		// queuedStateTransitions backlog below may still force this loop
		// around again (the "Check for queued state transitions" continue)
		// before the read-send-queue section ever reaches pipelinedOutbound
		// to send it. Gating on waitForAgency alone would let that next
		// iteration fall through to the pipelinedSendAllowed() branch instead,
		// which reads sendQueueChan and would silently overwrite
		// pipelinedOutbound with a newly dequeued message -- dropping the
		// held one (blinklabs-io/gouroboros#2494).
		if pipelinedOutbound != nil {
			select {
			case <-p.stopChan:
				return
			case <-p.recvDoneChan:
				return
			case <-p.sendReadyChan:
				haveAgency = true
				pipelinedOutbound.waitForAgency = false
			}
		} else if p.pipelinedSendAllowed() {
			select {
			case <-p.stopChan:
				return
			case <-p.recvDoneChan:
				// Break out of send loop if we're shutting down
				return
			case <-p.sendReadyChan:
				// We are ready to send based on state map
				haveAgency = true
			case outbound, ok := <-p.sendQueueChan:
				if !ok {
					// We're shutting down
					return
				}
				tmpOutbound := outbound
				if p.pipelinedDequeueHook != nil {
					p.pipelinedDequeueHook()
				}
				var err error
				pipelinedOutbound, haveAgency, err = p.resolvePipelinedDequeue(
					&tmpOutbound,
					haveAgency,
					&queuedStateTransitions,
				)
				if err != nil {
					if errors.Is(err, ErrProtocolShuttingDown) {
						return
					}
					p.SendError(err)
					return
				}
				if pipelinedOutbound != nil && pipelinedOutbound.waitForAgency {
					// resolvePipelinedDequeue decided this message must wait
					// for real agency: it neither fits the pipelined path nor
					// may be sent yet as an ordinary transition. Loop back to
					// the top immediately rather than falling through into
					// the read-send-queue section below, which treats any
					// non-nil pipelinedOutbound as ready to write regardless
					// of waitForAgency and would put it on the wire right now
					// while the peer still holds agency
					// (blinklabs-io/gouroboros#2494).
					continue waitSendReadyChan
				}
			}
		} else {
			select {
			case <-p.stopChan:
				return
			case <-p.recvDoneChan:
				// Break out of send loop if we're shutting down
				return
			case <-p.sendReadyChan:
				// We are ready to send based on state map
				haveAgency = true
			}
		}

		// Check for queued state transitions. A pipelined message is written
		// while the peer holds agency, so it can never satisfy a deferred
		// transition and must not delay the bytes we already have in hand.
		if haveAgency && len(queuedStateTransitions) > 0 {
			if err := p.transitionState(queuedStateTransitions[0]); err != nil {
				if errors.Is(err, ErrProtocolShuttingDown) {
					// Graceful shutdown in progress
					return
				}
				p.SendError(
					fmt.Errorf(
						"%s: error sending message: %w",
						p.config.Name,
						err,
					),
				)
				return
			}
			queuedStateTransitions = slices.Delete(queuedStateTransitions, 0, 1)
			// Don't read more messages from the send queue until all state transitions from
			// previously sent pipelined messages have been completed
			continue waitSendReadyChan
		}

		// Read queued messages and write into buffer
		payloadBuf := bytes.NewBuffer(nil)
		msgCount := 0
		// A message taken from the pipelined send path goes out while the peer
		// holds agency, so its transition is deferred along with those of any
		// messages batched behind it.
		queueTransition := !haveAgency
		var deliveryChan chan error
	readSendQueueLoop:
		for {
			// Get next message from send queue
			var outbound outboundMessage
			fromPipelinedDequeue := pipelinedOutbound != nil
			if pipelinedOutbound != nil {
				outbound = *pipelinedOutbound
				pipelinedOutbound = nil
			} else {
				select {
				case <-p.stopChan:
					return
				case <-p.recvDoneChan:
					// Break out of send loop if we're shutting down
					return
				case queued, ok := <-p.sendQueueChan:
					if !ok {
						// We're shutting down
						return
					}
					outbound = queued
				}
			}
			msg := outbound.message
			if queueTransition && !fromPipelinedDequeue {
				if p.batchRecheckHook != nil {
					p.batchRecheckHook()
				}
				// takeAgencyToken consumes the token setState deposited for
				// currentState only when the backlog is empty and this role
				// holds agency there, i.e. exactly when msg's transition is
				// applied below without waiting on sendReadyChan. Left in
				// the channel, that token would wake a later iteration in
				// whatever state msg's transition leads to.
				var currentState State
				var currentEntry StateMapEntry
				bypassDeferral := false
				if len(queuedStateTransitions) == 0 {
					currentState, currentEntry, bypassDeferral = p.takeAgencyToken()
				} else {
					currentState = p.getCurrentState()
					currentEntry = p.config.StateMap[currentState]
				}
				// Bypassing deferral here applies msg's transition
				// immediately via transitionState below instead of queueing
				// it. That is only safe when queuedStateTransitions is empty:
				// an earlier message in this same batch may already be
				// waiting there for its own deferred transition, and
				// currentState can reflect a real, concurrent transition
				// (e.g. a peer reply) that has nothing to do with that
				// backlog. Applying msg's transition ahead of it would run
				// transitions out of the order the messages were sent in and
				// leave the backlog's own agency token undrained -- the same
				// token/state pairing problem this whole fix exists to
				// prevent (blinklabs-io/gouroboros#2494). When the backlog is
				// non-empty this falls through to the ordinary
				// pipelined-or-wait-for-agency handling below instead.
				if bypassDeferral {
					queueTransition = false
				} else if !p.roleMayPipeline(currentEntry) ||
					!pipelinedMessageFits(currentEntry, msg) {
					if !p.messageHasAgencyTransition(msg) {
						p.SendError(p.errPipelinedMessageNotAllowed(currentState, msg))
						return
					}
					// Keep the message for the next agency window. Earlier
					// transitions in this batch may return agency before this
					// message becomes legal (for example, Done after RequestNext).
					tmpOutbound := outbound
					tmpOutbound.waitForAgency = true
					pipelinedOutbound = &tmpOutbound
					break readSendQueueLoop
				}
			}
			msgCount = msgCount + 1

			data := outbound.data
			payloadBuf.Write(data)
			// After sending, decrement pendingSendBytes
			p.pendingBytesMu.Lock()
			p.pendingSendBytes -= len(data)
			if p.pendingSendBytes < 0 {
				p.Logger().Warn(
					"negative pendingSendBytes reset",
					"protocol", p.config.Name,
					"value", p.pendingSendBytes,
				)
				p.pendingSendBytes = 0
			}
			p.pendingBytesMu.Unlock()

			if queueTransition {
				queuedStateTransitions = append(queuedStateTransitions, msg)
			} else {
				if err := p.transitionState(msg); err != nil {
					if errors.Is(err, ErrProtocolShuttingDown) {
						// Graceful shutdown in progress
						return
					}
					p.SendError(
						fmt.Errorf(
							"%s: error sending message: %w",
							p.config.Name,
							err,
						),
					)
					return
				}
				// Queue any state transitions after the initial one for this message batch
				queueTransition = true
			}
			if outbound.deliveryChan != nil {
				deliveryChan = outbound.deliveryChan
				break readSendQueueLoop
			}

			// We don't want more than maxMessagesPerSegment messages in a segment
			if msgCount >= maxMessagesPerSegment {
				break readSendQueueLoop
			}
			// We don't want to add more messages once we spill over into a second segment
			if payloadBuf.Len() > muxer.SegmentMaxPayloadLength {
				break readSendQueueLoop
			}
			// Check if there are any more queued messages
			if len(p.sendQueueChan) == 0 {
				break readSendQueueLoop
			}
		}

		// Send messages in multiple segments (if needed)
		for {
			// Determine segment payload length
			segmentPayloadLength := min(
				payloadBuf.Len(),
				muxer.SegmentMaxPayloadLength,
			)
			// Send current segment
			segmentPayload := payloadBuf.Bytes()[:segmentPayloadLength]
			isResponse := p.Role() == ProtocolRoleServer
			segment := muxer.NewSegment(
				p.config.ProtocolId,
				segmentPayload,
				isResponse,
			)
			if segment == nil {
				p.SendError(
					fmt.Errorf(
						"%s: failed to create muxer segment",
						p.config.Name,
					),
				)
				return
			}
			if deliveryChan != nil && segmentPayloadLength == payloadBuf.Len() {
				segment.SetDeliveryChan(deliveryChan)
			}
			select {
			case <-p.stopChan:
				return
			case <-p.recvDoneChan:
				return
			case p.muxerSendChan <- segment:
			}
			// Remove current segment's data from buffer
			if payloadBuf.Len() > segmentPayloadLength {
				payloadBuf = bytes.NewBuffer(
					payloadBuf.Bytes()[segmentPayloadLength:],
				)
			} else {
				break
			}
		}
	}
}

// reserveReadBuffer aligns this protocol's share of the connection-wide
// reassembly allowance with readBuffer's current length, reporting whether
// the connection still had room. Shrinking always succeeds.
func (p *Protocol) reserveReadBuffer(current int, reserved *int) bool {
	if p.config.Muxer == nil {
		*reserved = current
		return true
	}
	switch {
	case current > *reserved:
		if !p.config.Muxer.ReserveReadBuffer(current - *reserved) {
			return false
		}
	case current < *reserved:
		p.config.Muxer.ReleaseReadBuffer(*reserved - current)
	}
	*reserved = current
	return true
}

// errReadBufferBudget reports that other mini-protocols on this connection
// already hold its reassembly allowance.
func (p *Protocol) errReadBufferBudget(size int) error {
	budget := 0
	if p.config.Muxer != nil {
		budget = p.config.Muxer.ReadBufferBudget()
	}
	return fmt.Errorf(
		"%s: connection read buffer budget exhausted reassembling"+
			" %d bytes (connection limit %d bytes)",
		p.config.Name,
		size,
		budget,
	)
}

// appendSegment bounds readBuffer before it grows. The per-protocol cap
// decides "too big" and is therefore checked first: the connection-wide
// allowance is the largest registered cap, so reserving first would report
// contention against a message that simply exceeds this protocol's own
// limit. Both checks precede bytes.Buffer.Write, which grows and copies,
// so writing first would take the allocation the allowance exists to
// refuse.
func (p *Protocol) appendSegment(
	readBuffer *bytes.Buffer,
	payload []byte,
	reserved *int,
) error {
	pendingLen := readBuffer.Len() + len(payload)
	if pendingLen > p.config.maxReadBufferSize() {
		return fmt.Errorf(
			"%s: read buffer exceeded maximum size (%d bytes)",
			p.config.Name,
			pendingLen,
		)
	}
	if !p.reserveReadBuffer(pendingLen, reserved) {
		return p.errReadBufferBudget(pendingLen)
	}
	readBuffer.Write(payload)
	return nil
}

func (p *Protocol) readLoop() {
	defer p.recoverLoop("read loop")
	leftoverData := false
	readBuffer := bytes.NewBuffer(nil)
	// Bytes this protocol holds against the connection-wide reassembly
	// allowance. The per-protocol cap below bounds one mini-protocol; this
	// is what keeps every mini-protocol on the connection bounded together.
	reserved := 0
	defer func() {
		if p.config.Muxer != nil && reserved > 0 {
			p.config.Muxer.ReleaseReadBuffer(reserved)
		}
	}()

	for {
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			select {
			case <-p.stopChan:
				return
			case <-p.sendDoneChan:
				// Break out of receive loop if we're shutting down
				return
			case <-p.muxerDoneChan:
				return
			case segment, ok := <-p.muxerRecvChan:
				if !ok {
					return
				}
				if err := p.appendSegment(
					readBuffer, segment.Payload, &reserved,
				); err != nil {
					p.SendError(err)
					return
				}
			}
			// Opportunistically drain any additional segments the muxer
			// has already queued for us before spending a decode attempt.
			// cbor.Decode has no way to resume a prior partial parse -- on
			// an incomplete buffer it walks from byte 0 through every
			// already-buffered element before rediscovering
			// io.ErrUnexpectedEOF, so a message spanning many segments
			// pays that full-buffer cost again on every single one.
			// Batching whatever the muxer has already queued into one
			// attempt reduces how many times that happens for a message
			// arriving as a fast back-to-back segment burst -- confirmed
			// live against a real ~3.17M-entry LocalStateQuery
			// GetUTxOWhole reply, which pegged the receiving process at
			// 100% CPU for 25+ minutes and never finished
			// (blinklabs-io/dingo#1900) -- without changing behavior for
			// the common case (nothing to drain when a message completes
			// in its first segment or two).
			//
			// A further, size-growth-gated skip (only re-attempt once the
			// buffer has grown substantially since the last attempt) was
			// tried here and reverted: it broke
			// TestUnpipelinedIdleLimitRejectsMainnetSizedBlock by skipping
			// forever once a message's growth stopped satisfying the gate
			// with no further segments ever arriving to satisfy it (the
			// message was already fully buffered and complete, but never
			// got decoded). A correct version of that optimization needs a
			// bounded fallback -- e.g. a short timeout alongside the drain
			// select -- rather than an unconditional size gate; left as
			// future work rather than shipped without full validation.
			//
			// This select must watch the same shutdown channels the outer
			// one above does, and must bound readBuffer itself: a peer that
			// keeps the channel non-empty by sending segments as fast as this
			// loop drains them would otherwise let it spin unboundedly on both
			// counts -- ignoring shutdown, and growing readBuffer past
			// p.config.maxReadBufferSize() before ever returning control to
			// check it (CWE-400, caught in review on
			// blinklabs-io/gouroboros#2291). appendSegment is that bound, and
			// is the one place that decides "too big" for both receive paths.
		drainQueued:
			for {
				select {
				case <-p.stopChan:
					return
				case <-p.sendDoneChan:
					return
				case <-p.muxerDoneChan:
					return
				case segment, ok := <-p.muxerRecvChan:
					if !ok {
						return
					}
					if err := p.appendSegment(
						readBuffer, segment.Payload, &reserved,
					); err != nil {
						p.SendError(err)
						return
					}
				default:
					break drainQueued
				}
			}
		}
		leftoverData = false
		// Check for zero-byte read before attempting to decode
		if readBuffer.Len() == 0 {
			// This can happen when the remote host closes the connection unexpectedly
			// or sends an empty segment payload.
			// Don't report an error if we're in the Done state (AgencyNone) or initial state,
			// as this is expected behavior when the remote client gracefully stopped the protocol.
			if !p.IsInTerminalOrIdleState() {
				p.SendError(
					fmt.Errorf(
						"%s: received zero-byte read, connection may have been closed",
						p.config.Name,
					),
				)
			}
			return
		}
		// Decode message into generic list until we can determine what type of message it is.
		// This also lets us determine how many bytes the message is. We use RawMessage here to
		// avoid parsing things that we may not be able to parse
		tmpMsg := []cbor.RawMessage{}
		numBytesRead, err := cbor.Decode(readBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) && readBuffer.Len() > 0 {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				continue
			}
			p.SendError(fmt.Errorf("%s: decode error: %w", p.config.Name, err))
			return
		}
		// Check for zero bytes read or empty message array after decoding
		if numBytesRead == 0 || len(tmpMsg) == 0 {
			// This can happen when the remote host closes the connection unexpectedly
			// and we receive a segment that decodes to an empty array.
			// Don't report an error if we're in the Done state (AgencyNone) or initial state,
			// as this is expected behavior when the remote client gracefully stopped the protocol.
			if !p.IsInTerminalOrIdleState() {
				p.SendError(
					fmt.Errorf(
						"%s: received empty message (zero bytes read or empty CBOR array), connection may have been closed",
						p.config.Name,
					),
				)
			}
			return
		}
		// Decode first list item to determine message type
		var msgType uint
		if _, err := cbor.Decode(tmpMsg[0], &msgType); err != nil {
			p.SendError(fmt.Errorf("%s: decode error: %w", p.config.Name, err))
			return
		}
		// Create Message object from CBOR
		msgData := readBuffer.Bytes()[:numBytesRead]
		msg, err := p.config.MessageFromCborFunc(msgType, msgData)
		if err != nil {
			p.SendError(err)
			return
		}
		if msg == nil {
			p.SendError(
				fmt.Errorf(
					"%s: received unknown message type: %#v",
					p.config.Name,
					tmpMsg,
				),
			)
			return
		}
		// Calculate message size
		msgLen := len(msgData)
		// Wait for pending recv bytes to drop below limit before accepting.
		// This applies TCP backpressure to the remote peer instead of
		// disconnecting with a protocol violation during rapid catch-up sync.
		currentState := p.getCurrentState()
		limit := 0
		if entry, ok := p.config.StateMap[currentState]; ok {
			limit = entry.PendingMessageByteLimit
		}
		if limit > 0 {
			// Fail fast if a single message exceeds the limit to prevent
			// a livelock where the backpressure loop can never make progress.
			if msgLen > limit {
				p.SendError(
					fmt.Errorf(
						"%s: received oversized message (%d bytes) exceeding limit (%d bytes)",
						p.config.Name,
						msgLen,
						limit,
					),
				)
				return
			}
			for {
				p.pendingBytesMu.Lock()
				if p.pendingRecvBytes+msgLen <= limit {
					p.pendingRecvBytes += msgLen
					p.pendingRecvSizes = append(p.pendingRecvSizes, msgLen)
					p.pendingBytesMu.Unlock()
					break
				}
				p.pendingBytesMu.Unlock()
				// Wait briefly for recvLoop to drain pending bytes
				select {
				case <-p.stopChan:
					return
				case <-p.muxerDoneChan:
					return
				case <-time.After(time.Millisecond):
				}
			}
		} else {
			p.pendingBytesMu.Lock()
			p.pendingRecvBytes += msgLen
			p.pendingRecvSizes = append(p.pendingRecvSizes, msgLen)
			p.pendingBytesMu.Unlock()
		}
		// Add message to receive queue (blocking with shutdown checks)
		select {
		case p.recvQueueChan <- msg:
		case <-p.stopChan:
			return
		case <-p.muxerDoneChan:
			return
		}
		if numBytesRead < readBuffer.Len() {
			// There is another message in the same muxer segment, so we reset the buffer with just
			// the remaining data
			readBuffer = bytes.NewBuffer(readBuffer.Bytes()[numBytesRead:])
			leftoverData = true
		} else {
			// Empty out our buffer since we successfully processed the message
			readBuffer.Reset()
		}
		// Hand the consumed bytes back to the connection-wide allowance.
		_ = p.reserveReadBuffer(readBuffer.Len(), &reserved)
	}
}

func (p *Protocol) recvLoop() {
	defer func() {
		close(p.recvDoneChan)
	}()
	defer p.recoverLoop("receive loop")

	for {
		// Wait until ready to receive based on state map
		select {
		case <-p.stopChan:
			return
		case <-p.sendDoneChan:
			// Break out of receive loop if we're shutting down
			return
		case <-p.muxerDoneChan:
			return
		case <-p.recvReadyChan:
		}
		// Read next message from queue
		select {
		case <-p.stopChan:
			return
		case <-p.sendDoneChan:
			// Break out of receive loop if we're shutting down
			return
		case <-p.muxerDoneChan:
			return
		case msg := <-p.recvQueueChan:
			// Handle message
			if err := p.handleMessage(msg); err != nil {
				if errors.Is(err, ErrProtocolShuttingDown) {
					// Graceful shutdown in progress
					return
				}
				p.SendError(err)
				return
			}
			// After handling, decrement pendingRecvBytes by the actual message size
			p.pendingBytesMu.Lock()
			if len(p.pendingRecvSizes) > 0 {
				size := p.pendingRecvSizes[0]
				p.pendingRecvSizes = p.pendingRecvSizes[1:]
				p.pendingRecvBytes -= size
				if p.pendingRecvBytes < 0 {
					p.pendingRecvBytes = 0
				}
			}
			p.pendingBytesMu.Unlock()
		}
	}
}

func (p *Protocol) stateLoop(ch <-chan protocolStateTransition) {
	defer p.recoverLoop("state loop")
	var transitionTimer *time.Timer
	var initialStateSet bool

	setState := func(s State) {
		// Disable any previous state transition timer
		if transitionTimer != nil && !transitionTimer.Stop() {
			<-transitionTimer.C
		}
		transitionTimer = nil

		// Set the new state and, in the same critical section, mark the
		// protocol ready to send/receive based on the new state's role and
		// agency. Folding the sendReadyChan/recvReadyChan signal into the
		// same lock as the state write is what makes
		// observeStateAndDrainSendReady's paired read-and-drain sound: any
		// caller taking currentStateMu is now guaranteed to see this whole
		// state transition -- state and its token together -- or none of
		// it, never a state visible with its token still pending. Before
		// this, the token write happened after Unlock with no
		// synchronization of its own, so a concurrent reader could observe
		// the new state via currentStateMu while the token that belongs to
		// it had not yet been written, and a non-blocking drain taken just
		// before that reader's state read would miss it entirely --
		// stranding the token to be misread by a later, unrelated
		// sendLoop iteration as agency for a different state (see
		// resolvePipelinedDequeue and blinklabs-io/gouroboros#2494).
		p.currentStateMu.Lock()
		p.currentState = s
		skipTimeout := false
		switch p.agencyHolderFor(p.config.StateMap[s]) {
		case agencyLocal:
			select {
			case p.sendReadyChan <- true:
			default:
			}
		case agencyPeer:
			select {
			case p.recvReadyChan <- true:
			default:
			}
		case agencyNeither:
			skipTimeout = true
		}
		p.currentStateMu.Unlock()

		if skipTimeout {
			return
		}

		// Don't activate timeouts on initial protocol state unless explicitly
		// enabled by the protocol owner.
		if !initialStateSet && !p.config.InitialStateTimeout {
			return
		}

		// Set timeout for state transition
		entry := p.config.StateMap[s]
		timeout := entry.Timeout
		if entry.TimeoutFunc != nil {
			timeout = entry.TimeoutFunc()
		}
		if timeout > 0 {
			transitionTimer = time.NewTimer(timeout)
		}
	}
	getTimerChan := func() <-chan time.Time {
		if transitionTimer == nil {
			return nil
		}
		return transitionTimer.C
	}

	// Set initial state
	setState(p.config.InitialState)
	initialStateSet = true

	for {
		select {
		case <-p.stopChan:
			// Disable any previous state transition timer, as they are no longer needed
			if transitionTimer != nil && !transitionTimer.Stop() {
				<-transitionTimer.C
			}
			return
		case t := <-ch:
			nextState, err := p.nextState(p.getCurrentState(), t.msg)
			if err != nil {
				t.errorChan <- fmt.Errorf(
					"%s: error handling protocol state transition: %w",
					p.config.Name,
					err,
				)

				// It is the responsibility of the caller to initiate the shutdown of the protocol,
				// so the state handler should keep running to ensure other state transitions
				// requesters do not encounter a deadlock
				continue
			}

			setState(nextState)
			t.errorChan <- nil

		case <-getTimerChan():
			transitionTimer = nil

			p.SendError(
				fmt.Errorf(
					"%s: timeout waiting on transition from protocol state %s",
					p.config.Name,
					p.getCurrentState(),
				),
			)

		case <-p.doneChan:
			// Disable any previous state transition timer, as they are no longer needed
			if transitionTimer != nil && !transitionTimer.Stop() {
				<-transitionTimer.C
			}
			return
		}
	}
}

func (p *Protocol) nextState(currentState State, msg Message) (State, error) {
	for _, transition := range p.config.StateMap[currentState].Transitions {
		if transition.MsgType == msg.Type() {
			if transition.MatchFunc != nil {
				// Skip item if match function returns false
				if !transition.MatchFunc(p.config.StateContext, msg) {
					continue
				}
			}
			return transition.NewState, nil
		}
	}

	return State{}, fmt.Errorf(
		"message %T not allowed in current protocol state %s",
		msg,
		currentState,
	)
}

func (p *Protocol) transitionState(msg Message) error {
	errorChan := make(chan error, 1)
	select {
	case <-p.stopChan:
		return ErrProtocolShuttingDown
	case <-p.doneChan:
		return ErrProtocolShuttingDown
	case p.stateTransitionChan <- protocolStateTransition{msg, errorChan}:
	}

	select {
	case err := <-errorChan:
		return err
	case <-p.stopChan:
		return ErrProtocolShuttingDown
	case <-p.doneChan:
		return ErrProtocolShuttingDown
	}
}

func (p *Protocol) handleMessage(msg Message) error {
	if err := p.transitionState(msg); err != nil {
		return fmt.Errorf("%s: error handling message: %w", p.config.Name, err)
	}

	// Call handler function
	return p.config.MessageHandlerFunc(msg)
}
