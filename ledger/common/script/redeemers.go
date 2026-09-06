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
	"maps"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// ValidateRequiredRedeemers checks that every script purpose of a transaction
// whose script is Plutus and resolvable -- whether provided as an explicit
// witness script or as a CIP-33 reference script -- has a matching redeemer at
// the purpose's canonical index.
//
// This closes a gap left by the existing script/redeemer checks:
// ValidateScriptWitnesses only confirms the script itself is reachable
// (explicit witness or reference script), and UtxoValidatePlutusScripts only
// executes redeemers that already exist in the witness set. A reference script
// satisfies the former without ever requiring a redeemer, so a script-locked
// input backed by a reference script -- or, just as easily, an explicit witness
// script missing its own redeemer while other redeemers are present --
// previously spent with no script execution and no error at all.
//
// Every script purpose is covered, not only spending. cardano-ledger's
// hasExactSetOfRedeemers (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Rules/Utxow.hs)
// derives its redeemer pointers from the whole of scriptsNeeded, which
// getConwayScriptsNeeded and getDijkstraScriptsNeeded build from spending,
// withdrawing, certifying, minting, voting, proposing and guarding purposes
// alike (eras/conway/impl/src/Cardano/Ledger/Conway/UTxO.hs and
// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/UTxO.hs). Restricting the
// check to spending let a Plutus minting policy, certificate script,
// withdrawal script, voting script or proposal policy be witnessed and never
// executed.
//
// The purposes come from ScriptPurposes, the same walk neededScripts uses, so
// this check cannot come to disagree with the needed-script set about which
// purposes a transaction has.
//
// Dijkstra sub-transactions are checked as their own levels. cardano-ledger
// runs hasExactSetOfRedeemers per transaction level -- once in DijkstraUTXOW
// over the top-level scriptsNeeded and once per sub-transaction in
// DijkstraSUBUTXOW (eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxow.hs
// and .../SubUtxow.hs) -- because a redeemer pointer indexes its own level's
// collections. Script availability is aggregated across levels, matching
// getDijkstraScriptsProvided.
//
// This check applies regardless of the transaction's IsValid flag.
// AlonzoUTXOW's hasExactSetOfRedeemers (the source of AlonzoUtxowPredFailure
// MissingRedeemers) is a witnessing-completeness check, not a phase-2
// execution check: IsValid is read only by UTXOS to decide how an already
// phase-1-valid transaction is applied to the ledger state, never by UTXOW to
// decide which witnesses are required. A transaction the submitter marked
// invalid still must carry every redeemer its script purposes require.
func ValidateRequiredRedeemers(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) error {
	if tx == nil || ls == nil {
		return nil
	}
	// NewTxScriptView resolves every input, so an unresolvable one surfaces
	// here as InputResolutionError/ReferenceInputResolutionError even though
	// shelley.UtxoValidateBadInputsUtxo and
	// common.ValidateRedeemerAndScriptWitnesses -- both registered ahead of
	// this rule in every era's rule list -- already report the same failure
	// more specifically. Not reachable in production today because of that
	// ordering, but reachable by any caller that runs this check first.
	// TODO(#2162): switch to NewTxScriptViewSkippingUnresolved once it
	// lands, so this rule -- which only ever reads the resolved view --
	// stops being a second, competing source of input-resolution failures.
	view, err := NewTxScriptView(tx, ls)
	if err != nil {
		return err
	}
	subLevels, available := subTransactionLevels(tx, ls, view.Available)
	// Checked only once the sub-transaction reference scripts are folded in.
	// availableScripts resolves reference scripts from the top-level inputs
	// alone, so a sub-transaction whose script arrives purely as a CIP-33
	// reference script on its own input leaves view.Available empty; taking
	// the shortcut on that would skip the level that needs the check. For a
	// transaction with no sub-transactions -- every era before Dijkstra, and
	// the overwhelming majority during a sync -- available is still
	// view.Available and this is the same early return as before.
	if len(available) == 0 {
		return nil
	}
	if err := validateLevelRedeemers(
		tx,
		tx.Witnesses(),
		view.ResolvedInputs,
		available,
	); err != nil {
		return err
	}
	for _, level := range subLevels {
		if err := validateLevelRedeemers(
			level.body,
			level.witnesses,
			level.resolved,
			available,
		); err != nil {
			return err
		}
	}
	return nil
}

// subTransactionLevel is one Dijkstra sub-transaction: a body, the witness set
// holding its own redeemers, and whatever of its inputs resolved.
type subTransactionLevel struct {
	body      lcommon.TransactionBody
	witnesses lcommon.TransactionWitnessSet
	resolved  []lcommon.Utxo
}

