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
	MessageTypeQueryReply      = 3
)

// Refusal reasons
const (
	RefuseReasonVersionMismatch uint64 = 0
	RefuseReasonDecodeError     uint64 = 1
	RefuseReasonRefused         uint64 = 2
)

// RefusalError represents a handshake refusal error
type RefusalError interface {
	error
	ReasonCode() uint64
}

// VersionMismatchError represents a version mismatch refusal
// Format: [0, [*anyVersionNumber]]
type VersionMismatchError struct {
	SupportedVersions []uint16
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("%s: version mismatch (supported versions: %v)", ProtocolName, e.SupportedVersions)
}

func (e *VersionMismatchError) ReasonCode() uint64 {
	return RefuseReasonVersionMismatch
}

// DecodeError represents a handshake decode error refusal
// Format: [1, anyVersionNumber, tstr]
type DecodeError struct {
	Version uint16
	Message string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("%s: decode error (version %d): %s", ProtocolName, e.Version, e.Message)
}

func (e *DecodeError) ReasonCode() uint64 {
	return RefuseReasonDecodeError
}

// RefusedError represents a general refusal
// Format: [2, anyVersionNumber, tstr]
type RefusedError struct {
	Version uint16
	Message string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%s: refused (version %d): %s", ProtocolName, e.Version, e.Message)
}

func (e *RefusedError) ReasonCode() uint64 {
	return RefuseReasonRefused
}

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
	case MessageTypeQueryReply:
		ret = &MsgQueryReply{}
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

type MsgProposeVersions struct {
	protocol.MessageBase
	VersionMap map[uint16]cbor.RawMessage
}

func NewMsgProposeVersions(
	versionMap protocol.ProtocolVersionMap,
) *MsgProposeVersions {
	rawVersionMap := map[uint16]cbor.RawMessage{}
	for version, versionData := range versionMap {
		cborData, err := cbor.Encode(&versionData)
		if err != nil {
			continue
		}
		rawVersionMap[version] = cbor.RawMessage(cborData)
	}
	m := &MsgProposeVersions{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeProposeVersions,
		},
		VersionMap: rawVersionMap,
	}
	return m
}

type MsgAcceptVersion struct {
	protocol.MessageBase
	Version     uint16
	VersionData cbor.RawMessage
}

func NewMsgAcceptVersion(
	version uint16,
	versionData protocol.VersionData,
) *MsgAcceptVersion {
	// This should never fail with our known VersionData types
	cborData, _ := cbor.Encode(&versionData)
	m := &MsgAcceptVersion{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptVersion,
		},
		Version:     version,
		VersionData: cbor.RawMessage(cborData),
	}
	return m
}

type MsgRefuse struct {
	protocol.MessageBase
	Reason []any
}

func NewMsgRefuse(reason []any) *MsgRefuse {
	m := &MsgRefuse{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRefuse,
		},
		Reason: reason,
	}
	return m
}

type MsgQueryReply struct {
	protocol.MessageBase
	VersionMap map[uint16]cbor.RawMessage
}

func NewMsgQueryReply(
	versionMap protocol.ProtocolVersionMap,
) *MsgQueryReply {
	rawVersionMap := map[uint16]cbor.RawMessage{}
	for version, versionData := range versionMap {
		cborData, err := cbor.Encode(&versionData)
		if err != nil {
			continue
		}
		rawVersionMap[version] = cbor.RawMessage(cborData)
	}
	m := &MsgQueryReply{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeQueryReply,
		},
		VersionMap: rawVersionMap,
	}
	return m
}
