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

package shelley

// Related files:
//   - errors.go: Error types returned by these validation rules
//   - ledger/common/rules.go: Shared validation utilities and base rules
//   - ledger/common/state.go: LedgerState interface used by validators
//   - internal/test/conformance/: Test vectors for validation rules
//   - Later eras (allegra, mary, alonzo, etc.) delegate to these rules

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"unicode/utf8"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
)

var utxoValidationRuleDescriptors = []common.UtxoValidationRuleDescriptor{
	{Id: common.UtxoValidationRuleMetadata, Validator: UtxoValidateMetadata},
	{
		Id:        common.UtxoValidationRuleRequiredVKeyWitnesses,
		Validator: UtxoValidateRequiredVKeyWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleSignatures,
		Validator: UtxoValidateSignatures,
	},
	{
		Id:        common.UtxoValidationRuleProtocolParameterUpdates,
		Validator: UtxoValidateProtocolParameterUpdates,
	},
	{
		Id:        common.UtxoValidationRuleTimeToLive,
		Validator: UtxoValidateTimeToLive,
	},
	{
		Id:        common.UtxoValidationRuleInputSetEmpty,
		Validator: UtxoValidateInputSetEmptyUtxo,
	},
	{
		Id:        common.UtxoValidationRuleNoDuplicateInputs,
		Validator: UtxoValidateNoDuplicateInputs,
	},
	{
		Id:        common.UtxoValidationRuleFeeTooSmall,
		Validator: UtxoValidateFeeTooSmallUtxo,
	},
	{
		Id:        common.UtxoValidationRuleBadInputs,
		Validator: UtxoValidateBadInputsUtxo,
	},
	{
		Id:        common.UtxoValidationRuleNativeScripts,
		Validator: UtxoValidateNativeScripts,
	},
	{
		Id:        common.UtxoValidationRuleScriptWitnesses,
		Validator: UtxoValidateScriptWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetwork,
		Validator: UtxoValidateWrongNetwork,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetworkWithdrawal,
		Validator: UtxoValidateWrongNetworkWithdrawal,
	},
	{
		Id:        common.UtxoValidationRuleValueNotConserved,
		Validator: UtxoValidateValueNotConservedUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutputTooSmall,
		Validator: UtxoValidateOutputTooSmallUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
		Validator: UtxoValidateOutputBootAddrAttrsTooBig,
	},
	{
		Id:        common.UtxoValidationRuleMaxTxSize,
		Validator: UtxoValidateMaxTxSizeUtxo,
	},
	{
		Id:        common.UtxoValidationRuleDelegation,
		Validator: UtxoValidateDelegation,
	},
	{
		Id:        common.UtxoValidationRuleMIRGenesisQuorum,
		Validator: UtxoValidateMIRGenesisQuorum,
	},
	{
		Id:        common.UtxoValidationRuleWithdrawals,
		Validator: UtxoValidateWithdrawals,
	},
	{
		Id:        common.UtxoValidationRulePoolCertificates,
		Validator: UtxoValidatePoolCertificates,
	},
}

// UtxoValidationRuleDescriptors returns the authoritative ordered rule
// descriptors. The returned slice is a defensive copy and may be modified by
// callers without changing package state.
func UtxoValidationRuleDescriptors() []common.UtxoValidationRuleDescriptor {
	return append(
		[]common.UtxoValidationRuleDescriptor(nil),
		utxoValidationRuleDescriptors...,
	)
}

// UtxoValidationRules is initialized from the authoritative descriptors. It
// remains mutable for compatibility; mutations are not reflected by
// UtxoValidationRuleDescriptors.
var UtxoValidationRules = common.MustUtxoValidationRulesFromDescriptors(
	utxoValidationRuleDescriptors,
)

// UtxoValidateTimeToLive ensures that the current tip slot is not after the
// specified TTL value, matching cardano-ledger's validateTimeToLive
// (Cardano.Ledger.Shelley.Rules.Utxo): failureUnless (ttl >= slot).
//
// The reference has no exemption for a zero TTL, because key 3 is mandatory
// in Shelley and zero is a real bound that expires at every slot above zero.
// Presence is consulted rather than the value so that a transaction type
// unable to express presence keeps the legacy "nonzero means present"
// behavior.
func UtxoValidateTimeToLive(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	ttl, present := common.TransactionValidityIntervalUpperBound(tx)
	if !present || ttl >= slot {
		return nil
	}
	return ExpiredUtxoError{
		Ttl:  ttl,
		Slot: slot,
	}
}

// UtxoValidateInputSetEmptyUtxo ensures that the input set is not empty
func UtxoValidateInputSetEmptyUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if len(tx.Inputs()) > 0 {
		return nil
	}
	return InputSetEmptyUtxoError{}
}

// UtxoValidateScriptWitnesses checks that every required script is provided.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateScriptWitnesses(tx, ls)
}

// UtxoValidateNativeScripts evaluates native scripts in the transaction.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if scriptHash, failed := common.FirstInvalidNativeScript(tx, slot); failed {
		return NativeScriptFailedError{ScriptHash: scriptHash}
	}
	return nil
}

// UtxoValidateNoDuplicateInputs ensures that there are no duplicate inputs in any input set
func UtxoValidateNoDuplicateInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Check regular inputs
	if input := firstDuplicateInput(tx.Inputs()); input != nil {
		return DuplicateInputError{Input: input, InputType: "regular"}
	}
	// Check collateral inputs
	if input := firstDuplicateInput(tx.Collateral()); input != nil {
		return DuplicateInputError{Input: input, InputType: "collateral"}
	}
	// Check reference inputs
	if input := firstDuplicateInput(tx.ReferenceInputs()); input != nil {
		return DuplicateInputError{Input: input, InputType: "reference"}
	}
	return nil
}

type duplicateInputKey struct {
	id    common.Blake2b256
	index uint32
}

// firstDuplicateInput returns the first input that repeats an earlier one,
// compared by transaction ID and output index, or nil.
func firstDuplicateInput(
	inputs []common.TransactionInput,
) common.TransactionInput {
	if len(inputs) < 2 {
		return nil
	}
	seen := make(map[duplicateInputKey]struct{}, len(inputs))
	for _, input := range inputs {
		key := duplicateInputKey{id: input.Id(), index: input.Index()}
		if _, ok := seen[key]; ok {
			return input
		}
		seen[key] = struct{}{}
	}
	return nil
}

