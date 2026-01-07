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

package conway

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateIsValidFlag,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	UtxoValidateSignatures,
	UtxoValidateCostModelsPresent,
	UtxoValidateInlineDatumsWithPlutusV1,
	UtxoValidateDisjointRefInputs,
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	UtxoValidateCollateralContainsNonAda,
	UtxoValidateCollateralEqBalance,
	UtxoValidateNoCollateralInputs,
	UtxoValidateBadInputsUtxo,
	// Ensure script witness presence/absence is validated after redeemer/script relation
	UtxoValidateScriptWitnesses,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputTooBigUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateTransactionNetworkId,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateExUnitsTooBigUtxo,
	UtxoValidateTooManyCollateralInputs,
}

func UtxoValidateDisjointRefInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	commonInputs := []common.TransactionInput{}
	for _, refInput := range tx.ReferenceInputs() {
		for _, input := range tx.Inputs() {
			if refInput.String() != input.String() {
				continue
			}
			commonInputs = append(commonInputs, input)
		}
	}
	if len(commonInputs) == 0 {
		return nil
	}
	return NonDisjointRefInputsError{
		Inputs: commonInputs,
	}
}

// UtxoValidateIsValidFlag ensures transactions marked invalid have Plutus scripts
func UtxoValidateIsValidFlag(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// If IsValid is true, no check needed
	if tx.IsValid() {
		return nil
	}

	// If IsValid is false, transaction must have redeemers (indicating phase-2 validation)
	w := tx.Witnesses()
	if w != nil && w.Redeemers() != nil {
		for range w.Redeemers().Iter() {
			// Has at least one redeemer
			return nil
		}
	}

	// IsValid=false but no redeemers present
	return common.InvalidIsValidFlagError{}
}

// UtxoValidateRequiredVKeyWitnesses ensures required signers are accompanied by vkey witnesses
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRequiredVKeyWitnesses(tx)
}

// UtxoValidateCollateralVKeyWitnesses ensures collateral inputs are backed by vkey witnesses
func UtxoValidateCollateralVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateCollateralVKeyWitnesses(tx, ls)
}

// UtxoValidateRedeemerAndScriptWitnesses performs lightweight UTXOW checks for presence/absence of scripts vs redeemers
// Note: Conway needs custom handling for reference scripts. This function
// intentionally performs its own reference-input-aware checks instead of
// delegating to common.ValidateRedeemerAndScriptWitnesses because Conway
// treats reference scripts differently for extraneous witness validation:
// reference scripts alone do not trigger an extraneous witness error if no
// redeemers are present, unlike the common helper which considers any Plutus
// scripts (witnessed or referenced) as extraneous without redeemers.
func UtxoValidateRedeemerAndScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Redeemer/script relation applies only to Plutus scripts. Native scripts
	// do NOT require redeemers.
	wits := tx.Witnesses()
	redeemerCount := 0
	if wits != nil {
		if r := wits.Redeemers(); r != nil {
			for range r.Iter() {
				redeemerCount++
			}
		}
	}
	// Detect Plutus availability separately for explicit witnesses and reference scripts.
	// This prevents reference scripts from being treated the same as provided script witnesses
	// when checking for extraneous witnesses.
	hasPlutusWitness := false
	hasPlutusReference := false
	if wits != nil {
		hasPlutusWitness = len(wits.PlutusV1Scripts()) > 0 ||
			len(wits.PlutusV2Scripts()) > 0 ||
			len(wits.PlutusV3Scripts()) > 0
	}

	// Consider Plutus reference scripts on reference inputs. If a reference input
	// cannot be resolved (UTxO lookup fails) validation should fail fast because
	// we cannot determine script availability deterministically without the UTxO.
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case *common.PlutusV1Script, common.PlutusV1Script, *common.PlutusV2Script, common.PlutusV2Script, *common.PlutusV3Script, common.PlutusV3Script:
			hasPlutusReference = true
		}
		if hasPlutusReference {
			break
		}
	}

	// Per CIP-33, ScriptRef can also be provided via regular (spent) inputs.
	// Check regular inputs if not found in reference inputs.
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
			case *common.PlutusV1Script, common.PlutusV1Script, *common.PlutusV2Script, common.PlutusV2Script, *common.PlutusV3Script, common.PlutusV3Script:
				hasPlutusReference = true
			}
			if hasPlutusReference {
				break
			}
		}
	}

	// If the body carries a script data hash, redeemers must be present
	if tx.ScriptDataHash() != nil && redeemerCount == 0 {
		return MissingRedeemersForScriptDataHashError{}
	}

	// If redeemers are present, we expect either a provided Plutus script witness
	// or a Plutus reference script on a reference input.
	if redeemerCount > 0 && (!hasPlutusWitness && !hasPlutusReference) {
		return MissingPlutusScriptWitnessesError{}
	}

	// If no redeemers are present but explicit Plutus script witnesses are supplied,
	// treat those supplied witnesses as extraneous. Reference scripts alone should
	// not trigger an extraneous-witness error since they don't represent supplied
	// script witnesses in the transaction.
	if redeemerCount == 0 && hasPlutusWitness {
		return ExtraneousPlutusScriptWitnessesError{}
	}

	return nil
}

