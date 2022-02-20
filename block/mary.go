package block

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	BLOCK_TYPE_MARY = 4

	BLOCK_HEADER_TYPE_MARY = 3

	TX_TYPE_MARY = 3
)

type MaryBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                      struct{} `cbor:",toarray"`
	Header                 ShelleyBlockHeader
	TransactionBodies      []MaryTransaction
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	// TODO: figure out how to parse properly
	// We use RawMessage here because the content is arbitrary and can contain data that
	// cannot easily be represented in Go (such as maps with bytestring keys)
	TransactionMetadataSet map[uint]cbor.RawMessage
}

func (b *MaryBlock) Id() string {
	return b.Header.Id()
}

type MaryTransaction struct {
	AllegraTransaction
	//Outputs []MaryTransactionOutput `cbor:"1,keyasint,omitempty"`
	Outputs []cbor.RawMessage `cbor:"1,keyasint,omitempty"`
	// TODO: further parsing of this field
	Mint cbor.RawMessage `cbor:"9,keyasint,omitempty"`
}

// TODO: support both forms
/*
transaction_output = [address, amount : value]
value = coin / [coin,multiasset<uint>]
*/
//type MaryTransactionOutput interface{}

type MaryTransactionOutput cbor.RawMessage
