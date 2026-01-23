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
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateProposalProcedures,
	UtxoValidateProposalNetworkIds,
	UtxoValidateEmptyTreasuryWithdrawals,
	UtxoValidateIsValidFlag,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	UtxoValidateSignatures,
	UtxoValidateCostModelsPresent,
	UtxoValidateScriptDataHash,
	UtxoValidateInlineDatumsWithPlutusV1,
	UtxoValidateConwayFeaturesWithPlutusV1V2,
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
	UtxoValidateSupplementalDatums,
	UtxoValidateExtraneousRedeemers,
	UtxoValidatePlutusScripts,
	UtxoValidateNativeScripts,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
	UtxoValidateCommitteeCertificates,
	UtxoValidateMalformedReferenceScripts,
}

func UtxoValidateDisjointRefInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return babbage.UtxoValidateDisjointRefInputs(tx, slot, ls, pp)
}

// UtxoValidateProposalProcedures validates governance proposal contents
func UtxoValidateProposalProcedures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if govAction == nil {
			continue
		}

		// Check if this is a ParameterChangeGovAction
		paramChangeAction, ok := govAction.(*ConwayParameterChangeGovAction)
		if !ok {
			continue
		}

		// Validate the protocol parameter update
		if err := validateProtocolParameterUpdate(&paramChangeAction.ParamUpdate); err != nil {
			return err
		}
	}
	return nil
}

// UtxoValidateEmptyTreasuryWithdrawals validates that TreasuryWithdrawalGovAction proposals
// do not have empty withdrawal maps and have at least one non-zero withdrawal amount.
// This is distinct from transaction reward withdrawals.
func UtxoValidateEmptyTreasuryWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if govAction == nil {
			continue
		}

		// Check if this is a TreasuryWithdrawalGovAction with empty withdrawals
		if twAction, ok := govAction.(*common.TreasuryWithdrawalGovAction); ok {
			if len(twAction.Withdrawals) == 0 {
				return EmptyTreasuryWithdrawalsError{}
			}
			// Check that at least one withdrawal has a non-zero amount
			hasNonZero := false
			for _, amount := range twAction.Withdrawals {
				if amount > 0 {
					hasNonZero = true
					break
				}
			}
			if !hasNonZero {
				return ZeroTreasuryWithdrawalAmountError{}
			}
		}
	}
	return nil
}

// UtxoValidateProposalNetworkIds validates that all addresses in proposal procedures use correct network ID
func UtxoValidateProposalNetworkIds(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	networkId := ls.NetworkId()
	badAddrs := []common.Address{}

	for _, proposal := range tx.ProposalProcedures() {
		// Check the return address (where deposit goes back)
		returnAddr := proposal.RewardAccount()
		if returnAddr.NetworkId() != networkId {
			badAddrs = append(badAddrs, returnAddr)
		}

		// Check addresses within governance actions
		govAction := proposal.GovAction()
		if govAction == nil {
			continue
		}

		// TreasuryWithdrawalGovAction contains withdrawal addresses
		if twAction, ok := govAction.(*common.TreasuryWithdrawalGovAction); ok {
			for addr := range twAction.Withdrawals {
				if addr.NetworkId() != networkId {
					badAddrs = append(badAddrs, *addr)
				}
			}
		}
	}

	if len(badAddrs) == 0 {
		return nil
	}
	return WrongNetworkProposalAddressError{
		NetId: networkId,
		Addrs: badAddrs,
	}
}

// validateProtocolParameterUpdate validates that a PPU is well-formed
func validateProtocolParameterUpdate(ppu *ConwayProtocolParameterUpdate) error {
	// Check if PPU is empty (no fields set)
	if ppu.MinFeeA == nil &&
		ppu.MinFeeB == nil &&
		ppu.MaxBlockBodySize == nil &&
		ppu.MaxTxSize == nil &&
		ppu.MaxBlockHeaderSize == nil &&
		ppu.KeyDeposit == nil &&
		ppu.PoolDeposit == nil &&
		ppu.MaxEpoch == nil &&
		ppu.NOpt == nil &&
		ppu.A0 == nil &&
		ppu.Rho == nil &&
		ppu.Tau == nil &&
		ppu.ProtocolVersion == nil &&
		ppu.MinPoolCost == nil &&
		ppu.AdaPerUtxoByte == nil &&
		len(ppu.CostModels) == 0 &&
		ppu.ExecutionCosts == nil &&
		ppu.MaxTxExUnits == nil &&
		ppu.MaxBlockExUnits == nil &&
		ppu.MaxValueSize == nil &&
		ppu.CollateralPercentage == nil &&
		ppu.MaxCollateralInputs == nil &&
		ppu.PoolVotingThresholds == nil &&
		ppu.DRepVotingThresholds == nil &&
		ppu.MinCommitteeSize == nil &&
		ppu.CommitteeTermLimit == nil &&
		ppu.GovActionValidityPeriod == nil &&
		ppu.GovActionDeposit == nil &&
		ppu.DRepDeposit == nil &&
		ppu.DRepInactivityPeriod == nil &&
		ppu.MinFeeRefScriptCostPerByte == nil {
		return ProtocolParameterUpdateEmptyError{}
	}

	// Validate individual fields that cannot be zero
	if ppu.MaxBlockHeaderSize != nil && *ppu.MaxBlockHeaderSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBHSize",
			Value:     *ppu.MaxBlockHeaderSize,
		}
	}

	if ppu.MaxTxSize != nil && *ppu.MaxTxSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxTxSize",
			Value:     *ppu.MaxTxSize,
		}
	}

	if ppu.MaxValueSize != nil && *ppu.MaxValueSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxValSize",
			Value:     uint(*ppu.MaxValueSize),
		}
	}

	if ppu.MaxBlockBodySize != nil && *ppu.MaxBlockBodySize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBlockBodySize",
			Value:     *ppu.MaxBlockBodySize,
		}
	}

	return nil
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
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script, common.PlutusV2Script, common.PlutusV3Script:
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
			case common.PlutusV1Script, common.PlutusV2Script, common.PlutusV3Script:
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

