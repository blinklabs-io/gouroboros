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
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/utils"
)

// This is completely arbitrary, but the line had to be drawn somewhere
const maxMessagesPerSegment = 20

// Protocol implements the base functionality of an Ouroboros mini-protocol
type Protocol struct {
	config              ProtocolConfig
	muxerSendChan       chan *muxer.Segment
	muxerRecvChan       chan *muxer.Segment
	muxerDoneChan       chan bool
	sendQueueChan       chan Message
	recvReadyChan       chan bool
	sendReadyChan       chan bool
	stateTransitionChan chan<- protocolStateTransition
	doneSignal          *utils.DoneSignal
	waitGroup           sync.WaitGroup
	onceStart           sync.Once
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
	StateContext        interface{}
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
	ConnectionId connection.ConnectionId
	Muxer        *muxer.Muxer
	ErrorChan    chan error
	Mode         ProtocolMode
	// TODO: remove me
	Role    ProtocolRole
	Version uint16
}

type protocolStateTransition struct {
	msg       Message
	errorChan chan<- error
}

// MessageHandlerFunc represents a function that handles an incoming message
type MessageHandlerFunc func(Message) error

// MessageFromCborFunc represents a function that parses a mini-protocol message
type MessageFromCborFunc func(uint, []byte) (Message, error)

