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
	ERA_ID_ALLEGRA = 2

	BLOCK_TYPE_ALLEGRA = 3

	BLOCK_HEADER_TYPE_ALLEGRA = 2

	TX_TYPE_ALLEGRA = 2
)

type AllegraBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *AllegraBlockHeader
	TransactionBodies      []AllegraTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]cbor.Value
}

func (b *AllegraBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AllegraBlock) Hash() string {
	return b.Header.Hash()
}

func (b *AllegraBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *AllegraBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *AllegraBlock) Era() Era {
	return eras[ERA_ID_ALLEGRA]
}

func (b *AllegraBlock) Transactions() []Transaction {
	ret := []Transaction{}
	for idx := range b.TransactionBodies {
		tmpTransaction := AllegraTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
		ret = append(ret, &tmpTransaction)
	}
	return ret
}

type AllegraBlockHeader struct {
	ShelleyBlockHeader
}

func (h *AllegraBlockHeader) Era() Era {
	return eras[ERA_ID_ALLEGRA]
}

type AllegraTransactionBody struct {
	ShelleyTransactionBody
	ValidityIntervalStart uint64 `cbor:"8,keyasint,omitempty"`
}

func (b *AllegraTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

type AllegraTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       AllegraTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	TxMetadata cbor.Value
}

func (t AllegraTransaction) Hash() string {
	return t.Body.Hash()
}

func (t AllegraTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t AllegraTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t AllegraTransaction) Metadata() cbor.Value {
	return t.TxMetadata
}

func NewAllegraBlockFromCbor(data []byte) (*AllegraBlock, error) {
	var allegraBlock AllegraBlock
	if _, err := cbor.Decode(data, &allegraBlock); err != nil {
		return nil, fmt.Errorf("Allegra block decode error: %s", err)
	}
	return &allegraBlock, nil
}

func NewAllegraTransactionBodyFromCbor(data []byte) (*AllegraTransactionBody, error) {
	var allegraTx AllegraTransactionBody
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("Allegra transaction body decode error: %s", err)
	}
	return &allegraTx, nil
}

func NewAllegraTransactionFromCbor(data []byte) (*AllegraTransaction, error) {
	var allegraTx AllegraTransaction
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("Allegra transaction decode error: %s", err)
	}
	return &allegraTx, nil
}
