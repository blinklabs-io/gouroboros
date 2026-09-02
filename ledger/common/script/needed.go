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
	"reflect"
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

var errLedgerStateUnavailable = errors.New("ledger state unavailable")

// TxScriptView is a transaction's script picture, resolved once.
//
// Available holds every script the transaction makes reachable: witness-set
// scripts plus scripts carried as a reference script on any resolved input,
// consumed or reference. Needed holds the subset that some script purpose of
// this transaction actually requires.
//
// The distinction matters for the Plutus language restrictions. A script that
// is merely reachable does not constrain the transaction; only a script that
// must run does. Checking availability instead rejects an ordinary transaction
// that happens to spend a UTxO carrying an unrelated reference script.
type TxScriptView struct {
	ResolvedInputs          []lcommon.Utxo
	ResolvedReferenceInputs []lcommon.Utxo
	Available               map[lcommon.ScriptHash]lcommon.Script
	Needed                  map[lcommon.ScriptHash]lcommon.Script

	// allResolvedInputs caches the concatenation NewTxScriptView already had to
	// build, so repeated AllResolvedInputs calls on the hot path do not each
	// allocate a fresh len(inputs)+len(referenceInputs) slice. A view assembled
	// field-by-field rather than through NewTxScriptView leaves this nil, and
	// AllResolvedInputs falls back to building the slice on demand.
	allResolvedInputs []lcommon.Utxo
}

// AllResolvedInputs returns the consumed and reference inputs together.
func (v TxScriptView) AllResolvedInputs() []lcommon.Utxo {
	if v.allResolvedInputs != nil {
		return v.allResolvedInputs
	}
	return ConcatResolvedInputs(v.ResolvedInputs, v.ResolvedReferenceInputs)
}

// ConcatResolvedInputs returns the consumed and reference inputs
// concatenated, consumed first, matching AllResolvedInputs order.
func ConcatResolvedInputs(inputs, refInputs []lcommon.Utxo) []lcommon.Utxo {
	out := make(
		[]lcommon.Utxo,
		0,
		len(inputs)+len(refInputs),
	)
	out = append(out, inputs...)
	out = append(out, refInputs...)
	return out
}

// NeedsAny reports whether any needed script satisfies match.
func (v TxScriptView) NeedsAny(match func(lcommon.Script) bool) bool {
	for _, s := range v.Needed {
		if match(s) {
			return true
		}
	}
	return false
}

// WithAvailableScripts returns a copy of the view whose needed scripts are
// resolved against available. ResolvedInputs and ResolvedReferenceInputs stay
// scoped to tx, so a caller can share script availability across transaction
// levels without importing another level's script purposes.
//
// available is treated as read-only and is not copied.
func (v TxScriptView) WithAvailableScripts(
	tx lcommon.Transaction,
	available map[lcommon.ScriptHash]lcommon.Script,
) TxScriptView {
	v.Available = available
	v.Needed = neededScripts(tx, v)
	return v
}

