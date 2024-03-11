// Copyright 2023 Blink Labs Software
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
)

// Magic number chosen to represent unknown protocols
const ProtocolUnknown uint16 = 0xabcd

// DiffusionMode is an enum for the valid muxer difficusion modes
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
	protocolSenders        map[uint16]map[ProtocolRole]chan *Segment
	protocolReceivers      map[uint16]map[ProtocolRole]chan *Segment
	protocolReceiversMutex sync.Mutex
	diffusionMode          DiffusionMode
	onceStop               sync.Once
}

// New creates a new Muxer object and starts the read loop
func New(conn net.Conn) *Muxer {
	m := &Muxer{
		conn:              conn,
		startChan:         make(chan bool, 1),
		doneChan:          make(chan bool),
		errorChan:         make(chan error, 10),
		protocolSenders:   make(map[uint16]map[ProtocolRole]chan *Segment),
		protocolReceivers: make(map[uint16]map[ProtocolRole]chan *Segment),
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
		m.waitGroup.Wait()
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

// Stop shuts down the muxer
func (m *Muxer) Stop() {
	m.onceStop.Do(func() {
		// Close doneChan to signify that we're shutting down
		close(m.doneChan)
	})
}

// SetDiffusionMode sets the muxer diffusion mode after the handshake completes
func (m *Muxer) SetDiffusionMode(diffusionMode DiffusionMode) {
	m.diffusionMode = diffusionMode
}

// sendError sends the specified error to the error channel and stops the muxer
func (m *Muxer) sendError(err error) {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return
	default:
	}
	// Send error to consumer
	m.errorChan <- err
	// Stop the muxer on any error
	m.Stop()
}

// RegisterProtocol registers the provided protocol ID with the muxer. It returns a channel for sending,
// a channel for receiving, and a channel to know when the muxer is shutting down
func (m *Muxer) RegisterProtocol(
	protocolId uint16,
	protocolRole ProtocolRole,
) (chan *Segment, chan *Segment, chan bool) {
	// Generate channels
	senderChan := make(chan *Segment, 10)
	receiverChan := make(chan *Segment, 10)
	// Record channels in protocol sender/receiver maps
	m.protocolReceiversMutex.Lock()
	if _, ok := m.protocolSenders[protocolId]; !ok {
		m.protocolSenders[protocolId] = make(map[ProtocolRole]chan *Segment)
		m.protocolReceivers[protocolId] = make(map[ProtocolRole]chan *Segment)
	}
	m.protocolSenders[protocolId][protocolRole] = senderChan
	m.protocolReceivers[protocolId][protocolRole] = receiverChan
	m.protocolReceiversMutex.Unlock()
	// Start Goroutine to handle outbound messages
	m.waitGroup.Add(1)
	go func() {
		defer m.waitGroup.Done()
		for {
			select {
			case _, ok := <-m.doneChan:
				// doneChan has been closed, which means we're shutting down
				if !ok {
					return
				}
			case msg, ok := <-senderChan:
				if !ok {
					return
				}
				if err := m.Send(msg); err != nil {
					m.sendError(err)
					return
				}
			}
		}
	}()
	return senderChan, receiverChan, m.doneChan
}

// Send takes a populated Segment and writes it to the connection. A mutex is used to prevent more than
// one protocol from sending at once
func (m *Muxer) Send(msg *Segment) error {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return fmt.Errorf("shutting down")
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
			for _, recvChan := range protocolRoles {
				close(recvChan)
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
		header := SegmentHeader{}
		if err := binary.Read(m.conn, binary.BigEndian, &header); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				err = io.EOF
			}
			m.sendError(err)
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
			m.sendError(err)
			return
		}
		// Check for message from initiator when we're not configured as a responder
		if m.diffusionMode == DiffusionModeInitiator && !msg.IsResponse() {
			m.sendError(
				fmt.Errorf(
					"received message from initiator when not configured as a responder",
				),
			)
			return
		}
		// Check for message from responder when we're not configured as an initiator
		if m.diffusionMode == DiffusionModeResponder && msg.IsResponse() {
			m.sendError(
				fmt.Errorf(
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
		if recvChan != nil {
			recvChan <- msg
		}
		// Wait until the muxer is started to continue
		// We don't want to read more than one segment until the handshake is complete
		if !started {
			select {
			case <-m.doneChan:
				// Break out of read loop if we're shutting down
				return
			case <-m.startChan:
				started = true
			}
		}
	}
}
