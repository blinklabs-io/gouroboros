package protocol

// Provide a common interface for message utility functions
type Message interface {
	SetCbor([]byte)
	Cbor() []byte
	Type() uint8
}

type MessageBase struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	rawCbor     []byte
	MessageType uint8
}

func (m *MessageBase) SetCbor(data []byte) {
	m.rawCbor = make([]byte, len(data))
	copy(m.rawCbor, data)
}

func (m *MessageBase) Cbor() []byte {
	return m.rawCbor
}

func (m *MessageBase) Type() uint8 {
	return m.MessageType
}
