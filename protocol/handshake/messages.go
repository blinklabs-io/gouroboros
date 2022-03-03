package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	MESSAGE_TYPE_PROPOSE_VERSIONS = 0
	MESSAGE_TYPE_ACCEPT_VERSION   = 1
	MESSAGE_TYPE_REFUSE           = 2

	REFUSE_REASON_VERSION_MISMATCH = 0
	REFUSE_REASON_DECODE_ERROR     = 1
	REFUSE_REASON_REFUSED          = 2
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_PROPOSE_VERSIONS:
		ret = &msgProposeVersions{}
	case MESSAGE_TYPE_ACCEPT_VERSION:
		ret = &msgAcceptVersion{}
	case MESSAGE_TYPE_REFUSE:
		ret = &msgRefuse{}
	}
	if _, err := utils.CborDecode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type msgProposeVersions struct {
	protocol.MessageBase
	VersionMap map[uint16]interface{}
}

func newMsgProposeVersions(versionMap map[uint16]interface{}) *msgProposeVersions {
	m := &msgProposeVersions{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_PROPOSE_VERSIONS,
		},
		VersionMap: versionMap,
	}
	return m
}

type msgAcceptVersion struct {
	protocol.MessageBase
	Version     uint16
	VersionData interface{}
}

func newMsgAcceptVersion(version uint16, versionData interface{}) *msgAcceptVersion {
	m := &msgAcceptVersion{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_ACCEPT_VERSION,
		},
		Version:     version,
		VersionData: versionData,
	}
	return m
}

type msgRefuse struct {
	protocol.MessageBase
	Reason []interface{}
}