// UtxoValidateScriptWitnesses checks that script witnesses are provided for all script address inputs
// and that there are no extraneous script witnesses.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateScriptWitnesses(tx, ls)
}

// UtxoValidateCostModelsPresent ensures Plutus scripts have corresponding cost models in protocol parameters
func UtxoValidateCostModelsPresent(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	required := map[uint]struct{}{}
	wits := tmpTx.WitnessSet
	if len(wits.WsPlutusV1Scripts.Items()) > 0 {
		required[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts.Items()) > 0 {
		required[1] = struct{}{}
	}
	if len(wits.WsPlutusV3Scripts.Items()) > 0 {
		required[2] = struct{}{}
	}
	// Also include reference scripts on reference inputs
	for _, refInput := range tmpTx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case *common.PlutusV1Script, common.PlutusV1Script:
			required[0] = struct{}{}
		case *common.PlutusV2Script, common.PlutusV2Script:
			required[1] = struct{}{}
		case *common.PlutusV3Script, common.PlutusV3Script:
			required[2] = struct{}{}
		}
	}

	// Per CIP-33, also include reference scripts on regular (spent) inputs
	for _, input := range tmpTx.Inputs() {
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
		case *common.PlutusV1Script, common.PlutusV1Script:
			required[0] = struct{}{}
		case *common.PlutusV2Script, common.PlutusV2Script:
			required[1] = struct{}{}
		case *common.PlutusV3Script, common.PlutusV3Script:
			required[2] = struct{}{}
		}
	}

	if len(required) == 0 {
		return nil
	}

	for version := range required {
		model, ok := tmpPparams.CostModels[version]
		if !ok || len(model) == 0 {
			return common.MissingCostModelError{Version: version}
		}
	}

	return nil
}

// UtxoValidateSignatures verifies vkey and bootstrap signatures present in the transaction.
func UtxoValidateSignatures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.UtxoValidateSignatures(tx, slot, ls, pp)
}

func UtxoValidateOutsideValidityIntervalUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return allegra.UtxoValidateOutsideValidityIntervalUtxo(tx, slot, ls, pp)
}

func UtxoValidateInputSetEmptyUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateInputSetEmptyUtxo(tx, slot, ls, pp)
}

func UtxoValidateFeeTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	minFee, err := MinFeeTx(tx, pp)
	if err != nil {
		return err
	}
	if tx.Fee() >= minFee {
		return nil
	}
	return shelley.FeeTooSmallUtxoError{
		Provided: tx.Fee(),
		Min:      minFee,
	}
}

func UtxoValidateInsufficientCollateral(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
		return nil
	}
	var totalCollateral uint64
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		totalCollateral += utxo.Output.Amount()
	}
	minCollateral := tmpTx.Fee() * uint64(tmpPparams.CollateralPercentage) / 100
	if totalCollateral >= minCollateral {
		return nil
	}
	return alonzo.InsufficientCollateralError{
		Provided: totalCollateral,
		Required: minCollateral,
	}
}

func UtxoValidateCollateralContainsNonAda(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
		return nil
	}
	badOutputs := []common.TransactionOutput{}
	var totalCollateral uint64
	totalAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](nil)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		totalCollateral += utxo.Output.Amount()
		totalAssets.Add(utxo.Output.Assets())
		if utxo.Output.Assets() == nil ||
			len(utxo.Output.Assets().Policies()) == 0 {
			continue
		}
		badOutputs = append(badOutputs, utxo.Output)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	// Check if all collateral assets are accounted for in the collateral return
	collReturn := tx.CollateralReturn()
	if collReturn != nil {
		collReturnAssets := collReturn.Assets()
		if (&totalAssets).Compare(collReturnAssets) {
			return nil
		}
	}
	return alonzo.CollateralContainsNonAdaError{
		Provided: totalCollateral,
	}
}

// UtxoValidateCollateralEqBalance ensures that the collateral return amount is equal to the collateral input amount minus the total collateral
func UtxoValidateCollateralEqBalance(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return babbage.UtxoValidateCollateralEqBalance(tx, slot, ls, pp)
}

func UtxoValidateNoCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
		return nil
	}
	if len(tx.Collateral()) > 0 {
		return nil
	}
	return alonzo.NoCollateralInputsError{}
}

func UtxoValidateBadInputsUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateBadInputsUtxo(tx, slot, ls, pp)
}

func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	// Calculate consumed value
	// consumed = value from input(s) + withdrawals + refunds
	var consumedValue uint64
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		// Ignore errors fetching the UTxO and exclude it from calculations
		if err != nil {
			continue
		}
		consumedValue += tmpUtxo.Output.Amount()
	}
	for _, tmpWithdrawalAmount := range tx.Withdrawals() {
		consumedValue += tmpWithdrawalAmount
	}
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.DeregistrationCertificate:
			// CIP-0094 deregistration uses Amount field for refund (symmetric with registration deposit)
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			consumedValue += uint64(tmpCert.Amount)
		case *common.DeregistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			consumedValue += uint64(tmpCert.Amount)
		case *common.StakeDeregistrationCertificate:
			// Traditional stake deregistration uses protocol KeyDeposit parameter
			consumedValue += uint64(tmpPparams.KeyDeposit)
		}
	}
	// Add minted/burned ADA
	if tx.AssetMint() != nil {
		mintedAda := tx.AssetMint().Asset(common.Blake2b224{}, []byte{})
		if mintedAda > 0 {
			consumedValue += uint64(mintedAda) //nolint:gosec // G115
		} else if mintedAda < 0 {
			burned := -mintedAda
			consumedValue -= uint64(burned) //nolint:gosec // G115
		}
	}
	// Calculate produced value
	// produced = value from output(s) + fee + deposits
	var producedValue uint64
	for _, tmpOutput := range tx.Outputs() {
		producedValue += tmpOutput.Amount()
	}
	producedValue += tx.Fee()
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.PoolRegistrationCertificate:
			reg, _, err := ls.PoolCurrentState(common.Blake2b224(tmpCert.Operator))
			if err != nil {
				return err
			}
			if reg == nil {
				producedValue += uint64(tmpPparams.PoolDeposit)
			}
		case *common.RegistrationCertificate:
			// CIP-0094 registration uses Amount field for deposit
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue += uint64(tmpCert.Amount)
		case *common.RegistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue += uint64(tmpCert.Amount)
		case *common.StakeRegistrationCertificate:
			// Traditional stake registration uses protocol KeyDeposit parameter
			producedValue += uint64(tmpPparams.KeyDeposit)
		case *common.StakeRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue += uint64(tmpCert.Amount)
		case *common.StakeVoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue += uint64(tmpCert.Amount)
		case *common.VoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue += uint64(tmpCert.Amount)
		}
	}
	for _, proposal := range tx.ProposalProcedures() {
		producedValue += proposal.Deposit()
	}
	// Add treasury donation - value leaving the transaction to go to the treasury
	// Treasury donations are a Conway feature and cannot be used with PlutusV1/V2 scripts
	donation := tx.Donation()
	if donation > 0 {
		// Check if transaction uses PlutusV1 or PlutusV2 scripts in witnesses
		witnesses := tx.Witnesses()
		plutusVersion := ""
		if witnesses != nil {
			if len(witnesses.PlutusV1Scripts()) > 0 {
				plutusVersion = "PlutusV1"
			} else if len(witnesses.PlutusV2Scripts()) > 0 {
				plutusVersion = "PlutusV2"
			}
		}
		// Also check reference scripts on reference inputs
		if plutusVersion == "" {
			for _, refInput := range tx.ReferenceInputs() {
				utxo, err := ls.UtxoById(refInput)
				if err != nil {
					return common.ReferenceInputResolutionError{
						Input: refInput,
						Err:   err,
					}
				}
				script := utxo.Output.ScriptRef()
				if script != nil {
					switch script.(type) {
					case *common.PlutusV1Script, common.PlutusV1Script:
						plutusVersion = "PlutusV1"
					case *common.PlutusV2Script, common.PlutusV2Script:
						plutusVersion = "PlutusV2"
					}
					if plutusVersion != "" {
						break
					}
				}
			}
		}
		// Return explicit error if donation is used with PlutusV1/V2 scripts
		if plutusVersion != "" {
			return TreasuryDonationWithPlutusV1V2Error{
				Donation:      donation,
				PlutusVersion: plutusVersion,
			}
		}
		// Only apply donation if not using PlutusV1/V2 scripts
		producedValue += donation
	}
	if consumedValue == producedValue {
		return nil
	}
	return shelley.ValueNotConservedUtxoError{
		Consumed: consumedValue,
		Produced: producedValue,
	}
}