// UtxoValidateFeeTooSmallUtxo ensures that the fee is at least the calculated minimum
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
	minFeeBig := new(big.Int).SetUint64(minFee)
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	if fee.Cmp(minFeeBig) >= 0 {
		return nil
	}
	return FeeTooSmallUtxoError{
		Provided: fee,
		Min:      minFeeBig,
	}
}

// UtxoValidateBadInputsUtxo ensures that all inputs are present in the ledger state (have not been spent)
func UtxoValidateBadInputsUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var badInputs []common.TransactionInput
	for _, tmpInput := range tx.Inputs() {
		_, err := ls.UtxoById(tmpInput)
		if err != nil {
			badInputs = append(badInputs, tmpInput)
		}
	}
	if len(badInputs) == 0 {
		return nil
	}
	return BadInputsUtxoError{
		Inputs: badInputs,
	}
}

// UtxoValidateWrongNetwork ensures that all output addresses use the correct network ID
func UtxoValidateWrongNetwork(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	networkId := ls.NetworkId()
	badAddrs := []common.Address{}
	for _, tmpOutput := range common.TransactionOutputsAndCollateralReturn(tx) {
		addr := tmpOutput.Address()
		if addr.NetworkId() == networkId {
			continue
		}
		badAddrs = append(badAddrs, addr)
	}
	if len(badAddrs) == 0 {
		return nil
	}
	return WrongNetworkError{
		NetId: networkId,
		Addrs: badAddrs,
	}
}

// UtxoValidateWrongNetworkWithdrawal ensures that all withdrawal addresses use the correct network ID
func UtxoValidateWrongNetworkWithdrawal(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	networkId := ls.NetworkId()
	badAddrs := []common.Address{}
	for addr := range tx.Withdrawals() {
		if addr.NetworkId() == networkId {
			continue
		}
		badAddrs = append(badAddrs, *addr)
	}
	if len(badAddrs) == 0 {
		return common.ValidateWithdrawalAddresses(tx.Withdrawals())
	}
	return WrongNetworkWithdrawalError{
		NetId: networkId,
		Addrs: badAddrs,
	}
}

// UtxoValidateValueNotConservedUtxo ensures that the consumed value equals the produced value
func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ShelleyProtocolParameters)
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
	seenPoolRegistrations := make(map[common.PoolKeyHash]struct{})
	type stakeCredentialKey struct {
		credType uint
		hash     string
	}
	keyForCredential := func(cred common.Credential) stakeCredentialKey {
		return stakeCredentialKey{
			credType: cred.CredType,
			hash:     string(cred.Credential[:]),
		}
	}
	stakeRegistered := make(map[stakeCredentialKey]bool)
	stakeDeposits := make(map[stakeCredentialKey]uint64)
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.StakeDeregistrationCertificate:
			cred := tmpCert.StakeCredential
			key := keyForCredential(cred)
			registered, ok := stakeRegistered[key]
			if !ok {
				registered = ls.IsStakeCredentialRegistered(cred)
				if registered {
					deposit, err := common.StakeCredentialDepositOrDefault(ls, cred, uint64(tmpPparams.KeyDeposit))
					if err != nil {
						return err
					}
					stakeDeposits[key] = deposit
				}
			}
			if registered {
				consumedValue.Add(consumedValue, new(big.Int).SetUint64(stakeDeposits[key]))
				stakeRegistered[key] = false
			}
		case *common.StakeRegistrationCertificate:
			key := keyForCredential(tmpCert.StakeCredential)
			stakeRegistered[key] = true
			stakeDeposits[key] = uint64(tmpPparams.KeyDeposit)
			// Note: PoolRetirementCertificate does NOT refund the deposit as part of the transaction.
			// Pool deposits are refunded to the reward account at the end of the retiring epoch.
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
			operator := common.Blake2b224(tmpCert.Operator)
			if _, seen := seenPoolRegistrations[operator]; seen {
				continue
			}
			seenPoolRegistrations[operator] = struct{}{}
			depositDue, err := common.PoolRegistrationDepositDue(
				ls, slot, operator,
			)
			if err != nil {
				return err
			}
			if depositDue {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.PoolDeposit)))
			}
		case *common.StakeRegistrationCertificate:
			producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
		}
	}
	if consumedValue.Cmp(producedValue) == 0 {
		return nil
	}
	return ValueNotConservedUtxoError{
		Consumed: consumedValue,
		Produced: producedValue,
	}
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

// UtxoValidateOutputTooSmallUtxo ensures that outputs have at least the minimum value
func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	minCoin, err := MinCoinTxOut(tx, pp)
	if err != nil {
		return err
	}
	minCoinBig := new(big.Int).SetUint64(minCoin)
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
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
	return OutputTooSmallUtxoError{
		Outputs: badOutputs,
	}
}

// UtxoValidateOutputBootAddrAttrsTooBig ensures that bootstrap (Byron) addresses don't have attributes that are too large
func UtxoValidateOutputBootAddrAttrsTooBig(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	badOutputs := []common.TransactionOutput{}
	for _, tmpOutput := range common.TransactionOutputsAndCollateralReturn(tx) {
		addr := tmpOutput.Address()
		if addr.Type() != common.AddressTypeByron {
			continue
		}
		if addr.ByronAttr().ExtraSize() <= 64 {
			continue
		}
		badOutputs = append(badOutputs, tmpOutput)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return OutputBootAddrAttrsTooBigError{
		Outputs: badOutputs,
	}
}

// UtxoValidateMaxTxSizeUtxo ensures that a transaction does not exceed the max size
func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ShelleyProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	txSize, sizeErr := common.TxSize(tx)
	if sizeErr != nil {
		return sizeErr
	}
	if uint(txSize) <= tmpPparams.MaxTxSize {
		return nil
	}
	return MaxTxSizeUtxoError{
		TxSize:    uint(txSize),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
}

// UtxoValidateRequiredVKeyWitnesses ensures each required signer key hash is
// provided by a vkey witness or a bootstrap witness address root
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRequiredVKeyWitnesses(tx)
}

