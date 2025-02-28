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
	MessageTypeKeepAlive         = 0
	MessageTypeKeepAliveResponse = 1
	MessageTypeDone              = 2
)

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

type MsgKeepAlive struct {
	protocol.MessageBase
	Cookie uint16
}

func NewMsgKeepAlive(cookie uint16) *MsgKeepAlive {
	msg := &MsgKeepAlive{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeKeepAlive,
		},
		Cookie: cookie,
	}
	return msg
}

type MsgKeepAliveResponse struct {
	protocol.MessageBase
	Cookie uint16
}

func NewMsgKeepAliveResponse(cookie uint16) *MsgKeepAliveResponse {
	msg := &MsgKeepAliveResponse{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeKeepAliveResponse,
		},
		Cookie: cookie,
	}
	return msg
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
	return m
}