// subTransactionLevels pairs each sub-transaction body with its witness set and
// returns them alongside the script availability the whole transaction has,
// which is topLevel plus any reference script a sub-transaction's own input
// carries. cardano-ledger aggregates availability the same way, in
// getDijkstraScriptsProvided
// (eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/UTxO.hs).
//
// A length disagreement between the two accessors means the pairing is not
// trustworthy. Checking levels that cannot be paired would attribute one
// sub-transaction's redeemers to another's purposes and reject a valid
// transaction, so no level is returned and the sub-transactions are left to the
// era's own per-level rules. For every era before Dijkstra both accessors
// return nothing and this is the identity.
func subTransactionLevels(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
	topLevel map[lcommon.ScriptHash]lcommon.Script,
) ([]subTransactionLevel, map[lcommon.ScriptHash]lcommon.Script) {
	bodies := lcommon.SubTransactionBodiesFromTransaction(tx)
	witnessSets := lcommon.SubTransactionWitnessSetsFromTransaction(tx)
	if len(bodies) == 0 || len(bodies) != len(witnessSets) {
		return nil, topLevel
	}
	available := make(
		map[lcommon.ScriptHash]lcommon.Script,
		len(topLevel),
	)
	maps.Copy(available, topLevel)
	levels := make([]subTransactionLevel, 0, len(bodies))
	for idx, witnesses := range witnessSets {
		resolved := resolveBodyInputs(bodies[idx], ls)
		for _, utxo := range resolved {
			if utxo.Output == nil {
				continue
			}
			if s := utxo.Output.ScriptRef(); s != nil {
				available[s.Hash()] = s
			}
		}
		levels = append(levels, subTransactionLevel{
			body:      bodies[idx],
			witnesses: witnesses,
			resolved:  resolved,
		})
	}
	return levels, available
}

// validateLevelRedeemers requires a redeemer for every Plutus script purpose
// of one transaction level, indexed within that level.
func validateLevelRedeemers(
	body lcommon.TransactionBody,
	witnesses lcommon.TransactionWitnessSet,
	resolvedInputs []lcommon.Utxo,
	available map[lcommon.ScriptHash]lcommon.Script,
) error {
	purposes := ScriptPurposes(body, resolvedInputs)
	if len(purposes) == 0 {
		return nil
	}
	provided := make(map[lcommon.RedeemerKey]struct{})
	if witnesses != nil {
		if redeemers := witnesses.Redeemers(); redeemers != nil {
			for key := range redeemers.Iter() {
				provided[key] = struct{}{}
			}
		}
	}
	for _, needed := range purposes {
		scriptHash := needed.Purpose.ScriptHash()
		s, ok := available[scriptHash]
		if !ok {
			// A script that is missing entirely is
			// ValidateScriptWitnesses's error to report, not this check's.
			continue
		}
		if _, isPlutus := lcommon.PlutusScriptVersion(s); !isPlutus {
			// Native scripts carry no redeemer. cardano-ledger drops them
			// from redeemersNeeded with the same test
			// (not (isNativeScript script) in hasExactSetOfRedeemers).
			continue
		}
		if _, ok := provided[needed.Key]; !ok {
			return lcommon.MissingRedeemerForScriptError{
				ScriptHash:  scriptHash,
				Tag:         needed.Key.Tag,
				Index:       needed.Key.Index,
				RedeemerKey: needed.Key,
			}
		}
	}
	return nil
}

// resolveBodyInputs resolves whatever of a transaction body's consumed and
// reference inputs the ledger state can supply, skipping the rest.
//
// Skipping is deliberate. An unresolvable input is
// UtxoValidateBadInputsUtxo's failure to report, and this rule is registered
// after it in every era's rule list; surfacing a resolution error here would
// make it a second, competing source of the same failure.
func resolveBodyInputs(
	body lcommon.TransactionBody,
	ls lcommon.LedgerState,
) []lcommon.Utxo {
	// ledgerStateIsNil additionally covers a LedgerState interface holding a
	// nil pointer, which ls == nil does not. NewTxScriptView guards the same
	// case, but it reports the transaction's first input as unresolvable and a
	// transaction whose only inputs live in a sub-transaction has no top-level
	// input to report, so it returns no error and this walk still runs.
	if body == nil || ls == nil || ledgerStateIsNil(ls) {
		return nil
	}
	inputs := body.Inputs()
	refInputs := body.ReferenceInputs()
	if len(inputs) == 0 && len(refInputs) == 0 {
		return nil
	}
	ret := make([]lcommon.Utxo, 0, len(inputs)+len(refInputs))
	for _, input := range inputs {
		if utxo, err := ls.UtxoById(input); err == nil {
			ret = append(ret, utxo)
		}
	}
	for _, input := range refInputs {
		if utxo, err := ls.UtxoById(input); err == nil {
			ret = append(ret, utxo)
		}
	}
	return ret
}
