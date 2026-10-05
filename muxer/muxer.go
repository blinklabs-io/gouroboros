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

// Package muxer implements the muxer/demuxer that allows multiple mini-protocols to run
// over a single connection.
//
// It's not generally intended for this package to be used outside of this library, but it's
// possible to use it to do more advanced things than the library interface allows for.
package muxer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Magic number chosen to represent unknown protocols
const ProtocolUnknown uint16 = 0xabcd

// defaultSegmentReadTimeout is the default maximum time to wait for the next
// segment before closing the connection. This is meant to prevent
// slowloris-style DoS attacks against an untrusted remote peer.
//
// This is a gouroboros-specific implementation choice, not a requirement of
// the Ouroboros Network Specification: the spec's Multiplexing chapter
// defines no timeout at the mux/transport layer at all, and per-protocol
// timeouts are instead specified individually, per state, in each
// mini-protocol's own chapter. LocalStateQuery's own timeout table
// (section 3.13.4) reads "No timeouts" -- a large query is expected to be
// able to take an arbitrarily long time. The real ouroboros-network
// (Haskell) implementation matches this: its own mux-level SDU timeout (30s)
// only bounds an already-in-progress segment read (a minimum-bandwidth
// guard), never how long a peer may take before replying at all, and it is
// not applied at all on local Unix-domain-socket connections -- exactly the
// transport LocalStateQuery normally uses. Applying a fixed, unconditional
// deadline to every connection (as this constant did on its own, with no
// way to disable it) killed legitimate, still-computing LocalStateQuery
// replies that the spec says must not be timed out. See
// WithMuxerSegmentReadTimeout to override or disable this for a connection
// known to be a trusted NtC channel.
const defaultSegmentReadTimeout = 120 * time.Second

// segmentWriteTimeout is the maximum time to wait for a complete segment
// write. The deadline is refreshed for every segment so long-lived healthy
// connections are not killed by one absolute connection deadline.
const segmentWriteTimeout = 2 * time.Minute

// DefaultIngressLimit is the ingress limit a receiver gets when it is
// registered with RegisterProtocol and no limit is set for it with
// SetIngressLimit. It is not a specification value: it matches the default
// size of the largest message a mini-protocol will reassemble, so a receiver
// registered directly on the muxer is bounded somewhere. The protocol
// package sets each mini-protocol's own limit as it registers.
const DefaultIngressLimit = 16 * 1024 * 1024 // 16MB

// DefaultIngressBudget is the connection-wide ingress budget a muxer starts
// with: the most segment payload, summed over every protocol role, that may
// be received and not yet delivered to its protocol. The Ouroboros Network
// Specification does not mandate an aggregate figure, so this is an
// implementation bound. It stops many individually bounded queues from
// adding up to an unbounded total. A protocol that solicits more than its
// ordinary limit extends the budget by the excess for as long as it has
// asked for it (see SetIngressBudgetExtension), so the budget never refuses
// a reply the protocol has said it will accept.
const DefaultIngressBudget = 64 * 1024 * 1024 // 64MB

// ErrIngressOverflow is reported when the segment payload queued for one
// protocol role, received and not yet delivered to that protocol, would
// exceed its ingress limit, or when the payload queued across every protocol
// role would exceed the connection's ingress budget. The muxer stops with
// this error rather than waiting, because its read loop is shared by every
// protocol on the connection, unless backpressure is enabled for that
// protocol role (see SetIngressBackpressure).
var ErrIngressOverflow = errors.New(
	"muxer: protocol ingress exceeded an ingress limit",
)

// errSegmentChannelClosed marks ingress arriving for a receiver that has
// already been unregistered; the muxer read loop drops it silently rather
// than treating it as a protocol violation.
var errSegmentChannelClosed = errors.New("segment channel closed")

// errMuxerStopping marks ingress abandoned because the muxer is stopping
// while the read loop waited for room under backpressure.
var errMuxerStopping = errors.New("muxer stopping")

// DiffusionMode is an enum for the valid muxer diffusion modes
type DiffusionMode int

// Diffusion modes
const (
	DiffusionModeNone                  DiffusionMode = 0 // Default (invalid) diffusion mode
	DiffusionModeInitiator             DiffusionMode = 1 // Initiator-only (client) diffusion mode
	DiffusionModeResponder             DiffusionMode = 2 // Responder-only (server) diffusion mode
	DiffusionModeInitiatorAndResponder DiffusionMode = 3 // Initiator and responder (full duplex) mode
)

// ProtocolRole is an enum of the protocol roles
type ProtocolRole uint

// Protocol roles
const (
	ProtocolRoleNone      ProtocolRole = 0 // Default (invalid) protocol role
	ProtocolRoleInitiator ProtocolRole = 1 // Initiator (client) protocol role
	ProtocolRoleResponder ProtocolRole = 2 // Responder (server) protocol role
)