// UtxoValidateMIRGenesisQuorum ensures a move instantaneous rewards
// certificate is authorized by a quorum of the current genesis delegates.
//
// Not yet registered in any era's UtxoValidationRules. The rule fails closed
// for a ledger state that does not implement common.GenesisDelegationState, so
// registering it before a consumer implements the capability would stop a
// syncing node on the first MIR certificate in Shelley-through-Babbage
// history. Register it once consumers are wired: blinklabs-io/dingo#3748.
func UtxoValidateMIRGenesisQuorum(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateMIRGenesisQuorum(tx, slot, ls)
}

// UtxoValidateProtocolParameterUpdates validates classic PPUP authorization,
// target epoch, and version-dependent cost-model updates.
func UtxoValidateProtocolParameterUpdates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateClassicProtocolParameterUpdates(tx, slot, ls, pp)
}

// MinFeeTx calculates the minimum required fee for a transaction based on
// protocol parameters. The fee-relevant transaction size is determined by
// common.TxSizeForFee, which uses the original on-wire CBOR length. For
// pre-Alonzo eras the full CBOR length is the fee-relevant size; for Alonzo+
// the IsValid byte is subtracted.
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ShelleyProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txSize, err := common.TxSizeForFee(tx)
	if err != nil {
		return 0, err
	}
	minFee, err := common.CalculateMinFee(
		txSize,
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
	if err != nil {
		return 0, err
	}
	return minFee, nil
}

// MinCoinTxOut calculates the minimum coin for a transaction output based on protocol parameters
func MinCoinTxOut(
	_ common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ShelleyProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	minCoinTxOut := uint64(tmpPparams.MinUtxoValue)
	return minCoinTxOut, nil
}

// ValidateMetadataContent checks that metadata contains valid data according to Cardano rules.
// Like validMetadatum in cardano-ledger, it places no bound on nesting depth;
// depth is bounded only by the decoder.
func ValidateMetadataContent(metadata common.TransactionMetadatum) error {
	if metadata == nil {
		return nil
	}
	return validateMetadatumContent(metadata)
}

func validateMetadatumContent(md common.TransactionMetadatum) error {
	switch m := md.(type) {
	case common.MetaText:
		if !utf8.ValidString(m.Value) {
			return errors.New("metadata contains invalid UTF-8 text")
		}
		// Cardano spec: metadata text strings must not exceed 64 bytes
		if len(m.Value) > 64 {
			return fmt.Errorf("metadata text exceeds 64 byte limit: %d bytes", len(m.Value))
		}
	case common.MetaBytes:
		// Cardano spec: metadata byte strings must not exceed 64 bytes
		if len(m.Value) > 64 {
			return fmt.Errorf("metadata byte string exceeds 64 byte limit: %d bytes", len(m.Value))
		}
	case common.MetaInt:
		if m.Value == nil {
			return errors.New("metadata contains nil integer value")
		}
	case common.MetaList:
		for _, item := range m.Items {
			if err := validateMetadatumContent(item); err != nil {
				return err
			}
		}
	case common.MetaMap:
		for _, pair := range m.Pairs {
			if err := validateMetadatumContent(pair.Key); err != nil {
				return err
			}
			if err := validateMetadatumContent(pair.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// UtxoValidateMetadata validates that auxiliary data (metadata) matches the hash in transaction body
func UtxoValidateMetadata(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	bodyAuxDataHash := tx.AuxDataHash()
	txAuxData := tx.Metadata()
	var rawAuxData []byte
	if aux := tx.AuxiliaryData(); aux != nil {
		ac := aux.Cbor()
		// Treat single-byte CBOR simple-value placeholders as absence
		// of auxiliary data so we can fall back to block-level
		// metadata stored in TransactionMetadataSet. Historically some
		// inputs used CBOR null (0xf6) as a placeholder; we've observed
		// producers that use CBOR true (0xf5) or false (0xf4) as a
		// placeholder as well. If the auxiliary-data is exactly one
		// simple-value byte, ignore it here.
		if len(ac) > 0 {
			if len(ac) != 1 ||
				(ac[0] != 0xF6 && ac[0] != 0xF5 && ac[0] != 0xF4) {
				rawAuxData = ac
			}
		}
	}
	if len(rawAuxData) == 0 && txAuxData != nil {
		rawAuxData = txAuxData.Cbor()
	}

	// Case 1: Neither body hash nor aux data present - OK
	if bodyAuxDataHash == nil && txAuxData == nil && len(rawAuxData) == 0 {
		return nil
	}

	// Case 2: Body has hash but no aux data provided - error
	// We rely on rawAuxData for hashing; if it's empty while body declares a hash,
	// treat it as missing metadata regardless of txAuxData pointer presence.
	if bodyAuxDataHash != nil && len(rawAuxData) == 0 {
		return common.MissingTransactionMetadataError{
			Hash: *bodyAuxDataHash,
		}
	}

	// Case 3: Aux data provided but body has no hash - error
	if bodyAuxDataHash == nil && len(rawAuxData) > 0 {
		actualHash := common.Blake2b256Hash(rawAuxData)
		return common.MissingTransactionAuxiliaryDataHashError{
			Hash: actualHash,
		}
	}

	// Case 4: Both present - verify hash matches
	// Use raw auxiliary data (includes scripts) for hashing, not just metadata
	if bodyAuxDataHash != nil && len(rawAuxData) > 0 {
		actualHash := common.Blake2b256Hash(rawAuxData)

		if *bodyAuxDataHash != actualHash {
			return common.ConflictingMetadataHashError{
				Supplied: *bodyAuxDataHash,
				Expected: actualHash,
			}
		}

		// Validate metadata content
		if txAuxData != nil {
			if err := ValidateMetadataContent(txAuxData); err != nil {
				return err
			}
		}
	}

	return nil
}

// UtxoValidateDelegation validates delegation certificates against ledger state.
// Certificates apply in order, each seeing the state earlier ones produced.
// It checks:
//   - StakeRegistrationCertificate: the credential is not registered
//     (StakeKeyAlreadyRegisteredError)
//   - StakeDeregistrationCertificate: the credential is registered
//     (StakeKeyNotRegisteredError) and its reward balance, after the
//     transaction's withdrawals, is zero (StakeKeyNonZeroAccountBalanceError)
//   - StakeDelegationCertificate: the pool and the stake credential are
//     registered
//
// It also enforces the DELEG predicates of move instantaneous rewards
// certificates; see validateMirCertificates.
func UtxoValidateDelegation(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if err := validateMirCertificates(tx, slot, ls, pp); err != nil {
		return err
	}

	// Track the effective registration state produced by earlier
	// certificates. Credential type is part of the identity, and a false
	// entry must override a registration in the starting ledger state.
	type stakeCredentialKey struct {
		credType uint
		hash     common.Blake2b224
	}
	stakeKey := func(cred common.Credential) stakeCredentialKey {
		return stakeCredentialKey{
			credType: cred.CredType,
			hash:     cred.Credential,
		}
	}
	inTxStakeState := make(map[stakeCredentialKey]bool)
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)

	isStakeRegistered := func(cred common.Credential) bool {
		if registered, ok := inTxStakeState[stakeKey(cred)]; ok {
			return registered
		}
		return ls.IsStakeCredentialRegistered(cred)
	}

	isPoolRegistered := func(poolKeyHash common.PoolKeyHash) bool {
		return ls.IsPoolRegistered(poolKeyHash) || inTxPoolRegs[poolKeyHash]
	}

	// Withdrawals are applied before the certificates, and
	// UtxoValidateWithdrawals rejects one that does not drain its account,
	// so a credential the transaction withdraws from has an empty account at
	// deregistration.
	withdrawn := make(map[stakeCredentialKey]struct{})
	for addr := range tx.Withdrawals() {
		if addr == nil {
			continue
		}
		// An invalid reward address is the withdrawals rule's to reject.
		if cred, err := addr.RewardAccountCredential(); err == nil {
			withdrawn[stakeKey(cred)] = struct{}{}
		}
	}

	var genesis *genesisDelegations
	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *common.StakeRegistrationCertificate:
			if isStakeRegistered(c.StakeCredential) {
				return StakeKeyAlreadyRegisteredError{
					Credential: c.StakeCredential,
				}
			}
			inTxStakeState[stakeKey(c.StakeCredential)] = true

		case *common.StakeDeregistrationCertificate:
			if !isStakeRegistered(c.StakeCredential) {
				return StakeKeyNotRegisteredError{
					Credential: c.StakeCredential,
				}
			}
			// An account registered earlier in this transaction holds no
			// rewards, and a ledger-state balance is only live for an
			// account that existed before it.
			_, openedInTx := inTxStakeState[stakeKey(c.StakeCredential)]
			_, drained := withdrawn[stakeKey(c.StakeCredential)]
			if !openedInTx && !drained {
				balance, err := ls.RewardAccountBalance(c.StakeCredential)
				if err != nil {
					return err
				}
				if balance != nil && *balance != 0 {
					return StakeKeyNonZeroAccountBalanceError{
						Credential: c.StakeCredential,
						Balance:    *balance,
					}
				}
			}
			inTxStakeState[stakeKey(c.StakeCredential)] = false

		case *common.GenesisKeyDelegationCertificate:
			if genesis == nil {
				genesis = &genesisDelegations{}
				if err := genesis.load(ls, slot); err != nil {
					return err
				}
			}
			if err := genesis.apply(c); err != nil {
				return err
			}

		case *common.PoolRegistrationCertificate:
			inTxPoolRegs[c.Operator] = true

		case *common.PoolRetirementCertificate:
			// Retirement does not remove the pool from the active pool set.
			// POOLREAP removes it only after the retirement epoch.

		case *common.StakeDelegationCertificate:
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			if c.StakeCredential != nil && !isStakeRegistered(*c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: *c.StakeCredential}
			}
		}
	}
	return nil
}

