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

// Package protocol provides the common functionality for mini-protocols
package protocol

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
)

// This is completely arbitrary, but the line had to be drawn somewhere
const maxMessagesPerSegment = 20

// Protocol implements the base functionality of an Ouroboros mini-protocol
type Protocol struct {
	config               ProtocolConfig
	muxerSendChan        chan *muxer.Segment
	muxerRecvChan        chan *muxer.Segment
	muxerDoneChan        chan bool
	state                State
	stateMutex           sync.Mutex
	recvBuffer           *bytes.Buffer
	sendQueueChan        chan Message
	sendStateQueueChan   chan Message
	recvReadyChan        chan bool
	sendReadyChan        chan bool
	doneChan             chan bool
	waitGroup            sync.WaitGroup
	stateTransitionTimer *time.Timer
	onceStart            sync.Once
}

// ProtocolConfig provides the configuration for Protocol
type ProtocolConfig struct {
	Name                string
	ProtocolId          uint16
	ErrorChan           chan error
	Muxer               *muxer.Muxer
	Mode                ProtocolMode
	Role                ProtocolRole
	MessageHandlerFunc  MessageHandlerFunc
	MessageFromCborFunc MessageFromCborFunc
	StateMap            StateMap
	InitialState        State
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
	Muxer     *muxer.Muxer
	ErrorChan chan error
	Mode      ProtocolMode
	// TODO: remove me
	Role    ProtocolRole
	Version uint16
}

// MessageHandlerFunc represents a function that handles an incoming message
type MessageHandlerFunc func(Message, bool) error

// MessageFromCborFunc represents a function that parses a mini-protocol message
type MessageFromCborFunc func(uint, []byte) (Message, error)

// New returns a new Protocol object
func New(config ProtocolConfig) *Protocol {
	p := &Protocol{
		config:   config,
		doneChan: make(chan bool),
	}
	return p
}

