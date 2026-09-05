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
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Magic number chosen to represent unknown protocols
const ProtocolUnknown uint16 = 0xabcd

// segmentReadTimeout is the maximum time to wait for a complete segment read
// before closing the connection. This prevents slowloris-style DoS attacks.
const segmentReadTimeout = 120 * time.Second

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
	sendMutex              sync.Mutex
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
}

type segmentChannel struct {
	mu       sync.Mutex
	ch       chan *Segment
	done     chan struct{}
	onceStop sync.Once
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
	// Wake a delivery that is waiting for channel capacity before taking the
	// mutex needed to close the receiver. Otherwise unregistration can wait
	// forever behind that delivery while the protocol waits for
	// unregistration to finish.
	s.onceStop.Do(func() {
		close(s.done)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
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

// New creates a new Muxer object and starts the read loop
func New(conn net.Conn) *Muxer {
	m := &Muxer{
		conn:               conn,
		startChan:          make(chan bool, 1),
		doneChan:           make(chan bool),
		errorChan:          make(chan error, 10),
		protocolSenders:    make(map[uint16]map[ProtocolRole]*segmentSender),
		protocolReceivers:  make(map[uint16]map[ProtocolRole]*segmentChannel),
		protocolTombstones: make(map[uint16]map[ProtocolRole]struct{}),
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
	// Generate channels
	senderChan := make(chan *Segment, 10)
	receiver := make(chan *Segment, 10)
	receiverChan := &segmentChannel{
		ch:   receiver,
		done: make(chan struct{}),
	}
	sender := &segmentSender{ch: senderChan, done: make(chan struct{})}
	// Record channels in protocol sender/receiver maps
	m.protocolReceiversMutex.Lock()
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

// Send takes a populated Segment and writes it to the connection. A mutex is used to prevent more than
// one protocol from sending at once
func (m *Muxer) Send(msg *Segment) error {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return errors.New("shutting down")
	default:
	}
	// We use a mutex to make sure only one protocol can send at a time
	m.sendMutex.Lock()
	defer m.sendMutex.Unlock()
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.BigEndian, msg.SegmentHeader)
	if err != nil {
		return err
	}
	buf.Write(msg.Payload)
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
		// Set read deadline to prevent slowloris-style DoS attacks
		_ = m.conn.SetReadDeadline(time.Now().Add(segmentReadTimeout))
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

		recvChan.mu.Lock()
		if recvChan.ch == nil {
			recvChan.mu.Unlock()
			continue
		}

		select {
		case <-m.doneChan:
			recvChan.mu.Unlock()
			return
		case <-recvChan.done:
			recvChan.mu.Unlock()
			continue
		case recvChan.ch <- msg:
			recvChan.mu.Unlock()
		}
	}
}
