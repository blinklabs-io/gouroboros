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

import "fmt"

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

func (e MissingRequiredVKeyWitnessForSignerError) Error() string {
	return fmt.Sprintf(
		"missing required vkey witness for required signer %x",
		e.Signer,
	)
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

type MissingScriptWitnessesError struct {
	ScriptHash ScriptHash
}

func (e MissingScriptWitnessesError) Error() string {
	return fmt.Sprintf(
		"missing script witness for script hash %x",
		e.ScriptHash[:],
	)
}

type ExtraneousScriptWitnessesError struct {
	ScriptHash ScriptHash
}

func (e ExtraneousScriptWitnessesError) Error() string {
	return fmt.Sprintf(
		"extraneous script witness for script hash %x",
		e.ScriptHash[:],
	)
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

// ValidateScriptWitnesses checks that script witnesses are provided for all script address inputs
// and that there are no extraneous script witnesses.
func ValidateScriptWitnesses(tx Transaction, ls LedgerState) error {
	if ls == nil {
		return nil
	}

	// If IsValid=false, the transaction is expected to fail phase-2 validation.
	// Phase-1 validation should still pass even without script witnesses.
	if !tx.IsValid() {
		return nil
	}

	wits := tx.Witnesses()

	// Collect all script hashes required by script address inputs
	requiredScriptHashes := make(map[ScriptHash]bool)
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// If we can't resolve the UTxO, we can't validate script witnesses
			// This should be caught by BadInputsUtxo validation
			continue
		}
		if utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address()

		// Check if this is a script address (payment part is script)
		if (addr.Type() & AddressTypeScriptBit) != 0 {
			paymentScriptHash := addr.PaymentKeyHash()
			// This is a script payment address that needs a script witness.
			// The script can be provided via the witness set or via ScriptRef
			// from any input (including the spent UTxO itself or reference inputs).
			requiredScriptHashes[ScriptHash(paymentScriptHash)] = true
		}
		// Note: Staking script validation is handled separately in delegation rules
	}

	// Collect explicit provided script witnesses (those carried in the tx)
	explicitProvided := make(map[ScriptHash]bool)
	if wits != nil {
		// Native scripts
		for _, script := range wits.NativeScripts() {
			explicitProvided[script.Hash()] = true
		}

		// Plutus scripts
		for _, script := range wits.PlutusV1Scripts() {
			explicitProvided[script.Hash()] = true
		}
		for _, script := range wits.PlutusV2Scripts() {
			explicitProvided[script.Hash()] = true
		}
		for _, script := range wits.PlutusV3Scripts() {
			explicitProvided[script.Hash()] = true
		}
	}

	// Collect reference-provided scripts from both reference inputs AND regular inputs
	// According to CIP-33, scripts can be provided via ScriptRef from any resolved UTxO
	referenceProvided := make(map[ScriptHash]bool)

	// From reference inputs
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			// If we can't resolve the reference UTxO deterministically, fail
			return ReferenceInputResolutionError{Input: refInput, Err: err}
		}
		if utxo.Output == nil {
			continue
		}
		if script := utxo.Output.ScriptRef(); script != nil {
			referenceProvided[script.Hash()] = true
		}
	}

	// From regular (spent) inputs - their ScriptRef can also satisfy script requirements
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// If we can't resolve the UTxO, skip - BadInputsUtxo will catch this
			continue
		}
		if utxo.Output == nil {
			continue
		}
		if script := utxo.Output.ScriptRef(); script != nil {
			referenceProvided[script.Hash()] = true
		}
	}

	// Collect script hashes required by minting policies
	if mint := tx.AssetMint(); mint != nil {
		for policy := range mint.data {
			requiredScriptHashes[ScriptHash(policy)] = true
		}
	}

	// Track scripts that are optional (allowed but not required) for registration certificates.
	// Registration doesn't require authorization, but if the script is provided, it's valid.
	optionalScriptHashes := make(map[ScriptHash]bool)

	// Collect script hashes required by certificates
	// Note: Registration certificates with script credentials do NOT require the script witness
	// (registration doesn't need authorization), but providing the script is allowed.
	// Deregistration, delegation, and withdrawal DO require both the script and a redeemer.
	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *StakeRegistrationCertificate:
			// Registration: script is optional (allowed but not required)
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				optionalScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *StakeDeregistrationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *StakeDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *RegistrationCertificate:
			// Registration: script is optional (allowed but not required)
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				optionalScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *DeregistrationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *VoteDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *StakeVoteDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *StakeRegistrationDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *VoteRegistrationDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *StakeVoteRegistrationDelegationCertificate:
			if c.StakeCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.StakeCredential.Credential)] = true
			}
		case *AuthCommitteeHotCertificate:
			if c.ColdCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.ColdCredential.Credential)] = true
			}
		case *ResignCommitteeColdCertificate:
			if c.ColdCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.ColdCredential.Credential)] = true
			}
		case *RegistrationDrepCertificate:
			if c.DrepCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.DrepCredential.Credential)] = true
			}
		case *DeregistrationDrepCertificate:
			if c.DrepCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.DrepCredential.Credential)] = true
			}
		case *UpdateDrepCertificate:
			if c.DrepCredential.CredType == CredentialTypeScriptHash {
				requiredScriptHashes[ScriptHash(c.DrepCredential.Credential)] = true
			}
		case *PoolRegistrationCertificate, *PoolRetirementCertificate:
			// These certificates use key-only credentials
		default:
			// Other certificate types do not have script credentials
		}
	}

	// Collect script hashes required by withdrawals
	for addr := range tx.Withdrawals() {
		// For stake addresses, check if stake credential is script (LSB of type indicates script)
		if (addr.Type() & AddressTypeScriptBit) != 0 {
			stakeScriptHash := addr.StakeKeyHash()
			requiredScriptHashes[ScriptHash(stakeScriptHash)] = true
		}
	}

	// Collect script hashes required by voting procedures (script-type voters)
	for voter := range tx.VotingProcedures() {
		if voter == nil {
			continue
		}
		// Check for script-type voters: CC script (1) or DRep script (3)
		if voter.Type == VoterTypeConstitutionalCommitteeHotScriptHash ||
			voter.Type == VoterTypeDRepScriptHash {
			requiredScriptHashes[ScriptHash(NewBlake2b224(voter.Hash[:]))] = true
		}
	}

	// Collect script hashes required by proposal procedures (governance policy scripts)
	for _, proposal := range tx.ProposalProcedures() {
		if proposal == nil {
			continue
		}
		govAction := proposal.GovAction()
		if govAction == nil {
			continue
		}
		// Check if governance action has a policy script
		if actionWithPolicy, ok := govAction.(GovActionWithPolicy); ok {
			policyHash := actionWithPolicy.GetPolicyHash()
			if len(policyHash) > 0 {
				requiredScriptHashes[ScriptHash(policyHash)] = true
			}
		}
	}

	// Check for missing script witnesses. A required script is satisfied if
	// it appears in either explicit witnesses or reference scripts.
	for required := range requiredScriptHashes {
		if !explicitProvided[required] && !referenceProvided[required] {
			return MissingScriptWitnessesError{ScriptHash: required}
		}
	}

	// Check for extraneous explicit script witnesses. Reference scripts are
	// not considered explicit witnesses and therefore are not extraneous.
	// Scripts are allowed if they are either required OR optional (e.g., registration scripts).
	for provided := range explicitProvided {
		if !requiredScriptHashes[provided] && !optionalScriptHashes[provided] {
			return ExtraneousScriptWitnessesError{ScriptHash: provided}
		}
	}

	return nil
}