// Muxer wraps a connection to allow running multiple mini-protocols over a single connection
type Muxer struct {
	errorChan              chan error
	conn                   net.Conn
	egress                 egress
	startChan              chan bool
	doneChan               chan bool
	waitGroup              sync.WaitGroup
	waitGroupMutex         sync.Mutex
	protocolSenders        map[uint16]map[ProtocolRole]*segmentSender
	protocolReceivers      map[uint16]map[ProtocolRole]*segmentChannel
	protocolTombstones     map[uint16]map[ProtocolRole]struct{}
	protocolReceiversMutex sync.Mutex
	diffusionMode          atomic.Int64
	onceStop               sync.Once
	// segmentReadTimeout bounds how long the read loop waits for the next
	// segment before closing the connection. <= 0 disables the deadline
	// entirely (no timeout, matching real cardano-node's own behavior on a
	// trusted local/NtC connection). See defaultSegmentReadTimeout's doc
	// comment for the full rationale.
	segmentReadTimeout time.Duration
	// readBufferMu guards the connection-wide message reassembly
	// allowance below.
	readBufferMu sync.Mutex
	// readBufferBudget is the total bytes every mini-protocol on this
	// connection may hold in an incomplete-message reassembly buffer at
	// once. Each mini-protocol caps its own buffer independently, so
	// without this the connection's exposure is that cap times the number
	// of registered protocol roles -- 16 on a full-duplex node-to-node
	// connection. Zero means unmetered, which is what a Muxer with no
	// registered protocol has.
	readBufferBudget int
	// readBufferUsed is the portion of readBufferBudget currently reserved.
	readBufferUsed int
	// ingress accounts the segment payload queued across every protocol
	// role on this connection against the connection's ingress budget.
	ingress *ingressAccount
	metrics atomic.Pointer[Metrics]
	// registerHook, when non-nil, runs in RegisterProtocol between its
	// first shutdown check and the insertion of the new receiver. It exists
	// only so a test can run a complete Stop inside that window, which is
	// otherwise a few statements wide. Production code never sets it.
	registerHook func()
}

// Metrics is an optional set of callbacks for observing per-protocol
// ingress, for a consumer to wire to a metrics library of its own choosing.
// The muxer's read loop never waits on a protocol's consumer, so these
// report where waiting happens instead: in each protocol role's own ingress
// queue and delivery.
type Metrics interface {
	// IngressQueueDepth reports the segment payload bytes received for a
	// protocol role and not yet delivered to its receive channel. It is
	// called with the new depth after every enqueue and every dequeue,
	// while that queue's lock is held, so the values arrive in order; an
	// implementation must return promptly and must not call back into the
	// muxer. A depth that stays near the protocol's IngressLimit means its
	// consumer is not keeping up.
	IngressQueueDepth(protocolId uint16, protocolRole ProtocolRole, bytes int)
	// IngressDeliveryBlocked reports how long delivery of one segment to a
	// protocol role waited for that protocol to make room in its receive
	// channel. It is called only for deliveries that had to wait. This is
	// the time a slow consumer holds up its own protocol, and only that
	// protocol.
	IngressDeliveryBlocked(
		protocolId uint16,
		protocolRole ProtocolRole,
		d time.Duration,
	)
	// IngressBackpressure reports how long the read loop stopped reading
	// the connection because a protocol role with backpressure enabled
	// (see Muxer.SetIngressBackpressure) had reached its ingress limit. It
	// is called only for reads that had to wait. Unlike
	// IngressDeliveryBlocked, this wait holds up every protocol on the
	// connection, keep-alive included.
	IngressBackpressure(
		protocolId uint16,
		protocolRole ProtocolRole,
		d time.Duration,
	)
}

// SetMetrics registers optional hooks for observing muxer ingress. It may be
// called at any time, and applies to every event after it returns. A nil
// value removes the hooks.
func (m *Muxer) SetMetrics(metrics Metrics) {
	if metrics == nil {
		m.metrics.Store(nil)
		return
	}
	m.metrics.Store(&metrics)
}

func (m *Muxer) getMetrics() Metrics {
	p := m.metrics.Load()
	if p == nil {
		return nil
	}
	return *p
}

// SetIngressBudget sets the most segment payload, in bytes, the muxer will
// hold across every protocol role on the connection between reading it from
// the connection and delivering it to the protocols. It bounds the sum of the
// per-role ingress queues, which each have their own limit under
// SetIngressLimit. Ingress past the budget stops the muxer with
// ErrIngressOverflow, except for a protocol role with backpressure enabled
// (see SetIngressBackpressure), which pauses the read loop instead. A budget
// of zero or less restores DefaultIngressBudget. It applies from the next
// segment received.
func (m *Muxer) SetIngressBudget(budget int) {
	m.ingress.setBudget(budget)
}

// IngressBudget returns the connection-wide ingress budget in bytes: the
// budget set with SetIngressBudget plus every protocol role's extension.
func (m *Muxer) IngressBudget() int {
	return m.ingress.getBudget()
}

// IngressInUse returns the segment payload currently queued across every
// protocol role on the connection.
func (m *Muxer) IngressInUse() int {
	return m.ingress.inUse()
}

