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

package common

import (
	"errors"
	"fmt"
	"math"
)

type transactionInputKey struct {
	id    Blake2b256
	index uint32
}

func newTransactionInputKey(input TransactionInput) transactionInputKey {
	return transactionInputKey{id: input.Id(), index: input.Index()}
}

// ConsumedReferenceScriptSize returns the total original encoded size of the
// reference scripts at a transaction body's regular and reference inputs. An
// input present in both sets is counted once. Distinct inputs are counted
// separately even when they contain identical scripts.
func ConsumedReferenceScriptSize(
	tx TransactionBody,
	utxoState UtxoState,
) (uint64, error) {
	if tx == nil {
		return 0, errors.New("transaction body is nil")
	}
	inputs := make(
		map[transactionInputKey]TransactionInput,
		len(tx.Inputs())+len(tx.ReferenceInputs()),
	)
	for _, input := range tx.Inputs() {
		if input != nil {
			inputs[newTransactionInputKey(input)] = input
		}
	}
	for _, input := range tx.ReferenceInputs() {
		if input != nil {
			inputs[newTransactionInputKey(input)] = input
		}
	}
	if len(inputs) == 0 {
		return 0, nil
	}
	if utxoState == nil {
		return 0, errors.New(
			"ledger state is required to resolve consumed reference scripts",
		)
	}
	var total uint64
	for _, input := range inputs {
		utxo, err := utxoState.UtxoById(input)
		if err != nil {
			return 0, fmt.Errorf(
				"resolve consumed reference-script input %s: %w",
				input,
				err,
			)
		}
		if utxo.Output == nil || utxo.Output.ScriptRef() == nil {
			continue
		}
		scriptSize := uint64(len(utxo.Output.ScriptRef().RawScriptBytes()))
		if total > math.MaxUint64-scriptSize {
			return 0, errors.New("consumed reference-script size overflow")
		}
		total += scriptSize
	}
	return total, nil
}

// TransactionReferenceScriptSizeFunc measures the consumed reference scripts
// of one top-level transaction. Dijkstra supplies a function that also counts
// each sub-transaction body.
type TransactionReferenceScriptSizeFunc func(
	Transaction,
	UtxoState,
) (uint64, error)

type blockUtxoState struct {
	base     UtxoState
	produced map[transactionInputKey]Utxo
	consumed map[transactionInputKey]struct{}
}

func (s *blockUtxoState) UtxoById(input TransactionInput) (Utxo, error) {
	key := newTransactionInputKey(input)
	if utxo, ok := s.produced[key]; ok {
		return utxo, nil
	}
	if _, ok := s.consumed[key]; ok {
		return Utxo{}, fmt.Errorf("utxo not found: %s", input)
	}
	if s.base == nil {
		return Utxo{}, errors.New(
			"ledger state is required to resolve consumed reference scripts",
		)
	}
	return s.base.UtxoById(input)
}

func (s *blockUtxoState) apply(tx Transaction) {
	for _, input := range tx.Consumed() {
		key := newTransactionInputKey(input)
		delete(s.produced, key)
		s.consumed[key] = struct{}{}
	}
	for _, utxo := range tx.Produced() {
		if utxo.Id == nil {
			continue
		}
		key := newTransactionInputKey(utxo.Id)
		delete(s.consumed, key)
		s.produced[key] = utxo
	}
}

// ConsumedReferenceScriptSizePerBlock measures transactions in block order.
// When includeProduced is true, each transaction sees the UTxO changes made by
// preceding transactions, matching the PV11+ Conway block rule.
func ConsumedReferenceScriptSizePerBlock(
	block Block,
	utxoState UtxoState,
	includeProduced bool,
	sizeTx TransactionReferenceScriptSizeFunc,
) (uint64, error) {
	if block == nil {
		return 0, errors.New("block is nil")
	}
	if sizeTx == nil {
		sizeTx = func(tx Transaction, state UtxoState) (uint64, error) {
			return ConsumedReferenceScriptSize(tx, state)
		}
	}
	state := &blockUtxoState{
		base:     utxoState,
		produced: make(map[transactionInputKey]Utxo),
		consumed: make(map[transactionInputKey]struct{}),
	}
	var total uint64
	for _, tx := range block.Transactions() {
		txState := utxoState
		if includeProduced {
			txState = state
		}
		txSize, err := sizeTx(tx, txState)
		if err != nil {
			return 0, err
		}
		if total > math.MaxUint64-txSize {
			return 0, errors.New("block consumed reference-script size overflow")
		}
		total += txSize
		if includeProduced {
			state.apply(tx)
		}
	}
	return total, nil
}