// Start initializes the mini-protocol
func (p *Protocol) Start() {
	p.onceStart.Do(func() {
		// Register protocol with muxer
		muxerProtocolRole := muxer.ProtocolRoleInitiator
		if p.config.Role == ProtocolRoleServer {
			muxerProtocolRole = muxer.ProtocolRoleResponder
		}
		p.muxerSendChan, p.muxerRecvChan, p.muxerDoneChan = p.config.Muxer.RegisterProtocol(
			p.config.ProtocolId,
			muxerProtocolRole,
		)
		// Create buffers and channels
		p.recvBuffer = bytes.NewBuffer(nil)
		p.sendQueueChan = make(chan Message, 50)
		p.sendStateQueueChan = make(chan Message, 50)
		p.recvReadyChan = make(chan bool, 1)
		p.sendReadyChan = make(chan bool, 1)
		// Start goroutine to cleanup when shutting down
		go func() {
			// Wait for doneChan to be closed
			<-p.doneChan
			// Wait for all other goroutines to finish
			p.waitGroup.Wait()
			// Close channels
			close(p.sendQueueChan)
			close(p.sendStateQueueChan)
			close(p.recvReadyChan)
			close(p.sendReadyChan)
			// Cancel any timer
			if p.stateTransitionTimer != nil {
				// Stop timer and drain channel
				if !p.stateTransitionTimer.Stop() {
					<-p.stateTransitionTimer.C
				}
				p.stateTransitionTimer = nil
			}
		}()
		// Set initial state
		p.setState(p.config.InitialState)
		// Start our send and receive Goroutines
		p.waitGroup.Add(2)
		go p.recvLoop()
		go p.sendLoop()
	})
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
func (p *Protocol) DoneChan() chan bool {
	return p.doneChan
}

// SendMessage appends a message to the send queue
func (p *Protocol) SendMessage(msg Message) error {
	p.sendQueueChan <- msg
	return nil
}

// SendError sends an error to the handler in the Ouroboros object
func (p *Protocol) SendError(err error) {
	p.config.ErrorChan <- err
}

func (p *Protocol) sendLoop() {
	defer p.waitGroup.Done()
	var setNewState bool
	var newState State
	var err error
	for {
		select {
		case <-p.doneChan:
			// We are responsible for closing this channel as the sender, even through it
			// was created by the muxer
			close(p.muxerSendChan)
			// Break out of send loop if we're shutting down
			return
		case <-p.sendReadyChan:
			// We are ready to send based on state map
		}
		// Lock the state to prevent collisions
		p.stateMutex.Lock()
		// Check for queued state changes from previous pipelined sends
		setNewState = false
		if len(p.sendStateQueueChan) > 0 {
			msg := <-p.sendStateQueueChan
			newState, err = p.getNewState(msg)
			if err != nil {
				p.SendError(
					fmt.Errorf(
						"%s: error sending message: %s",
						p.config.Name,
						err,
					),
				)
				return
			}
			setNewState = true
			// If there are no queued messages, set the new state now
			if len(p.sendQueueChan) == 0 {
				p.setState(newState)
				p.stateMutex.Unlock()
				continue
			}
		}
		// Read queued messages and write into buffer
		payloadBuf := bytes.NewBuffer(nil)
		msgCount := 0
		for {
			// Get next message from send queue
			msg, ok := <-p.sendQueueChan
			if !ok {
				// We're shutting down
				return
			}
			msgCount = msgCount + 1
			// Write the message into the send state queue if we already have a new state
			if setNewState {
				p.sendStateQueueChan <- msg
			}
			// Get raw CBOR from message
			data := msg.Cbor()
			// If message has no raw CBOR, encode the message
			if data == nil {
				var err error
				data, err = cbor.Encode(msg)
				if err != nil {
					p.SendError(err)
					return
				}
			}
			payloadBuf.Write(data)
			if !setNewState {
				newState, err = p.getNewState(msg)
				if err != nil {
					p.SendError(
						fmt.Errorf(
							"%s: error sending message: %s",
							p.config.Name,
							err,
						),
					)
					return
				}
				setNewState = true
			}
			// We don't want more than maxMessagesPerSegment messages in a segment
			if msgCount >= maxMessagesPerSegment {
				break
			}
			// We don't want to add more messages once we spill over into a second segment
			if payloadBuf.Len() > muxer.SegmentMaxPayloadLength {
				break
			}
			// Check if there are any more queued messages
			if len(p.sendQueueChan) == 0 {
				break
			}
			// We don't want to block on writes to the send state queue
			if len(p.sendStateQueueChan) == cap(p.sendStateQueueChan) {
				break
			}
		}
		// Send messages in multiple segments (if needed)
		for {
			// Determine segment payload length
			segmentPayloadLength := payloadBuf.Len()
			if segmentPayloadLength > muxer.SegmentMaxPayloadLength {
				segmentPayloadLength = muxer.SegmentMaxPayloadLength
			}
			// Send current segment
			segmentPayload := payloadBuf.Bytes()[:segmentPayloadLength]
			isResponse := false
			if p.Role() == ProtocolRoleServer {
				isResponse = true
			}
			segment := muxer.NewSegment(
				p.config.ProtocolId,
				segmentPayload,
				isResponse,
			)
			p.muxerSendChan <- segment
			// Remove current segment's data from buffer
			if payloadBuf.Len() > segmentPayloadLength {
				payloadBuf = bytes.NewBuffer(
					payloadBuf.Bytes()[segmentPayloadLength:],
				)
			} else {
				break
			}
		}
		// Set new state and unlock
		p.setState(newState)
		p.stateMutex.Unlock()
	}
}

func (p *Protocol) recvLoop() {
	defer p.waitGroup.Done()
	leftoverData := false
	isResponse := false
	for {
		var err error
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			select {
			case <-p.muxerDoneChan:
				close(p.doneChan)
				return
			case segment, ok := <-p.muxerRecvChan:
				if !ok {
					close(p.doneChan)
					return
				}
				// Add segment payload to buffer
				p.recvBuffer.Write(segment.Payload)
				// Save whether it's a response
				isResponse = segment.IsResponse()
			}
		}
		leftoverData = false
		// Wait until ready to receive based on state map
		select {
		case <-p.muxerDoneChan:
			close(p.doneChan)
			return
		case <-p.recvReadyChan:
		}
		// Decode message into generic list until we can determine what type of message it is.
		// This also lets us determine how many bytes the message is. We use RawMessage here to
		// avoid parsing things that we may not be able to parse
		var tmpMsg []cbor.RawMessage
		numBytesRead, err := cbor.Decode(p.recvBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if err == io.ErrUnexpectedEOF && p.recvBuffer.Len() > 0 {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				p.recvReadyChan <- true
				continue
			}
			p.SendError(fmt.Errorf("%s: decode error: %s", p.config.Name, err))
			return
		}
		// Decode first list item to determine message type
		var msgType uint
		if _, err := cbor.Decode(tmpMsg[0], &msgType); err != nil {
			p.SendError(fmt.Errorf("%s: decode error: %s", p.config.Name, err))
		}
		// Create Message object from CBOR
		msgData := p.recvBuffer.Bytes()[:numBytesRead]
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
		// Handle message
		if err := p.handleMessage(msg, isResponse); err != nil {
			p.SendError(err)
			return
		}
		if numBytesRead < p.recvBuffer.Len() {
			// There is another message in the same muxer segment, so we reset the buffer with just
			// the remaining data
			p.recvBuffer = bytes.NewBuffer(p.recvBuffer.Bytes()[numBytesRead:])
			leftoverData = true
		} else {
			// Empty out our buffer since we successfully processed the message
			p.recvBuffer.Reset()
		}
	}
}

