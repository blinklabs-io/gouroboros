// Copyright 2023 Blink Labs Software
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

package keepalive

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	// MessageTypeKeepAlive is the message type for keep-alive requests.
	MessageTypeKeepAlive = 0
	// MessageTypeKeepAliveResponse is the message type for keep-alive responses.
	MessageTypeKeepAliveResponse = 1
	// MessageTypeDone is the message type for done messages.
	MessageTypeDone = 2
)

// NewMsgFromCbor decodes a CBOR-encoded message of the given type and returns the corresponding protocol.Message.
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeKeepAlive:
		ret = &MsgKeepAlive{}
	case MessageTypeKeepAliveResponse:
		ret = &MsgKeepAliveResponse{}
	case MessageTypeDone:
		ret = &MsgDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

// MsgKeepAlive represents a keep-alive request message containing a cookie value.
type MsgKeepAlive struct {
	protocol.MessageBase
	Cookie uint16
}

// NewMsgKeepAlive creates and returns a new keep-alive request message with the given cookie value.
func NewMsgKeepAlive(cookie uint16) *MsgKeepAlive {
	msg := &MsgKeepAlive{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeKeepAlive,
		},
		Cookie: cookie,
	}
	return msg
}

// MsgKeepAliveResponse represents a keep-alive response message containing a cookie value.
type MsgKeepAliveResponse struct {
	protocol.MessageBase
	Cookie uint16
}

// NewMsgKeepAliveResponse creates and returns a new keep-alive response message with the given cookie value.
func NewMsgKeepAliveResponse(cookie uint16) *MsgKeepAliveResponse {
	msg := &MsgKeepAliveResponse{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeKeepAliveResponse,
		},
		Cookie: cookie,
	}
	return msg
}

// MsgDone represents a done message in the keep-alive protocol, indicating the end of the session.
type MsgDone struct {
	protocol.MessageBase
}

// NewMsgDone creates and returns a new done message.
func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
	return m
}
