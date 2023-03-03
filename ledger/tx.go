package ledger

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

type TransactionBody interface {
	Hash() string
	Cbor() []byte
	// TODO: add additional functions for things like inputs, outputs, etc.
}

func NewTransactionFromCbor(txType uint, data []byte) (interface{}, error) {
	switch txType {
	case TX_TYPE_BYRON:
		return NewByronTransactionFromCbor(data)
	case TX_TYPE_SHELLEY:
		return NewShelleyTransactionFromCbor(data)
	case TX_TYPE_ALLEGRA:
		return NewAllegraTransactionFromCbor(data)
	case TX_TYPE_MARY:
		return NewMaryTransactionFromCbor(data)
	case TX_TYPE_ALONZO:
		return NewAlonzoTransactionFromCbor(data)
	case TX_TYPE_BABBAGE:
		return NewBabbageTransactionFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

func NewTransactionBodyFromCbor(txType uint, data []byte) (interface{}, error) {
	switch txType {
	case TX_TYPE_BYRON:
		return NewByronTransactionBodyFromCbor(data)
	case TX_TYPE_SHELLEY:
		return NewShelleyTransactionBodyFromCbor(data)
	case TX_TYPE_ALLEGRA:
		return NewAllegraTransactionBodyFromCbor(data)
	case TX_TYPE_MARY:
		return NewMaryTransactionBodyFromCbor(data)
	case TX_TYPE_ALONZO:
		return NewAlonzoTransactionBodyFromCbor(data)
	case TX_TYPE_BABBAGE:
		return NewBabbageTransactionBodyFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

func generateTransactionHash(data []byte, prefix []byte) string {
	// We can ignore the error return here because our fixed size/key arguments will
	// never trigger an error
	tmpHash, _ := blake2b.New256(nil)
	if prefix != nil {
		tmpHash.Write(prefix)
	}
	tmpHash.Write(data)
	return hex.EncodeToString(tmpHash.Sum(nil))
}