// genesisDelegations is the genesis delegation state a transaction's genesis
// key delegation certificates are checked against: the delegations in force,
// and those scheduled by earlier certificates.
type genesisDelegations struct {
	current []common.GenesisDelegation
	future  []common.GenesisDelegation
	// scheduled holds this slot's delegations, keyed by genesis key.
	scheduled []common.GenesisDelegation
}

func (g *genesisDelegations) load(
	ls common.LedgerState,
	slot uint64,
) error {
	current, future, scheduled, known, err := common.GenesisDelegationsFor(
		ls,
		slot,
	)
	if err != nil {
		return err
	}
	if !known {
		return common.GenesisDelegationRegistryStateUnavailableError{}
	}
	g.current = current
	g.future = future
	g.scheduled = scheduled
	return nil
}

// apply checks a genesis key delegation certificate and schedules its
// delegation, replacing one this slot scheduled for the same genesis key. The
// delegate and VRF key may not belong to any other genesis key, current or
// scheduled; the certificate's own genesis key may reuse its keys.
//
// Reference: the GenesisDelegTxCert case of delegationTransition,
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs.
func (g *genesisDelegations) apply(
	cert *common.GenesisKeyDelegationCertificate,
) error {
	genesisKey, err := common.NewBlake2b224Checked(cert.GenesisHash)
	if err != nil {
		return fmt.Errorf("genesis key hash: %w", err)
	}
	delegation, err := common.GenesisDelegationFromCertificate(cert)
	if err != nil {
		return err
	}
	inMapping := false
	for _, current := range g.current {
		if current.GenesisKeyHash == genesisKey {
			inMapping = true
			break
		}
	}
	if !inMapping {
		return GenesisKeyNotInMappingError{GenesisKeyHash: genesisKey}
	}
	var otherDelegates []common.Blake2b224
	var otherVrfs []common.Blake2b256
	for _, others := range [][]common.GenesisDelegation{
		g.current,
		g.future,
		g.scheduled,
	} {
		for _, other := range others {
			if other.GenesisKeyHash == genesisKey {
				continue
			}
			otherDelegates = append(otherDelegates, other.DelegateKeyHash)
			otherVrfs = append(otherVrfs, other.VrfKeyHash)
		}
	}
	if slices.Contains(otherDelegates, delegation.DelegateKeyHash) {
		return DuplicateGenesisDelegateError{
			DelegateKeyHash: delegation.DelegateKeyHash,
		}
	}
	if slices.Contains(otherVrfs, delegation.VrfKeyHash) {
		return DuplicateGenesisVRFError{VrfKeyHash: delegation.VrfKeyHash}
	}
	g.scheduled = common.ScheduleGenesisDelegation(g.scheduled, delegation)
	return nil
}