// UtxoValidateExtraneousRedeemers checks that all redeemers have valid purposes.
// A redeemer is "extraneous" if its index is out of bounds for its purpose type:
// - Spending redeemer index >= number of transaction inputs
// - Minting redeemer index >= number of distinct mint policies
// - Certificate redeemer index >= number of certificates
// - Reward redeemer index >= number of withdrawals
// - Voting redeemer index >= number of voters
// - Proposing redeemer index >= number of proposals
func UtxoValidateExtraneousRedeemers(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	wits := tx.Witnesses()
	if wits == nil {
		return nil
	}
	redeemers := wits.Redeemers()
	if redeemers == nil {
		return nil
	}

	// Get counts for each purpose type
	inputCount := len(tx.Inputs())
	certCount := len(tx.Certificates())
	withdrawalCount := len(tx.Withdrawals())
	proposalCount := len(tx.ProposalProcedures())

	// Count distinct mint policies
	mintPolicyCount := 0
	if mint := tx.AssetMint(); mint != nil {
		mintPolicyCount = len(mint.Policies())
	}

	// Count voters (each voter is a separate purpose index)
	voterCount := 0
	if votingProcs := tx.VotingProcedures(); votingProcs != nil {
		voterCount = len(votingProcs)
	}

	// Check each redeemer
	for redeemerKey := range redeemers.Iter() {
		var maxIndex int
		switch redeemerKey.Tag {
		case common.RedeemerTagSpend:
			maxIndex = inputCount
		case common.RedeemerTagMint:
			maxIndex = mintPolicyCount
		case common.RedeemerTagCert:
			maxIndex = certCount
		case common.RedeemerTagReward:
			maxIndex = withdrawalCount
		case common.RedeemerTagVoting:
			maxIndex = voterCount
		case common.RedeemerTagProposing:
			maxIndex = proposalCount
		default:
			continue
		}

		if int(redeemerKey.Index) >= maxIndex {
			return ExtraRedeemerError{RedeemerKey: redeemerKey}
		}
	}

	return nil
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
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			required[0] = struct{}{}
		case common.PlutusV2Script:
			required[1] = struct{}{}
		case common.PlutusV3Script:
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
		case common.PlutusV1Script:
			required[0] = struct{}{}
		case common.PlutusV2Script:
			required[1] = struct{}{}
		case common.PlutusV3Script:
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

// UtxoValidateScriptDataHash validates the transaction's ScriptDataHash against the expected hash
// computed from redeemers, datums, and cost models (language views).
func UtxoValidateScriptDataHash(
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

	wits := tmpTx.WitnessSet
	hasRedeemers := len(wits.WsRedeemers.Redeemers) > 0
	hasDatums := len(wits.WsPlutusData.Items()) > 0

	// Determine which Plutus versions are used
	usedVersions := make(map[uint]struct{})
	if len(wits.WsPlutusV1Scripts.Items()) > 0 {
		usedVersions[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts.Items()) > 0 {
		usedVersions[1] = struct{}{}
	}
	if len(wits.WsPlutusV3Scripts.Items()) > 0 {
		usedVersions[2] = struct{}{}
	}

	// Also check reference scripts
	for _, refInput := range tmpTx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			usedVersions[0] = struct{}{}
		case common.PlutusV2Script:
			usedVersions[1] = struct{}{}
		case common.PlutusV3Script:
			usedVersions[2] = struct{}{}
		}
	}

	// Check scripts in regular inputs
	for _, input := range tmpTx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
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
		case common.PlutusV1Script:
			usedVersions[0] = struct{}{}
		case common.PlutusV2Script:
			usedVersions[1] = struct{}{}
		case common.PlutusV3Script:
			usedVersions[2] = struct{}{}
		}
	}

	hasPlutusScripts := len(usedVersions) > 0
	declaredHash := tx.ScriptDataHash()

	// If no Plutus scripts and no redeemers/datums, ScriptDataHash should be absent
	if !hasPlutusScripts && !hasRedeemers && !hasDatums {
		if declaredHash != nil {
			return common.ExtraneousScriptDataHashError{Provided: *declaredHash}
		}
		return nil
	}

	// If there are Plutus scripts/redeemers/datums, ScriptDataHash is required
	if hasPlutusScripts || hasRedeemers || hasDatums {
		if declaredHash == nil {
			return common.MissingScriptDataHashError{}
		}
	}

	// Verify cost models are present for all used Plutus versions
	// (required for phase-2 validation even if we can't verify the exact hash)
	for version := range usedVersions {
		if _, ok := tmpPparams.CostModels[version]; !ok {
			return common.MissingCostModelError{Version: version}
		}
	}

	// Compute the expected ScriptDataHash
	// ScriptDataHash = blake2b256(redeemers_cbor || datums_cbor || langviews_cbor)

	// Get preserved CBOR bytes for redeemers
	redeemersCbor := wits.WsRedeemers.Cbor()
	if len(redeemersCbor) == 0 {
		// Fall back to re-encoding if no preserved CBOR
		var err error
		redeemersCbor, err = cbor.Encode(wits.WsRedeemers)
		if err != nil {
			return err
		}
	}

	// Get preserved CBOR bytes for datums (only if non-empty)
	var datumsCbor []byte
	if hasDatums {
		datumsCbor = wits.WsPlutusData.Cbor()
		if len(datumsCbor) == 0 {
			// Fall back to re-encoding if no preserved CBOR
			var err error
			datumsCbor, err = cbor.Encode(wits.WsPlutusData)
			if err != nil {
				return err
			}
		}
	}

	// Encode language views per the Cardano spec
	langViewsCbor, err := common.EncodeLangViews(
		usedVersions,
		tmpPparams.CostModels,
	)
	if err != nil {
		return err
	}

	// Concatenate and hash
	hashInput := make(
		[]byte,
		0,
		len(redeemersCbor)+len(datumsCbor)+len(langViewsCbor),
	)
	hashInput = append(hashInput, redeemersCbor...)
	hashInput = append(hashInput, datumsCbor...)
	hashInput = append(hashInput, langViewsCbor...)

	computedHash := common.Blake2b256Hash(hashInput)

	// Compare with declared hash
	// Note: declaredHash is guaranteed non-nil here due to earlier checks,
	// but we add an explicit check to satisfy static analysis
	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}
	if *declaredHash != computedHash {
		return common.ScriptDataHashMismatchError{
			Declared: *declaredHash,
			Computed: computedHash,
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
	minFeeBigInt := new(big.Int).SetUint64(minFee)
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	if fee.Cmp(minFeeBigInt) >= 0 {
		return nil
	}
	return shelley.FeeTooSmallUtxoError{
		Provided: fee,
		Min:      minFeeBigInt,
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
	totalCollateral := new(big.Int)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
	}
	// minCollateral = fee * collateralPercentage / 100
	fee := tmpTx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	minCollateral := new(
		big.Int,
	).Mul(fee, new(big.Int).SetUint64(uint64(tmpPparams.CollateralPercentage)))
	minCollateral.Div(minCollateral, big.NewInt(100))
	if totalCollateral.Cmp(minCollateral) >= 0 {
		return nil
	}
	// Convert to uint64 for error struct (best effort)
	var providedU, requiredU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	if minCollateral.IsUint64() {
		requiredU = minCollateral.Uint64()
	}
	return alonzo.InsufficientCollateralError{
		Provided: providedU,
		Required: requiredU,
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
	totalCollateral := new(big.Int)
	totalAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](nil)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		amount := utxo.Output.Amount()
		if amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
		assets := utxo.Output.Assets()
		totalAssets.Add(assets)
		if assets == nil || len(assets.Policies()) == 0 {
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
	var providedU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	return alonzo.CollateralContainsNonAdaError{
		Provided: providedU,
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
	consumedValue := new(big.Int)
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		// Ignore errors fetching the UTxO and exclude it from calculations
		if err != nil {
			continue
		}
		if amount := tmpUtxo.Output.Amount(); amount != nil {
			consumedValue.Add(consumedValue, amount)
		}
	}
	for _, tmpWithdrawalAmount := range tx.Withdrawals() {
		if tmpWithdrawalAmount != nil {
			consumedValue.Add(consumedValue, tmpWithdrawalAmount)
		}
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
			consumedValue.Add(consumedValue, big.NewInt(tmpCert.Amount))
		case *common.DeregistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			consumedValue.Add(consumedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeDeregistrationCertificate:
			// Traditional stake deregistration uses protocol KeyDeposit parameter
			consumedValue.Add(consumedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
		}
	}
	// Add minted/burned ADA
	if tx.AssetMint() != nil {
		mintedAda := tx.AssetMint().Asset(common.Blake2b224{}, []byte{})
		if mintedAda != nil {
			consumedValue.Add(consumedValue, mintedAda)
		}
	}
	// Calculate produced value
	// produced = value from output(s) + fee + deposits
	producedValue := new(big.Int)
	for _, tmpOutput := range tx.Outputs() {
		if amount := tmpOutput.Amount(); amount != nil {
			producedValue.Add(producedValue, amount)
		}
	}
	if fee := tx.Fee(); fee != nil {
		producedValue.Add(producedValue, fee)
	}
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.PoolRegistrationCertificate:
			reg, _, err := ls.PoolCurrentState(common.Blake2b224(tmpCert.Operator))
			if err != nil {
				return err
			}
			if reg == nil {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.PoolDeposit)))
			}
		case *common.RegistrationCertificate:
			// CIP-0094 registration uses Amount field for deposit
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.RegistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeRegistrationCertificate:
			// Traditional stake registration uses protocol KeyDeposit parameter
			producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
		case *common.StakeRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeVoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.VoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		}
	}
	for _, proposal := range tx.ProposalProcedures() {
		producedValue.Add(
			producedValue,
			new(big.Int).SetUint64(proposal.Deposit()),
		)
	}
	// Add treasury donation - value leaving the transaction to go to the treasury
	// Treasury donations are a Conway feature and cannot be used with PlutusV1/V2 scripts
	donation := tx.Donation()
	if donation != nil && donation.Sign() > 0 {
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
				if utxo.Output == nil {
					continue
				}
				script := utxo.Output.ScriptRef()
				if script != nil {
					switch script.(type) {
					case common.PlutusV1Script:
						plutusVersion = "PlutusV1"
					case common.PlutusV2Script:
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
			var donationU uint64
			if donation.IsUint64() {
				donationU = donation.Uint64()
			}
			return TreasuryDonationWithPlutusV1V2Error{
				Donation:      donationU,
				PlutusVersion: plutusVersion,
			}
		}
		// Only apply donation if not using PlutusV1/V2 scripts
		producedValue.Add(producedValue, donation)
	}
	if consumedValue.Cmp(producedValue) != 0 {
		return shelley.ValueNotConservedUtxoError{
			Consumed: consumedValue,
			Produced: producedValue,
		}
	}

	// Multi-asset value conservation check
	// For each policy and asset: consumed + minted == produced
	type assetKey struct {
		policy common.Blake2b224
		asset  string
	}

	consumedAssets := make(map[assetKey]*big.Int)
	producedAssets := make(map[assetKey]*big.Int)

	// Collect consumed multi-assets from inputs
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		if err != nil {
			continue
		}
		if assets := tmpUtxo.Output.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if consumedAssets[key] == nil {
						consumedAssets[key] = new(big.Int)
					}
					consumedAssets[key].Add(consumedAssets[key], amount)
				}
			}
		}
	}

	// Add minted/burned assets to consumed (positive for mint, negative for burn)
	if mint := tx.AssetMint(); mint != nil {
		for _, policy := range mint.Policies() {
			// Skip ADA (empty policy ID) as it's tracked separately in consumed/produced value
			if policy == (common.Blake2b224{}) {
				continue
			}
			for _, assetName := range mint.Assets(policy) {
				amount := mint.Asset(policy, assetName)
				if amount == nil {
					continue
				}
				key := assetKey{policy: policy, asset: string(assetName)}
				if consumedAssets[key] == nil {
					consumedAssets[key] = new(big.Int)
				}
				consumedAssets[key].Add(consumedAssets[key], amount)
			}
		}
	}

	// Collect produced multi-assets from outputs
	for _, tmpOutput := range tx.Outputs() {
		if assets := tmpOutput.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if producedAssets[key] == nil {
						producedAssets[key] = new(big.Int)
					}
					producedAssets[key].Add(producedAssets[key], amount)
				}
			}
		}
	}

	// Check that all consumed assets match produced assets
	allKeys := make(map[assetKey]bool)
	for k := range consumedAssets {
		allKeys[k] = true
	}
	for k := range producedAssets {
		allKeys[k] = true
	}

	for key := range allKeys {
		consumed := consumedAssets[key]
		produced := producedAssets[key]
		if consumed == nil {
			consumed = new(big.Int)
		}
		if produced == nil {
			produced = new(big.Int)
		}
		if consumed.Cmp(produced) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: consumed,
				Produced: produced,
			}
		}
	}

	return nil
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
		minCoinBig := new(big.Int).SetUint64(minCoin)
		amount := tmpOutput.Amount()
		if amount == nil {
			amount = new(big.Int)
		}
		if amount.Cmp(minCoinBig) < 0 {
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

// UtxoValidateConwayFeaturesWithPlutusV1V2 ensures Conway-specific features
// (CurrentTreasuryValue, ProposalProcedures, VotingProcedures, Conway certificates)
// are not used with PlutusV1/V2 scripts.
// These features are only available in the PlutusV3 script context.
func UtxoValidateConwayFeaturesWithPlutusV1V2(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// First check for PlutusV1/V2 scripts in witness set
	plutusVersion := ""
	witnesses := tx.Witnesses()
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
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script != nil {
				switch script.(type) {
				case common.PlutusV1Script:
					plutusVersion = "PlutusV1"
				case common.PlutusV2Script:
					plutusVersion = "PlutusV2"
				}
				if plutusVersion != "" {
					break
				}
			}
		}
	}

	// Also check reference scripts on regular inputs
	if plutusVersion == "" {
		for _, input := range tx.Inputs() {
			utxo, err := ls.UtxoById(input)
			if err != nil {
				continue
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script != nil {
				switch script.(type) {
				case common.PlutusV1Script:
					plutusVersion = "PlutusV1"
				case common.PlutusV2Script:
					plutusVersion = "PlutusV2"
				}
				if plutusVersion != "" {
					break
				}
			}
		}
	}

	// No V1/V2 scripts found, Conway features are fine
	if plutusVersion == "" {
		return nil
	}

	// Check for Conway-specific features
	hasCurrentTreasuryValue := tx.CurrentTreasuryValue() != nil &&
		tx.CurrentTreasuryValue().Sign() > 0
	hasProposalProcedures := len(tx.ProposalProcedures()) > 0
	hasVotingProcedures := tx.VotingProcedures() != nil &&
		len(tx.VotingProcedures()) > 0

	// Return appropriate error based on which Conway feature is present
	if hasCurrentTreasuryValue {
		return CurrentTreasuryValueWithPlutusV1V2Error{
			PlutusVersion: plutusVersion,
		}
	}
	if hasProposalProcedures {
		return ProposalProceduresWithPlutusV1V2Error{
			PlutusVersion: plutusVersion,
		}
	}
	if hasVotingProcedures {
		return VotingProceduresWithPlutusV1V2Error{PlutusVersion: plutusVersion}
	}

	// Check for Conway-specific certificates that are NOT representable in V1/V2 contexts
	// Note: RegistrationCertificate and DeregistrationCertificate ARE supported (mapped to V1/V2 equivalents)
	for _, cert := range tx.Certificates() {
		certType := ""
		switch cert.(type) {
		case *common.AuthCommitteeHotCertificate:
			certType = "AuthCommitteeHot"
		case *common.ResignCommitteeColdCertificate:
			certType = "ResignCommitteeCold"
		case *common.RegistrationDrepCertificate:
			certType = "DRepRegistration"
		case *common.DeregistrationDrepCertificate:
			certType = "DRepDeregistration"
		case *common.UpdateDrepCertificate:
			certType = "DRepUpdate"
		case *common.StakeVoteDelegationCertificate:
			certType = "StakeVoteDelegation"
		case *common.StakeRegistrationDelegationCertificate:
			certType = "StakeRegistrationDelegation"
		case *common.VoteDelegationCertificate:
			certType = "VoteDelegation"
		case *common.VoteRegistrationDelegationCertificate:
			certType = "VoteRegistrationDelegation"
		case *common.StakeVoteRegistrationDelegationCertificate:
			certType = "StakeVoteRegistrationDelegation"
		}
		if certType != "" {
			return ConwayCertificateWithPlutusV1V2Error{
				PlutusVersion:   plutusVersion,
				CertificateType: certType,
			}
		}
	}

	return nil
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

// UtxoValidateSupplementalDatums checks that all datums in the witness set are
// justified by being referenced by a script input's datum hash.
// Inline datums are not considered - only non-inline datum hashes require witness datums.
func UtxoValidateSupplementalDatums(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}

	// Get all datums from witness set
	witnessDatums := witnesses.PlutusData()
	if len(witnessDatums) == 0 {
		return nil
	}

	// Collect all "justified" datum hashes - those referenced by UTxOs being spent
	justifiedHashes := make(map[common.Blake2b256]bool)

	// Check regular inputs
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue // UTxO not found - will fail BadInputsUtxo rule
		}
		if utxo.Output == nil {
			continue
		}
		// Only non-inline datums justify witness datums
		if utxo.Output.Datum() == nil {
			if datumHash := utxo.Output.DatumHash(); datumHash != nil {
				justifiedHashes[*datumHash] = true
			}
		}
	}

	// Check reference inputs as well - datums referenced there are also justified
	for _, input := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		if utxo.Output.Datum() == nil {
			if datumHash := utxo.Output.DatumHash(); datumHash != nil {
				justifiedHashes[*datumHash] = true
			}
		}
	}

	// Check for supplemental (unjustified) datums
	var supplementalHashes []common.Blake2b256
	for _, datum := range witnessDatums {
		datumHash := datum.Hash()
		if !justifiedHashes[datumHash] {
			supplementalHashes = append(supplementalHashes, datumHash)
		}
	}

	if len(supplementalHashes) > 0 {
		return NotAllowedSupplementalDatumsError{
			DatumHashes: supplementalHashes,
		}
	}

	return nil
}