// SetIngressBudgetExtension raises the connection's ingress budget by extra
// bytes for as long as it is set, on behalf of a registered protocol role
// whose ingress limit is that much above its ordinary limit. A protocol that
// solicits a large amount of data raises its limit with SetIngressLimit and
// extends the budget by the same excess, so the budget bounds ordinary
// ingress without refusing data the protocol asked for. The extension is
// replaced by each call, a value of zero or less clears it, and it ends when
// the role is unregistered. It reports false when the protocol role is not
// registered.
func (m *Muxer) SetIngressBudgetExtension(
	protocolId uint16,
	protocolRole ProtocolRole,
	extra int,
) bool {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	recvChan, ok := m.protocolReceivers[protocolId][protocolRole]
	if !ok {
		return false
	}
	recvChan.setBudgetExtension(extra)
	return true
}

// SetIngressLimit sets the most segment payload, in bytes, the muxer will
// hold for a registered protocol role between reading it from the
// connection and delivering it to the protocol. Ingress past the limit stops
// the muxer with ErrIngressOverflow. It applies from the next segment
// received, and may be changed at any time; a mini-protocol that solicits a
// variable amount of data raises it before soliciting and lowers it once the
// data has been consumed. A limit of zero or less restores
// DefaultIngressLimit. It reports false when the protocol role is not
// registered.
func (m *Muxer) SetIngressLimit(
	protocolId uint16,
	protocolRole ProtocolRole,
	limit int,
) bool {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	recvChan, ok := m.protocolReceivers[protocolId][protocolRole]
	if !ok {
		return false
	}
	if limit <= 0 {
		limit = DefaultIngressLimit
	}
	recvChan.setLimit(limit)
	return true
}

// SetIngressBackpressure sets what happens to ingress for a registered
// protocol role past its ingress limit. Disabled, the default, it stops the
// muxer with ErrIngressOverflow. Enabled, the read loop stops reading the
// connection until the protocol has taken enough queued ingress for the next
// segment to fit, so the queue stays within the limit and the peer is slowed
// rather than dropped. The read loop serves every protocol on the
// connection, so while it waits no protocol receives anything; enable it
// only while a protocol has solicited more than it can bound. It reports
// false when the protocol role is not registered.
func (m *Muxer) SetIngressBackpressure(
	protocolId uint16,
	protocolRole ProtocolRole,
	enabled bool,
) bool {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	recvChan, ok := m.protocolReceivers[protocolId][protocolRole]
	if !ok {
		return false
	}
	recvChan.setBackpressure(enabled)
	return true
}

// IngressBackpressure reports whether backpressure is enabled for a
// registered protocol role. It reports false when the role is not
// registered.
func (m *Muxer) IngressBackpressure(
	protocolId uint16,
	protocolRole ProtocolRole,
) bool {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	recvChan, ok := m.protocolReceivers[protocolId][protocolRole]
	if !ok {
		return false
	}
	recvChan.mu.Lock()
	defer recvChan.mu.Unlock()
	return recvChan.backpressure
}

// IngressLimit returns the ingress limit of a registered protocol role, or
// zero when it is not registered.
func (m *Muxer) IngressLimit(protocolId uint16, protocolRole ProtocolRole) int {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	recvChan, ok := m.protocolReceivers[protocolId][protocolRole]
	if !ok {
		return 0
	}
	recvChan.mu.Lock()
	defer recvChan.mu.Unlock()
	return recvChan.limit
}

// RaiseReadBufferBudget raises this connection's message reassembly
// allowance to at least n bytes. Protocols call it as they register, with
// their own read-buffer cap, which makes the connection allowance the
// largest single protocol's cap rather than the sum of all of them. The
// largest message any node-to-node mini-protocol admits is bounded by its
// own pending-message byte limit -- block-fetch's 2,500,000 bytes is the
// largest of those -- so one protocol's default 16MB cap covers every
// legitimate reassembly on the connection with room to spare. A caller that
// needs more headroom raises the cap of the protocol that needs it, which
// raises this allowance with it.
func (m *Muxer) RaiseReadBufferBudget(n int) {
	if n <= 0 {
		return
	}
	m.readBufferMu.Lock()
	defer m.readBufferMu.Unlock()
	if n > m.readBufferBudget {
		m.readBufferBudget = n
	}
}

// ReserveReadBuffer claims n more bytes of this connection's reassembly
// allowance, reporting whether the claim fit. A refused claim means another
// mini-protocol on the same connection is already holding the allowance;
// the caller must fail its protocol rather than retry, since nothing
// guarantees the holder ever completes its message.
func (m *Muxer) ReserveReadBuffer(n int) bool {
	if n <= 0 {
		return true
	}
	m.readBufferMu.Lock()
	defer m.readBufferMu.Unlock()
	if m.readBufferBudget <= 0 {
		return true
	}
	if m.readBufferUsed+n > m.readBufferBudget {
		return false
	}
	m.readBufferUsed += n
	return true
}

// ReleaseReadBuffer returns n bytes of previously reserved allowance.
func (m *Muxer) ReleaseReadBuffer(n int) {
	if n <= 0 {
		return
	}
	m.readBufferMu.Lock()
	defer m.readBufferMu.Unlock()
	m.readBufferUsed -= n
	if m.readBufferUsed < 0 {
		m.readBufferUsed = 0
	}
}

