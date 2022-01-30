package block

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	BLOCK_TYPE_ALLEGRA = 3

	BLOCK_HEADER_TYPE_ALLEGRA = 2
)

type AllegraBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                      struct{} `cbor:",toarray"`
	Header                 ShelleyBlockHeader
	TransactionBodies      []AllegraTransaction
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	// TODO: figure out how to parse properly
	// We use RawMessage here because the content is arbitrary and can contain data that
	// cannot easily be represented in Go (such as maps with bytestring keys)
	TransactionMetadataSet map[uint]cbor.RawMessage
}

func (b *AllegraBlock) Id() string {
	return b.Header.Id()
}

type AllegraTransaction struct {
	ShelleyTransaction
	ValidityIntervalStart uint64 `cbor:"8,keyasint,omitempty"`
}
