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

type consumedReferenceScriptInput struct {
	input       TransactionInput
	isReference bool
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
	return consumedReferenceScriptSize(tx, utxoState, false)
}

// consumedReferenceScriptSize implements ConsumedReferenceScriptSize. When
// ignoreAbsent is set, a reference input the state reports as absent with
// ErrUtxoNotFound contributes zero instead of an error.
func consumedReferenceScriptSize(
	tx TransactionBody,
	utxoState UtxoState,
	ignoreAbsent bool,
) (uint64, error) {
	if tx == nil {
		return 0, errors.New("transaction body is nil")
	}
	inputs := make(
		map[transactionInputKey]consumedReferenceScriptInput,
		len(tx.Inputs())+len(tx.ReferenceInputs()),
	)
	for _, input := range tx.Inputs() {
		if input != nil {
			inputs[newTransactionInputKey(input)] = consumedReferenceScriptInput{
				input: input,
			}
		}
	}
	for _, input := range tx.ReferenceInputs() {
		if input != nil {
			inputs[newTransactionInputKey(input)] = consumedReferenceScriptInput{
				input:       input,
				isReference: true,
			}
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
	for _, consumedInput := range inputs {
		utxo, err := utxoState.UtxoById(consumedInput.input)
		if err != nil {
			if !consumedInput.isReference ||
				(ignoreAbsent && errors.Is(err, ErrUtxoNotFound)) {
				continue
			}
			return 0, fmt.Errorf(
				"resolve consumed reference-script input %s: %w",
				consumedInput.input,
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
// of one top-level transaction for ConsumedReferenceScriptSizePerBlock. A nil
// function counts only the transaction's own inputs and reference inputs; it
// counts an unresolved regular input, and a reference input reported absent
// with ErrUtxoNotFound, as zero.
type TransactionReferenceScriptSizeFunc func(
	Transaction,
	UtxoState,
) (uint64, error)

// blockUtxoState is the starting UTxO unioned with the outputs of earlier
// transactions. Spent inputs are never removed: the reference rule only adds.
type blockUtxoState struct {
	base     UtxoState
	produced map[transactionInputKey]Utxo
	// unmeasured holds outputs created in the block that the reference rule
	// leaves out of its accumulated UTxO, such as Dijkstra sub-transaction
	// outputs. They resolve without a script so that they contribute zero
	// instead of falling through to a base state that predates them.
	unmeasured map[transactionInputKey]struct{}
}

func (s *blockUtxoState) UtxoById(input TransactionInput) (Utxo, error) {
	key := newTransactionInputKey(input)
	if utxo, ok := s.produced[key]; ok {
		return utxo, nil
	}
	if _, ok := s.unmeasured[key]; ok {
		return Utxo{Id: input}, nil
	}
	if s.base == nil {
		return Utxo{}, errors.New(
			"ledger state is required to resolve consumed reference scripts",
		)
	}
	return s.base.UtxoById(input)
}

func (s *blockUtxoState) apply(tx Transaction) {
	// The reference rule adds only the outputs of the top-level body, or its
	// collateral return when phase-2 invalid; both are keyed by the
	// transaction's own ID. An output already recorded is kept, as the
	// reference's left-biased union keeps the accumulated entry.
	txId := tx.Hash()
	for _, utxo := range tx.Produced() {
		if utxo.Id == nil {
			continue
		}
		key := newTransactionInputKey(utxo.Id)
		if key.id == txId {
			if _, ok := s.produced[key]; !ok {
				s.produced[key] = utxo
			}
		} else {
			s.unmeasured[key] = struct{}{}
		}
	}
}

// ConsumedReferenceScriptSizePerBlock measures transactions in block order.
// When includeProduced is true, each transaction also sees the top-level
// outputs (or, for a phase-2-invalid transaction, the collateral return) of
// preceding transactions, matching the PV11+ Conway block rule; spent inputs
// stay resolvable. Otherwise every transaction is measured against the
// starting state. Outputs of sub-transactions are not added and contribute
// nothing when referenced.
//
// With a nil sizeTx, a regular input that fails to resolve contributes zero
// whatever the error, because through protocol version 10 an input created
// earlier in the block is absent from the starting state. A reference input
// contributes zero only when the lookup error wraps ErrUtxoNotFound; any other
// error, including the not-found error of a UtxoState that does not wrap
// ErrUtxoNotFound, is returned.
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
			return consumedReferenceScriptSize(tx, state, true)
		}
	}
	state := &blockUtxoState{
		base:       utxoState,
		produced:   make(map[transactionInputKey]Utxo),
		unmeasured: make(map[transactionInputKey]struct{}),
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
			return 0, errors.New(
				"block consumed reference-script size overflow",
			)
		}
		total += txSize
		if includeProduced {
			state.apply(tx)
		}
	}
	return total, nil
}
