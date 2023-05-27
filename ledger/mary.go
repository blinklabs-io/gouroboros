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
	ERA_ID_MARY = 3

	BLOCK_TYPE_MARY = 4

	BLOCK_HEADER_TYPE_MARY = 3

	TX_TYPE_MARY = 3
)

type MaryBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *MaryBlockHeader
	TransactionBodies      []MaryTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]cbor.Value
}

func (b *MaryBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *MaryBlock) Hash() string {
	return b.Header.Hash()
}

func (b *MaryBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *MaryBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *MaryBlock) Era() Era {
	return eras[ERA_ID_MARY]
}

func (b *MaryBlock) Transactions() []TransactionBody {
	ret := []TransactionBody{}
	for _, v := range b.TransactionBodies {
		// Create temp var since we take the address and the loop var gets reused
		tmpVal := v
		ret = append(ret, &tmpVal)
	}
	return ret
}

type MaryBlockHeader struct {
	ShelleyBlockHeader
}

func (h *MaryBlockHeader) Era() Era {
	return eras[ERA_ID_MARY]
}

type MaryTransactionBody struct {
	AllegraTransactionBody
	Outputs []MaryTransactionOutput `cbor:"1,keyasint,omitempty"`
	// TODO: further parsing of this field
	Mint cbor.Value `cbor:"9,keyasint,omitempty"`
}

func (b *MaryTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

type MaryTransaction struct {
	cbor.StructAsArray
	Body       MaryTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	Metadata   cbor.Value
}

type MaryTransactionOutput struct {
	cbor.StructAsArray
	Address Blake2b256
	Amount  MaryTransactionOutputValue
}

type MaryTransactionOutputValue struct {
	cbor.StructAsArray
	Amount uint64
	Assets map[Blake2b224]map[cbor.ByteString]uint64
}

func (v *MaryTransactionOutputValue) UnmarshalCBOR(data []byte) error {
	if _, err := cbor.Decode(data, &(v.Amount)); err == nil {
		return nil
	}
	if err := cbor.DecodeGeneric(data, v); err != nil {
		return err
	}
	return nil
}

func (v *MaryTransactionOutputValue) MarshalCBOR() ([]byte, error) {
	if v.Assets == nil {
		return cbor.Encode(v.Amount)
	} else {
		return cbor.EncodeGeneric(v)
	}
}

func NewMaryBlockFromCbor(data []byte) (*MaryBlock, error) {
	var maryBlock MaryBlock
	if _, err := cbor.Decode(data, &maryBlock); err != nil {
		return nil, fmt.Errorf("Mary block decode error: %s", err)
	}
	return &maryBlock, nil
}

func NewMaryTransactionBodyFromCbor(data []byte) (*MaryTransactionBody, error) {
	var maryTx MaryTransactionBody
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("Mary transaction body decode error: %s", err)
	}
	return &maryTx, nil
}

func NewMaryTransactionFromCbor(data []byte) (*MaryTransaction, error) {
	var maryTx MaryTransaction
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("Mary transaction decode error: %s", err)
	}
	return &maryTx, nil
}