// ReadBufferBudget returns this connection's total message reassembly
// allowance in bytes. Zero means unmetered.
func (m *Muxer) ReadBufferBudget() int {
	m.readBufferMu.Lock()
	defer m.readBufferMu.Unlock()
	return m.readBufferBudget
}

// ReadBufferInUse returns how much of this connection's message reassembly
// allowance is currently reserved by its mini-protocols.
func (m *Muxer) ReadBufferInUse() int {
	m.readBufferMu.Lock()
	defer m.readBufferMu.Unlock()
	return m.readBufferUsed
}

// ingressAccount tracks the segment payload queued across every protocol
// role of one connection. It never calls back into a segmentChannel, so a
// channel may use it while holding its own lock.
type ingressAccount struct {
	mu     sync.Mutex
	budget int
	room   chan struct{}
	// extended is the sum of the budget extensions of every protocol role.
	extended big.Int
	limit    int
	used     int
}

// limitLocked returns the budget with every extension added, saturating
// rather than overflowing. The caller must hold mu.
func (a *ingressAccount) limitLocked() int {
	return a.limit
}

func (a *ingressAccount) updateLimitLocked() {
	maxExtension := big.NewInt(int64(math.MaxInt - a.budget))
	if a.extended.Cmp(maxExtension) > 0 {
		a.limit = math.MaxInt
	} else {
		a.limit = a.budget + int(a.extended.Int64())
	}
}

func (a *ingressAccount) extend(delta int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.extended.Add(&a.extended, big.NewInt(int64(delta)))
	if a.extended.Sign() < 0 {
		a.extended.SetInt64(0)
	}
	a.updateLimitLocked()
	a.wakeLocked()
}

// maxBudgetExtension is the most one protocol role extends the budget by.
const maxBudgetExtension = math.MaxInt / 64

func (a *ingressAccount) wakeLocked() {
	if a.room != nil {
		close(a.room)
		a.room = nil
	}
}

func newIngressAccount() *ingressAccount {
	return &ingressAccount{
		budget: DefaultIngressBudget,
		limit:  DefaultIngressBudget,
	}
}

func (a *ingressAccount) setBudget(budget int) {
	if budget <= 0 {
		budget = DefaultIngressBudget
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.budget = budget
	a.updateLimitLocked()
	a.wakeLocked()
}

func (a *ingressAccount) getBudget() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.limitLocked()
}

func (a *ingressAccount) inUse() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.used
}

// reserve claims n bytes of the budget. It reports false, claiming nothing,
// when they do not fit and force is false. force claims them regardless, for
// a caller that must make progress.
func (a *ingressAccount) reserve(n int, force bool) (bool, <-chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !force && n > a.limitLocked()-a.used {
		if a.room == nil {
			a.room = make(chan struct{})
		}
		return false, a.room
	}
	a.used += n
	return true, nil
}

func (a *ingressAccount) release(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.used = max(a.used-n, 0)
	a.wakeLocked()
}

// segmentChannel holds one protocol role's inbound segments. The muxer read
// loop, shared by every protocol on the connection, appends to queue without
// waiting unless backpressure is enabled; forward drains queue into ch, and
// is the only goroutine that waits on the protocol's consumer.
type segmentChannel struct {
	protocolId   uint16
	protocolRole ProtocolRole
	metrics      func() Metrics
	// muxerDone is the muxer's doneChan, which a read loop waiting under
	// backpressure also watches.
	muxerDone <-chan bool
	// account is the connection-wide ingress budget this queue draws on.
	account *ingressAccount
	// mu guards every field below it except the channels, which are
	// written only at construction and in stop.
	mu    sync.Mutex
	limit int
	// extension is this role's share of the connection budget's extension.
	extension    int
	backpressure bool
	// room, when non-nil, is closed and cleared whenever a waiting enqueue
	// might now succeed: a dequeue, or a change of limit or backpressure.
	room          chan struct{}
	cond          *sync.Cond
	queue         []*Segment
	pendingBytes  int
	closed        bool
	ch            chan *Segment
	done          chan struct{}
	forwarderDone chan struct{}
	onceStop      sync.Once
}

func newSegmentChannel(
	protocolId uint16,
	protocolRole ProtocolRole,
	metrics func() Metrics,
	muxerDone <-chan bool,
	account *ingressAccount,
) *segmentChannel {
	sc := &segmentChannel{
		protocolId:    protocolId,
		protocolRole:  protocolRole,
		metrics:       metrics,
		muxerDone:     muxerDone,
		account:       account,
		limit:         DefaultIngressLimit,
		ch:            make(chan *Segment, 10),
		done:          make(chan struct{}),
		forwarderDone: make(chan struct{}),
	}
	sc.cond = sync.NewCond(&sc.mu)
	return sc
}

func (sc *segmentChannel) setLimit(limit int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.limit = limit
	sc.wakeEnqueueLocked()
}

func (sc *segmentChannel) setBudgetExtension(extra int) {
	// Clamped so the sum over every role cannot overflow an int.
	extra = min(max(extra, 0), maxBudgetExtension)
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if extra == sc.extension {
		return
	}
	sc.account.extend(extra - sc.extension)
	sc.extension = extra
	sc.wakeEnqueueLocked()
}

