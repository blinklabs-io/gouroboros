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
	"github.com/blinklabs-io/gouroboros/cbor"
)

type VkeyWitness struct {
	cbor.StructAsArray
	Vkey      []byte
	Signature []byte
}

type BootstrapWitness struct {
	cbor.StructAsArray
	PublicKey  []byte
	Signature  []byte
	ChainCode  []byte
	Attributes []byte
}

// ValidateCollateralVKeyWitnesses ensures collateral inputs are backed by vkey witnesses (payment key).
// This is a shared helper used across Alonzo, Babbage, and Conway eras.
//
// The phase-2 guard below is deliberately broader than the reference's. This
// helper covers two requirements that cardano-ledger keeps apart:
//
//   - collateral must be key-locked, from UTXO's validateScriptsNotPaidUTxO,
//     which is inside feesOK's redeemer guard; and
//   - each collateral input must have a matching vkey witness, from UTXOW's
//     witsVKeyNeeded (Alonzo adds collateral inputs to that set), which is not
//     redeemer-gated in the reference.
//
// Gating both means a no-redeemer transaction with an unwitnessed key-locked
// collateral input is not rejected here, where the reference would reject it in
// UTXOW. That is a missed rejection, not a false one: it accepts a transaction
// the reference rejects rather than rejecting one the reference accepts, so it
// cannot wedge a node on a canonical block. Splitting the witsVKeyNeeded half
// back out to run ungated is the correct end state; it is not done here because
// this change is scoped to the false-rejection fix.
func ValidateCollateralVKeyWitnesses(
	tx Transaction,
	ls LedgerState,
) error {
	collateral := tx.Collateral()
	if len(collateral) == 0 {
		return nil
	}
	// Collateral exists to pay for phase-2 script execution that fails, so a
	// transaction that runs no phase-2 scripts has nothing for it to cover and
	// is not held to these rules. Declaring collateral it does not need is
	// pointless but harmless, and the chain accepts it: Preview transaction
	// 9ce59ee0dc6abee0 at slot 15148509 carries two vkey witnesses, one native
	// script, no Plutus scripts and no redeemers, and a collateral input at an
	// enterprise-script address. Holding it to the key-locked rule rejected a
	// canonical block (blinklabs-io/dingo#3896).
	//
	// The presence of redeemers is the condition rather than the presence of
	// Plutus scripts in the witness set: a script supplied by a reference input
	// is not in the witness set, and gating on that would skip the check for
	// exactly the transactions that most need it. Every phase-2 execution has a
	// redeemer regardless of where its script came from.
	if !TransactionRunsPhase2Scripts(tx) {
		return nil
	}
	// Collect vkey hashes from witnesses
	w := tx.Witnesses()
	if w == nil || len(w.Vkey()) == 0 {
		return NewValidationError(
			ValidationErrorTypeTransaction,
			"missing vkey witnesses for collateral",
			nil,
			nil,
		)
	}
	hashes := make(map[Blake2b224]struct{}, len(w.Vkey()))
	for _, vw := range w.Vkey() {
		hashes[Blake2b224Hash(vw.Vkey)] = struct{}{}
	}
	// Ensure each collateral input is owned by a provided vkey witness
	for _, input := range collateral {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"UTxO not found for collateral input",
				map[string]any{"input": input.String()},
				err,
			)
		}
		addr := utxo.Output.Address()
		cred := addr.PayloadPayload()
		pk, ok := cred.(AddressPayloadKeyHash)
		if !ok {
			// Collateral should be key-locked; scripts cannot serve
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"collateral input must be key-locked",
				map[string]any{"input": input.String()},
				nil,
			)
		}
		h := pk.Hash
		if _, ok := hashes[h]; !ok {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"missing vkey witness for collateral input",
				map[string]any{
					"input":   input.String(),
					"keyhash": h.String(),
				},
				nil,
			)
		}
	}
	return nil
}

// TransactionRunsPhase2Scripts reports whether the transaction executes any
// Plutus script, by looking for redeemers rather than for scripts in the
// witness set. A reference input can supply the script, in which case the
// witness set holds none but the redeemer is still present.
//
// This is the condition cardano-ledger's feesOK uses to gate the collateral
// rule group:
//
//	unless (null $ tx ^. witsTxL . rdmrsTxWitsL . unRedeemersL) $
//	  validateCollateral pp txBody utxoCollateral
//
// Sub-transaction witness sets count too. A Dijkstra transaction can carry its
// redeemers only in a sub-transaction, and reading just the top level would
// report no phase-2 execution and skip the collateral rules for it — the one
// direction this guard must never fail in.
func TransactionRunsPhase2Scripts(tx Transaction) bool {
	if witnessSetHasRedeemers(tx.Witnesses()) {
		return true
	}
	for _, sub := range SubTransactionWitnessSetsFromTransaction(tx) {
		if witnessSetHasRedeemers(sub) {
			return true
		}
	}
	return false
}

func witnessSetHasRedeemers(w TransactionWitnessSet) bool {
	if w == nil {
		return false
	}
	redeemers := w.Redeemers()
	if redeemers == nil {
		return false
	}
	for range redeemers.Iter() {
		return true
	}
	return false
}
