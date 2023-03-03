package ledger

import (
	"fmt"

	"github.com/cloudstruct/go-ouroboros-network/cbor"
)

const (
	ERA_ID_SHELLEY = 1

	BLOCK_TYPE_SHELLEY = 2

	BLOCK_HEADER_TYPE_SHELLEY = 1

	TX_TYPE_SHELLEY = 1
)

type ShelleyBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *ShelleyBlockHeader
	TransactionBodies      []ShelleyTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]cbor.Value
}

func (b *ShelleyBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

func (b *ShelleyBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ShelleyBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ShelleyBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ShelleyBlock) Era() Era {
	return eras[ERA_ID_SHELLEY]
}

func (b *ShelleyBlock) Transactions() []TransactionBody {
	ret := []TransactionBody{}
	for _, v := range b.TransactionBodies {
		ret = append(ret, &v)
	}
	return ret
}

type ShelleyBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash string
	Body struct {
		cbor.StructAsArray
		BlockNumber          uint64
		Slot                 uint64
		PrevHash             Blake2b256
		IssuerVkey           interface{}
		VrfKey               interface{}
		NonceVrf             interface{}
		LeaderVrf            interface{}
		BlockBodySize        uint32
		BlockBodyHash        Blake2b256
		OpCertHotVkey        interface{}
		OpCertSequenceNumber uint32
		OpCertKesPeriod      uint32
		OpCertSignature      interface{}
		ProtoMajorVersion    uint64
		ProtoMinorVersion    uint64
	}
	Signature interface{}
}

func (h *ShelleyBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCborGeneric(cborData, h)
}

func (h *ShelleyBlockHeader) Hash() string {
	if h.hash == "" {
		h.hash = generateBlockHeaderHash(h.Cbor(), nil)
	}
	return h.hash
}

func (h *ShelleyBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *ShelleyBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *ShelleyBlockHeader) Era() Era {
	return eras[ERA_ID_SHELLEY]
}

type ShelleyTransactionBody struct {
	cbor.DecodeStoreCbor
	hash    string
	Inputs  []ShelleyTransactionInput  `cbor:"0,keyasint,omitempty"`
	Outputs []ShelleyTransactionOutput `cbor:"1,keyasint,omitempty"`
	Fee     uint64                     `cbor:"2,keyasint,omitempty"`
	Ttl     uint64                     `cbor:"3,keyasint,omitempty"`
	// TODO: figure out how to parse properly
	Certificates []cbor.Value `cbor:"4,keyasint,omitempty"`
	// TODO: figure out how to parse this correctly
	// We keep the raw CBOR because it can contain a map with []byte keys, which
	// Go does not allow
	Withdrawals cbor.Value `cbor:"5,keyasint,omitempty"`
	Update      struct {
		cbor.StructAsArray
		ProtocolParamUpdates cbor.Value
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	MetadataHash Blake2b256 `cbor:"7,keyasint,omitempty"`
}

func (b *ShelleyTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCborGeneric(cborData, b)
}

func (b *ShelleyTransactionBody) Hash() string {
	if b.hash == "" {
		b.hash = generateTransactionHash(b.Cbor(), nil)
	}
	return b.hash
}

type ShelleyTransactionInput struct {
	cbor.StructAsArray
	Id    Blake2b256
	Index uint32
}

type ShelleyTransactionOutput struct {
	cbor.StructAsArray
	Address Blake2b256
	Amount  uint64
}

type ShelleyTransactionWitnessSet struct {
	VkeyWitnesses      []interface{} `cbor:"0,keyasint,omitempty"`
	MultisigScripts    []interface{} `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []interface{} `cbor:"2,keyasint,omitempty"`
}

type ShelleyTransaction struct {
	cbor.StructAsArray
	Body       ShelleyTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	Metadata   cbor.Value
}

func NewShelleyBlockFromCbor(data []byte) (*ShelleyBlock, error) {
	var shelleyBlock ShelleyBlock
	if _, err := cbor.Decode(data, &shelleyBlock); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &shelleyBlock, nil
}

func NewShelleyBlockHeaderFromCbor(data []byte) (*ShelleyBlockHeader, error) {
	var shelleyBlockHeader ShelleyBlockHeader
	if _, err := cbor.Decode(data, &shelleyBlockHeader); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &shelleyBlockHeader, nil
}

func NewShelleyTransactionBodyFromCbor(data []byte) (*ShelleyTransactionBody, error) {
	var shelleyTx ShelleyTransactionBody
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionFromCbor(data []byte) (*ShelleyTransaction, error) {
	var shelleyTx ShelleyTransaction
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("decode error: %s", err)
	}
	return &shelleyTx, nil
}
