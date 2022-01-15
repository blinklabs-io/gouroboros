package block

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	BLOCK_TYPE_ALONZO = 5
)

type AlonzoBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                      struct{} `cbor:",toarray"`
	Header                 ShelleyBlockHeader
	TransactionBodies      []AlonzoTransaction
	TransactionWitnessSets []AlonzoTransactionWitnessSet
	// TODO: figure out how to parse properly
	// We use RawMessage here because the content is arbitrary and can contain data that
	// cannot easily be represented in Go (such as maps with bytestring keys)
	TransactionMetadataSet map[uint]cbor.RawMessage
	InvalidTransactions    []uint
}

type AlonzoTransaction struct {
	MaryTransaction
	ScriptDataHash  Blake2b256                `cbor:"11,keyasint,omitempty"`
	Collateral      []ShelleyTransactionInput `cbor:"13,keyasint,omitempty"`
	RequiredSigners []Blake2b224              `cbor:"14,keyasint,omitempty"`
	NetworkId       uint8                     `cbor:"15,keyasint,omitempty"`
}

type AlonzoTransactionWitnessSet struct {
	ShelleyTransactionWitnessSet
	PlutusScripts interface{} `cbor:"3,keyasint,omitempty"`
	// TODO: figure out how to parse properly
	// We use RawMessage here because the content is arbitrary and can contain data that
	// cannot easily be represented in Go (such as maps with bytestring keys)
	PlutusData []cbor.RawMessage `cbor:"4,keyasint,omitempty"`
	Redeemers  []cbor.RawMessage `cbor:"5,keyasint,omitempty"`
}
