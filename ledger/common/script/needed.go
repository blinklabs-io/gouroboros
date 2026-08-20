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
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

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
}

// AllResolvedInputs returns the consumed and reference inputs together.
func (v TxScriptView) AllResolvedInputs() []lcommon.Utxo {
	out := make(
		[]lcommon.Utxo,
		0,
		len(v.ResolvedInputs)+len(v.ResolvedReferenceInputs),
	)
	out = append(out, v.ResolvedInputs...)
	out = append(out, v.ResolvedReferenceInputs...)
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

// NewTxScriptView resolves the transaction's inputs and reference inputs once,
// collects the scripts they and the witness set make available, and determines
// which of those some script purpose requires.
//
// Input resolution failures are returned as InputResolutionError or
// ReferenceInputResolutionError. Callers enforcing a language restriction
// generally want to skip those rather than report them, because
// UtxoValidateBadInputsUtxo reports an unresolvable input with the right error.
func NewTxScriptView(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) (TxScriptView, error) {
	var view TxScriptView
	if tx == nil || ls == nil {
		return view, nil
	}
	view.ResolvedInputs = make([]lcommon.Utxo, 0, len(tx.Inputs()))
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return view, lcommon.InputResolutionError{Input: input, Err: err}
		}
		view.ResolvedInputs = append(view.ResolvedInputs, utxo)
	}
	view.ResolvedReferenceInputs = make(
		[]lcommon.Utxo,
		0,
		len(tx.ReferenceInputs()),
	)
	for _, input := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return view, lcommon.ReferenceInputResolutionError{
				Input: input,
				Err:   err,
			}
		}
		view.ResolvedReferenceInputs = append(
			view.ResolvedReferenceInputs,
			utxo,
		)
	}
	view.Available = availableScripts(tx, view.AllResolvedInputs())
	view.Needed = neededScripts(tx, view)
	return view, nil
}

// availableScripts collects witness-set scripts and reference scripts from the
// given resolved inputs, keyed by hash.
func availableScripts(
	tx lcommon.Transaction,
	resolved []lcommon.Utxo,
) map[lcommon.ScriptHash]lcommon.Script {
	out := make(map[lcommon.ScriptHash]lcommon.Script)
	if witnesses := tx.Witnesses(); witnesses != nil {
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

// neededScripts walks every script purpose the transaction requires and keeps
// the available script each one resolves to.
//
// The walk order matches the canonical redeemer order per tag, so a caller that
// also cares about redeemer indices sees the same sequence: spending inputs
// (sorted), minting policies (sorted), certificates, withdrawals (sorted),
// voters (sorted), proposal procedures. Eras without voting or proposing simply
// produce none of those.
func neededScripts(
	tx lcommon.Transaction,
	view TxScriptView,
) map[lcommon.ScriptHash]lcommon.Script {
	out := make(map[lcommon.ScriptHash]lcommon.Script)
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
			Index:       uint32(idx), // #nosec G115 -- certificate count is bounded
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
		if voter == nil {
			continue
		}
		keep(ScriptPurposeVoting{Voter: *voter})
	}
	for idx, proposal := range tx.ProposalProcedures() {
		keep(ScriptPurposeProposing{
			Index:             uint32(idx), // #nosec G115 -- proposal count is bounded
			ProposalProcedure: proposal,
		})
	}
	return out
}
