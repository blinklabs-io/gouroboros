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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
)

// EvaluatePlutusScripts executes every redeemer against its required script.
// The transaction's IsValid declaration is intentionally ignored.
// maxPlutusVersion is the highest language number supported by the era.
func EvaluatePlutusScripts(
	tx common.Transaction,
	ls common.LedgerState,
	protocolMajor uint,
	costModels map[uint][]int64,
	maxPlutusVersion uint,
	era string,
	budget *common.ExUnits,
) (map[common.RedeemerKey]common.ExUnits, error) {
	if tx == nil {
		return nil, errors.New("nil transaction")
	}
	used := make(map[common.RedeemerKey]common.ExUnits)
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return used, nil
	}
	redeemers := witnesses.Redeemers()
	if redeemers == nil {
		return used, nil
	}

	// Count redeemers to see if we have any scripts to execute
	redeemerCount := 0
	for range redeemers.Iter() {
		redeemerCount++
	}
	if redeemerCount == 0 {
		return used, nil
	}

	// Resolve all inputs (regular + reference) for building script context
	inputsResolved, refInputsResolved, err := ResolveTxInputs(tx, ls)
	if err != nil {
		return nil, err
	}
	resolvedInputs := ConcatResolvedInputs(
		inputsResolved,
		refInputsResolved,
	)
	resolvedInputsMap := make(
		map[string]common.Utxo,
		len(inputsResolved)+len(refInputsResolved),
	)
	for i, input := range tx.Inputs() {
		resolvedInputsMap[input.String()] = inputsResolved[i]
	}
	for i, refInput := range tx.ReferenceInputs() {
		resolvedInputsMap[refInput.String()] = refInputsResolved[i]
	}

	// Build TxInfo lazily based on script version
	var txInfoV1 TxInfoV1
	// Cached so each redeemer reuses one PlutusData conversion of the TxInfo.
	var txInfoV1Cached *CachedTxInfo
	var txInfoV2 TxInfoV2
	var txInfoV2Cached *CachedTxInfo
	var txInfoV3 TxInfoV3
	var txInfoV3Cached *CachedTxInfo
	var txInfoV1Built, txInfoV2Built, txInfoV3Built bool

	// Collect all available scripts (witness scripts + reference scripts)
	availableScripts := AvailablePlutusScripts(tx, resolvedInputs)

	// Get sorted inputs for redeemer index mapping.
	// The Cardano ledger spec requires spend redeemer indices to
	// reference positions in the canonically sorted input list.
	inputs := SortInputs(tx.Inputs())
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &common.MultiAsset[common.MultiAssetTypeMint]{}
	}
	withdrawals := tx.Withdrawals()
	votes := tx.VotingProcedures()
	proposalProcedures := tx.ProposalProcedures()
	certificates := tx.Certificates()

	// Build witness datums map for datum lookup
	plutusData := witnesses.PlutusData()
	witnessDatums := make(map[common.Blake2b256]*common.Datum)
	for i := range plutusData {
		datum := plutusData[i]
		witnessDatums[datum.Hash()] = &datum
	}

	// Execute each redeemer's script
	for redeemerKey, redeemerValue := range redeemers.Iter() {
		// Build script purpose for this redeemer
		purpose, err := BuildScriptPurpose(
			redeemerKey,
			resolvedInputsMap,
			inputs,
			*assetMint,
			certificates,
			withdrawals,
			votes,
			proposalProcedures,
			witnessDatums,
			protocolMajor,
		)
		if err != nil {
			// Redeemer doesn't match any valid purpose (index out of bounds, etc.)
			return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}

		// Check if the purpose actually requires a script
		// If not, this redeemer is "extra" (mismatched)
		switch p := purpose.(type) {
		case ScriptPurposeSpending:
			// For spending purposes, verify the input is at a script address
			if p.Input.Output != nil {
				addr := p.Input.Output.Address()
				if (addr.Type() & common.AddressTypeScriptBit) == 0 {
					// Input is at a key address, not a script address
					return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
				}
			}
		case ScriptPurposeCertifying:
			// For certifying purposes, check if the certificate has a script credential
			// ScriptHash() returns empty hash for key credentials
			if p.ScriptHash() == (common.ScriptHash{}) {
				return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case ScriptPurposeRewarding:
			// For rewarding purposes, check if the credential is a script
			if p.StakeCredential.CredType != common.CredentialTypeScriptHash {
				return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case ScriptPurposeProposing:
			// For proposing purposes, check if the proposal has a policy script
			// If not (empty ScriptHash), this redeemer is "extra"
			if p.ScriptHash() == (common.ScriptHash{}) {
				return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case ScriptPurposeVoting:
			if !VoterUsesScriptCredential(p.Voter) {
				return nil, common.ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		}

		// Find the script for this purpose
		scriptHash := purpose.ScriptHash()
		plutusScript, ok := availableScripts[scriptHash]
		if !ok {
			// Missing script should be caught by MissingScriptWitnesses
			continue
		}

		// Get datum for V1/V2 scripts (spending purpose only)
		var datum data.PlutusData
		var spendInput common.TransactionInput
		if spendPurpose, ok := purpose.(ScriptPurposeSpending); ok {
			if spendPurpose.Datum != nil {
				datum = spendPurpose.Datum
			}
			spendInput = spendPurpose.Input.Id
		}

		units := redeemerValue.ExUnits
		if budget != nil {
			units = *budget
		}

		// Execute based on script version
		var execErr error
		var usedUnits common.ExUnits
		switch s := plutusScript.(type) {
		case common.PlutusV4Script:
			return nil, common.PlutusScriptValidationUnsupportedError{Era: era}
		case common.PlutusV3Script:
			if maxPlutusVersion < 3 {
				return nil, common.PlutusScriptValidationUnsupportedError{Era: era}
			}
			// Build V3 TxInfo lazily
			if !txInfoV3Built {
				if err := ValidatePlutusV3ReferenceInputs(
					tx,
					protocolMajor,
				); err != nil {
					return nil, common.ScriptContextConstructionError{Err: err}
				}
				var err error
				txInfoV3, err = NewTxInfoV3FromTransaction(
					ls,
					tx,
					resolvedInputs,
					protocolMajor,
				)
				if err != nil {
					return nil, common.ScriptContextConstructionError{Err: err}
				}
				txInfoV3Built = true
				txInfoV3Cached = NewCachedTxInfo(txInfoV3)
			}
			// Build V3 context
			redeemer := Redeemer{
				Tag:     redeemerKey.Tag,
				Index:   redeemerKey.Index,
				Data:    data.Normalize(redeemerValue.Data.Data),
				ExUnits: redeemerValue.ExUnits,
			}
			ctx := NewScriptContextV3(txInfoV3Cached, redeemer, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := common.PooledEvalContext(
				lang.LanguageVersionV3,
				protocolMajor,
				costModels[2],
			)
			if err != nil {
				return nil, fmt.Errorf("build evaluation context: %w", err)
			}
			usedUnits, execErr = s.Evaluate(ctxData, units, evalContext)
		case common.PlutusV2Script:
			if maxPlutusVersion < 2 {
				return nil, common.PlutusScriptValidationUnsupportedError{Era: era}
			}
			// V2 scripts require a datum for spending purposes
			if _, isSpend := purpose.(ScriptPurposeSpending); isSpend && datum == nil {
				return nil, common.MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V2 TxInfo lazily
			if !txInfoV2Built {
				var err error
				txInfoV2, err = NewTxInfoV2FromTransaction(
					ls, tx, resolvedInputs,
					StrictValidityUpperBoundForTransaction(tx),
					protocolMajor,
				)
				if err != nil {
					return nil, common.ScriptContextConstructionError{Err: err}
				}
				txInfoV2Built = true
				txInfoV2Cached = NewCachedTxInfo(txInfoV2)
			}
			// Build V1V2 context
			ctx := NewScriptContextV1V2(txInfoV2Cached, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := common.PooledEvalContext(
				lang.LanguageVersionV2,
				protocolMajor,
				costModels[1],
			)
			if err != nil {
				return nil, fmt.Errorf("build evaluation context: %w", err)
			}
			usedUnits, execErr = s.Evaluate(datum, data.Normalize(redeemerValue.Data.Data), ctxData, units, evalContext)
		case common.PlutusV1Script:
			if maxPlutusVersion < 1 {
				return nil, common.PlutusScriptValidationUnsupportedError{Era: era}
			}
			// V1 scripts require a datum for spending purposes
			if _, isSpend := purpose.(ScriptPurposeSpending); isSpend && datum == nil {
				return nil, common.MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V1 TxInfo lazily
			if !txInfoV1Built {
				var err error
				txInfoV1, err = NewTxInfoV1FromTransaction(
					ls, tx, resolvedInputs,
					StrictValidityUpperBoundForTransaction(tx),
					protocolMajor,
				)
				if err != nil {
					return nil, common.ScriptContextConstructionError{Err: err}
				}
				txInfoV1Built = true
				txInfoV1Cached = NewCachedTxInfo(txInfoV1)
			}
			// Build V1V2 context
			ctx := NewScriptContextV1V2(txInfoV1Cached, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := common.PooledEvalContext(
				lang.LanguageVersionV1,
				protocolMajor,
				costModels[0],
			)
			if err != nil {
				return nil, fmt.Errorf("build evaluation context: %w", err)
			}
			usedUnits, execErr = s.Evaluate(datum, data.Normalize(redeemerValue.Data.Data), ctxData, units, evalContext)
		default:
			continue
		}

		if execErr != nil {
			return nil, common.PlutusScriptFailedError{
				ScriptHash: scriptHash,
				Tag:        redeemerKey.Tag,
				Index:      redeemerKey.Index,
				Err:        execErr,
			}
		}
		used[redeemerKey] = usedUnits
	}

	return used, nil
}
