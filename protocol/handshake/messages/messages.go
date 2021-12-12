package messages

const (
	MESSAGE_TYPE_REQUEST           = 0
	MESSAGE_TYPE_RESPONSE_ACCEPT   = 1
	MESSAGE_TYPE_RESPONSE_REFUSE   = 2
	REFUSE_REASON_VERSION_MISMATCH = 0
	REFUSE_REASON_DECODE_ERROR     = 1
	REFUSE_REASON_REFUSED          = 2
)

type BaseMessage struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type Request struct {
	BaseMessage
	VersionMap map[uint16]uint32
}

func NewRequest(versionMap map[uint16]uint32) *Request {
	r := &Request{
		BaseMessage: BaseMessage{
			MessageType: MESSAGE_TYPE_REQUEST,
		},
		VersionMap: versionMap,
	}
	return r
}

type ResponseAccept struct {
	BaseMessage
	Version      uint16
	NetworkMagic uint32
}

type ResponseRefuse struct {
	BaseMessage
	Reason []interface{}
}
