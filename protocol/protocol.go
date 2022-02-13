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
	protocolId      uint16
	name            string
	errorChan       chan error
	sendChan        chan *muxer.Segment
	recvChan        chan *muxer.Segment
	state           State
	stateMutex      sync.Mutex
	recvBuffer      *bytes.Buffer
	msgHandlerFunc  MessageHandlerFunc
	msgFromCborFunc MessageFromCborFunc
}

type State struct {
	Id   uint
	Name string
}

func NewState(id uint, name string) State {
	return State{
		Id:   id,
		Name: name,
	}
}

func (s State) String() string {
	return s.Name
}

type MessageHandlerFunc func(Message) error
type MessageFromCborFunc func(uint, []byte) (Message, error)

func New(name string, protocolId uint16, m *muxer.Muxer, errorChan chan error, handlerFunc MessageHandlerFunc, msgFromCborFunc MessageFromCborFunc) *Protocol {
	sendChan, recvChan := m.RegisterProtocol(protocolId)
	p := &Protocol{
		name:            name,
		protocolId:      protocolId,
		errorChan:       errorChan,
		sendChan:        sendChan,
		recvChan:        recvChan,
		recvBuffer:      bytes.NewBuffer(nil),
		msgHandlerFunc:  handlerFunc,
		msgFromCborFunc: msgFromCborFunc,
	}
	// Start our receiver Goroutine
	go p.recvLoop()
	return p
}

func (p *Protocol) GetState() State {
	return p.state
}

func (p *Protocol) SetState(state State) {
	p.state = state
}

func (p *Protocol) LockState(allowedStates []State) error {
	p.stateMutex.Lock()
	inAllowedState := false
	for _, state := range allowedStates {
		if state == p.state {
			inAllowedState = true
			break
		}
	}
	if !inAllowedState {
		p.stateMutex.Unlock()
		return fmt.Errorf("protocol is not in allowed state (currently in state %s)", p.state.Name)
	}
	return nil
}

func (p *Protocol) UnlockState(newState State) {
	p.state = newState
	p.stateMutex.Unlock()
}

func (p *Protocol) SendMessage(msg interface{}, isResponse bool) error {
	data, err := utils.CborEncode(msg)
	if err != nil {
		return err
	}
	segment := muxer.NewSegment(p.protocolId, data, isResponse)
	p.sendChan <- segment
	return nil
}

func (p *Protocol) recvLoop() {
	leftoverData := false
	for {
		var err error
		// Don't grab the next segment from the muxer if we still have data in the buffer
		if !leftoverData {
			// Wait for segment
			segment := <-p.recvChan
			// Add segment payload to buffer
			p.recvBuffer.Write(segment.Payload)
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
			p.errorChan <- fmt.Errorf("%s: decode error: %s", p.name, err)
		}
		msgType := uint(tmpMsg[0].(uint64))
		msg, err := p.msgFromCborFunc(msgType, p.recvBuffer.Bytes())
		if err != nil {
			p.errorChan <- err
		}
		if msg == nil {
			p.errorChan <- fmt.Errorf("%s: received unknown message type: %#v", p.name, tmpMsg)
		}
		if err := p.msgHandlerFunc(msg); err != nil {
			p.errorChan <- err
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