func (p *Protocol) getNewState(msg Message) (State, error) {
	var newState State
	matchFound := false
	for _, transition := range p.config.StateMap[p.state].Transitions {
		if transition.MsgType == msg.Type() {
			if transition.MatchFunc != nil {
				// Skip item if match function returns false
				if !transition.MatchFunc(msg) {
					continue
				}
			}
			newState = transition.NewState
			matchFound = true
			break
		}
	}
	if !matchFound {
		return newState, fmt.Errorf(
			"message %s not allowed in current protocol state %s",
			reflect.TypeOf(msg).Name(),
			p.state,
		)
	}
	return newState, nil
}

func (p *Protocol) setState(state State) {
	// Disable any previous state transition timer
	if p.stateTransitionTimer != nil {
		// Stop timer and drain channel
		if !p.stateTransitionTimer.Stop() {
			<-p.stateTransitionTimer.C
		}
		p.stateTransitionTimer = nil
	}
	// Set the new state
	p.state = state
	// Mark protocol as ready to send/receive based on role and agency of the new state
	switch p.config.StateMap[p.state].Agency {
	case AgencyClient:
		switch p.config.Role {
		case ProtocolRoleClient:
			p.sendReadyChan <- true
		case ProtocolRoleServer:
			p.recvReadyChan <- true
		}
	case AgencyServer:
		switch p.config.Role {
		case ProtocolRoleServer:
			p.sendReadyChan <- true
		case ProtocolRoleClient:
			p.recvReadyChan <- true
		}
	}
	// Set timeout for state transition
	if p.config.StateMap[p.state].Timeout > 0 {
		p.stateTransitionTimer = time.AfterFunc(
			p.config.StateMap[p.state].Timeout,
			func() {
				p.SendError(
					fmt.Errorf(
						"%s: timeout waiting on transition from protocol state %s",
						p.config.Name,
						p.state,
					),
				)
			},
		)
	}
}

func (p *Protocol) handleMessage(msg Message, isResponse bool) error {
	// Lock the state to prevent collisions
	p.stateMutex.Lock()
	newState, err := p.getNewState(msg)
	if err != nil {
		return fmt.Errorf("%s: error handling message: %s", p.config.Name, err)
	}
	// Set new state and unlock
	p.setState(newState)
	p.stateMutex.Unlock()
	// Call handler function
	return p.config.MessageHandlerFunc(msg, isResponse)
}
