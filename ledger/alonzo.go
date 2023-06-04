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
	return b.UnmarshalCbor(cborData, b)
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
		// Create temp var since we take the address and the loop var gets reused
		tmpVal := v
		ret = append(ret, &tmpVal)
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
	TxOutputs       []AlonzoTransactionOutput `cbor:"1,keyasint,omitempty"`
	ScriptDataHash  Blake2b256                `cbor:"11,keyasint,omitempty"`
	Collateral      []ShelleyTransactionInput `cbor:"13,keyasint,omitempty"`
	RequiredSigners []Blake2b224              `cbor:"14,keyasint,omitempty"`
	NetworkId       uint8                     `cbor:"15,keyasint,omitempty"`
}

func (b *AlonzoTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AlonzoTransactionBody) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, output)
	}
	return ret
}

type AlonzoTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress []byte
	OutputAmount  MaryTransactionOutputValue
	DatumHash     Blake2b256
}

func (o *AlonzoTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as Mary output first
	var tmpOutput MaryTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp Shelley output to Alonzo format
		o.OutputAddress = tmpOutput.OutputAddress
		o.OutputAmount = tmpOutput.OutputAmount
	} else {
		return o.UnmarshalCbor(cborData, o)
	}
	return nil
}

func (o AlonzoTransactionOutput) Address() []byte {
	return o.OutputAddress
}

func (o AlonzoTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o AlonzoTransactionOutput) Assets() *MultiAsset[uint64] {
	return o.OutputAmount.Assets
}

type AlonzoTransactionWitnessSet struct {
	ShelleyTransactionWitnessSet
	PlutusScripts []cbor.RawMessage `cbor:"3,keyasint,omitempty"`
	PlutusData    []cbor.RawMessage `cbor:"4,keyasint,omitempty"`
	Redeemers     []cbor.RawMessage `cbor:"5,keyasint,omitempty"`
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
		return nil, fmt.Errorf("Alonzo block decode error: %s", err)
	}
	return &alonzoBlock, nil
}

func NewAlonzoTransactionBodyFromCbor(data []byte) (*AlonzoTransactionBody, error) {
	var alonzoTx AlonzoTransactionBody
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction body decode error: %s", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionFromCbor(data []byte) (*AlonzoTransaction, error) {
	var alonzoTx AlonzoTransaction
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction decode error: %s", err)
	}
	return &alonzoTx, nil
}
