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
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// ValidateRequiredRedeemers checks that every script-address input whose
// spending script is Plutus and resolvable -- whether provided as an
// explicit witness script or as a CIP-33 reference script -- has a matching
// spend redeemer at its canonical sorted-input position.
//
// This closes a gap left by the existing script/redeemer checks:
// ValidateScriptWitnesses only confirms the script itself is reachable
// (explicit witness or reference script), and UtxoValidatePlutusScripts only
// executes redeemers that already exist in the witness set. A reference
// script satisfies the former without ever requiring a redeemer, so a
// script-locked input backed by a reference script -- or, just as easily, an
// explicit witness script missing its own redeemer while other redeemers are
// present -- previously spent with no script execution and no error at all.
//
// Eras share this single implementation rather than each re-deriving the
// sorted-input walk and its script-availability lookup.
func ValidateRequiredRedeemers(
	tx lcommon.Transaction,
	ls lcommon.LedgerState,
) error {
	if ls == nil || !tx.IsValid() {
		return nil
	}
	view, err := NewTxScriptView(tx, ls)
	if err != nil {
		return err
	}
	if len(view.Available) == 0 {
		return nil
	}
	var redeemers lcommon.TransactionWitnessRedeemers
	if wits := tx.Witnesses(); wits != nil {
		redeemers = wits.Redeemers()
	}
	spendIndexes := make(map[uint32]struct{})
	if redeemers != nil {
		for _, idx := range redeemers.Indexes(lcommon.RedeemerTagSpend) {
			spendIndexes[uint32(idx)] = struct{}{} // #nosec G115 -- input count is bounded
		}
	}
	byId := make(map[string]lcommon.Utxo, len(view.ResolvedInputs))
	for _, utxo := range view.ResolvedInputs {
		if utxo.Id != nil {
			byId[utxo.Id.String()] = utxo
		}
	}
	for idx, input := range SortInputs(tx.Inputs()) {
		utxo, ok := byId[input.String()]
		if !ok || utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address()
		if addr.Type()&lcommon.AddressTypeScriptBit == 0 {
			continue
		}
		scriptHash := lcommon.ScriptHash(addr.PaymentKeyHash())
		s, available := view.Available[scriptHash]
		if !available {
			// A script that is missing entirely is
			// ValidateScriptWitnesses's error to report, not this check's.
			continue
		}
		if _, isPlutus := lcommon.PlutusScriptVersion(s); !isPlutus {
			continue
		}
		index := uint32(idx) // #nosec G115 -- input count is bounded
		if _, ok := spendIndexes[index]; !ok {
			return lcommon.MissingRedeemerForScriptError{
				ScriptHash: scriptHash,
				Tag:        lcommon.RedeemerTagSpend,
				Index:      index,
			}
		}
	}
	return nil
}