// validateMirCertificates enforces the DELEG predicates of move instantaneous
// rewards certificates, in certificate order. Each certificate must come
// before the cutoff slot, the first slot of the next epoch minus the stability
// window (MIRCertificateTooLateinEpochError), and the ledger state must
// provide common.StabilityWindowState and
// common.InstantaneousRewardsTotalsState; without them the certificate cannot
// be checked and is rejected.
//
// Before the Alonzo hard fork a pot-to-pot transfer
// (MIRTransferNotCurrentlyAllowed) and a negative delta
// (MIRNegativesNotCurrentlyAllowed) are rejected, and a credential's delta
// replaces its pending rewards. From Alonzo a delta adds to them and is
// rejected when it drives the credential's pending rewards in its pot below
// zero (MIRProducesNegativeUpdate); the pending rewards are those the ledger
// state reports through common.PendingInstantaneousRewardsState plus the
// deltas of earlier certificates in the block and transaction. A state that
// does not report them but reports nothing accumulated in the pot through
// common.InstantaneousRewardsTotalsState has nothing pending. Otherwise a
// pre-Alonzo certificate, or a delta those deltas do not cover, is rejected
// with PendingInstantaneousRewardsUnavailableError.
//
// The rewards accumulated for stake credentials in a pot may not exceed the
// pot (InsufficientForInstantaneousRewardsError), and a pot-to-pot transfer
// may not exceed the pot net of its accumulated rewards
// (InsufficientForTransferError). From Alonzo the pot includes its pending
// pot-to-pot transfers. Totals are summed as arbitrary precision integers so
// an oversized delta is rejected rather than wrapped.
//
// A negative pot-to-pot amount (MIRNegativeTransfer) cannot occur: the amount
// is an unsigned integer on the wire and in
// common.MoveInstantaneousRewardsCertificateReward.
//
// Reference: delegationTransition, checkSlotNotTooLate and
// updateReservesAndTreasury, eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs;
// availableAfterMIR, eras/shelley/impl/src/Cardano/Ledger/Shelley/LedgerState/NewEpochState.hs.
func validateMirCertificates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	mirCerts := make([]*common.MoveInstantaneousRewardsCertificate, 0)
	for _, cert := range tx.Certificates() {
		if c, ok := cert.(*common.MoveInstantaneousRewardsCertificate); ok &&
			c != nil {
			mirCerts = append(mirCerts, c)
		}
	}
	if len(mirCerts) == 0 {
		// Leave transactions that carry no MIR certificate untouched,
		// including their protocol parameters, so the rule cannot reject
		// anything the DELEG transition would never have seen.
		return nil
	}
	versionedPparams, ok := pp.(interface {
		ProtocolMajorVersion() uint
	})
	if !ok {
		return errors.New("pparams are not expected type")
	}
	allowed := common.MirTransferAllowed(
		versionedPparams.ProtocolMajorVersion(),
	)
	if err := validateMirNotTooLate(slot, ls); err != nil {
		return err
	}
	type pendingKey struct {
		source   uint
		credType uint
		hash     common.Blake2b224
	}
	type pendingTotal struct {
		amount *big.Int
		known  bool
	}
	type potTotals struct {
		accumulated *big.Int
		transfer    *big.Int
	}
	pending := make(map[pendingKey]pendingTotal)
	pots := make(map[uint]*potTotals)
	loadPot := func(source uint) (*potTotals, error) {
		if source > 1 {
			return nil, fmt.Errorf("invalid MIR source pot: %d", source)
		}
		if pot, ok := pots[source]; ok {
			return pot, nil
		}
		accumulated, transfer, known, err := common.InstantaneousRewardsTotalsFor(
			ls,
			source,
		)
		if err != nil {
			return nil, err
		}
		if !known {
			return nil, common.InstantaneousRewardsTotalsStateUnavailableError{}
		}
		pot := &potTotals{accumulated: accumulated, transfer: transfer}
		pots[source] = pot
		return pot, nil
	}
	potAmount := func(source uint) *big.Int {
		adaPots := ls.GetAdaPots()
		if source == 0 {
			return new(big.Int).SetUint64(adaPots.Reserves)
		}
		return new(big.Int).SetUint64(adaPots.Treasury)
	}
	for _, cert := range mirCerts {
		reward := cert.Reward
		if reward.Rewards == nil {
			if !allowed {
				return MIRTransferNotCurrentlyAllowedError{
					Source: reward.Source,
					Amount: reward.OtherPot,
				}
			}
			pot, err := loadPot(reward.Source)
			if err != nil {
				return err
			}
			other, err := loadPot(1 - reward.Source)
			if err != nil {
				return err
			}
			available := potAmount(reward.Source)
			available.Add(available, pot.transfer)
			available.Sub(available, pot.accumulated)
			amount := new(big.Int).SetUint64(reward.OtherPot)
			if amount.Cmp(available) > 0 {
				return InsufficientForTransferError{
					Source:    reward.Source,
					Amount:    reward.OtherPot,
					Available: available,
				}
			}
			pot.transfer.Sub(pot.transfer, amount)
			other.transfer.Add(other.transfer, amount)
			continue
		}
		pot, err := loadPot(reward.Source)
		if err != nil {
			return err
		}
		// replaced is what the certificate's credentials held before it,
		// which a pre-Alonzo certificate overwrites; sum is what it sets
		// them to, or adds to them from Alonzo.
		replaced := new(big.Int)
		sum := new(big.Int)
		for cred, delta := range reward.Rewards {
			if delta == nil {
				continue
			}
			var tmpCred common.Credential
			if cred != nil {
				tmpCred = *cred
			}
			if !allowed && delta.Sign() < 0 {
				return MIRNegativesNotCurrentlyAllowedError{
					Credential: tmpCred,
					Delta:      new(big.Int).Set(delta),
				}
			}
			key := pendingKey{
				source:   reward.Source,
				credType: tmpCred.CredType,
				hash:     tmpCred.Credential,
			}
			total, seen := pending[key]
			if !seen {
				amount, known, err := common.PendingInstantaneousRewardsFor(
					ls,
					reward.Source,
					tmpCred,
				)
				if err != nil {
					return err
				}
				total = pendingTotal{amount: amount, known: known}
			}
			sum.Add(sum, delta)
			if !allowed {
				if !total.known {
					return PendingInstantaneousRewardsUnavailableError{
						Credential: tmpCred,
						Source:     reward.Source,
						Delta:      new(big.Int).Set(delta),
					}
				}
				replaced.Add(replaced, total.amount)
				pending[key] = pendingTotal{
					amount: new(big.Int).Set(delta),
					known:  total.known,
				}
				continue
			}
			next := new(big.Int).Add(total.amount, delta)
			if next.Sign() < 0 {
				if !total.known {
					return PendingInstantaneousRewardsUnavailableError{
						Credential: tmpCred,
						Source:     reward.Source,
						Delta:      new(big.Int).Set(delta),
					}
				}
				return MIRProducesNegativeUpdateError{
					Credential:   tmpCred,
					Source:       reward.Source,
					Pending:      new(big.Int).Set(total.amount),
					Delta:        new(big.Int).Set(delta),
					PendingAfter: next,
				}
			}
			pending[key] = pendingTotal{amount: next, known: total.known}
		}
		required := new(big.Int).Sub(pot.accumulated, replaced)
		required.Add(required, sum)
		available := potAmount(reward.Source)
		if allowed {
			available.Add(available, pot.transfer)
		}
		if required.Cmp(available) > 0 {
			return InsufficientForInstantaneousRewardsError{
				Source:    reward.Source,
				Required:  required,
				Available: available,
			}
		}
		pot.accumulated = required
	}
	return nil
}

