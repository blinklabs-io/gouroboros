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
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"golang.org/x/crypto/blake2b"
)

type Transaction interface {
	TransactionBody
	Metadata() cbor.Value
}

type TransactionBody interface {
	Hash() string
	Cbor() []byte
	Inputs() []TransactionInput
	Outputs() []TransactionOutput
}

type TransactionInput interface {
	Id() Blake2b256
	Index() uint32
}

type TransactionOutput interface {
	Address() Address
	Amount() uint64
	Assets() *MultiAsset[MultiAssetTypeOutput]
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
