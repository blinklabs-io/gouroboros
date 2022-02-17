package muxer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

const (
	// Magic number chosen to represent unknown protocols
	PROTOCOL_UNKNOWN uint16 = 0xabcd
)

type Muxer struct {
	conn              io.ReadWriteCloser
	sendMutex         sync.Mutex
	ErrorChan         chan error
	protocolSenders   map[uint16]chan *Segment
	protocolReceivers map[uint16]chan *Segment
}

func New(conn io.ReadWriteCloser) *Muxer {
	m := &Muxer{
		conn:              conn,
		ErrorChan:         make(chan error, 10),
		protocolSenders:   make(map[uint16]chan *Segment),
		protocolReceivers: make(map[uint16]chan *Segment),
	}
	go m.readLoop()
	return m
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
			msg := <-senderChan
			if err := m.Send(msg); err != nil {
				m.ErrorChan <- err
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
	for {
		header := SegmentHeader{}
		if err := binary.Read(m.conn, binary.BigEndian, &header); err != nil {
			m.ErrorChan <- err
		}
		msg := &Segment{
			SegmentHeader: header,
			Payload:       make([]byte, header.PayloadLength),
		}
		// We use ReadFull because it guarantees to read the expected number of bytes or
		// return an error
		if _, err := io.ReadFull(m.conn, msg.Payload); err != nil {
			m.ErrorChan <- err
		}
		// Send message payload to proper receiver
		recvChan := m.protocolReceivers[msg.GetProtocolId()]
		if recvChan == nil {
			// Try the "unknown protocol" receiver if we didn't find an explicit one
			recvChan = m.protocolReceivers[PROTOCOL_UNKNOWN]
			if recvChan == nil {
				m.ErrorChan <- fmt.Errorf("received message for unknown protocol ID %d", msg.GetProtocolId())
			}
		}
		if recvChan != nil {
			recvChan <- msg
		}
	}
}