// validateMirNotTooLate rejects a slot at or after the first slot of the next
// epoch minus the stability window.
func validateMirNotTooLate(slot uint64, ls common.LedgerState) error {
	windowState, ok := common.StabilityWindowStateFor(ls)
	if !ok {
		return common.StabilityWindowStateUnavailableError{}
	}
	nextEpoch, err := windowState.NextEpochFirstSlot(slot)
	if err != nil {
		return err
	}
	window, err := windowState.StabilityWindow()
	if err != nil {
		return err
	}
	// A window longer than the epoch leaves no slot for a certificate.
	var cutoff uint64
	if nextEpoch > window {
		cutoff = nextEpoch - window
	}
	if slot >= cutoff {
		return MIRCertificateTooLateinEpochError{Slot: slot, Cutoff: cutoff}
	}
	return nil
}

// UtxoValidateWithdrawals validates withdrawals against ledger state.
// Before Dijkstra, every withdrawal must drain a registered reward account's
// current balance exactly. Dijkstra permits partial withdrawals when the
// transaction does not use Plutus V1-V3, but no withdrawal may exceed the
// account balance.
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
	if err := common.ValidateWithdrawalAddresses(withdrawals); err != nil {
		return err
	}

	requireExactAmount := true
	useWithdrawalExactMode := false
	if modeProvider, ok := tx.(interface {
		WithdrawalRequiresExactAmount() (bool, bool)
	}); ok {
		exact, configured := modeProvider.WithdrawalRequiresExactAmount()
		if configured {
			requireExactAmount = exact
			useWithdrawalExactMode = true
		}
	}
	if !useWithdrawalExactMode {
		if versionedPparams, ok := pp.(interface {
			ProtocolMajorVersion() uint
		}); ok {
			if versionedPparams.ProtocolMajorVersion() >=
				common.ProtocolVersionDijkstra {
				view, err := script.NewTxScriptView(tx, ls)
				if err != nil {
					return err
				}
				requireExactAmount = view.NeedsAny(func(s common.Script) bool {
					version, ok := common.PlutusScriptVersion(s)
					return ok && version <= 2
				})
			}
		}
	}

	for addr, amount := range withdrawals {
		cred, err := addr.RewardAccountCredential()
		if err != nil {
			return err
		}

		balance, err := ls.RewardAccountBalance(cred)
		if err != nil {
			return err
		}
		if balance == nil {
			return WithdrawalFromUnregisteredRewardAccountError{
				RewardAddress: *addr,
			}
		}
		amountValid := amount != nil && amount.IsUint64()
		if amountValid {
			if requireExactAmount {
				amountValid = amount.Uint64() == *balance
			} else {
				amountValid = amount.Uint64() <= *balance
			}
		}
		if !amountValid {
			var provided *big.Int
			if amount != nil {
				provided = new(big.Int).Set(amount)
			}
			return IncorrectWithdrawalAmountError{
				RewardAddress: *addr,
				Provided:      provided,
				Balance:       *balance,
			}
		}
	}
	return nil
}