// UtxoValidatePlutusScripts executes all Plutus scripts in the transaction
// and validates that they pass. This is the phase-2 validation.
func UtxoValidatePlutusScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	// Skip if transaction is marked as invalid (phase-2 failure already indicated)
	if !tx.IsValid() {
		return nil
	}

	// Check if there are any redeemers
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}
	redeemers := witnesses.Redeemers()
	if redeemers == nil {
		return nil
	}

	// Count redeemers to see if we have any scripts to execute
	redeemerCount := 0
	for range redeemers.Iter() {
		redeemerCount++
	}
	if redeemerCount == 0 {
		return nil
	}

	// Resolve all inputs (regular + reference) for building script context
	inputCount := len(tx.Inputs()) + len(tx.ReferenceInputs())
	resolvedInputs := make([]common.Utxo, 0, inputCount)
	resolvedInputsMap := make(map[string]common.Utxo)
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		resolvedInputs = append(resolvedInputs, utxo)
		resolvedInputsMap[input.String()] = utxo
	}
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		resolvedInputs = append(resolvedInputs, utxo)
		resolvedInputsMap[refInput.String()] = utxo
	}

	// Build TxInfo lazily based on script version
	var txInfoV1 script.TxInfoV1
	var txInfoV2 script.TxInfoV2
	var txInfoV3 script.TxInfoV3
	var txInfoV1Built, txInfoV2Built, txInfoV3Built bool

	// Collect all available scripts (witness scripts + reference scripts)
	availableScripts := make(map[common.ScriptHash]common.Script)

	// Add witness scripts
	for _, s := range witnesses.PlutusV1Scripts() {
		availableScripts[s.Hash()] = s
	}
	for _, s := range witnesses.PlutusV2Scripts() {
		availableScripts[s.Hash()] = s
	}
	for _, s := range witnesses.PlutusV3Scripts() {
		availableScripts[s.Hash()] = s
	}

	// Add reference scripts from resolved inputs
	for _, utxo := range resolvedInputs {
		if utxo.Output == nil {
			continue
		}
		scriptRef := utxo.Output.ScriptRef()
		if scriptRef == nil {
			continue
		}
		availableScripts[scriptRef.Hash()] = scriptRef
	}

	// Get sorted inputs for redeemer index mapping
	inputs := tx.Inputs()
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
		purpose := script.BuildScriptPurpose(
			redeemerKey,
			resolvedInputsMap,
			inputs,
			*assetMint,
			certificates,
			withdrawals,
			votes,
			proposalProcedures,
			witnessDatums,
		)
		if purpose == nil {
			// Redeemer doesn't match any valid purpose (index out of bounds, etc.)
			return ExtraRedeemerError{RedeemerKey: redeemerKey}
		}

		// Check if the purpose actually requires a script
		// If not, this redeemer is "extra" (mismatched)
		switch p := purpose.(type) {
		case script.ScriptPurposeSpending:
			// For spending purposes, verify the input is at a script address
			if p.Input.Output != nil {
				addr := p.Input.Output.Address()
				if (addr.Type() & common.AddressTypeScriptBit) == 0 {
					// Input is at a key address, not a script address
					return ExtraRedeemerError{RedeemerKey: redeemerKey}
				}
			}
		case script.ScriptPurposeCertifying:
			// For certifying purposes, check if the certificate has a script credential
			// ScriptHash() returns empty hash for key credentials
			if p.ScriptHash() == (common.ScriptHash{}) {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case script.ScriptPurposeRewarding:
			// For rewarding purposes, check if the credential is a script
			if p.StakeCredential.CredType != common.CredentialTypeScriptHash {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case script.ScriptPurposeProposing:
			// For proposing purposes, check if the proposal has a policy script
			// If not (empty ScriptHash), this redeemer is "extra"
			if p.ScriptHash() == (common.ScriptHash{}) {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
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
		if spendPurpose, ok := purpose.(script.ScriptPurposeSpending); ok {
			if spendPurpose.Datum != nil {
				datum = spendPurpose.Datum
			}
			spendInput = spendPurpose.Input.Id
		}

		// Execute based on script version
		var execErr error
		switch s := plutusScript.(type) {
		case common.PlutusV3Script:
			// Build V3 TxInfo lazily
			if !txInfoV3Built {
				var err error
				txInfoV3, err = script.NewTxInfoV3FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV3Built = true
			}
			// Build V3 context
			redeemer := script.Redeemer{
				Tag:     redeemerKey.Tag,
				Index:   redeemerKey.Index,
				Data:    redeemerValue.Data.Data,
				ExUnits: redeemerValue.ExUnits,
			}
			ctx := script.NewScriptContextV3(txInfoV3, redeemer, purpose)
			ctxData := ctx.ToPlutusData()
			costModel, err := cek.CostModelFromList(lang.LanguageVersionV3, conwayPparams.CostModels[2])
			if err != nil {
				return fmt.Errorf("build cost model: %w", err)
			}
			_, execErr = s.Evaluate(ctxData, redeemerValue.ExUnits, costModel)
		case common.PlutusV2Script:
			// V2 scripts require a datum for spending purposes
			if _, isSpend := purpose.(script.ScriptPurposeSpending); isSpend && datum == nil {
				return MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V2 TxInfo lazily
			if !txInfoV2Built {
				var err error
				txInfoV2, err = script.NewTxInfoV2FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV2Built = true
			}
			// Build V1V2 context
			ctx := script.NewScriptContextV1V2(txInfoV2, purpose)
			ctxData := ctx.ToPlutusData()
			costModel, err := cek.CostModelFromList(lang.LanguageVersionV2, conwayPparams.CostModels[1])
			if err != nil {
				return fmt.Errorf("build cost model: %w", err)
			}
			_, execErr = s.Evaluate(datum, redeemerValue.Data.Data, ctxData, redeemerValue.ExUnits, costModel)
		case common.PlutusV1Script:
			// V1 scripts require a datum for spending purposes
			if _, isSpend := purpose.(script.ScriptPurposeSpending); isSpend && datum == nil {
				return MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V1 TxInfo lazily
			if !txInfoV1Built {
				var err error
				txInfoV1, err = script.NewTxInfoV1FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV1Built = true
			}
			// Build V1V2 context
			ctx := script.NewScriptContextV1V2(txInfoV1, purpose)
			ctxData := ctx.ToPlutusData()
			costModel, err := cek.CostModelFromList(lang.LanguageVersionV1, conwayPparams.CostModels[0])
			if err != nil {
				return fmt.Errorf("build cost model: %w", err)
			}
			_, execErr = s.Evaluate(datum, redeemerValue.Data.Data, ctxData, redeemerValue.ExUnits, costModel)
		default:
			continue
		}

		if execErr != nil {
			return PlutusScriptFailedError{
				ScriptHash: scriptHash,
				Tag:        redeemerKey.Tag,
				Index:      redeemerKey.Index,
				Err:        execErr,
			}
		}
	}

	return nil
}

// UtxoValidateNativeScripts evaluates native scripts in the transaction.
// Native scripts (timelock scripts) are evaluated based on:
// - Signatures present in the transaction
// - Transaction's validity interval
// This is phase-1 validation for native scripts.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}

	nativeScripts := witnesses.NativeScripts()
	if len(nativeScripts) == 0 {
		return nil
	}

	// Collect key hashes from VKey witnesses
	keyHashes := make(map[common.Blake2b224]bool)
	for _, vkw := range witnesses.Vkey() {
		// VKey witnesses contain the public key, we need its hash
		keyHash := common.Blake2b224Hash(vkw.Vkey)
		keyHashes[keyHash] = true
	}
	// Also collect key hashes from bootstrap witnesses (Byron-era)
	for _, bw := range witnesses.Bootstrap() {
		keyHash := common.Blake2b224Hash(bw.PublicKey)
		keyHashes[keyHash] = true
	}

	// Get transaction validity interval
	validityStart := tx.ValidityIntervalStart()
	validityEnd := tx.TTL()
	if validityEnd == 0 {
		validityEnd = ^uint64(0) // Max uint64 if not set
	}

	// Evaluate each native script
	for _, nscript := range nativeScripts {
		scriptHash := nscript.Hash()
		if !nscript.Evaluate(slot, validityStart, validityEnd, keyHashes) {
			return NativeScriptFailedError{ScriptHash: scriptHash}
		}
	}

	return nil
}

// UtxoValidateDelegation validates delegation certificates against ledger state.
// It checks:
// - Pool registration status for stake delegations
// - Stake credential registration for non-registration delegations
//
// The function tracks in-transaction registrations to handle cases where
// registration and delegation are in the same transaction.
func UtxoValidateDelegation(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Track credentials/pools/DReps registered within this transaction
	inTxStakeRegs := make(map[common.Blake2b224]bool)
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)
	inTxDRepRegs := make(map[common.Blake2b224]bool)

	// Helper to check if stake credential is registered (in state or in-tx)
	isStakeRegistered := func(cred common.Credential) bool {
		return ls.IsStakeCredentialRegistered(cred) ||
			inTxStakeRegs[cred.Credential]
	}

	// Helper to check if pool is registered (in state or in-tx)
	isPoolRegistered := func(poolKeyHash common.PoolKeyHash) bool {
		return ls.IsPoolRegistered(poolKeyHash) || inTxPoolRegs[poolKeyHash]
	}

	// Helper to check if DRep is registered (in state or in-tx)
	// Returns true for special DRep types (Abstain, NoConfidence) as they don't need registration
	isDRepRegistered := func(drep common.Drep) bool {
		// Special DRep types don't require registration
		if drep.Type == common.DrepTypeAbstain ||
			drep.Type == common.DrepTypeNoConfidence {
			return true
		}
		// For key hash and script hash types, check registration
		if len(drep.Credential) != 28 {
			return false
		}
		var credHash common.Blake2b224
		copy(credHash[:], drep.Credential)
		// Check in-tx registrations first
		if inTxDRepRegs[credHash] {
			return true
		}
		// Check ledger state
		reg, err := ls.DRepRegistration(credHash)
		return err == nil && reg != nil
	}

	// Helper to convert Drep type to credential type safely
	drepTypeToCredType := func(drepType int) uint {
		switch drepType {
		case common.DrepTypeAddrKeyHash:
			return common.CredentialTypeAddrKeyHash
		case common.DrepTypeScriptHash:
			return common.CredentialTypeScriptHash
		default:
			return common.CredentialTypeAddrKeyHash // fallback
		}
	}

	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		// Track registrations for in-tx state
		case *common.RegistrationCertificate:
			inTxStakeRegs[c.StakeCredential.Credential] = true

		case *common.StakeRegistrationCertificate:
			inTxStakeRegs[c.StakeCredential.Credential] = true

		case *common.PoolRegistrationCertificate:
			inTxPoolRegs[c.Operator] = true

		case *common.RegistrationDrepCertificate:
			inTxDRepRegs[c.DrepCredential.Credential] = true

		// Track deregistrations for in-tx state
		case *common.StakeDeregistrationCertificate:
			delete(inTxStakeRegs, c.StakeCredential.Credential)

		case *common.DeregistrationCertificate:
			delete(inTxStakeRegs, c.StakeCredential.Credential)

		case *common.PoolRetirementCertificate:
			delete(inTxPoolRegs, c.PoolKeyHash)

		case *common.DeregistrationDrepCertificate:
			delete(inTxDRepRegs, c.DrepCredential.Credential)

		// Check delegations
		case *common.StakeDelegationCertificate:
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if stake credential is registered
			if c.StakeCredential != nil && !isStakeRegistered(*c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: *c.StakeCredential}
			}

		case *common.VoteDelegationCertificate:
			// Check if stake credential is registered
			if !isStakeRegistered(c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: c.StakeCredential}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			if !isDRepRegistered(c.Drep) {
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   drepTypeToCredType(c.Drep.Type),
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeVoteDelegationCertificate:
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if stake credential is registered
			if !isStakeRegistered(c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: c.StakeCredential}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			if !isDRepRegistered(c.Drep) {
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   drepTypeToCredType(c.Drep.Type),
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeRegistrationDelegationCertificate:
			// This cert registers AND delegates, so mark as registered first
			inTxStakeRegs[c.StakeCredential.Credential] = true
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}

		case *common.VoteRegistrationDelegationCertificate:
			// This cert registers AND delegates, so mark as registered first
			inTxStakeRegs[c.StakeCredential.Credential] = true
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			if !isDRepRegistered(c.Drep) {
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   drepTypeToCredType(c.Drep.Type),
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeVoteRegistrationDelegationCertificate:
			// This cert registers AND delegates, so mark as registered first
			inTxStakeRegs[c.StakeCredential.Credential] = true
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			if !isDRepRegistered(c.Drep) {
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   drepTypeToCredType(c.Drep.Type),
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}
		}
	}
	return nil
}

// UtxoValidateWithdrawals validates withdrawals against ledger state.
// It checks that reward accounts are registered and that withdrawal amounts
// match the account balance (when balance tracking is available).
func UtxoValidateWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	withdrawals := tx.Withdrawals()
	if withdrawals == nil {
		return nil
	}

	for addr, amount := range withdrawals {
		// Extract credential from reward address staking payload
		stakingPayload := addr.StakingPayload()
		if stakingPayload == nil {
			continue
		}

		var cred common.Credential
		switch p := stakingPayload.(type) {
		case common.AddressPayloadKeyHash:
			cred = common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.NewBlake2b224(p.Hash.Bytes()),
			}
		case common.AddressPayloadScriptHash:
			cred = common.Credential{
				CredType:   common.CredentialTypeScriptHash,
				Credential: common.NewBlake2b224(p.Hash.Bytes()),
			}
		default:
			continue // Pointer addresses not supported
		}

		// Check if reward account is registered
		if !ls.IsRewardAccountRegistered(cred) {
			return WithdrawalFromUnregisteredRewardAccountError{
				RewardAddress: *addr,
			}
		}

		// NOTE: Withdrawal amount validation (checking amount == balance) is disabled.
		// Accurate validation requires tracking balance changes through each transaction
		// which is complex for multi-TX test scenarios. The registration check above
		// catches the most important case (unregistered reward account).
		_ = amount // Prevent unused variable warning
	}
	return nil
}

