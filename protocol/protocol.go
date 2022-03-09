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
	config     ProtocolConfig
	sendChan   chan *muxer.Segment
	recvChan   chan *muxer.Segment
	state      State
	stateMutex sync.Mutex
	recvBuffer *bytes.Buffer
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
		config:     config,
		sendChan:   sendChan,
		recvChan:   recvChan,
		recvBuffer: bytes.NewBuffer(nil),
	}
	// Set initial state
	p.state = config.InitialState
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
	// Lock the state to prevent collisions
	p.stateMutex.Lock()
	if err := p.checkCurrentState(); err != nil {
		return fmt.Errorf("%s: error sending message: %s", p.config.Name, err)
	}
	newState, err := p.getNewState(msg)
	if err != nil {
		return fmt.Errorf("%s: error sending message: %s", p.config.Name, err)
	}
	data, err := utils.CborEncode(msg)
	if err != nil {
		return err
	}
	segment := muxer.NewSegment(p.config.ProtocolId, data, isResponse)
	p.sendChan <- segment
	// Set new state and unlock
	p.state = newState
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
		// Decode message into generic list until we can determine what type of message it is
		// This also lets us determine how many bytes the message is
		var tmpMsg []interface{}
		numBytesRead, err := utils.CborDecode(p.recvBuffer.Bytes(), &tmpMsg)
		if err != nil {
			if err == io.EOF && p.recvBuffer.Len() > 0 {
				// This is probably a multi-part message, so we wait until we get more of the message
				// before trying to process it
				continue
			}
			p.config.ErrorChan <- fmt.Errorf("%s: decode error: %s", p.config.Name, err)
		}
		// Create Message object from CBOR
		msgType := uint(tmpMsg[0].(uint64))
		msg, err := p.config.MessageFromCborFunc(msgType, p.recvBuffer.Bytes())
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
	p.state = newState
	p.stateMutex.Unlock()
	// Call handler function
	return p.config.MessageHandlerFunc(msg, isResponse)
}