// NewTxScriptView resolves the transaction's inputs and reference inputs once,
// collects the scripts they and the witness set make available, and determines
// which of those some script purpose requires.
//
// Input resolution failures are returned as InputResolutionError or
// ReferenceInputResolutionError together with a partial view of witness-set
// scripts and non-spending purposes. Callers can enforce restrictions that do
// not depend on UTxO resolution before deferring the resolution failure to
// UtxoValidateBadInputsUtxo.
func NewTxScriptView(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) (TxScriptView, error) {
	var view TxScriptView
	if tx == nil {
		return view, nil
	}
	// The ls == nil arm is spelled out rather than left to ledgerStateIsNil so
	// that nil analysis can see the guard; ledgerStateIsNil additionally covers
	// a typed nil pointer, which it cannot.
	if ls == nil || ledgerStateIsNil(ls) {
		// Witness scripts and non-spending purposes do not require UTxO
		// resolution. Build that partial view before reporting unavailable
		// inputs so callers can still enforce independent language restrictions.
		view.Available = availableScripts(tx, nil)
		view.Needed = neededScripts(tx, view)
		if inputs := tx.Inputs(); len(inputs) > 0 {
			return view, lcommon.InputResolutionError{
				Input: inputs[0],
				Err:   errLedgerStateUnavailable,
			}
		}
		if refInputs := tx.ReferenceInputs(); len(refInputs) > 0 {
			return view, lcommon.ReferenceInputResolutionError{
				Input: refInputs[0],
				Err:   errLedgerStateUnavailable,
			}
		}
		return view, nil
	}
	inputs, refInputs, err := ResolveTxInputs(tx, ls)
	if err != nil {
		// Preserve witness-only script information even when a concrete ledger
		// state cannot resolve one of the transaction's inputs. ResolveTxInputs
		// stops at the first failure and discards what it had, so a failed
		// input would otherwise hide every script carried by an input that did
		// resolve, and a language restriction on such a script would go
		// unchecked before the failure is deferred to UtxoValidateBadInputsUtxo.
		// The view stays partial: ResolvedInputs and ResolvedReferenceInputs
		// are left unset.
		view.Available = availableScripts(tx, resolvableInputs(tx, ls))
		view.Needed = neededScripts(tx, view)
		return view, err
	}
	view.ResolvedInputs = inputs
	view.ResolvedReferenceInputs = refInputs
	view.allResolvedInputs = ConcatResolvedInputs(inputs, refInputs)
	view.Available = availableScripts(tx, view.allResolvedInputs)
	view.Needed = neededScripts(tx, view)
	return view, nil
}

// resolvableInputs resolves whatever consumed and reference inputs the ledger
// state can supply and skips the rest. It is only used on the input-resolution
// failure path, where the failure is already being reported to the caller.
//
// NewTxScriptView establishes that ls is usable before it reaches this path,
// but the path is reachable from a rule invoked with no ledger state at all,
// so ls is checked here rather than assumed.
func resolvableInputs(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) []lcommon.Utxo {
	if ls == nil {
		return nil
	}
	inputs := tx.Inputs()
	refInputs := tx.ReferenceInputs()
	if len(inputs) == 0 && len(refInputs) == 0 {
		return nil
	}
	ret := make([]lcommon.Utxo, 0, len(inputs)+len(refInputs))
	for _, input := range append(append(
		make([]lcommon.TransactionInput, 0, len(inputs)+len(refInputs)),
		inputs...,
	), refInputs...) {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		ret = append(ret, utxo)
	}
	return ret
}

func ledgerStateIsNil(ls lcommon.LedgerState) bool {
	if ls == nil {
		return true
	}
	rv := reflect.ValueOf(ls)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// ResolveTxInputs resolves a transaction's consumed inputs and reference
// inputs against ls, one UtxoById call per input, in tx.Inputs() then
// tx.ReferenceInputs() order. It performs no nil check on tx or ls; a caller
// that must tolerate either being nil checks before calling, as
// NewTxScriptView does.
//
// Input-resolution failures are reported as InputResolutionError or
// ReferenceInputResolutionError, the same errors NewTxScriptView reports, so
// a caller skipping them in favor of UtxoValidateBadInputsUtxo's own report
// can match on either type regardless of which resolver it called.
func ResolveTxInputs(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) (inputs, refInputs []lcommon.Utxo, err error) {
	inputs = make([]lcommon.Utxo, 0, len(tx.Inputs()))
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return nil, nil, lcommon.InputResolutionError{
				Input: input,
				Err:   err,
			}
		}
		inputs = append(inputs, utxo)
	}
	refInputs = make([]lcommon.Utxo, 0, len(tx.ReferenceInputs()))
	for _, input := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return nil, nil, lcommon.ReferenceInputResolutionError{
				Input: input,
				Err:   err,
			}
		}
		refInputs = append(refInputs, utxo)
	}
	return inputs, refInputs, nil
}

