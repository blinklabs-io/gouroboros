package block

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	BLOCK_TYPE_BYRON_EBB  = 0
	BLOCK_TYPE_BYRON_MAIN = 1

	BLOCK_HEADER_TYPE_BYRON = 0

	TX_TYPE_BYRON = 0
)

type ByronMainBlockHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_             struct{} `cbor:",toarray"`
	id            string
	ProtocolMagic uint32
	PrevBlock     Blake2b256
	BodyProof     interface{}
	ConsensusData struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_ struct{} `cbor:",toarray"`
		// [slotid, pubkey, difficulty, blocksig]
		SlotId struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_     struct{} `cbor:",toarray"`
			Epoch uint64
			Slot  uint16
		}
		PubKey     []byte
		Difficulty struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_       struct{} `cbor:",toarray"`
			Unknown uint64
		}
		BlockSig []interface{}
	}
	ExtraData struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_            struct{} `cbor:",toarray"`
		BlockVersion struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_       struct{} `cbor:",toarray"`
			Major   uint16
			Minor   uint16
			Unknown uint8
		}
		SoftwareVersion struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_       struct{} `cbor:",toarray"`
			Name    string
			Unknown uint32
		}
		Attributes interface{}
		ExtraProof Blake2b256
	}
}

func (h *ByronMainBlockHeader) Id() string {
	return h.id
}

type ByronMainBlockBody struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_         struct{} `cbor:",toarray"`
	TxPayload []interface{}
	// We keep this field as raw CBOR, since it contains a map with []byte
	// keys, which Go doesn't allow
	SscPayload cbor.RawMessage
	DlgPayload []interface{}
	UpdPayload []interface{}
}

type ByronEpochBoundaryBlockHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_             struct{} `cbor:",toarray"`
	id            string
	ProtocolMagic uint32
	PrevBlock     Blake2b256
	BodyProof     interface{}
	ConsensusData struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_          struct{} `cbor:",toarray"`
		Epoch      uint64
		Difficulty struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_     struct{} `cbor:",toarray"`
			Value uint64
		}
	}
	ExtraData interface{}
}

func (h *ByronEpochBoundaryBlockHeader) Id() string {
	return h.id
}

type ByronMainBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Header ByronMainBlockHeader
	Body   ByronMainBlockBody
	Extra  []interface{}
}

func (b *ByronMainBlock) Id() string {
	return b.Header.Id()
}

type ByronEpochBoundaryBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Header ByronEpochBoundaryBlockHeader
	Body   []Blake2b224
	Extra  []interface{}
}

func (b *ByronEpochBoundaryBlock) Id() string {
	return b.Header.Id()
}