// New returns a new Protocol object
func New(config ProtocolConfig) *Protocol {
	p := &Protocol{
		config:     config,
		doneSignal: utils.NewDoneSignal(),
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

		// Create channels
		p.sendQueueChan = make(chan Message, 50)
		p.recvReadyChan = make(chan bool, 1)
		p.sendReadyChan = make(chan bool, 1)

		stateTransitionChan := make(chan protocolStateTransition)
		p.stateTransitionChan = stateTransitionChan

		// Start our send and receive Goroutines
		p.waitGroup.Add(2)

		go p.stateLoop(stateTransitionChan)
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
func (p *Protocol) DoneChan() <-chan struct{} {
	return p.doneSignal.GetCh()
}

// SendMessage appends a message to the send queue
func (p *Protocol) SendMessage(msg Message) error {
	p.sendQueueChan <- msg
	return nil
}

// SendError sends an error to the handler in the Ouroboros object
func (p *Protocol) SendError(err error) {
	select {
	case p.config.ErrorChan <- err:
	default:
		// Discard error if the buffer is full
		// The connection will get closed on the first error, so any
		// additional errors are unnecessary
		return
	}
}

func (p *Protocol) sendLoop() {
	defer func() {
		p.waitGroup.Done()
		// Close muxer send channel
		// We are responsible for closing this channel as the sender, even through it
		// was created by the muxer
		close(p.muxerSendChan)
		p.doneSignal.Close()
	}()

	for {
		select {
		case <-p.doneSignal.GetCh():
			// Break out of send loop if we're shutting down
			return
		case <-p.sendReadyChan:
			// We are ready to send based on state map
		}

		// Read queued messages and write into buffer
		payloadBuf := bytes.NewBuffer(nil)
		msgCount := 0
		breakLoop := false
		for {
			// Get next message from send queue
			select {
			case <-p.doneSignal.GetCh():
				// Break out of send loop if we're shutting down
				return
			case msg, ok := <-p.sendQueueChan:
				if !ok {
					// We're shutting down
					return
				}
				msgCount = msgCount + 1

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

				if err := p.transitionState(msg); err != nil {
					p.SendError(
						fmt.Errorf(
							"%s: error sending message: %s",
							p.config.Name,
							err,
						),
					)
					return
				}

				// We don't want more than maxMessagesPerSegment messages in a segment
				if msgCount >= maxMessagesPerSegment {
					breakLoop = true
					break
				}
				// We don't want to add more messages once we spill over into a second segment
				if payloadBuf.Len() > muxer.SegmentMaxPayloadLength {
					breakLoop = true
					break
				}
				// Check if there are any more queued messages
				if len(p.sendQueueChan) == 0 {
					breakLoop = true
					break
				}
			}
			if breakLoop {
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
			isResponse := p.Role() == ProtocolRoleServer
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
	}
}

func (p *Protocol) recvLoop() {
	defer func() {
		p.waitGroup.Done()
		p.doneSignal.Close()
	}()

	leftoverData := false
	recvBuffer := bytes.NewBuffer(nil)

	for {
		var err error
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			select {
			case <-p.doneSignal.GetCh():
				// Break out of receive loop if we're shutting down
				return
			case <-p.muxerDoneChan:
				return
			case segment, ok := <-p.muxerRecvChan:
				if !ok {
					return
				}
				// Add segment payload to buffer
				recvBuffer.Write(segment.Payload)
			}
		}
		leftoverData = false
		// Wait until ready to receive based on state map
		select {
		case <-p.doneSignal.GetCh():
			// Break out of receive loop if we're shutting down
			return
		case <-p.muxerDoneChan:
			return
		case <-p.recvReadyChan:
		}
		// Decode message into generic list until we can determine what type of message it is.
		// This also lets us determine how many bytes the message is. We use RawMessage here to
		// avoid parsing things that we may not be able to parse
		tmpMsg := []cbor.RawMessage{}
		numBytesRead, err := cbor.Decode(recvBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if err == io.ErrUnexpectedEOF && recvBuffer.Len() > 0 {
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
		msgData := recvBuffer.Bytes()[:numBytesRead]
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
		if err := p.handleMessage(msg); err != nil {
			p.SendError(err)
			return
		}
		if numBytesRead < recvBuffer.Len() {
			// There is another message in the same muxer segment, so we reset the buffer with just
			// the remaining data
			recvBuffer = bytes.NewBuffer(recvBuffer.Bytes()[numBytesRead:])
			leftoverData = true
		} else {
			// Empty out our buffer since we successfully processed the message
			recvBuffer.Reset()
		}
	}
}

func (p *Protocol) stateLoop(ch <-chan protocolStateTransition) {
	var currentState State
	var transitionTimer *time.Timer

	setState := func(s State) {
		// Disable any previous state transition timer
		if transitionTimer != nil && !transitionTimer.Stop() {
			<-transitionTimer.C
		}
		transitionTimer = nil

		// Set the new state
		currentState = s

		// Mark protocol as ready to send/receive based on role and agency of the new state
		switch p.config.StateMap[currentState].Agency {
		case AgencyClient:
			switch p.config.Role {
			case ProtocolRoleClient:
				select {
				case p.sendReadyChan <- true:
				default:
				}
			case ProtocolRoleServer:
				select {
				case p.recvReadyChan <- true:
				default:
				}
			}
		case AgencyServer:
			switch p.config.Role {
			case ProtocolRoleServer:
				select {
				case p.sendReadyChan <- true:
				default:
				}
			case ProtocolRoleClient:
				select {
				case p.recvReadyChan <- true:
				default:
				}
			}
		}

		// Set timeout for state transition
		if p.config.StateMap[currentState].Timeout > 0 {
			transitionTimer = time.NewTimer(p.config.StateMap[currentState].Timeout)
		}
	}
	getTimerChan := func() <-chan time.Time {
		if transitionTimer == nil {
			return nil
		}
		return transitionTimer.C
	}

	protocolDoneChan := p.doneSignal.GetCh()
	stateDoneChan := make(chan struct{})

	setState(p.config.InitialState)

	for {
		select {
		case t := <-ch:
			nextState, err := p.nextState(currentState, t.msg)
			if err != nil {
				t.errorChan <- fmt.Errorf(
					"%s: error handling protocol state transition: %s",
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
					currentState,
				),
			)

		case <-protocolDoneChan:
			// Disable this case so it doesn't block
			protocolDoneChan = nil

			// Wait for all other goroutines to finish before shutting down the state handler
			go func() {
				p.waitGroup.Wait()

				close(stateDoneChan)
			}()

		case <-stateDoneChan:
			// All other goroutines have finished, so we can stop the timer and return
			if transitionTimer != nil && !transitionTimer.Stop() {
				<-transitionTimer.C
			}
			transitionTimer = nil

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
	p.stateTransitionChan <- protocolStateTransition{msg, errorChan}

	return <-errorChan
}

func (p *Protocol) handleMessage(msg Message) error {
	if err := p.transitionState(msg); err != nil {
		return fmt.Errorf("%s: error handling message: %s", p.config.Name, err)
	}

	// Call handler function
	return p.config.MessageHandlerFunc(msg)
}