// ValidateRedeemerAndScriptWitnesses performs lightweight checks between redeemers and Plutus scripts.
func ValidateRedeemerAndScriptWitnesses(tx Transaction, ls LedgerState) error {
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

	// If there are inputs (reference or regular) and a LedgerState is provided,
	// resolve them to detect Plutus reference scripts. Per CIP-33, ScriptRef can
	// be provided via both reference inputs AND regular (spent) inputs.
	hasPlutusReference := false
	if ls != nil {
		// Check reference inputs
		for _, refInput := range tx.ReferenceInputs() {
			utxo, err := ls.UtxoById(refInput)
			if err != nil {
				return ReferenceInputResolutionError{Input: refInput, Err: err}
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script == nil {
				continue
			}
			switch script.(type) {
			case *PlutusV1Script, *PlutusV2Script, *PlutusV3Script:
				hasPlutusReference = true
			case PlutusV1Script, PlutusV2Script, PlutusV3Script:
				// Also handle non-pointer types
				hasPlutusReference = true
			}
			if hasPlutusReference {
				break
			}
		}
		// Check regular inputs if not found in reference inputs
		if !hasPlutusReference {
			for _, input := range tx.Inputs() {
				utxo, err := ls.UtxoById(input)
				if err != nil {
					// Skip errors - BadInputsUtxo will catch this
					continue
				}
				if utxo.Output == nil {
					continue
				}
				script := utxo.Output.ScriptRef()
				if script == nil {
					continue
				}
				switch script.(type) {
				case *PlutusV1Script, *PlutusV2Script, *PlutusV3Script:
					hasPlutusReference = true
				case PlutusV1Script, PlutusV2Script, PlutusV3Script:
					// Also handle non-pointer types
					hasPlutusReference = true
				}
				if hasPlutusReference {
					break
				}
			}
		}
	}

	// Check witness PlutusData (datums)
	hasWitnessPlutusData := false
	if wits != nil {
		if len(wits.PlutusData()) > 0 {
			hasWitnessPlutusData = true
		}
	}

	// ScriptDataHash covers redeemers, datums, and language views.
	// It's valid to have ScriptDataHash with no redeemers if there are witness datums.
	if tx.ScriptDataHash() != nil && redeemerCount == 0 &&
		!hasWitnessPlutusData {
		return MissingRedeemersForScriptDataHashError{}
	}
	if redeemerCount > 0 && !hasPlutus && !hasPlutusReference {
		return MissingPlutusScriptWitnessesError{}
	}
	if redeemerCount == 0 && hasPlutus {
		return ExtraneousPlutusScriptWitnessesError{}
	}
	return nil
}
