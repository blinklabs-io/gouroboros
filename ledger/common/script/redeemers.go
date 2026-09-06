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
	if len(view.Available) == 0 {
		return nil
	}
	subBodies := lcommon.SubTransactionBodiesFromTransaction(tx)
	subWitnesses := lcommon.SubTransactionWitnessSetsFromTransaction(tx)
	// The two accessors project the same ordered sub-transaction list, so a
	// length disagreement means the pairing is not trustworthy. Checking the
	// levels we cannot pair would attribute one sub-transaction's redeemers to
	// another's purposes and reject a valid transaction, so the sub-levels are
	// left to the era's own per-level rules instead.
	pairable := len(subBodies) == len(subWitnesses)
	available := view.Available
	subResolved := make([][]lcommon.Utxo, len(subBodies))
	if pairable && len(subBodies) > 0 {
		// A reference script carried by a sub-transaction's own input is part
		// of the aggregated ScriptsProvided in getDijkstraScriptsProvided, so
		// it has to be visible before any level is checked against it.
		augmented := make(
			map[lcommon.ScriptHash]lcommon.Script,
			len(view.Available),
		)
		maps.Copy(augmented, view.Available)
		for idx, body := range subBodies {
			resolved := resolveBodyInputs(body, ls)
			subResolved[idx] = resolved
			for _, utxo := range resolved {
				if utxo.Output == nil {
					continue
				}
				if s := utxo.Output.ScriptRef(); s != nil {
					augmented[s.Hash()] = s
				}
			}
		}
		available = augmented
	}
	if err := validateLevelRedeemers(
		tx,
		tx.Witnesses(),
		view.ResolvedInputs,
		available,
	); err != nil {
		return err
	}
	if !pairable {
		return nil
	}
	for idx, body := range subBodies {
		if err := validateLevelRedeemers(
			body,
			subWitnesses[idx],
			subResolved[idx],
			available,
		); err != nil {
			return err
		}
	}
	return nil
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
	if body == nil || ls == nil {
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
