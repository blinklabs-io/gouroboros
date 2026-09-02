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

package script

import (
	"bytes"
	"errors"
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// NativeScriptEnv is the transaction-derived input a native script evaluation
// reads: the slot the transaction is validated at, its validity interval, and
// the key hashes it witnesses.
type NativeScriptEnv struct {
	Slot          uint64
	ValidityStart uint64
	ValidityEnd   uint64
	KeyHashes     map[lcommon.Blake2b224]bool
}

// NewNativeScriptEnv derives the evaluation environment from the transaction.
// An absent validity upper bound becomes the maximum slot, so an
// InvalidHereafter-free script is not rejected for lack of a bound.
func NewNativeScriptEnv(
	tx lcommon.Transaction,
	slot uint64,
) NativeScriptEnv {
	env := NativeScriptEnv{
		Slot:        slot,
		KeyHashes:   make(map[lcommon.Blake2b224]bool),
		ValidityEnd: ^uint64(0),
	}
	if tx == nil {
		return env
	}
	env.ValidityStart = tx.ValidityIntervalStart()
	if end, ok := lcommon.TransactionValidityIntervalUpperBound(tx); ok {
		env.ValidityEnd = end
	}
	if witnesses := tx.Witnesses(); witnesses != nil {
		for _, vkw := range witnesses.Vkey() {
			env.KeyHashes[lcommon.Blake2b224Hash(vkw.Vkey)] = true
		}
		for _, bw := range witnesses.Bootstrap() {
			env.KeyHashes[lcommon.Blake2b224Hash(bw.PublicKey)] = true
		}
	}
	return env
}

// NewTxScriptViewSkippingUnresolved builds the transaction's script view and
// reports an unresolvable input as an empty view rather than an error.
//
// UtxoValidateBadInputsUtxo already reports an unresolvable input with the
// right error, so a rule that only reads the resolved view must not become a
// second, competing source of input-resolution failures. Every other error is
// returned unchanged.
func NewTxScriptViewSkippingUnresolved(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) (TxScriptView, error) {
	view, err := NewTxScriptView(tx, ls)
	if err != nil {
		if errors.Is(err, lcommon.ErrInputResolution) ||
			errors.Is(err, lcommon.ErrReferenceInputResolution) {
			return TxScriptView{}, nil
		}
		return TxScriptView{}, err
	}
	return view, nil
}

// NativeScriptsToEvaluate returns the native scripts a phase-1 rule must run
// for this transaction: every native script in the witness set, plus every
// native script some script purpose requires that only a reference script
// supplies -- on a reference input or on the spent input itself, both
// permitted by CIP-33.
//
// A reference script no purpose requires is deliberately excluded. Spending a
// UTxO that happens to carry an unrelated reference script must not drag that
// script into validation; cardano-ledger evaluates the needed subset of the
// scripts the transaction provides
// (Cardano.Ledger.Babbage.Rules.Utxow, validateFailedBabbageScripts).
//
// Witness scripts keep their witness-set order and reference-only scripts
// follow sorted by hash, so a transaction with more than one failing script
// reports the same hash on every run. An era before Babbage has no reference
// scripts and can pass the zero view, which yields the witness scripts alone.
func NativeScriptsToEvaluate(
	tx lcommon.Transaction,
	view TxScriptView,
) []lcommon.NativeScript {
	if tx == nil {
		return nil
	}
	var witnessScripts []lcommon.NativeScript
	if witnesses := tx.Witnesses(); witnesses != nil {
		witnessScripts = witnesses.NativeScripts()
	}
	seen := make(map[lcommon.ScriptHash]struct{}, len(witnessScripts))
	out := make([]lcommon.NativeScript, 0, len(witnessScripts))
	for _, nativeScript := range witnessScripts {
		hash := nativeScript.Hash()
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		out = append(out, nativeScript)
	}
	referenceOnly := make([]lcommon.NativeScript, 0, len(view.Needed))
	for hash, needed := range view.Needed {
		if _, ok := seen[hash]; ok {
			continue
		}
		nativeScript, ok := asNativeScript(needed)
		if !ok {
			continue
		}
		seen[hash] = struct{}{}
		referenceOnly = append(referenceOnly, nativeScript)
	}
	slices.SortFunc(
		referenceOnly,
		func(a, b lcommon.NativeScript) int {
			aHash, bHash := a.Hash(), b.Hash()
			return bytes.Compare(aHash.Bytes(), bHash.Bytes())
		},
	)
	return append(out, referenceOnly...)
}

// asNativeScript unwraps a script that is a native script. A witness-set
// script is held by value while a reference script arrives through the Script
// interface and may be either, so both shapes are accepted.
func asNativeScript(s lcommon.Script) (lcommon.NativeScript, bool) {
	switch script := s.(type) {
	case lcommon.NativeScript:
		return script, true
	case *lcommon.NativeScript:
		if script == nil {
			return lcommon.NativeScript{}, false
		}
		return *script, true
	default:
		return lcommon.NativeScript{}, false
	}
}