func (sc *segmentChannel) setBackpressure(enabled bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.backpressure = enabled
	sc.wakeEnqueueLocked()
}

// wakeEnqueueLocked wakes an enqueue waiting for room. The caller must hold
// mu.
func (sc *segmentChannel) wakeEnqueueLocked() {
	if sc.room != nil {
		close(sc.room)
		sc.room = nil
	}
}

// reportDepthLocked reports the queue depth. The caller must hold mu, which
// is what keeps the reports from the read loop and from forward in order.
func (sc *segmentChannel) reportDepthLocked() {
	if metrics := sc.metrics(); metrics != nil {
		metrics.IngressQueueDepth(
			sc.protocolId,
			sc.protocolRole,
			sc.pendingBytes,
		)
	}
}

// enqueue appends msg to the queue. Payload past the ingress limit, or past
// the connection's ingress budget, is refused with ErrIngressOverflow: the
// caller is the read loop every protocol on the connection depends on, so it
// does not wait for this protocol's consumer to make room. The exception is a
// protocol role with backpressure enabled, for which enqueue waits for room
// instead, holding up the whole read loop, until the segment fits,
// backpressure is disabled, or the receiver or muxer stops.
func (sc *segmentChannel) enqueue(msg *Segment) error {
	var waitStart time.Time
	defer func() {
		if waitStart.IsZero() {
			return
		}
		if metrics := sc.metrics(); metrics != nil {
			metrics.IngressBackpressure(
				sc.protocolId,
				sc.protocolRole,
				time.Since(waitStart),
			)
		}
	}()
	for {
		sc.mu.Lock()
		if sc.closed {
			sc.mu.Unlock()
			return errSegmentChannelClosed
		}
		newPending := sc.pendingBytes + len(msg.Payload)
		// Under backpressure an empty queue always admits, so a limit set
		// below one segment cannot wedge the read loop, and neither can a
		// connection budget held entirely by other protocols: no dequeue of
		// this queue would ever make room for it.
		fitsQueue := newPending <= sc.limit ||
			(sc.backpressure && len(sc.queue) == 0)
		var accountRoom <-chan struct{}
		if fitsQueue {
			reserved, wait := sc.account.reserve(
				len(msg.Payload),
				sc.backpressure && len(sc.queue) == 0,
			)
			accountRoom = wait
			if reserved {
				sc.queue = append(sc.queue, msg)
				sc.pendingBytes = newPending
				sc.reportDepthLocked()
				sc.cond.Broadcast()
				sc.mu.Unlock()
				return nil
			}
		}
		if !sc.backpressure {
			limit := sc.limit
			sc.mu.Unlock()
			if newPending > limit {
				return fmt.Errorf(
					"%w: %d bytes queued exceeds %d byte limit",
					ErrIngressOverflow,
					newPending,
					limit,
				)
			}
			return fmt.Errorf(
				"%w: connection holds %d bytes across all protocols, "+
					"%d more exceeds the %d byte connection budget",
				ErrIngressOverflow,
				sc.account.inUse(),
				len(msg.Payload),
				sc.account.getBudget(),
			)
		}
		if sc.room == nil {
			sc.room = make(chan struct{})
		}
		room := sc.room
		sc.mu.Unlock()
		if waitStart.IsZero() {
			waitStart = time.Now()
		}
		select {
		case <-room:
		case <-accountRoom:
		case <-sc.done:
			return errSegmentChannelClosed
		case <-sc.muxerDone:
			return errMuxerStopping
		}
	}
}

// forward drains queue into ch, waiting on the protocol's consumer when ch
// is full.
func (sc *segmentChannel) forward() {
	defer close(sc.forwarderDone)
	for {
		sc.mu.Lock()
		for len(sc.queue) == 0 && !sc.closed {
			sc.cond.Wait()
		}
		if len(sc.queue) == 0 {
			sc.mu.Unlock()
			return
		}
		seg := sc.queue[0]
		sc.pendingBytes -= len(seg.Payload)
		sc.account.release(len(seg.Payload))
		sc.queue = sc.queue[1:]
		if len(sc.queue) == 0 {
			// Release the backing array, which still references every
			// segment delivered since it was allocated.
			sc.queue = nil
		}
		sc.reportDepthLocked()
		sc.wakeEnqueueLocked()
		sc.mu.Unlock()

		select {
		case sc.ch <- seg:
			continue
		case <-sc.done:
			return
		default:
		}
		waitStart := time.Now()
		select {
		case sc.ch <- seg:
		case <-sc.done:
			return
		}
		if metrics := sc.metrics(); metrics != nil {
			metrics.IngressDeliveryBlocked(
				sc.protocolId,
				sc.protocolRole,
				time.Since(waitStart),
			)
		}
	}
}

type segmentSender struct {
	ch       chan *Segment
	done     chan struct{}
	onceStop sync.Once
	mu       sync.Mutex
}

func (s *segmentSender) stop() {
	s.onceStop.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		close(s.done)
		for {
			select {
			case msg, ok := <-s.ch:
				if !ok {
					return
				}
				if msg != nil {
					msg.reportDelivery(errors.New("protocol unregistered"))
				}
			default:
				return
			}
		}
	})
}