// UtxoValidatePoolCertificates applies the Shelley POOL rule to every stake
// pool certificate in a transaction, in certificate order.
//
// Reference: poolTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs. Every later era
// re-exports that transition unchanged: Rules/Pool.hs in eras/allegra,
// eras/mary, eras/alonzo, eras/babbage, eras/conway and eras/dijkstra each
// declare only the EraRuleFailure and EraRuleEvent instances and import
// Cardano.Ledger.Shelley.Rules. So every era from Shelley onwards runs the
// same predicates and differs only in the protocol version that gates them.
//
// Predicates enforced here, all from that file:
//
//   - StakePoolCostTooLowPOOL: a registration's cost must be at least
//     minPoolCost. Unconditional in every era.
//   - WrongNetworkPOOL: a registration's reward account must be on the
//     ledger's network. Gated on major protocol version > 4
//     (hardforkAlonzoValidatePoolAccountAddressNetID).
//   - PoolMedataHashTooBig: a registration's metadata hash may be at most 32
//     bytes. Gated on major protocol version > 4
//     (SoftForks.restrictPoolMetadataHash). The hash is an unbounded byte
//     string on the wire, so this is a rule predicate rather than a decode
//     constraint.
//   - VRFKeyHashAlreadyRegistered: a registration may not claim a VRF key hash
//     another pool holds. Gated on major protocol version > 10
//     (hardforkConwayDisallowDuplicatedVRFKeys).
//   - StakePoolNotRegisteredOnKeyPOOL: a retirement must name a registered
//     pool.
//   - StakePoolRetirementWrongEpochPOOL: a retirement epoch e must satisfy
//     cEpoch < e <= cEpoch + eMax.
//
// The registration and re-registration state updates poolTransition performs
// (psStakePools, psFutureStakePoolParams, psRetiring, psVRFKeyHashes) are
// ledger-state transitions rather than predicates, and belong to the consumer
// applying the certificate.
//
// The era-specific pool metadata URL bound (64 bytes before protocol version 9,
// 128 thereafter) is enforced while decoding pool metadata, including genesis
// pool configuration, rather than by the reference POOL predicate. The relay
// dns_name bound (64 bytes before protocol version 9, 128 thereafter) is also a
// decoder bound in the reference; the wire decoder here carries no protocol
// version, so registration applies it through
// common.ValidatePoolRelaysForProtocolVersion.
func UtxoValidatePoolCertificates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// A phase-2-invalid transaction never reaches the POOL rule. From
	// Alonzo onwards ledgerTransition runs DELEGS, and so DELPL and POOL,
	// only for a phase-2-valid transaction:
	//
	//	certState' <-
	//	  if tx ^. isPhase2ValidTxL == Phase2Valid
	//	    then ... trans @(EraRule "DELEGS" era) ...
	//	    else pure certState
	//
	// in eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Rules/Ledger.hs. Its
	// certificates are not applied, so none of the predicates below may
	// reject it. IsValid reports true in the eras before the phase-2
	// concept exists, so this is inert for Shelley through Mary.
	if !tx.IsValid() {
		return nil
	}
	certs := tx.Certificates()
	if !hasPoolCertificate(certs) {
		// Leave transactions that carry no pool certificate untouched,
		// including their protocol parameters, so the rule cannot reject
		// anything the POOL transition would never have seen.
		return nil
	}
	poolPparams, ok := pp.(common.PoolRuleProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	protocolMajor := poolPparams.ProtocolMajorVersion()
	checkNetworkId := common.PoolAccountNetworkIdValidated(protocolMajor)
	checkVrfKeys := common.DuplicateVrfKeysDisallowed(protocolMajor)
	checkMetadataHash := common.PoolMetadataHashRestricted(protocolMajor)
	minPoolCost := poolPparams.MinPoolCostValue()
	networkId := ls.NetworkId()

	// Certificates are applied in order, so a pool registered by an earlier
	// certificate in the same transaction is registered for the purposes of
	// a later one. A retirement certificate does not undo that: the
	// reference rule only adds the pool to psRetiring and leaves it in
	// psStakePools until POOLREAP runs at the epoch boundary.
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)
	poolState := newPoolCertificateState(ls)

	for _, cert := range certs {
		switch c := cert.(type) {
		case *common.PoolRegistrationCertificate:
			if err := validatePoolRegistration(
				c,
				poolState,
				networkId,
				minPoolCost,
				protocolMajor,
				checkNetworkId,
				checkMetadataHash,
				checkVrfKeys,
			); err != nil {
				return err
			}
			inTxPoolRegs[c.Operator] = true
			if checkVrfKeys {
				if err := poolState.applyRegistration(c); err != nil {
					return err
				}
			}
		case *common.PoolRetirementCertificate:
			if err := validatePoolRetirement(
				c,
				slot,
				ls,
				poolPparams,
				inTxPoolRegs,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// poolCertificateState is the effective POOL state after the registrations
// already visited in one transaction. New pools become current immediately;
// registrations for existing pools replace their deferred parameters.
type poolCertificateState struct {
	base      common.LedgerState
	current   map[common.PoolKeyHash]*common.PoolRegistrationCertificate
	future    map[common.PoolKeyHash]*common.PoolRegistrationCertificate
	vrfOwners map[common.VrfKeyHash]common.PoolKeyHash
}

func newPoolCertificateState(ls common.LedgerState) *poolCertificateState {
	return &poolCertificateState{
		base: ls,
		current: make(
			map[common.PoolKeyHash]*common.PoolRegistrationCertificate,
		),
		future: make(
			map[common.PoolKeyHash]*common.PoolRegistrationCertificate,
		),
		vrfOwners: make(map[common.VrfKeyHash]common.PoolKeyHash),
	}
}

func (s *poolCertificateState) currentRegistration(
	pool common.PoolKeyHash,
) (*common.PoolRegistrationCertificate, error) {
	if current, ok := s.current[pool]; ok {
		return current, nil
	}
	current, _, err := s.base.PoolCurrentState(pool)
	return current, err
}

func (s *poolCertificateState) applyRegistration(
	cert *common.PoolRegistrationCertificate,
) error {
	current, err := s.currentRegistration(cert.Operator)
	if err != nil {
		return err
	}
	if current == nil {
		s.current[cert.Operator] = cert
	} else {
		if previous, ok := s.future[cert.Operator]; ok &&
			previous.VrfKeyHash != current.VrfKeyHash &&
			s.vrfOwners[previous.VrfKeyHash] == cert.Operator {
			delete(s.vrfOwners, previous.VrfKeyHash)
		}
		s.future[cert.Operator] = cert
	}
	s.vrfOwners[cert.VrfKeyHash] = cert.Operator
	return nil
}

// hasPoolCertificate reports whether any certificate is a pool registration or
// pool retirement, the two signals the POOL rule handles.
func hasPoolCertificate(certs []common.Certificate) bool {
	for _, cert := range certs {
		switch cert.(type) {
		case *common.PoolRegistrationCertificate,
			*common.PoolRetirementCertificate:
			return true
		}
	}
	return false
}

// validatePoolRegistration applies the RegPool branch of poolTransition.
func validatePoolRegistration(
	cert *common.PoolRegistrationCertificate,
	state *poolCertificateState,
	networkId uint,
	minPoolCost uint64,
	protocolMajor uint,
	checkNetworkId bool,
	checkMetadataHash bool,
	checkVrfKeys bool,
) error {
	if err := common.ValidatePoolMetadataForProtocolVersion(
		cert.PoolMetadata,
		protocolMajor,
	); err != nil {
		return err
	}
	if err := common.ValidatePoolRelaysForProtocolVersion(
		cert.Relays,
		protocolMajor,
	); err != nil {
		return err
	}

	// WrongNetworkPOOL: actualNetID == suppliedNetID.
	//
	// The supplied value is the network id in the reward account's address
	// header byte. When the certificate was not decoded from a wire
	// reward_account carrying that header there is nothing to compare, so
	// the check is skipped rather than assuming a network.
	if checkNetworkId {
		if suppliedNetworkId, known := cert.RewardAccountNetworkId(); known &&
			suppliedNetworkId != networkId {
			return WrongNetworkPoolError{
				PoolKeyHash: cert.Operator,
				Supplied:    suppliedNetworkId,
				Expected:    networkId,
			}
		}
	}

	// PoolMedataHashTooBig: sizeofByteArray (pmHash pmd) <= hashSize HASH.
	//
	// pool_metadata_hash is an unbounded byte string on the wire, so an
	// oversized hash reaches the rule instead of failing to decode.
	if checkMetadataHash && cert.PoolMetadata != nil &&
		len(cert.PoolMetadata.Hash) > common.Blake2b256Size {
		return PoolMetadataHashTooBigError{
			PoolKeyHash: cert.Operator,
			Supplied:    len(cert.PoolMetadata.Hash),
			Max:         common.Blake2b256Size,
		}
	}

	// StakePoolCostTooLowPOOL: sppCost >= minPoolCost.
	if cert.Cost < minPoolCost {
		return StakePoolCostTooLowError{
			PoolKeyHash: cert.Operator,
			Supplied:    cert.Cost,
			Min:         minPoolCost,
		}
	}

	if !checkVrfKeys {
		return nil
	}
	// VRFKeyHashAlreadyRegistered. The reference splits on whether the pool
	// is already in psStakePools: a new registration requires
	// Map.notMember sppVrf psVRFKeyHashes, while a re-registration also
	// accepts sppVrf == the pool's own registered VRF key hash. Only
	// registrations put entries into psVRFKeyHashes, so both branches
	// reduce to the same predicate here: the VRF key hash must be unused,
	// or held by this same pool.
	//
	futureState, ok := common.FuturePoolParametersStateFor(state.base)
	if !ok {
		return common.FuturePoolParametersStateUnavailableError{}
	}
	if owner, claimed := state.vrfOwners[cert.VrfKeyHash]; claimed {
		current, err := state.currentRegistration(cert.Operator)
		if err != nil {
			return err
		}
		if owner != cert.Operator || current == nil ||
			current.VrfKeyHash != cert.VrfKeyHash {
			return VrfKeyHashAlreadyRegisteredError{
				PoolKeyHash:  cert.Operator,
				VrfKeyHash:   cert.VrfKeyHash,
				RegisteredBy: owner,
			}
		}
	}
	inUse, owningPool, err := state.base.IsVrfKeyInUse(cert.VrfKeyHash)
	if err != nil {
		return err
	}
	if inUse && owningPool == cert.Operator {
		current, err := state.currentRegistration(cert.Operator)
		if err != nil {
			return err
		}
		if current == nil || current.VrfKeyHash != cert.VrfKeyHash {
			return VrfKeyHashAlreadyRegisteredError{
				PoolKeyHash:  cert.Operator,
				VrfKeyHash:   cert.VrfKeyHash,
				RegisteredBy: owningPool,
			}
		}
	}
	if inUse && owningPool != cert.Operator {
		return VrfKeyHashAlreadyRegisteredError{
			PoolKeyHash:  cert.Operator,
			VrfKeyHash:   cert.VrfKeyHash,
			RegisteredBy: owningPool,
		}
	}
	futureInUse, futureOwner, err := futureState.IsFutureVrfKeyInUse(
		cert.VrfKeyHash,
	)
	if err != nil {
		return err
	}
	if replacement, replaced := state.future[futureOwner]; replaced &&
		replacement.VrfKeyHash != cert.VrfKeyHash {
		futureInUse = false
	}
	if futureInUse {
		current, err := state.currentRegistration(cert.Operator)
		if err != nil {
			return err
		}
		if futureOwner != cert.Operator || current == nil ||
			current.VrfKeyHash != cert.VrfKeyHash {
			return VrfKeyHashAlreadyRegisteredError{
				PoolKeyHash:  cert.Operator,
				VrfKeyHash:   cert.VrfKeyHash,
				RegisteredBy: futureOwner,
			}
		}
	}
	return nil
}

// validatePoolRetirement applies the RetirePool branch of poolTransition.
func validatePoolRetirement(
	cert *common.PoolRetirementCertificate,
	slot uint64,
	ls common.LedgerState,
	pp common.PoolRuleProtocolParameters,
	inTxPoolRegs map[common.PoolKeyHash]bool,
) error {
	// StakePoolNotRegisteredOnKeyPOOL: Map.member sppId psStakePools.
	if !ls.IsPoolRegistered(cert.PoolKeyHash) &&
		!inTxPoolRegs[cert.PoolKeyHash] {
		return StakePoolNotRegisteredOnKeyError{
			PoolKeyHash: cert.PoolKeyHash,
		}
	}
	// StakePoolRetirementWrongEpochPOOL: cEpoch < e && e <= cEpoch + eMax.
	//
	// The current epoch is required to evaluate the retirement bound.
	epochState, ok := common.EpochStateFor(ls)
	if !ok {
		if cert.Epoch == 0 {
			return StakePoolRetirementWrongEpochError{
				PoolKeyHash: cert.PoolKeyHash,
				Supplied:    cert.Epoch,
			}
		}
		return common.EpochStateUnavailableError{}
	}
	currentEpoch, err := epochState.EpochForSlot(slot)
	if err != nil {
		return err
	}
	// addEpochInterval in cardano-base adds a Word32 interval to a Word64
	// epoch and cannot overflow for real values. eMax is a uint here, so
	// saturate instead of wrapping: a saturated limit leaves the upper
	// bound vacuous rather than rejecting a valid retirement.
	limitEpoch := currentEpoch + pp.PoolRetirementMaxEpoch()
	if limitEpoch < currentEpoch {
		limitEpoch = math.MaxUint64
	}
	if cert.Epoch <= currentEpoch || cert.Epoch > limitEpoch {
		return StakePoolRetirementWrongEpochError{
			PoolKeyHash:  cert.PoolKeyHash,
			Supplied:     cert.Epoch,
			CurrentEpoch: currentEpoch,
			LimitEpoch:   limitEpoch,
		}
	}
	return nil
}