func UtxoValidateCommitteeCertificates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	members, err := ls.CommitteeMembers()
	hasCommitteeState := err == nil && len(members) > 0

	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *common.AuthCommitteeHotCertificate:
			coldHash := c.ColdCredential.Credential
			member, err := ls.CommitteeMember(coldHash)
			if err != nil {
				return CommitteeMemberLookupError{Credential: coldHash, Err: err}
			}
			if member == nil && hasCommitteeState {
				return NotCommitteeMemberError{Credential: coldHash, Operation: "authorize hot key"}
			}
			if member != nil && member.Resigned {
				return ResignedCommitteeMemberHotKeyError{ColdKey: coldHash}
			}

		case *common.ResignCommitteeColdCertificate:
			coldHash := c.ColdCredential.Credential
			member, err := ls.CommitteeMember(coldHash)
			if err != nil {
				return CommitteeMemberLookupError{Credential: coldHash, Err: err}
			}
			if member == nil && hasCommitteeState {
				return NotCommitteeMemberError{Credential: coldHash, Operation: "resign"}
			}
		}
	}
	return nil
}

// UtxoValidateMalformedReferenceScripts checks that any reference scripts in
// transaction outputs are well-formed and can be deserialized.
func UtxoValidateMalformedReferenceScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var malformedHashes []common.ScriptHash

	for _, output := range tx.Outputs() {
		scriptRef := output.ScriptRef()
		if scriptRef == nil {
			continue
		}

		// Check if the script can be decoded properly
		var innerScript []byte
		var isPlutus bool
		var scriptBytes []byte
		var scriptHash common.ScriptHash

		switch s := scriptRef.(type) {
		case common.PlutusV1Script:
			isPlutus = true
			scriptBytes = []byte(s)
			scriptHash = s.Hash()
		case common.PlutusV2Script:
			isPlutus = true
			scriptBytes = []byte(s)
			scriptHash = s.Hash()
		case common.PlutusV3Script:
			isPlutus = true
			scriptBytes = []byte(s)
			scriptHash = s.Hash()
		default:
			// Native scripts don't need UPLC validation
			continue
		}

		if isPlutus {
			// Decode the outer CBOR wrapper to get the actual script bytes
			if _, err := cbor.Decode(scriptBytes, &innerScript); err != nil {
				malformedHashes = append(malformedHashes, scriptHash)
				continue
			}
			// Try to decode as UPLC program
			if _, err := syn.Decode[syn.DeBruijn](innerScript); err != nil {
				malformedHashes = append(malformedHashes, scriptHash)
			}
		}
	}

	if len(malformedHashes) > 0 {
		return common.MalformedReferenceScriptsError{
			ScriptHashes: malformedHashes,
		}
	}
	return nil
}
