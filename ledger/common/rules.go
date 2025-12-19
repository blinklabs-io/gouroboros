// Copyright 2025 Blink Labs Software

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

// UtxoValidationRuleFunc represents a function that validates a transaction
// against a specific UTXO validation rule.
type UtxoValidationRuleFunc func(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
) error

// VerifyTransaction runs the provided validation rules in order and wraps
// the first error encountered into a ValidationError.
func VerifyTransaction(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
	validationRules []UtxoValidationRuleFunc,
) error {
	for i, rule := range validationRules {
		if err := rule(tx, slot, ledgerState, protocolParams); err != nil {
			details := map[string]any{"rule_index": i, "slot": slot}
			if tx != nil {
				details["tx_hash"] = tx.Hash().String()
			}
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"transaction validation failed",
				details,
				err,
			)
		}
	}
	return nil
}

// CalculateMinFee computes the minimum fee for a transaction body given its CBOR-encoded size
// and the protocol parameters MinFeeA and MinFeeB.
func CalculateMinFee(bodySize int, minFeeA uint, minFeeB uint) uint64 {
	return uint64(minFeeA*uint(bodySize) + minFeeB) //nolint:gosec
}

// Common witness-related error types for lightweight UTXOW checks.
type MissingVKeyWitnessesError struct{}

func (MissingVKeyWitnessesError) Error() string { return "missing required vkey witnesses" }

type MissingRequiredVKeyWitnessForSignerError struct{ Signer Blake2b224 }

func (MissingRequiredVKeyWitnessForSignerError) Error() string {
	return "missing required vkey witness for required signer"
}

type MissingRedeemersForScriptDataHashError struct{}

func (MissingRedeemersForScriptDataHashError) Error() string {
	return "missing redeemers for script data hash"
}

type MissingPlutusScriptWitnessesError struct{}

func (MissingPlutusScriptWitnessesError) Error() string {
	return "missing Plutus script witnesses for redeemers"
}

type ExtraneousPlutusScriptWitnessesError struct{}

func (ExtraneousPlutusScriptWitnessesError) Error() string {
	return "extraneous Plutus script witnesses"
}

// ValidateRequiredVKeyWitnesses checks that all required signers have a vkey witness.
func ValidateRequiredVKeyWitnesses(tx Transaction) error {
	required := tx.RequiredSigners()
	if len(required) == 0 {
		return nil
	}
	w := tx.Witnesses()
	if w == nil || len(w.Vkey()) == 0 {
		return MissingVKeyWitnessesError{}
	}
	vkeyHashes := make(map[Blake2b224]struct{})
	for _, vw := range w.Vkey() {
		vkeyHashes[Blake2b224Hash(vw.Vkey)] = struct{}{}
	}
	for _, req := range required {
		if _, ok := vkeyHashes[req]; !ok {
			return MissingRequiredVKeyWitnessForSignerError{Signer: req}
		}
	}
	return nil
}

// ValidateRedeemerAndScriptWitnesses performs lightweight checks between redeemers and Plutus scripts.
func ValidateRedeemerAndScriptWitnesses(tx Transaction) error {
	wits := tx.Witnesses()
	redeemerCount := 0
	if wits != nil {
		if r := wits.Redeemers(); r != nil {
			for range r.Iter() {
				redeemerCount++
			}
		}
	}
	hasPlutus := false
	if wits != nil {
		if len(wits.PlutusV1Scripts()) > 0 || len(wits.PlutusV2Scripts()) > 0 ||
			len(wits.PlutusV3Scripts()) > 0 {
			hasPlutus = true
		}
	}
	if tx.ScriptDataHash() != nil && redeemerCount == 0 {
		return MissingRedeemersForScriptDataHashError{}
	}
	if redeemerCount > 0 && !hasPlutus {
		return MissingPlutusScriptWitnessesError{}
	}
	if redeemerCount == 0 && hasPlutus {
		return ExtraneousPlutusScriptWitnessesError{}
	}
	return nil
}