// availableScripts collects witness-set scripts and reference scripts from the
// given resolved inputs, keyed by hash.
func availableScripts(
	tx lcommon.Transaction,
	resolved []lcommon.Utxo,
) map[lcommon.ScriptHash]lcommon.Script {
	out := make(map[lcommon.ScriptHash]lcommon.Script)
	addWitnesses := func(witnesses lcommon.TransactionWitnessSet) {
		if witnesses == nil {
			return
		}
		for _, s := range witnesses.NativeScripts() {
			out[s.Hash()] = s
		}
		for _, s := range witnesses.PlutusV1Scripts() {
			out[s.Hash()] = s
		}
		for _, s := range witnesses.PlutusV2Scripts() {
			out[s.Hash()] = s
		}
		for _, s := range witnesses.PlutusV3Scripts() {
			out[s.Hash()] = s
		}
		for _, s := range lcommon.PlutusV4ScriptsFromWitnessSet(witnesses) {
			out[s.Hash()] = s
		}
	}
	addWitnesses(tx.Witnesses())
	for _, witnesses := range lcommon.SubTransactionWitnessSetsFromTransaction(tx) {
		addWitnesses(witnesses)
	}
	for _, utxo := range resolved {
		if utxo.Output == nil {
			continue
		}
		if s := utxo.Output.ScriptRef(); s != nil {
			out[s.Hash()] = s
		}
	}
	return out
}

// PlutusWitnessScripts collects a transaction's witness-set Plutus V1-V4
// scripts, keyed by hash. A nil witness set yields an empty, non-nil map.
//
// This is the single source for the "witness Plutus scripts" half of a
// script-execution availability map. Conway's UtxoValidatePlutusScripts and
// Dijkstra's guarding-redeemer execution both need it, plus per-sub-transaction
// in Dijkstra's case; a fourth hand-rolled copy is exactly the drift #1980 had
// to fix in a third one.
func PlutusWitnessScripts(
	wits lcommon.TransactionWitnessSet,
) map[lcommon.ScriptHash]lcommon.Script {
	out := make(map[lcommon.ScriptHash]lcommon.Script)
	if wits == nil {
		return out
	}
	for _, s := range wits.PlutusV1Scripts() {
		out[s.Hash()] = s
	}
	for _, s := range wits.PlutusV2Scripts() {
		out[s.Hash()] = s
	}
	for _, s := range wits.PlutusV3Scripts() {
		out[s.Hash()] = s
	}
	for _, s := range lcommon.PlutusV4ScriptsFromWitnessSet(wits) {
		out[s.Hash()] = s
	}
	return out
}

// AvailablePlutusScripts collects PlutusWitnessScripts plus any Plutus
// reference script carried by a resolved input, keyed by hash.
//
// Native scripts and non-Plutus reference scripts are deliberately excluded,
// unlike TxScriptView.Available. A caller executing redeemers keys on this
// map by script hash and falls through any non-Plutus type it finds there
// anyway, but a caller distinguishing "no script available" from "script
// available" for an unmatched redeemer -- Dijkstra's native-script guard
// fallback, for instance -- needs a native script absent from this map, not
// present and merely unexecutable by its Plutus type switch.
func AvailablePlutusScripts(
	tx lcommon.Transaction,
	resolved []lcommon.Utxo,
) map[lcommon.ScriptHash]lcommon.Script {
	out := PlutusWitnessScripts(tx.Witnesses())
	for _, utxo := range resolved {
		if utxo.Output == nil {
			continue
		}
		scriptRef := utxo.Output.ScriptRef()
		if scriptRef == nil {
			continue
		}
		if _, ok := lcommon.PlutusScriptVersion(scriptRef); !ok {
			continue
		}
		out[scriptRef.Hash()] = scriptRef
	}
	return out
}

// voterUsesScriptCredential reports whether a voter votes under a script
// credential rather than a key hash.
//
// ScriptPurposeVoting.ScriptHash returns Blake2b224(Voter.Hash) unconditionally,
// with no check on the voter type, so a key-hash voter yields its key hash typed
// as a script hash. Filtering here keeps a key hash from being looked up as a
// script and, on a collision, entered as a script this transaction requires.
// ScriptPurposeCertifying already performs the equivalent check internally, so
// certificates need no filter.
func voterUsesScriptCredential(voter lcommon.Voter) bool {
	switch voter.Type {
	case lcommon.VoterTypeConstitutionalCommitteeHotScriptHash,
		lcommon.VoterTypeDRepScriptHash:
		return true
	default:
		return false
	}
}