func (s *segmentChannel) stop() {
	// Wake forward() out of both a blocked send on ch and a Wait() for more
	// queue, then let it exit before closing ch - it is the only other
	// goroutine that touches ch, and closing from under a pending send
	// would panic. Unlock before waiting: forward() takes mu itself to
	// observe closed.
	s.onceStop.Do(func() {
		close(s.done)
	})
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()
	<-s.forwarderDone
	s.mu.Lock()
	defer s.mu.Unlock()
	// Segments still queued are never delivered, so their share of the
	// connection budget is returned here.
	s.account.release(s.pendingBytes)
	s.pendingBytes = 0
	s.account.extend(-s.extension)
	s.extension = 0
	s.queue = nil
	if s.ch != nil {
		close(s.ch)
		s.ch = nil
	}
}

type ConnectionClosedError struct {
	Context string
	Err     error
}

func (e *ConnectionClosedError) Error() string {
	return fmt.Sprintf(
		"peer closed the connection while %s: %v",
		e.Context,
		e.Err,
	)
}

func (e *ConnectionClosedError) Unwrap() error {
	return e.Err
}

// New creates a new Muxer object and starts the read loop, using
// defaultSegmentReadTimeout. Use NewWithSegmentReadTimeout to override or
// disable that timeout for a connection known to be a trusted NtC channel.
func New(conn net.Conn) *Muxer {
	return NewWithSegmentReadTimeout(conn, defaultSegmentReadTimeout)
}

// NewWithSegmentReadTimeout is like New, but lets the caller override how
// long the read loop waits for the next segment before closing the
// connection. segmentReadTimeout <= 0 disables the deadline entirely --
// see defaultSegmentReadTimeout's doc comment for why a caller might want
// that (e.g. a local/NtC LocalStateQuery connection, which the Ouroboros
// Network Specification says must not be timed out at all).
func NewWithSegmentReadTimeout(
	conn net.Conn,
	segmentReadTimeout time.Duration,
) *Muxer {
	m := &Muxer{
		conn:               conn,
		startChan:          make(chan bool, 1),
		doneChan:           make(chan bool),
		errorChan:          make(chan error, 10),
		protocolSenders:    make(map[uint16]map[ProtocolRole]*segmentSender),
		protocolReceivers:  make(map[uint16]map[ProtocolRole]*segmentChannel),
		protocolTombstones: make(map[uint16]map[ProtocolRole]struct{}),
		segmentReadTimeout: segmentReadTimeout,
		ingress:            newIngressAccount(),
	}
	// Start read goroutine
	m.waitGroup.Add(1)
	go m.readLoop()
	// Start cleanup routine
	go func() {
		// Wait for done signal
		<-m.doneChan
		// Close underlying connection
		// We must do this to break out of pending Read() calls to shut down cleanly
		_ = m.conn.Close()
		// Wait for other goroutines to shutdown
		m.waitGroupMutex.Lock()
		m.waitGroup.Wait()
		m.waitGroupMutex.Unlock()
		// Close ErrorChan to signify to consumer that we're shutting down
		close(m.errorChan)
	}()
	return m
}

func (m *Muxer) ErrorChan() chan error {
	return m.errorChan
}

// Start unblocks the read loop after the initial handshake to allow it to start processing messages
func (m *Muxer) Start() {
	select {
	case m.startChan <- true:
	default:
	}
}

// StartOnce unblocks the read loop for one iteration. This is generally used to perform the handshake before registering
// additional protocols and calling Start
func (m *Muxer) StartOnce() {
	select {
	case m.startChan <- false:
	default:
	}
}

// Stop shuts down the muxer
func (m *Muxer) Stop() {
	m.onceStop.Do(func() {
		// Close doneChan to signify that we're shutting down
		close(m.doneChan)
	})
}

// DoneChan returns a channel that closes when the muxer begins shutting down.
func (m *Muxer) DoneChan() <-chan bool {
	return m.doneChan
}

// SetDiffusionMode sets the muxer diffusion mode after the handshake completes
func (m *Muxer) SetDiffusionMode(diffusionMode DiffusionMode) {
	m.diffusionMode.Store(int64(diffusionMode))
}

// sendError sends the specified error to the error channel and stops the muxer.
// The send is non-blocking to prevent the read loop goroutine from being
// permanently blocked if the error channel buffer is full.
func (m *Muxer) sendError(err error) {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return
	default:
	}
	// Send error to consumer (non-blocking to prevent goroutine leak)
	select {
	case m.errorChan <- err:
	default:
	}
	// Stop the muxer on any error
	m.Stop()
}

