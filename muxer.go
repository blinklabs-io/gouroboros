package ouroboros

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

type MessageHeader struct {
	Timestamp     uint32
	ProtocolId    uint16
	PayloadLength uint16
}

type Message struct {
	MessageHeader
	Payload []byte
}

func newMessage(protocolId uint16, payload []byte) (*Message, error) {
	msgHeader := MessageHeader{
		Timestamp:  uint32(time.Now().UnixNano() & 0xffffffff),
		ProtocolId: protocolId,
	}
	msgHeader.PayloadLength = uint16(len(payload))
	msg := &Message{
		MessageHeader: msgHeader,
		Payload:       payload,
	}
	return msg, nil
}

type Muxer struct {
	conn              io.ReadWriteCloser
	errorChan         chan error
	respChan          chan *Message
	protocolSenders   map[uint16]chan []byte
	protocolReceivers map[uint16]chan []byte
}

func NewMuxer(conn io.ReadWriteCloser) *Muxer {
	m := &Muxer{
		conn:              conn,
		errorChan:         make(chan error, 10),
		respChan:          make(chan *Message, 10),
		protocolSenders:   make(map[uint16]chan []byte),
		protocolReceivers: make(map[uint16]chan []byte),
	}
	go m.readLoop()
	return m
}

func (m *Muxer) registerProtocol(senderProtocolId uint16, receiverProtocolId uint16) (chan []byte, chan []byte) {
	// Generate channels
	senderChan := make(chan []byte, 10)
	receiverChan := make(chan []byte, 10)
	// Record channels in protocol sender/receiver maps
	m.protocolSenders[senderProtocolId] = senderChan
	m.protocolReceivers[receiverProtocolId] = receiverChan
	// Start Goroutine to handle outbound messages
	go func() {
		for {
			payload := <-senderChan
			m.Send(senderProtocolId, payload)
		}
	}()
	return senderChan, receiverChan
}

func (m *Muxer) Send(protocolId uint16, payload []byte) error {
	msg, err := newMessage(protocolId, payload)
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.BigEndian, msg.MessageHeader)
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
		msgHeader := MessageHeader{}
		if err := binary.Read(m.conn, binary.BigEndian, &msgHeader); err != nil {
			m.errorChan <- err
		}
		msg := &Message{
			MessageHeader: msgHeader,
			Payload:       make([]byte, msgHeader.PayloadLength),
		}
		// We use ReadFull because it guarantees to read the expected number of bytes or
		// return an error
		if _, err := io.ReadFull(m.conn, msg.Payload); err != nil {
			m.errorChan <- err
		}
		// Send message payload to proper receiver
		m.protocolReceivers[msgHeader.ProtocolId] <- msg.Payload
	}
}
