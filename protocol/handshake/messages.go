package handshake

const (
	MESSAGE_TYPE_PROPOSE_VERSIONS = 0
	MESSAGE_TYPE_ACCEPT_VERSION   = 1
	MESSAGE_TYPE_REFUSE           = 2

	REFUSE_REASON_VERSION_MISMATCH = 0
	REFUSE_REASON_DECODE_ERROR     = 1
	REFUSE_REASON_REFUSED          = 2
)

type BaseMessage struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type msgProposeVersions struct {
	BaseMessage
	VersionMap map[uint16]interface{}
}

func newMsgProposeVersions(versionMap map[uint16]interface{}) *msgProposeVersions {
	r := &msgProposeVersions{
		BaseMessage: BaseMessage{
			MessageType: MESSAGE_TYPE_PROPOSE_VERSIONS,
		},
		VersionMap: versionMap,
	}
	return r
}

type msgAcceptVersion struct {
	BaseMessage
	Version     uint16
	VersionData interface{}
}

type msgRefuse struct {
	BaseMessage
	Reason []interface{}
}
