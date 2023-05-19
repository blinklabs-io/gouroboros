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

package peersharing

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeShareRequest = 0
	MessageTypeSharePeers   = 1
	MessageTypeDone         = 2
)

// NewMsgFromCbor parses a PeerSharing message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeShareRequest:
		ret = &MsgShareRequest{}
	case MessageTypeSharePeers:
		ret = &MsgSharePeers{}
	case MessageTypeDone:
		ret = &MsgDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgShareRequest struct {
	protocol.MessageBase
	Amount uint8
}

func NewMsgShareRequest(amount uint8) *MsgShareRequest {
	m := &MsgShareRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeShareRequest,
		},
		Amount: amount,
	}
	return m
}

type MsgSharePeers struct {
	protocol.MessageBase
	// TODO: parse peer addresses
	/*
		peerAddress = [0, word32, portNumber]
		      ; ipv6 + portNumber
		    / [1, word32, word32, word32, word32, flowInfo, scopeId, portNumber]

		portNumber = word16

		flowInfo = word32
		scopeId = word32
	*/
	PeerAddresses []interface{}
}

func NewMsgSharePeers(peerAddresses []interface{}) *MsgSharePeers {
	m := &MsgSharePeers{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSharePeers,
		},
		PeerAddresses: peerAddresses,
	}
	return m
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
