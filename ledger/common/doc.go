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

// Package common provides shared types, interfaces, and utilities for all Cardano eras.
//
// # AI Navigation Guide
//
// This is the foundational package. All era-specific packages (shelley, allegra, etc.)
// depend on types defined here.
//
// # Key Files by Purpose
//
// Interfaces (start here to understand the API):
//   - state.go: LedgerState, UtxoState, CertState, PoolState, etc.
//   - tx.go: Transaction, TransactionBody, TransactionInput, TransactionOutput
//   - pparams.go: ProtocolParameters interface
//
// Core Types:
//   - utxo.go: Utxo type and utilities
//   - address.go: Address parsing and validation
//   - certs.go: Certificate types (stake, pool, governance)
//   - datum.go: Datum and DatumOption types
//   - redeemer.go: Redeemer types for Plutus scripts
//   - witness.go: TransactionWitnessSet
//
// Validation:
//   - rules.go: UtxoValidationRuleFunc signature, shared validation rules
//   - errors.go: Common error types used across eras
//   - verify.go: Signature verification utilities
//   - verify_config.go: ValidationError and configuration
//
// Scripts:
//   - script/: Native and Plutus script handling (subdirectory)
//   - script_data_hash.go: ScriptDataHash computation
//
// # Common Patterns
//
// Era-specific packages embed or extend types from this package. When adding
// new functionality, check if it should be in common (shared) or era-specific.
//
// Validation rules have this signature:
//
//	func UtxoValidate{RuleName}(tx Transaction, slot uint64, ls LedgerState, pp ProtocolParameters) error
//
// # Testing
//
// Use MockLedgerState from internal/test/ledger/ledger.go for testing validation rules.
package common
