package protocol

import (
	"bytes"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"github.com/fxamacker/cbor/v2"
	"io"
	"reflect"
	"sync"
)

const (
	// This is completely arbitrary, but the line had to be drawn somewhere
	MAX_MESSAGES_PER_SEGMENT = 20
)

type Protocol struct {
	config             ProtocolConfig
	muxerSendChan      chan *muxer.Segment
	muxerRecvChan      chan *muxer.Segment
	state              State
	stateMutex         sync.Mutex
	recvBuffer         *bytes.Buffer
	sendQueueChan      chan Message
	sendStateQueueChan chan Message
	recvReadyChan      chan bool
	sendReadyChan      chan bool
	doneChan           chan bool
}

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

type ProtocolMode uint

const (
	ProtocolModeNone         ProtocolMode = 0
	ProtocolModeNodeToClient ProtocolMode = 1
	ProtocolModeNodeToNode   ProtocolMode = 2
)

type ProtocolRole uint

const (
	ProtocolRoleNone   ProtocolRole = 0
	ProtocolRoleClient ProtocolRole = 1
	ProtocolRoleServer ProtocolRole = 2
)

// Common arguments for individual mini-protocols
type ProtocolOptions struct {
	Muxer     *muxer.Muxer
	ErrorChan chan error
	Mode      ProtocolMode
	// TODO: remove me
	Role    ProtocolRole
	Version uint16
}

type MessageHandlerFunc func(Message, bool) error
type MessageFromCborFunc func(uint, []byte) (Message, error)

func New(config ProtocolConfig) *Protocol {
	p := &Protocol{
		config: config,
	}
	return p
}

func (p *Protocol) Start() {
	// Register protocol with muxer
	p.muxerSendChan, p.muxerRecvChan = p.config.Muxer.RegisterProtocol(p.config.ProtocolId)
	// Create buffers and channels
	p.recvBuffer = bytes.NewBuffer(nil)
	p.sendQueueChan = make(chan Message, 50)
	p.sendStateQueueChan = make(chan Message, 50)
	p.recvReadyChan = make(chan bool, 1)
	p.sendReadyChan = make(chan bool, 1)
	p.doneChan = make(chan bool)
	// Set initial state
	p.setState(p.config.InitialState)
	// Start our send and receive Goroutines
	go p.recvLoop()
	go p.sendLoop()
}

func (p *Protocol) Mode() ProtocolMode {
	return p.config.Mode
}

func (p *Protocol) Role() ProtocolRole {
	return p.config.Role
}

func (p *Protocol) SendMessage(msg Message) error {
	p.sendQueueChan <- msg
	return nil
}

func (p *Protocol) SendError(err error) {
	p.config.ErrorChan <- err
}

func (p *Protocol) sendLoop() {
	var setNewState bool
	var newState State
	var err error
	for {
		select {
		case <-p.sendReadyChan:
			// We are ready to send based on state map
		case <-p.doneChan:
			// We are responsible for closing this channel as the sender, even through it
			// was created by the muxer
			close(p.muxerSendChan)
			// Break out of send loop if we're shutting down
			return
		}
		// Lock the state to prevent collisions
		p.stateMutex.Lock()
		// Check for queued state changes from previous pipelined sends
		setNewState = false
		if len(p.sendStateQueueChan) > 0 {
			msg := <-p.sendStateQueueChan
			newState, err = p.getNewState(msg)
			if err != nil {
				p.SendError(fmt.Errorf("%s: error sending message: %s", p.config.Name, err))
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
			msg := <-p.sendQueueChan
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
				data, err = utils.CborEncode(msg)
				if err != nil {
					p.SendError(err)
					return
				}
			}
			payloadBuf.Write(data)
			if !setNewState {
				newState, err = p.getNewState(msg)
				if err != nil {
					p.SendError(fmt.Errorf("%s: error sending message: %s", p.config.Name, err))
					return
				}
				setNewState = true
			}
			// We don't want more than MAX_MESSAGES_PER_SEGMENT messages in a segment
			if msgCount >= MAX_MESSAGES_PER_SEGMENT {
				break
			}
			// We don't want to add more messages once we spill over into a second segment
			if payloadBuf.Len() > muxer.SEGMENT_MAX_PAYLOAD_LENGTH {
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
			if segmentPayloadLength > muxer.SEGMENT_MAX_PAYLOAD_LENGTH {
				segmentPayloadLength = muxer.SEGMENT_MAX_PAYLOAD_LENGTH
			}
			// Send current segment
			segmentPayload := payloadBuf.Bytes()[:segmentPayloadLength]
			isResponse := false
			if p.Role() == ProtocolRoleServer {
				isResponse = true
			}
			segment := muxer.NewSegment(p.config.ProtocolId, segmentPayload, isResponse)
			p.muxerSendChan <- segment
			// Remove current segment's data from buffer
			if payloadBuf.Len() > segmentPayloadLength {
				payloadBuf = bytes.NewBuffer(payloadBuf.Bytes()[segmentPayloadLength:])
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
	leftoverData := false
	isResponse := false
	for {
		var err error
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			segment, ok := <-p.muxerRecvChan
			// Break out of receive loop if channel is closed
			if !ok {
				close(p.doneChan)
				return
			}
			// Add segment payload to buffer
			p.recvBuffer.Write(segment.Payload)
			// Save whether it's a response
			isResponse = segment.IsResponse()
		}
		leftoverData = false
		// Wait until ready to receive based on state map
		<-p.recvReadyChan
		// Decode message into generic list until we can determine what type of message it is.
		// This also lets us determine how many bytes the message is. We use RawMessage here to
		// avoid parsing things that we may not be able to parse
		var tmpMsg []cbor.RawMessage
		numBytesRead, err := utils.CborDecode(p.recvBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if err == io.EOF && p.recvBuffer.Len() > 0 {
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
		if _, err := utils.CborDecode(tmpMsg[0], &msgType); err != nil {
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
			p.SendError(fmt.Errorf("%s: received unknown message type: %#v", p.config.Name, tmpMsg))
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
		return newState, fmt.Errorf("message %s not allowed in current protocol state %s", reflect.TypeOf(msg).Name(), p.state)
	}
	return newState, nil
}

func (p *Protocol) setState(state State) {
	// Set the new state
	p.state = state
	// Mark protocol as ready to send/receive based on role and agency of the new state
	switch p.config.StateMap[p.state].Agency {
	case AGENCY_CLIENT:
		switch p.config.Role {
		case ProtocolRoleClient:
			p.sendReadyChan <- true
		case ProtocolRoleServer:
			p.recvReadyChan <- true
		}
	case AGENCY_SERVER:
		switch p.config.Role {
		case ProtocolRoleServer:
			p.sendReadyChan <- true
		case ProtocolRoleClient:
			p.recvReadyChan <- true
		}
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
