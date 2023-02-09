package handshake

import (
	"fmt"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
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
	if _, err := utils.CborDecode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", protocolName, err)
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
