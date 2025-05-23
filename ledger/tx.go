// Copyright 2025 Blink Labs Software
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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Compatibility aliases
type (
	Transaction       = common.Transaction
	TransactionBody   = common.TransactionBody
	TransactionInput  = common.TransactionInput
	TransactionOutput = common.TransactionOutput
	Utxo              = common.Utxo
)

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
	case TxTypeConway:
		return NewConwayTransactionFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

func NewTransactionBodyFromCbor(
	txType uint,
	data []byte,
) (TransactionBody, error) {
	switch txType {
	case TxTypeByron:
		return nil, errors.New("no body for Byron transactions")
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
	case TxTypeConway:
		return NewConwayTransactionBodyFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

// NewTransactionOutputFromCbor attempts to parse the provided arbitrary CBOR data as a transaction output from
// each of the eras, returning the first one that we can successfully decode
func NewTransactionOutputFromCbor(data []byte) (TransactionOutput, error) {
	if txOut, err := NewByronTransactionOutputFromCbor(data); err == nil {
		return txOut, nil
	}
	if txOut, err := NewShelleyTransactionOutputFromCbor(data); err == nil {
		return txOut, nil
	}
	if txOut, err := NewMaryTransactionOutputFromCbor(data); err == nil {
		return txOut, nil
	}
	if txOut, err := NewAlonzoTransactionOutputFromCbor(data); err == nil {
		return txOut, nil
	}
	if txOut, err := NewBabbageTransactionOutputFromCbor(data); err == nil {
		return txOut, nil
	}
	return nil, errors.New("unknown transaction output type")
}

func DetermineTransactionType(data []byte) (uint, error) {
	if _, err := NewByronTransactionFromCbor(data); err == nil {
		return TxTypeByron, nil
	}
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
	if _, err := NewConwayTransactionFromCbor(data); err == nil {
		return TxTypeConway, nil
	}
	return 0, errors.New("unknown transaction type")
}
