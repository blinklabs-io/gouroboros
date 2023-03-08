package ledger

import (
	"fmt"

	"github.com/cloudstruct/go-ouroboros-network/cbor"
)

const (
	ERA_ID_BABBAGE = 5

	BLOCK_TYPE_BABBAGE = 6

	BLOCK_HEADER_TYPE_BABBAGE = 5

	TX_TYPE_BABBAGE = 5
)

type BabbageBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *BabbageBlockHeader
	TransactionBodies      []BabbageTransactionBody
	TransactionWitnessSets []BabbageTransactionWitnessSet
	TransactionMetadataSet map[uint]cbor.Value
	InvalidTransactions    []uint
}

func (b *BabbageBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

func (b *BabbageBlock) Hash() string {
	return b.Header.Hash()
}

func (b *BabbageBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *BabbageBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *BabbageBlock) Era() Era {
	return eras[ERA_ID_BABBAGE]
}

func (b *BabbageBlock) Transactions() []TransactionBody {
	ret := []TransactionBody{}
	for _, v := range b.TransactionBodies {
		ret = append(ret, &v)
	}
	return ret
}

type BabbageBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash string
	Body struct {
		cbor.StructAsArray
		BlockNumber   uint64
		Slot          uint64
		PrevHash      Blake2b256
		IssuerVkey    interface{}
		VrfKey        interface{}
		VrfResult     interface{}
		BlockBodySize uint32
		BlockBodyHash Blake2b256
		OpCert        struct {
			cbor.StructAsArray
			HotVkey        interface{}
			SequenceNumber uint32
			KesPeriod      uint32
			Signature      interface{}
		}
		ProtoVersion struct {
			cbor.StructAsArray
			Major uint64
			Minor uint64
		}
	}
	Signature interface{}
}

func (h *BabbageBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCborGeneric(cborData, h)
}

func (h *BabbageBlockHeader) Hash() string {
	if h.hash == "" {
		h.hash = generateBlockHeaderHash(h.Cbor(), nil)
	}
	return h.hash
}

func (h *BabbageBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *BabbageBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *BabbageBlockHeader) Era() Era {
	return eras[ERA_ID_BABBAGE]
}

type BabbageTransactionBody struct {
	AlonzoTransactionBody
	Outputs          []BabbageTransactionOutput `cbor:"1,keyasint,omitempty"`
	CollateralReturn BabbageTransactionOutput   `cbor:"16,keyasint,omitempty"`
	TotalCollateral  uint64                     `cbor:"17,keyasint,omitempty"`
	ReferenceInputs  []ShelleyTransactionInput  `cbor:"18,keyasint,omitempty"`
}

func (b *BabbageTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

type BabbageTransactionOutput struct {
	cbor.DecodeStoreCbor
	Address      Blake2b256        `cbor:"0,keyasint,omitempty"`
	Amount       cbor.Value        `cbor:"1,keyasint,omitempty"`
	DatumOption  []cbor.RawMessage `cbor:"2,keyasint,omitempty"`
	ScriptRef    []cbor.RawMessage `cbor:"3,keyasint,omitempty"`
	legacyOutput bool
}

func (o *BabbageTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as legacy output first
	var tmpOutput AlonzoTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp legacy object to Babbage format
		o.Address = tmpOutput.Address
		o.Amount = tmpOutput.Amount
		o.legacyOutput = true
	} else {
		return o.UnmarshalCborGeneric(cborData, o)
	}
	return nil
}

type BabbageTransactionWitnessSet struct {
	AlonzoTransactionWitnessSet
	PlutusV2Scripts []cbor.RawMessage `cbor:"6,keyasint,omitempty"`
}

type BabbageTransaction struct {
	cbor.StructAsArray
	Body       BabbageTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	IsValid    bool
	Metadata   cbor.Value
}

func NewBabbageBlockFromCbor(data []byte) (*BabbageBlock, error) {
	var babbageBlock BabbageBlock
	if _, err := cbor.Decode(data, &babbageBlock); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &babbageBlock, nil
}

func NewBabbageBlockHeaderFromCbor(data []byte) (*BabbageBlockHeader, error) {
	var babbageBlockHeader BabbageBlockHeader
	if _, err := cbor.Decode(data, &babbageBlockHeader); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &babbageBlockHeader, nil
}

func NewBabbageTransactionBodyFromCbor(data []byte) (*BabbageTransactionBody, error) {
	var babbageTx BabbageTransactionBody
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &babbageTx, nil
}

func NewBabbageTransactionFromCbor(data []byte) (*BabbageTransaction, error) {
	var babbageTx BabbageTransaction
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &babbageTx, nil
}
