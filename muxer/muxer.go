package muxer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type Muxer struct {
	conn              io.ReadWriteCloser
	ErrorChan         chan error
	protocolSenders   map[uint16]chan *Message
	protocolReceivers map[uint16]chan *Message
}

func New(conn io.ReadWriteCloser) *Muxer {
	m := &Muxer{
		conn:              conn,
		ErrorChan:         make(chan error, 10),
		protocolSenders:   make(map[uint16]chan *Message),
		protocolReceivers: make(map[uint16]chan *Message),
	}
	go m.readLoop()
	return m
}

func (m *Muxer) RegisterProtocol(protocolId uint16) (chan *Message, chan *Message) {
	// Generate channels
	senderChan := make(chan *Message, 10)
	receiverChan := make(chan *Message, 10)
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

func (m *Muxer) Send(msg *Message) error {
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.BigEndian, msg.MessageHeader)
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
		header := MessageHeader{}
		if err := binary.Read(m.conn, binary.BigEndian, &header); err != nil {
			m.ErrorChan <- err
		}
		msg := &Message{
			MessageHeader: header,
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
			m.ErrorChan <- fmt.Errorf("received message for unknown protocol ID %d", msg.GetProtocolId())
		} else {
			m.protocolReceivers[msg.GetProtocolId()] <- msg
		}
	}
}