func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
		minCoin, err := MinCoinTxOut(tmpOutput, pp)
		if err != nil {
			return err
		}
		if tmpOutput.Amount() < minCoin {
			badOutputs = append(badOutputs, tmpOutput)
		}
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return shelley.OutputTooSmallUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	badOutputs := []common.TransactionOutput{}
	for _, txOutput := range tx.Outputs() {
		tmpOutput, ok := txOutput.(*babbage.BabbageTransactionOutput)
		if !ok {
			return errors.New("transaction output is not expected type")
		}
		outputValBytes, err := cbor.Encode(tmpOutput.OutputAmount)
		if err != nil {
			return err
		}
		if uint(len(outputValBytes)) <= tmpPparams.MaxValueSize {
			continue
		}
		badOutputs = append(badOutputs, tmpOutput)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return mary.OutputTooBigUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputBootAddrAttrsTooBig(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(tx, slot, ls, pp)
}

func UtxoValidateInlineDatumsWithPlutusV1(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return babbage.UtxoValidateInlineDatumsWithPlutusV1(tx, slot, ls, pp)
}

func UtxoValidateWrongNetwork(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetwork(tx, slot, ls, pp)
}

func UtxoValidateWrongNetworkWithdrawal(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetworkWithdrawal(tx, slot, ls, pp)
}

// UtxoValidateTransactionNetworkId validates that if the transaction body
// specifies a NetworkId field, it must match the ledger state's network
func UtxoValidateTransactionNetworkId(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Only Conway transactions have the NetworkId field in the body
	conwayTx, ok := tx.(*ConwayTransaction)
	if !ok {
		// Not a Conway transaction, skip this validation
		return nil
	}

	// Get the transaction's optional NetworkId field
	txNetworkId := conwayTx.NetworkId()
	if txNetworkId == nil {
		// NetworkId not specified in transaction, that's fine
		return nil
	}

	// NetworkId is specified, must match ledger state
	ledgerNetworkId := ls.NetworkId()
	if uint(*txNetworkId) != ledgerNetworkId {
		return WrongTransactionNetworkIdError{
			TxNetworkId:     *txNetworkId,
			LedgerNetworkId: ledgerNetworkId,
		}
	}

	return nil
}

func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	txBytes, err := cbor.Encode(tx)
	if err != nil {
		return err
	}
	if uint(len(txBytes)) <= tmpPparams.MaxTxSize {
		return nil
	}
	return shelley.MaxTxSizeUtxoError{
		TxSize:    uint(len(txBytes)),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
}

func UtxoValidateExUnitsTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	var totalSteps, totalMemory int64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers.Redeemers {
		totalSteps += redeemer.ExUnits.Steps
		totalMemory += redeemer.ExUnits.Memory
	}
	if totalSteps <= tmpPparams.MaxTxExUnits.Steps &&
		totalMemory <= tmpPparams.MaxTxExUnits.Memory {
		return nil
	}
	return alonzo.ExUnitsTooBigUtxoError{
		TotalExUnits: common.ExUnits{
			Memory: totalMemory,
			Steps:  totalSteps,
		},
		MaxTxExUnits: tmpPparams.MaxTxExUnits,
	}
}

func UtxoValidateTooManyCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	collateralCount := uint(len(tx.Collateral()))
	if collateralCount <= tmpPparams.MaxCollateralInputs {
		return nil
	}
	return babbage.TooManyCollateralInputsError{
		Provided: collateralCount,
		Max:      tmpPparams.MaxCollateralInputs,
	}
}

// MinFeeTx calculates the minimum required fee for a transaction based on protocol parameters
// Fee is calculated using the transaction body CBOR size as per Cardano protocol
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ConwayProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return 0, errors.New("tx is not expected type")
	}
	// Encode a local copy of the body with TxFee set to 0 to calculate size without fee
	body := tmpTx.Body
	body.TxFee = 0
	txBytes, err := cbor.Encode(body)
	if err != nil {
		return 0, err
	}
	minFee := common.CalculateMinFee(
		len(txBytes),
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
	return minFee, nil
}

// MinCoinTxOut calculates the minimum coin for a transaction output based on protocol parameters
func MinCoinTxOut(
	txOut common.TransactionOutput,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ConwayProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txOutBytes, err := cbor.Encode(txOut)
	if err != nil {
		return 0, err
	}
	minCoinTxOut := tmpPparams.AdaPerUtxoByte * uint64(len(txOutBytes))
	return minCoinTxOut, nil
}

func UtxoValidateMetadata(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateMetadata(tx, slot, ls, pp)
}
