package protocol

import (
	"bytes"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"io"
	"sync"
)

type Protocol struct {
	config        ProtocolConfig
	sendChan      chan *muxer.Segment
	recvChan      chan *muxer.Segment
	state         State
	stateMutex    sync.Mutex
	recvBuffer    *bytes.Buffer
	recvReadyChan chan bool
	sendReadyChan chan bool
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

type ProtocolOptions struct {
	Muxer     *muxer.Muxer
	ErrorChan chan error
	Mode      ProtocolMode
	Role      ProtocolRole
}

type MessageHandlerFunc func(Message, bool) error
type MessageFromCborFunc func(uint, []byte) (Message, error)

func New(config ProtocolConfig) *Protocol {
	sendChan, recvChan := config.Muxer.RegisterProtocol(config.ProtocolId)
	p := &Protocol{
		config:        config,
		sendChan:      sendChan,
		recvChan:      recvChan,
		recvBuffer:    bytes.NewBuffer(nil),
		recvReadyChan: make(chan bool, 1),
		sendReadyChan: make(chan bool, 1),
	}
	// Set initial state
	p.setState(config.InitialState)
	// Start our receiver Goroutine
	go p.recvLoop()
	return p
}

func (p *Protocol) Mode() ProtocolMode {
	return p.config.Mode
}

func (p *Protocol) Role() ProtocolRole {
	return p.config.Role
}

func (p *Protocol) SendMessage(msg Message, isResponse bool) error {
	// Wait until ready to send based on state map
	<-p.sendReadyChan
	// Lock the state to prevent collisions
	p.stateMutex.Lock()
	if err := p.checkCurrentState(); err != nil {
		return fmt.Errorf("%s: error sending message: %s", p.config.Name, err)
	}
	newState, err := p.getNewState(msg)
	if err != nil {
		return fmt.Errorf("%s: error sending message: %s", p.config.Name, err)
	}
	// Get raw CBOR from message
	data := msg.Cbor()
	// If message has no raw CBOR, encode the message
	if data == nil {
		var err error
		data, err = utils.CborEncode(msg)
		if err != nil {
			return err
		}
	}
	// Send message in multiple segments (if needed)
	for {
		// Determine segment payload length
		segmentPayloadLength := len(data)
		if segmentPayloadLength > muxer.SEGMENT_MAX_PAYLOAD_LENGTH {
			segmentPayloadLength = muxer.SEGMENT_MAX_PAYLOAD_LENGTH
		}
		// Send current segment
		segmentPayload := data[:segmentPayloadLength]
		segment := muxer.NewSegment(p.config.ProtocolId, segmentPayload, isResponse)
		p.sendChan <- segment
		// Remove current segment's data from buffer
		if len(data) > segmentPayloadLength {
			data = data[segmentPayloadLength:]
		} else {
			break
		}
	}
	// Set new state and unlock
	p.setState(newState)
	p.stateMutex.Unlock()
	return nil
}

func (p *Protocol) SendError(err error) {
	p.config.ErrorChan <- err
}

func (p *Protocol) recvLoop() {
	leftoverData := false
	isResponse := false
	for {
		var err error
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			segment := <-p.recvChan
			// Add segment payload to buffer
			p.recvBuffer.Write(segment.Payload)
			// Save whether it's a response
			isResponse = segment.IsResponse()
		}
		leftoverData = false
		// Wait until ready to receive based on state map
		<-p.recvReadyChan
		// Decode message into generic list until we can determine what type of message it is
		// This also lets us determine how many bytes the message is
		var tmpMsg []interface{}
		numBytesRead, err := utils.CborDecode(p.recvBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if err == io.EOF && p.recvBuffer.Len() > 0 {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				p.recvReadyChan <- true
				continue
			}
			p.config.ErrorChan <- fmt.Errorf("%s: decode error: %s", p.config.Name, err)
		}
		// Create Message object from CBOR
		msgType := uint(tmpMsg[0].(uint64))
		msgData := p.recvBuffer.Bytes()[:numBytesRead]
		msg, err := p.config.MessageFromCborFunc(msgType, msgData)
		if err != nil {
			p.config.ErrorChan <- err
		}
		if msg == nil {
			p.config.ErrorChan <- fmt.Errorf("%s: received unknown message type: %#v", p.config.Name, tmpMsg)
		}
		// Handle message
		if err := p.handleMessage(msg, isResponse); err != nil {
			p.config.ErrorChan <- err
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

func (p *Protocol) checkCurrentState() error {
	if currentStateMapEntry, ok := p.config.StateMap[p.state]; ok {
		if currentStateMapEntry.Agency == AGENCY_NONE {
			return fmt.Errorf("protocol is in state with no agency")
		}
		// TODO: check client/server agency
	} else {
		return fmt.Errorf("protocol in unknown state")
	}
	return nil
}

func (p *Protocol) getNewState(msg Message) (State, error) {
	var newState State
	matchFound := false
	for _, transition := range p.config.StateMap[p.state].Transitions {
		if transition.MsgType == msg.Type() {
			newState = transition.NewState
			matchFound = true
			break
		}
	}
	if !matchFound {
		return newState, fmt.Errorf("message not allowed in current protocol state")
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
	if err := p.checkCurrentState(); err != nil {
		return fmt.Errorf("%s: error handling message: %s", p.config.Name, err)
	}
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