// RegisterProtocol registers the provided protocol ID with the muxer. It returns a channel for sending,
// a channel for receiving, and a channel to know when the muxer is shutting down. If the muxer is shutting
// down, this function will return nil values.
func (m *Muxer) RegisterProtocol(
	protocolId uint16,
	protocolRole ProtocolRole,
) (chan *Segment, chan *Segment, chan bool) {
	m.waitGroupMutex.Lock()
	defer m.waitGroupMutex.Unlock()
	// Check for shutdown
	select {
	case <-m.doneChan:
		return nil, nil, nil
	default:
	}
	if m.registerHook != nil {
		m.registerHook()
	}
	// Generate channels
	senderChan := make(chan *Segment, 10)
	receiverChan := newSegmentChannel(
		protocolId,
		protocolRole,
		m.getMetrics,
		m.doneChan,
		m.ingress,
	)
	receiver := receiverChan.ch
	sender := &segmentSender{ch: senderChan, done: make(chan struct{})}
	m.waitGroup.Go(receiverChan.forward)
	// Record channels in protocol sender/receiver maps
	m.protocolReceiversMutex.Lock()
	// Shutdown is checked again under the lock readLoop's exit sweep takes.
	// doneChan is always closed before that sweep runs, so a receiver
	// inserted after this check is one the sweep will stop, and a receiver
	// that would miss the sweep is never inserted. Checking only above
	// leaves a window where the receiver's forward goroutine is never
	// stopped and the muxer never finishes shutting down.
	select {
	case <-m.doneChan:
		m.protocolReceiversMutex.Unlock()
		receiverChan.stop()
		return nil, nil, nil
	default:
	}
	if _, ok := m.protocolSenders[protocolId]; !ok {
		m.protocolSenders[protocolId] = make(map[ProtocolRole]*segmentSender)
	}
	if _, ok := m.protocolReceivers[protocolId]; !ok {
		m.protocolReceivers[protocolId] = make(map[ProtocolRole]*segmentChannel)
	}
	m.protocolSenders[protocolId][protocolRole] = sender
	m.protocolReceivers[protocolId][protocolRole] = receiverChan
	if _, ok := m.protocolTombstones[protocolId]; !ok {
		m.protocolTombstones[protocolId] = make(map[ProtocolRole]struct{})
	}
	delete(m.protocolTombstones[protocolId], protocolRole)
	m.protocolReceiversMutex.Unlock()
	// Start Goroutine to handle outbound messages
	m.waitGroup.Go(func() {
		for {
			select {
			case _, ok := <-m.doneChan:
				// doneChan has been closed, which means we're shutting down
				if !ok {
					return
				}
			case <-sender.done:
				return
			case msg, ok := <-senderChan:
				if !ok {
					return
				}
				sender.mu.Lock()
				select {
				case <-sender.done:
					sender.mu.Unlock()
					msg.reportDelivery(errors.New("protocol unregistered"))
					continue
				default:
				}
				sender.mu.Unlock()
				err := m.Send(msg)
				msg.reportDelivery(err)
				if err != nil {
					m.sendError(err)
					return
				}
			}
		}
	})
	return senderChan, receiver, m.doneChan
}

func (m *Muxer) UnregisterProtocol(
	protocolId uint16,
	protocolRole ProtocolRole,
) {
	m.protocolReceiversMutex.Lock()
	defer m.protocolReceiversMutex.Unlock()
	removed := false
	// Remove both directions while holding the same lock. This prevents a
	// concurrent re-registration from observing only half of the old mapping.
	if protocolRoles, ok := m.protocolReceivers[protocolId]; ok {
		if recvChan, ok := protocolRoles[protocolRole]; ok {
			removed = true
			recvChan.stop()
			delete(protocolRoles, protocolRole)
		}
		if len(protocolRoles) == 0 {
			delete(m.protocolReceivers, protocolId)
		}
	}
	if protocolRoles, ok := m.protocolSenders[protocolId]; ok {
		if sender, ok := protocolRoles[protocolRole]; ok {
			removed = true
			sender.stop()
			delete(protocolRoles, protocolRole)
		}
		if len(protocolRoles) == 0 {
			delete(m.protocolSenders, protocolId)
		}
	}
	if removed {
		if _, ok := m.protocolTombstones[protocolId]; !ok {
			m.protocolTombstones[protocolId] = make(map[ProtocolRole]struct{})
		}
		m.protocolTombstones[protocolId][protocolRole] = struct{}{}
	}
}

// Send takes a populated Segment and writes it to the connection. Only one segment is written at a time:
// waiting Praos segments go before Leios ones, each class in arrival order, and a waiting Leios segment
// goes next after a bounded run of Praos segments. Leios segments are never dropped for waiting: segments
// carry no message boundary, so a dropped one would desynchronize the peer's stream.
func (m *Muxer) Send(msg *Segment) error {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return errors.New("shutting down")
	default:
	}
	// Only one protocol can write at a time; egress picks whose turn is next.
	waitStart := time.Now()
	blocked, err := m.egress.acquire(msg, m.doneChan)
	em, _ := m.getMetrics().(EgressMetrics)
	if err != nil {
		return err
	}
	waitDuration := time.Since(waitStart)
	defer func() {
		m.egress.release()
		if blocked && em != nil {
			em.EgressWait(
				msg.GetProtocolId(),
				EgressClassOf(msg.GetProtocolId()),
				waitDuration,
			)
		}
	}()
	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.BigEndian, msg.SegmentHeader)
	if err != nil {
		return err
	}
	buf.Write(msg.Payload)
	if err := m.conn.SetWriteDeadline(time.Now().Add(segmentWriteTimeout)); err != nil {
		return err
	}
	_, err = m.conn.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

