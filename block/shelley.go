package block

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	BLOCK_TYPE_SHELLEY = 2
)

type ShelleyBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Header ShelleyBlockHeader
	// TODO: create structure for transaction bodies that accounts for
	// map with bytestring keys for staking reward withdrawals
	TransactionBodies      []cbor.RawMessage
	TransactionWitnessSets interface{}
	TransactionMetadataSet interface{}
}

type ShelleyBlockHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Body struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_                  struct{} `cbor:",toarray"`
		BlockNumber        uint64
		Slot               uint64
		PrevHash           Blake2b256
		IssuerVkey         interface{}
		VrfKey             interface{}
		NonceVrf           interface{}
		LeaderVrf          interface{}
		BlockBodySize      uint32
		BlockBodyHash      Blake2b256
		HotVkey            interface{}
		SequenceNumber     uint32
		KesPeriod          uint32
		Sigma              interface{}
		OperationalCertNum uint
		ProtocolVersion    interface{}
	}
	Signature interface{}
}
