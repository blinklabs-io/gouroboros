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
	"cmp"
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

// UsedPlutusVersions returns the language-view version indices of the Plutus
// scripts some script purpose of this transaction requires, in the numbering
// lcommon.PlutusScriptVersion and EncodeLangViews share.
//
// This is the set the script data hash's language views are built from, and it
// has to be the needed scripts rather than the available ones for the same
// reason NativeScriptsToEvaluate does: a script that is merely reachable does
// not constrain the transaction. Counting availability adds a language view the
// producer did not, so the hash comes out different and a canonical transaction
// is rejected -- concretely, spending a UTxO that happens to carry an unrelated
// reference script, or witnessing a script no purpose needs.
//
// cardano-ledger derives the same set from the scripts actually used
// (Cardano.Ledger.Alonzo.Rules.Utxow passes plutusLanguagesUsed to
// mkScriptIntegrity, which maps getLanguageView over exactly that set).
func (v TxScriptView) UsedPlutusVersions() map[uint]struct{} {
	out := make(map[uint]struct{}, len(v.Needed))
	for _, s := range v.Needed {
		if version, ok := lcommon.PlutusScriptVersion(s); ok {
			out[version] = struct{}{}
		}
	}
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

// NeededScriptPurpose pairs a script purpose the transaction defines with the
// redeemer pointer cardano-ledger's rdptr derives for it.
type NeededScriptPurpose struct {
	Key     lcommon.RedeemerKey
	Purpose ScriptPurpose
}

// ScriptPurposes walks every script purpose a transaction level defines, in
// the order and at the redeemer indices cardano-ledger's scriptsNeeded
// assigns, and returns those that resolve to a script credential.
//
// This is the single enumeration of "which script purposes does this
// transaction have". neededScripts, ValidateRequiredRedeemers and Dijkstra's
// per-level witness rules all derive from it, so none of them can grow a
// purpose the others do not know about. Availability and Plutus-versus-native
// filtering are deliberately left to the caller: the needed-script set and the
// required-redeemer set keep different subsets of the same walk, and Dijkstra's
// missing-script-witness check needs the unfiltered list.
//
// Indices are positions in the full collection, not in the filtered result,
// matching zipAsIxItem in cardano-ledger
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/UTxO.hs): a purpose that carries
// no script credential still consumes its index. The collections are walked in
// the canonical order the reference's Set and Map key ordering implies --
// sorted inputs, sorted mint policies, reward accounts by
// SortWithdrawalAddresses and voters by SortVoters -- because the index is what
// the redeemer pointer names.
//
// The purpose list is a transaction-body property. Taking a body rather than a
// Transaction lets a Dijkstra sub-transaction, which is a body plus its own
// witness set and never a standalone Transaction, be walked by this same
// function instead of a parallel copy.
func ScriptPurposes(
	body lcommon.TransactionBody,
	resolvedInputs []lcommon.Utxo,
) []NeededScriptPurpose {
	if body == nil {
		return nil
	}
	ret := make([]NeededScriptPurpose, 0)
	add := func(tag lcommon.RedeemerTag, index uint32, purpose ScriptPurpose) {
		if purpose == nil || purpose.ScriptHash() == (lcommon.ScriptHash{}) {
			return
		}
		ret = append(ret, NeededScriptPurpose{
			Key:     lcommon.RedeemerKey{Tag: tag, Index: index},
			Purpose: purpose,
		})
	}
	byId := make(map[string]lcommon.Utxo, len(resolvedInputs))
	for _, utxo := range resolvedInputs {
		if utxo.Id != nil {
			byId[utxo.Id.String()] = utxo
		}
	}
	for idx, input := range SortInputs(body.Inputs()) {
		utxo, ok := byId[input.String()]
		if !ok || utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address()
		if addr.Type()&lcommon.AddressTypeScriptBit == 0 {
			continue
		}
		add(
			lcommon.RedeemerTagSpend,
			uint32(idx), // #nosec G115 -- input count is bounded
			ScriptPurposeSpending{Input: utxo},
		)
	}
	if mint := body.AssetMint(); mint != nil {
		policies := mint.Policies()
		slices.SortFunc(policies, func(a, b lcommon.Blake2b224) int {
			return bytes.Compare(a.Bytes(), b.Bytes())
		})
		for idx, policy := range policies {
			add(
				lcommon.RedeemerTagMint,
				uint32(idx), // #nosec G115 -- policy count is bounded
				ScriptPurposeMinting{PolicyId: policy},
			)
		}
	}
	for _, purpose := range certifyingPurposes(body.Certificates()) {
		add(lcommon.RedeemerTagCert, purpose.Index, purpose)
	}
	for idx, addr := range SortWithdrawalAddresses(body.Withdrawals()) {
		if addr == nil || addr.Type()&lcommon.AddressTypeScriptBit == 0 {
			continue
		}
		add(
			lcommon.RedeemerTagReward,
			uint32(idx), // #nosec G115 -- withdrawal count is bounded
			ScriptPurposeRewarding{
				StakeCredential: lcommon.Credential{
					CredType:   lcommon.CredentialTypeScriptHash,
					Credential: addr.StakeKeyHash(),
				},
			},
		)
	}
	for idx, voter := range SortVoters(body.VotingProcedures()) {
		if voter == nil || !voterUsesScriptCredential(*voter) {
			continue
		}
		add(
			lcommon.RedeemerTagVoting,
			uint32(idx), // #nosec G115 -- voter count is bounded
			ScriptPurposeVoting{Voter: *voter},
		)
	}
	for idx, proposal := range body.ProposalProcedures() {
		index := uint32(idx) // #nosec G115 -- proposal count is bounded
		add(
			lcommon.RedeemerTagProposing,
			index,
			ScriptPurposeProposing{
				Index:             index,
				ProposalProcedure: proposal,
			},
		)
	}
	if guardingTx, ok := body.(transactionWithGuardingCredentials); ok {
		for idx, guard := range guardingTx.GuardingCredentials() {
			add(
				lcommon.RedeemerTagGuarding,
				uint32(idx), // #nosec G115 -- guard count is bounded
				ScriptPurposeGuarding{Guard: guard},
			)
		}
	}
	return ret
}

