// Copyright 2023 Blink Labs, LLC.
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

package protocol

// Message provides a common interface for message utility functions
type Message interface {
	SetCbor([]byte)
	Cbor() []byte
	Type() uint8
}

// MessageBase is the minimum implementation for a mini-protocol message
type MessageBase struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	rawCbor     []byte
	MessageType uint8
}

// SetCbor stores the original CBOR that was parsed
func (m *MessageBase) SetCbor(data []byte) {
	m.rawCbor = make([]byte, len(data))
	copy(m.rawCbor, data)
}

// Cbor returns the original CBOR that was parsed
func (m *MessageBase) Cbor() []byte {
	return m.rawCbor
}

// Type returns the message type
func (m *MessageBase) Type() uint8 {
	return m.MessageType
}
