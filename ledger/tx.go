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
	Metadata() *cbor.Value
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
	Datum() *cbor.LazyValue
	DatumHash() *Blake2b256
	Cbor() []byte
}

func NewTransactionFromCbor(txType uint, data []byte) (Transaction, error) {
	switch txType {
	case TxTypeByron:
		return NewByronTransactionFromCbor(data)
	case TxTypeShelley:
		return NewShelleyTransactionFromCbor(data)
	case TxTypeAllegra:
		return NewAllegraTransactionFromCbor(data)
	case TxTypeMary:
		return NewMaryTransactionFromCbor(data)
	case TxTypeAlonzo:
		return NewAlonzoTransactionFromCbor(data)
	case TxTypeBabbage:
		return NewBabbageTransactionFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

func NewTransactionBodyFromCbor(txType uint, data []byte) (TransactionBody, error) {
	switch txType {
	case TxTypeByron:
		return nil, fmt.Errorf("Byron transactions do not contain a body")
	case TxTypeShelley:
		return NewShelleyTransactionBodyFromCbor(data)
	case TxTypeAllegra:
		return NewAllegraTransactionBodyFromCbor(data)
	case TxTypeMary:
		return NewMaryTransactionBodyFromCbor(data)
	case TxTypeAlonzo:
		return NewAlonzoTransactionBodyFromCbor(data)
	case TxTypeBabbage:
		return NewBabbageTransactionBodyFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

func DetermineTransactionType(data []byte) (uint, error) {
	// TODO: uncomment this once the following issue is resolved:
	// https://github.com/blinklabs-io/gouroboros/issues/206
	/*
		if _, err := NewByronTransactionFromCbor(data); err == nil {
			return TxTypeByron, nil
		}
	*/
	if _, err := NewShelleyTransactionFromCbor(data); err == nil {
		return TxTypeShelley, nil
	}
	if _, err := NewAllegraTransactionFromCbor(data); err == nil {
		return TxTypeAllegra, nil
	}
	if _, err := NewMaryTransactionFromCbor(data); err == nil {
		return TxTypeMary, nil
	}
	if _, err := NewAlonzoTransactionFromCbor(data); err == nil {
		return TxTypeAlonzo, nil
	}
	if _, err := NewBabbageTransactionFromCbor(data); err == nil {
		return TxTypeBabbage, nil
	}
	return 0, fmt.Errorf("unknown transaction type")
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
