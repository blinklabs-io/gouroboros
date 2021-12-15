package common

import (
	"encoding/hex"
)

type ByronMainBlockHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_             struct{} `cbor:",toarray"`
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

type ByronMainBlockBody struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_          struct{} `cbor:",toarray"`
	TxPayload  []interface{}
	SscPayload *BlockBodySscPayload
	DlgPayload []interface{}
	UpdPayload []interface{}
}

type ByronEpochBoundaryBlockHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_             struct{} `cbor:",toarray"`
	ProtocolMagic uint32
	PrevBlock     Blake2b256
	BodyProof     interface{}
	ConsensusData interface{}
	ExtraData     interface{}
}

// This mostly exists to override the below function
type BlockBodySscPayload struct{}

// Prevent unmarshaling of the SSC payload data, since it contains a map with
// []byte keys, which Go doesn't allow
func (payload *BlockBodySscPayload) UnmarshalCBOR(data []byte) error {
	return nil
}

type ByronMainBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Header ByronMainBlockHeader
	Body   ByronMainBlockBody
	Extra  []interface{}
}

type ByronEpochBoundaryBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Header ByronEpochBoundaryBlockHeader
	Body   []Blake2b224
	Extra  []interface{}
}

type Blake2b256 [32]byte

func (b Blake2b256) String() string {
	return hex.EncodeToString([]byte(b[:]))
}

type Blake2b224 [28]byte

func (b Blake2b224) String() string {
	return hex.EncodeToString([]byte(b[:]))
}

/*
blake2b-256 = bytes .size 32

txid = blake2b-256
blockid = blake2b-256
updid = blake2b-256
hash = blake2b-256

blake2b-224 = bytes .size 28

addressid = blake2b-224
stakeholderid = blake2b-224
*/

// block = [ blockHeader, blockBody ]
//
// blockHeader = [ headerHash, chainHash, headerSlot, headerBlockNo, headerBodyHash ]
// headerHash = int
// chainHash = genesisHash / blockHash
// genesisHash = [ ]
// blockHash = [ int ]
// blockBody = bstr
// heade rSlot = word64
// headerBlockNo = word64
// headerBodyHash = int