// certifyingPurposes assigns each certificate the redeemer index
// cardano-ledger's getAlonzoScriptsNeeded assigns it
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/UTxO.hs, addUniqueTxCertPurpose).
//
// The subtlety is duplicate certificates. Alonzo and Babbage encode
// certificates as a list and so admit two logically identical entries; the
// second one reuses the first's index rather than taking its own, which means
// one redeemer covers both. Conway onward encodes them as a set --
// common.ValidateCertificateSet rejects a duplicate at decode time -- so the
// rule degenerates to plain positional indexing there and this stays a single
// implementation for every era.
//
// Getting this wrong is not symmetric. Assigning the second duplicate its own
// positional index would demand a redeemer cardano-ledger never asks for, and
// a Babbage block carrying duplicate script-witnessed certificates would be
// rejected during sync.
func certifyingPurposes(
	certificates []lcommon.Certificate,
) []ScriptPurposeCertifying {
	ret := make([]ScriptPurposeCertifying, 0, len(certificates))
	seen := make(map[string]uint32, len(certificates))
	for idx, certificate := range certificates {
		if certificate == nil {
			continue
		}
		index := uint32(idx) // #nosec G115 -- certificate count is bounded
		purpose := ScriptPurposeCertifying{
			Index:       index,
			Certificate: certificate,
		}
		if purpose.ScriptHash() == (lcommon.ScriptHash{}) {
			// A certificate with no script credential defines no purpose. It
			// still consumes its index, and cardano-ledger does not record it
			// as "seen", so it cannot lend its index to a later duplicate.
			continue
		}
		key, err := lcommon.CertificateLogicalKey(certificate)
		if err != nil {
			// No identity means no duplicate can be proven. Keeping the
			// positional index is the reference's behavior for a
			// non-duplicate, which is what an unidentifiable certificate is
			// as far as this walk can tell.
			ret = append(ret, purpose)
			continue
		}
		if first, ok := seen[key]; ok {
			purpose.Index = first
		} else {
			seen[key] = index
		}
		ret = append(ret, purpose)
	}
	return ret
}

// SortVoters orders a transaction's voters the way cardano-ledger's Map of
// voting procedures is keyed, so that a voting redeemer index names the same
// voter on both sides.
//
// getConwayScriptsNeeded indexes voters by Map.keys
// (eras/conway/impl/src/Cardano/Ledger/Conway/UTxO.hs), which is the Voter Ord
// instance: constructor order first, then the credential hash.
func SortVoters(votes lcommon.VotingProcedures) []*lcommon.Voter {
	sorted := make([]*lcommon.Voter, 0, len(votes))
	for voter := range votes {
		sorted = append(sorted, voter)
	}
	slices.SortFunc(sorted, func(a, b *lcommon.Voter) int {
		if a == nil {
			return -1
		}
		if b == nil {
			return 1
		}
		if c := cmp.Compare(voterOrder(a), voterOrder(b)); c != 0 {
			return c
		}
		return bytes.Compare(a.Hash[:], b.Hash[:])
	})
	return sorted
}

func voterOrder(voter *lcommon.Voter) int {
	switch voter.Type {
	case lcommon.VoterTypeConstitutionalCommitteeHotScriptHash:
		return 0
	case lcommon.VoterTypeConstitutionalCommitteeHotKeyHash:
		return 1
	case lcommon.VoterTypeDRepScriptHash:
		return 2
	case lcommon.VoterTypeDRepKeyHash:
		return 3
	case lcommon.VoterTypeStakingPoolKeyHash:
		return 4
	default:
		return -1
	}
}

// neededScripts keeps the available script each of the transaction's script
// purposes resolves to.
//
// The walk itself is ScriptPurposes; this only filters it by availability. The
// result is a map, so neither the walk order nor the redeemer indices are
// observable here.
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
	for _, needed := range ScriptPurposes(tx, view.ResolvedInputs) {
		hash := needed.Purpose.ScriptHash()
		if s, ok := view.Available[hash]; ok {
			out[hash] = s
		}
	}
	return out
}