// readLoop waits for incoming data on the connection, parses the segment, and passes it to the appropriate
// protocol
func (m *Muxer) readLoop() {
	defer func() {
		m.waitGroup.Done()
		// Close receiver channels
		m.protocolReceiversMutex.Lock()
		for _, protocolRoles := range m.protocolReceivers {
			for protocolRole, recvChan := range protocolRoles {
				// Signal shutdown to protocol
				recvChan.stop()

				// Remove mapping
				delete(protocolRoles, protocolRole)
			}
		}
		m.protocolReceiversMutex.Unlock()
	}()
	started := false
	for {
		// Break out of read loop if we're shutting down
		select {
		case <-m.doneChan:
			return
		default:
		}
		// Wait until the muxer is started to continue
		if !started {
			select {
			case <-m.doneChan:
				// Break out of read loop if we're shutting down
				return
			case v := <-m.startChan:
				// We block again on the next iteration of we get 'false' from startChan
				started = v
			}
		}
		// Set read deadline to prevent slowloris-style DoS attacks against an
		// untrusted remote peer. A non-positive segmentReadTimeout disables
		// this entirely (time.Time{} clears any previously set deadline) --
		// see its doc comment for why a caller may need that.
		if m.segmentReadTimeout > 0 {
			_ = m.conn.SetReadDeadline(time.Now().Add(m.segmentReadTimeout))
		} else {
			_ = m.conn.SetReadDeadline(time.Time{})
		}
		header := SegmentHeader{}
		if err := binary.Read(m.conn, binary.BigEndian, &header); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				err = io.EOF
			}
			if errors.Is(err, io.EOF) {
				m.sendError(
					&ConnectionClosedError{Context: "reading header", Err: err},
				)
			} else {
				m.sendError(err)
			}
			return
		}
		// Check for zero-byte payload
		// This prevents certain types of DoS attacks
		if header.PayloadLength == 0 {
			m.sendError(
				errors.New("received zero-byte segment payload"),
			)
			return
		}
		msg := &Segment{
			SegmentHeader: header,
			Payload:       make([]byte, header.PayloadLength),
		}
		// We use ReadFull because it guarantees to read the expected number of bytes or
		// return an error
		if _, err := io.ReadFull(m.conn, msg.Payload); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				err = io.EOF
			}
			if errors.Is(err, io.EOF) {
				m.sendError(
					&ConnectionClosedError{
						Context: "reading payload",
						Err:     err,
					},
				)
			} else {
				m.sendError(err)
			}
			return
		}
		// Check for message from initiator when we're not configured as a responder
		if DiffusionMode(m.diffusionMode.Load()) == DiffusionModeInitiator && !msg.IsResponse() {
			m.sendError(
				errors.New(
					"received message from initiator when not configured as a responder",
				),
			)
			return
		}
		// Check for message from responder when we're not configured as an initiator
		if DiffusionMode(m.diffusionMode.Load()) == DiffusionModeResponder && msg.IsResponse() {
			m.sendError(
				errors.New(
					"received message from responder when not configured as an initiator",
				),
			)
			return
		}
		// Send message payload to proper receiver
		protocolRole := ProtocolRoleResponder
		if msg.IsResponse() {
			protocolRole = ProtocolRoleInitiator
		}
		m.protocolReceiversMutex.Lock()
		protocolRoles, ok := m.protocolReceivers[msg.GetProtocolId()]
		if !ok {
			if _, tombstoned := m.protocolTombstones[msg.GetProtocolId()][protocolRole]; tombstoned {
				m.protocolReceiversMutex.Unlock()
				continue
			}
			// Try the "unknown protocol" receiver if we didn't find an explicit one
			protocolRoles, ok = m.protocolReceivers[ProtocolUnknown]
			if !ok {
				m.protocolReceiversMutex.Unlock()
				m.sendError(
					fmt.Errorf(
						"received message for unknown protocol ID %d",
						msg.GetProtocolId(),
					),
				)
				return
			}
		}
		recvChan := protocolRoles[protocolRole]
		if recvChan == nil {
			if _, tombstoned := m.protocolTombstones[msg.GetProtocolId()][protocolRole]; tombstoned {
				m.protocolReceiversMutex.Unlock()
				continue
			}
		}
		m.protocolReceiversMutex.Unlock()
		if recvChan == nil {
			m.sendError(
				fmt.Errorf(
					"received message for unknown protocol ID %d",
					msg.GetProtocolId(),
				),
			)
			return
		}

		// Every protocol on the connection is read through this loop, so it
		// must not wait for one protocol's consumer: enqueue does not block,
		// and ingress past the protocol's limit ends the connection, unless
		// that protocol has enabled backpressure.
		if err := recvChan.enqueue(msg); err != nil {
			if errors.Is(err, errSegmentChannelClosed) {
				continue
			}
			if errors.Is(err, errMuxerStopping) {
				return
			}
			m.sendError(
				fmt.Errorf(
					"protocol %d: %w",
					msg.GetProtocolId(),
					err,
				),
			)
			return
		}
	}
}
