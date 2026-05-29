// Copyright 2026 Blink Labs Software
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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	dijkstraera "github.com/blinklabs-io/gouroboros/ledger/dijkstra"
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
	case TxTypeDijkstra:
		return NewDijkstraTransactionFromCbor(data)
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
	case TxTypeDijkstra:
		return NewDijkstraTransactionBodyFromCbor(data)
	}
	return nil, fmt.Errorf("unknown transaction type: %d", txType)
}

// NewTransactionOutputFromCbor attempts to parse the provided arbitrary CBOR data as a transaction output from
// each of the eras, returning the first one that we can successfully decode
func NewTransactionOutputFromCbor(data []byte) (TransactionOutput, error) {
	if len(data) == 0 {
		return nil, errors.New("unknown transaction output type")
	}
	// Check CBOR major type: 4 = array (Byron, Shelley, Mary, Alonzo), 5 = map (Babbage+)
	majorType := data[0] >> 5
	switch majorType {
	case 4:
		// Try array-encoded outputs: Byron, Shelley, Mary, Alonzo
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
	case 5:
		// Try map-encoded outputs: Babbage+
		if txOut, err := NewBabbageTransactionOutputFromCbor(data); err == nil {
			return txOut, nil
		}
	}
	return nil, errors.New("unknown transaction output type")
}

// DetermineTransactionType infers an era from standalone transaction CBOR.
// This is for mempool and submission paths where block type IDs are absent;
// chain-sync block decoding uses explicit type IDs instead of this heuristic.
func DetermineTransactionType(data []byte) (uint, error) {
	if len(data) == 0 {
		return 0, errors.New("unknown transaction type")
	}
	// Dijkstra detection is intentionally heuristic. Fields 23, 25, and
	// 26 are currently Dijkstra-only transaction body keys. Field 14 is
	// Dijkstra-only only when it carries credential guards, or when it appears
	// in Dijkstra's 3-component envelope; a 4-component key-hash guard is
	// indistinguishable from Conway required_signers without era context.
	var dijkstraDecoded bool
	var dijkstraCandidate bool
	var dijkstraTxArray []cbor.RawMessage
	tryDijkstra := func() bool {
		if !dijkstraDecoded {
			var err error
			dijkstraCandidate, dijkstraTxArray, _, err = looksLikeDijkstraTransaction(data)
			dijkstraDecoded = true
			if err != nil {
				dijkstraCandidate = false
			}
		}
		if !dijkstraCandidate {
			return false
		}
		_, err := dijkstraera.NewDijkstraTransactionFromCborComponents(
			data,
			dijkstraTxArray,
		)
		return err == nil
	}
	// Fast path: check first byte to distinguish Byron from post-Shelley eras
	firstByte := data[0]
	switch firstByte {
	case 0x82: // Byron signed transactions (TxAux) are 2-element arrays (body, witnesses)
		if _, err := NewByronTransactionFromCbor(data); err == nil {
			return TxTypeByron, nil
		}
	case 0x83: // Shelley, Allegra, and Mary transactions are 3-element arrays
		if tryDijkstra() {
			return TxTypeDijkstra, nil
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
	case 0x84: // Alonzo+ transactions are 4-element arrays
		if tryDijkstra() {
			return TxTypeDijkstra, nil
		}
		// Try most recent eras first for better performance
		if _, err := NewConwayTransactionFromCbor(data); err == nil {
			return TxTypeConway, nil
		}
		if _, err := NewBabbageTransactionFromCbor(data); err == nil {
			return TxTypeBabbage, nil
		}
		if _, err := NewAlonzoTransactionFromCbor(data); err == nil {
			return TxTypeAlonzo, nil
		}
	}

	// Fallback: only accept Dijkstra if the explicit Dijkstra-only body key
	// heuristic matched. This avoids classifying arbitrary malformed CBOR as
	// Dijkstra just because the permissive decoder accepted its shape.
	if tryDijkstra() {
		return TxTypeDijkstra, nil
	}

	return 0, errors.New("unknown transaction type")
}

func decodeTxComponents(
	data []byte,
) ([]cbor.RawMessage, map[uint]cbor.RawMessage, error) {
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(data, &txArray); err != nil {
		return nil, nil, err
	}
	if len(txArray) != 3 && len(txArray) != 4 {
		return nil, nil, fmt.Errorf(
			"invalid transaction component count %d",
			len(txArray),
		)
	}
	var txBody map[uint]cbor.RawMessage
	if _, err := cbor.Decode(txArray[0], &txBody); err != nil {
		return nil, nil, err
	}
	return txArray, txBody, nil
}

func looksLikeDijkstraTransaction(
	data []byte,
) (bool, []cbor.RawMessage, map[uint]cbor.RawMessage, error) {
	if len(data) > dijkstraera.MaxTxSize {
		return false, nil, nil, nil
	}
	txArray, txBody, err := decodeTxComponents(data)
	if err != nil {
		return false, nil, nil, err
	}
	if rawGuards, ok := txBody[14]; ok &&
		looksLikeDijkstraGuards(txArray, rawGuards) {
		return true, txArray, txBody, nil
	}
	// This is a heuristic, not a proof of era. Fields 23, 25, and 26 are
	// currently used only by Dijkstra transactions and protocol parameters;
	// future eras reusing them can create false positives.
	for _, field := range []uint{23, 25, 26} {
		if _, ok := txBody[field]; ok {
			return true, txArray, txBody, nil
		}
	}
	return false, txArray, txBody, nil
}

func looksLikeDijkstraGuards(
	txArray []cbor.RawMessage,
	rawGuards cbor.RawMessage,
) bool {
	var guards dijkstraera.DijkstraGuards
	if _, err := cbor.Decode(rawGuards, &guards); err != nil {
		return false
	}
	if len(guards.Credentials) > 0 {
		return true
	}
	return len(txArray) == 3 && len(guards.KeyHashes) > 0
}
