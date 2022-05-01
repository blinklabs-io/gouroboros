package muxer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	// Magic number chosen to represent unknown protocols
	PROTOCOL_UNKNOWN uint16 = 0xabcd

	// Handshake protocol ID
	PROTOCOL_HANDSHAKE = 0
)

type Muxer struct {
	conn              net.Conn
	sendMutex         sync.Mutex
	startChan         chan bool
	doneChan          chan bool
	ErrorChan         chan error
	protocolSenders   map[uint16]chan *Segment
	protocolReceivers map[uint16]chan *Segment
}

func New(conn net.Conn) *Muxer {
	m := &Muxer{
		conn:              conn,
		startChan:         make(chan bool, 1),
		doneChan:          make(chan bool),
		ErrorChan:         make(chan error, 10),
		protocolSenders:   make(map[uint16]chan *Segment),
		protocolReceivers: make(map[uint16]chan *Segment),
	}
	go m.readLoop()
	return m
}

func (m *Muxer) Start() {
	m.startChan <- true
}

func (m *Muxer) Stop() {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return
	default:
	}
	// Close protocol receive channels
	// We rely on the individual mini-protocols to close the sender channel
	for _, recvChan := range m.protocolReceivers {
		close(recvChan)
	}
	// Close ErrorChan to signify to consumer that we're shutting down
	close(m.ErrorChan)
	// Close doneChan to signify that we're shutting down
	close(m.doneChan)
}

func (m *Muxer) sendError(err error) {
	// Immediately return if we're already shutting down
	select {
	case <-m.doneChan:
		return
	default:
	}
	// Send error to consumer
	m.ErrorChan <- err
	// Stop the muxer on any error
	m.Stop()
}

func (m *Muxer) RegisterProtocol(protocolId uint16) (chan *Segment, chan *Segment) {
	// Generate channels
	senderChan := make(chan *Segment, 10)
	receiverChan := make(chan *Segment, 10)
	// Record channels in protocol sender/receiver maps
	m.protocolSenders[protocolId] = senderChan
	m.protocolReceivers[protocolId] = receiverChan
	// Start Goroutine to handle outbound messages
	go func() {
		for {
			select {
			case _, ok := <-m.doneChan:
				// doneChan has been closed, which means we're shutting down
				if !ok {
					return
				}
			case msg := <-senderChan:
				if err := m.Send(msg); err != nil {
					m.sendError(err)
					return
				}
			}
		}
	}()
	return senderChan, receiverChan
}

func (m *Muxer) Send(msg *Segment) error {
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

func (m *Muxer) readLoop() {
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
			m.sendError(err)
			return
		}
		// Send message payload to proper receiver
		recvChan := m.protocolReceivers[msg.GetProtocolId()]
		if recvChan == nil {
			// Try the "unknown protocol" receiver if we didn't find an explicit one
			recvChan = m.protocolReceivers[PROTOCOL_UNKNOWN]
			if recvChan == nil {
				m.sendError(fmt.Errorf("received message for unknown protocol ID %d", msg.GetProtocolId()))
				return
			}
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
