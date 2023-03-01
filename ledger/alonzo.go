package ledger

import (
	"fmt"

	"github.com/cloudstruct/go-ouroboros-network/cbor"
)

const (
	ERA_ID_ALONZO = 4

	BLOCK_TYPE_ALONZO = 5

	BLOCK_HEADER_TYPE_ALONZO = 4

	TX_TYPE_ALONZO = 4
)

type AlonzoBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *AlonzoBlockHeader
	TransactionBodies      []AlonzoTransactionBody
	TransactionWitnessSets []AlonzoTransactionWitnessSet
	TransactionMetadataSet map[uint]cbor.Value
	InvalidTransactions    []uint
}

func (b *AlonzoBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

func (b *AlonzoBlock) Hash() string {
	return b.Header.Hash()
}

func (b *AlonzoBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *AlonzoBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *AlonzoBlock) Era() Era {
	return eras[ERA_ID_ALONZO]
}

func (b *AlonzoBlock) Transactions() []TransactionBody {
	ret := []TransactionBody{}
	for _, v := range b.TransactionBodies {
		ret = append(ret, &v)
	}
	return ret
}

type AlonzoBlockHeader struct {
	ShelleyBlockHeader
}

func (h *AlonzoBlockHeader) Era() Era {
	return eras[ERA_ID_ALONZO]
}

type AlonzoTransactionBody struct {
	MaryTransactionBody
	ScriptDataHash  Blake2b256                `cbor:"11,keyasint,omitempty"`
	Collateral      []ShelleyTransactionInput `cbor:"13,keyasint,omitempty"`
	RequiredSigners []Blake2b224              `cbor:"14,keyasint,omitempty"`
	NetworkId       uint8                     `cbor:"15,keyasint,omitempty"`
}

func (b *AlonzoTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

type AlonzoTransactionWitnessSet struct {
	ShelleyTransactionWitnessSet
	PlutusScripts interface{}  `cbor:"3,keyasint,omitempty"`
	PlutusData    []cbor.Value `cbor:"4,keyasint,omitempty"`
	Redeemers     []cbor.Value `cbor:"5,keyasint,omitempty"`
}

type AlonzoTransaction struct {
	cbor.StructAsArray
	Body       AlonzoTransactionBody
	WitnessSet AlonzoTransactionWitnessSet
	IsValid    bool
	Metadata   cbor.Value
}

func NewAlonzoBlockFromCbor(data []byte) (*AlonzoBlock, error) {
	var alonzoBlock AlonzoBlock
	if _, err := cbor.Decode(data, &alonzoBlock); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &alonzoBlock, nil
}

func NewAlonzoTransactionBodyFromCbor(data []byte) (*AlonzoTransactionBody, error) {
	var alonzoTx AlonzoTransactionBody
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionFromCbor(data []byte) (*AlonzoTransaction, error) {
	var alonzoTx AlonzoTransaction
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &alonzoTx, nil
}
