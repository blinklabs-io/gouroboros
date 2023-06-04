// Copyright 2023 Blink Labs, LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ledger

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
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
	return b.UnmarshalCbor(cborData, b)
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
		// Create temp var since we take the address and the loop var gets reused
		tmpVal := v
		ret = append(ret, &tmpVal)
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
	return h.UnmarshalCbor(cborData, h)
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
	hash      string
	TxInputs  []ShelleyTransactionInput  `cbor:"0,keyasint,omitempty"`
	TxOutputs []ShelleyTransactionOutput `cbor:"1,keyasint,omitempty"`
	Fee       uint64                     `cbor:"2,keyasint,omitempty"`
	Ttl       uint64                     `cbor:"3,keyasint,omitempty"`
	// TODO: figure out how to parse properly
	Certificates cbor.RawMessage `cbor:"4,keyasint,omitempty"`
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
	return b.UnmarshalCbor(cborData, b)
}

func (b *ShelleyTransactionBody) Hash() string {
	if b.hash == "" {
		b.hash = generateTransactionHash(b.Cbor(), nil)
	}
	return b.hash
}

func (b *ShelleyTransactionBody) Inputs() []TransactionInput {
	ret := []TransactionInput{}
	for _, input := range b.TxInputs {
		ret = append(ret, input)
	}
	return ret
}

func (b *ShelleyTransactionBody) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, output)
	}
	return ret
}

type ShelleyTransactionInput struct {
	cbor.StructAsArray
	TxId        Blake2b256
	OutputIndex uint32
}

func (i ShelleyTransactionInput) Id() Blake2b256 {
	return i.TxId
}

func (i ShelleyTransactionInput) Index() uint32 {
	return i.OutputIndex
}

type ShelleyTransactionOutput struct {
	cbor.StructAsArray
	OutputAddress []byte
	OutputAmount  uint64
}

func (o ShelleyTransactionOutput) Address() []byte {
	return o.OutputAddress
}

func (o ShelleyTransactionOutput) Amount() uint64 {
	return o.OutputAmount
}

func (o ShelleyTransactionOutput) Assets() *MultiAsset[uint64] {
	return nil
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
		return nil, fmt.Errorf("Shelley block decode error: %s", err)
	}
	return &shelleyBlock, nil
}

func NewShelleyBlockHeaderFromCbor(data []byte) (*ShelleyBlockHeader, error) {
	var shelleyBlockHeader ShelleyBlockHeader
	if _, err := cbor.Decode(data, &shelleyBlockHeader); err != nil {
		return nil, fmt.Errorf("Shelley block header decode error: %s", err)
	}
	return &shelleyBlockHeader, nil
}

func NewShelleyTransactionBodyFromCbor(data []byte) (*ShelleyTransactionBody, error) {
	var shelleyTx ShelleyTransactionBody
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction body decode error: %s", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionFromCbor(data []byte) (*ShelleyTransaction, error) {
	var shelleyTx ShelleyTransaction
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction decode error: %s", err)
	}
	return &shelleyTx, nil
}