// transactionWithGuardingCredentials is implemented by transaction views
// whose body defines Dijkstra guarding script purposes. Keeping this optional
// leaves pre-Dijkstra transactions unchanged while allowing each Dijkstra
// transaction level to contribute only its own guards.
type transactionWithGuardingCredentials interface {
	GuardingCredentials() []lcommon.Credential
}

// neededScripts walks every script purpose the transaction requires and keeps
// the available script each one resolves to.
//
// The result is a map, so the walk order is not observable and no order is
// promised. Concretely: spending inputs and minting policies are walked in
// canonical order, but withdrawals and voting procedures come from Go maps and
// so are walked in randomized order. A future caller that wants redeemer
// indices must impose an order on those two itself; Transaction.Withdrawals and
// Transaction.VotingProcedures are both keyed by pointer, so that means
// deriving a canonical key rather than sorting the keys in place.
func neededScripts(
	tx lcommon.Transaction,
	view TxScriptView,
) map[lcommon.ScriptHash]lcommon.Script {
	out := make(map[lcommon.ScriptHash]lcommon.Script)
	if len(view.Available) == 0 {
		// keep only ever admits a hash present in Available, so with nothing
		// available the walk cannot produce anything. Skipping it keeps a
		// script-free transaction -- the overwhelming majority during a sync --
		// off the input sort, the by-id map build, and one script-hash
		// computation per purpose.
		return out
	}
	byId := make(map[string]lcommon.Utxo, len(view.ResolvedInputs))
	for _, utxo := range view.ResolvedInputs {
		if utxo.Id != nil {
			byId[utxo.Id.String()] = utxo
		}
	}
	keep := func(purpose ScriptPurpose) {
		if purpose == nil {
			return
		}
		hash := purpose.ScriptHash()
		if hash == (lcommon.ScriptHash{}) {
			return
		}
		if s, ok := view.Available[hash]; ok {
			out[hash] = s
		}
	}
	for _, input := range SortInputs(tx.Inputs()) {
		utxo, ok := byId[input.String()]
		if !ok || utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address()
		if addr.Type()&lcommon.AddressTypeScriptBit == 0 {
			continue
		}
		keep(ScriptPurposeSpending{Input: utxo})
	}
	if mint := tx.AssetMint(); mint != nil {
		policies := mint.Policies()
		slices.SortFunc(policies, func(a, b lcommon.Blake2b224) int {
			return bytes.Compare(a.Bytes(), b.Bytes())
		})
		for _, policy := range policies {
			keep(ScriptPurposeMinting{PolicyId: policy})
		}
	}
	for idx, cert := range tx.Certificates() {
		keep(ScriptPurposeCertifying{
			Index: uint32(
				idx,
			), // #nosec G115 -- certificate count is bounded
			Certificate: cert,
		})
	}
	for addr := range tx.Withdrawals() {
		if addr == nil || addr.Type()&lcommon.AddressTypeScriptBit == 0 {
			continue
		}
		keep(ScriptPurposeRewarding{
			StakeCredential: lcommon.Credential{
				CredType:   lcommon.CredentialTypeScriptHash,
				Credential: addr.StakeKeyHash(),
			},
		})
	}
	for voter := range tx.VotingProcedures() {
		if voter == nil || !voterUsesScriptCredential(*voter) {
			continue
		}
		keep(ScriptPurposeVoting{Voter: *voter})
	}
	for idx, proposal := range tx.ProposalProcedures() {
		keep(ScriptPurposeProposing{
			Index: uint32(
				idx,
			), // #nosec G115 -- proposal count is bounded
			ProposalProcedure: proposal,
		})
	}
	if guardingTx, ok := tx.(transactionWithGuardingCredentials); ok {
		for _, guard := range guardingTx.GuardingCredentials() {
			keep(ScriptPurposeGuarding{Guard: guard})
		}
	}
	return out
}
