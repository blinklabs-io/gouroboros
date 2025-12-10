// Copyright 2025 Blink Labs Software
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

// UtxoValidationRuleFunc represents a function that validates a transaction
// against a specific UTXO validation rule.
type UtxoValidationRuleFunc func(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
) error

// VerifyTransaction validates a transaction against the given validation rules.
// It runs all provided UtxoValidationRuleFunc functions and returns the first error encountered,
// or nil if all validations pass.
func VerifyTransaction(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
	validationRules []UtxoValidationRuleFunc,
) error {
	for i, rule := range validationRules {
		if err := rule(tx, slot, ledgerState, protocolParams); err != nil {
			details := map[string]any{
				"rule_index": i,
				"slot":       slot,
			}
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
// and the protocol parameters MinFeeA and MinFeeB
func CalculateMinFee(bodySize int, minFeeA uint, minFeeB uint) uint64 {
	return uint64(minFeeA*uint(bodySize) + minFeeB) //nolint:gosec
}
