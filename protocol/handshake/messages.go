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

package handshake

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeProposeVersions = 0
	MessageTypeAcceptVersion   = 1
	MessageTypeRefuse          = 2
)

// Refusal reasons
const (
	RefuseReasonVersionMismatch = 0
	RefuseReasonDecodeError     = 1
	RefuseReasonRefused         = 2
)

// NewMsgFromCbor parses a Handshake message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeProposeVersions:
		ret = &MsgProposeVersions{}
	case MessageTypeAcceptVersion:
		ret = &MsgAcceptVersion{}
	case MessageTypeRefuse:
		ret = &MsgRefuse{}
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

type MsgProposeVersions struct {
	protocol.MessageBase
	VersionMap map[uint16]interface{}
}

func NewMsgProposeVersions(versionMap map[uint16]interface{}) *MsgProposeVersions {
	m := &MsgProposeVersions{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeProposeVersions,
		},
		VersionMap: versionMap,
	}
	return m
}

type MsgAcceptVersion struct {
	protocol.MessageBase
	Version     uint16
	VersionData interface{}
}

func NewMsgAcceptVersion(version uint16, versionData interface{}) *MsgAcceptVersion {
	m := &MsgAcceptVersion{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptVersion,
		},
		Version:     version,
		VersionData: versionData,
	}
	return m
}

type MsgRefuse struct {
	protocol.MessageBase
	Reason []interface{}
}

func NewMsgRefuse(reason []interface{}) *MsgRefuse {
	m := &MsgRefuse{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRefuse,
		},
		Reason: reason,
	}
	return m
}
