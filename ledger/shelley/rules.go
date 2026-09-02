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
	"unicode/utf8"

	"github.com/blinklabs-io/gouroboros/cbor"
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

// UtxoValidateTimeToLive ensures that the current tip slot is not after the specified TTL value
func UtxoValidateTimeToLive(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	ttl := tx.TTL()
	if ttl == 0 || ttl >= slot {
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
	seen := make(map[string]bool)
	for _, input := range tx.Inputs() {
		key := input.String()
		if seen[key] {
			return DuplicateInputError{Input: input, InputType: "regular"}
		}
		seen[key] = true
	}
	// Check collateral inputs
	seen = make(map[string]bool)
	for _, input := range tx.Collateral() {
		key := input.String()
		if seen[key] {
			return DuplicateInputError{Input: input, InputType: "collateral"}
		}
		seen[key] = true
	}
	// Check reference inputs
	seen = make(map[string]bool)
	for _, input := range tx.ReferenceInputs() {
		key := input.String()
		if seen[key] {
			return DuplicateInputError{Input: input, InputType: "reference"}
		}
		seen[key] = true
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
	for _, tmpOutput := range tx.Outputs() {
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
	for _, cert := range tx.Certificates() {
		switch cert.(type) {
		case *common.StakeDeregistrationCertificate:
			consumedValue.Add(consumedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
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
			reg, _, err := ls.PoolCurrentState(common.Blake2b224(tmpCert.Operator))
			if err != nil {
				return err
			}
			if reg == nil {
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
	for _, tmpOutput := range tx.Outputs() {
		addr := tmpOutput.Address()
		if addr.Type() != common.AddressTypeByron {
			continue
		}
		attr := addr.ByronAttr()
		attrBytes, err := cbor.Encode(attr)
		if err != nil {
			return err
		}
		if len(attrBytes) <= 64 {
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
	txBytes := tx.Cbor()
	if len(txBytes) == 0 {
		var err error
		txBytes, err = cbor.Encode(tx)
		if err != nil {
			return err
		}
	}
	if uint(len(txBytes)) <= tmpPparams.MaxTxSize {
		return nil
	}
	return MaxTxSizeUtxoError{
		TxSize:    uint(len(txBytes)),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
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

const maxMetadataDepth = 64

// validateMetadataContent checks that metadata contains valid data according to Cardano rules
func validateMetadataContent(metadata common.TransactionMetadatum) error {
	if metadata == nil {
		return nil
	}
	return validateMetadatumContent(metadata, 0)
}

func validateMetadatumContent(md common.TransactionMetadatum, depth int) error {
	if depth >= maxMetadataDepth {
		return errors.New("metadata nesting depth exceeds maximum")
	}
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
			if err := validateMetadatumContent(item, depth+1); err != nil {
				return err
			}
		}
	case common.MetaMap:
		for _, pair := range m.Pairs {
			if err := validateMetadatumContent(pair.Key, depth+1); err != nil {
				return err
			}
			if err := validateMetadatumContent(pair.Value, depth+1); err != nil {
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
			if err := validateMetadataContent(txAuxData); err != nil {
				return err
			}
		}
	}

	return nil
}

// UtxoValidateDelegation validates delegation certificates against ledger state.
// For Shelley, it checks StakeDelegationCertificate:
// - Pool registration status
// - Stake credential registration status
func UtxoValidateDelegation(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Track credentials/pools registered within this transaction
	inTxStakeRegs := make(map[common.Blake2b224]bool)
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)

	isStakeRegistered := func(cred common.Credential) bool {
		return ls.IsStakeCredentialRegistered(cred) ||
			inTxStakeRegs[cred.Credential]
	}

	isPoolRegistered := func(poolKeyHash common.PoolKeyHash) bool {
		return ls.IsPoolRegistered(poolKeyHash) || inTxPoolRegs[poolKeyHash]
	}

	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *common.StakeRegistrationCertificate:
			inTxStakeRegs[c.StakeCredential.Credential] = true

		case *common.StakeDeregistrationCertificate:
			// Remove from in-tx registrations so subsequent delegations fail
			delete(inTxStakeRegs, c.StakeCredential.Credential)

		case *common.PoolRegistrationCertificate:
			inTxPoolRegs[c.Operator] = true

		case *common.PoolRetirementCertificate:
			// Remove from in-tx registrations so subsequent delegations fail
			delete(inTxPoolRegs, c.PoolKeyHash)

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
//   - VRFKeyHashAlreadyRegistered: a registration may not claim a VRF key hash
//     another pool holds. Gated on major protocol version > 10
//     (hardforkConwayDisallowDuplicatedVRFKeys).
//   - StakePoolNotRegisteredOnKeyPOOL: a retirement must name a registered
//     pool.
//   - StakePoolRetirementWrongEpochPOOL: a retirement epoch e must satisfy
//     cEpoch < e <= cEpoch + eMax.
//
// The reference rule's remaining predicate, PoolMedataHashTooBig (a metadata
// hash longer than 32 bytes, gated on SoftForks.restrictPoolMetadataHash), is
// not reimplemented because it cannot be reached: PoolMetadata.Hash is a fixed
// 32-byte PoolMetadataHash whose UnmarshalCBOR rejects any other length, so a
// certificate carrying an oversized hash never decodes. See
// TestPoolMetadataHashLengthIsFixed in ledger/common. That makes this package
// stricter than the reference for protocol versions at or below 4.0, which
// accepted oversized hashes.
//
// The registration and re-registration state updates poolTransition performs
// (psStakePools, psFutureStakePoolParams, psRetiring, psVRFKeyHashes) are
// ledger-state transitions rather than predicates, and belong to the consumer
// applying the certificate.
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
	minPoolCost := poolPparams.MinPoolCostValue()
	networkId := ls.NetworkId()

	// Certificates are applied in order, so a pool registered by an earlier
	// certificate in the same transaction is registered for the purposes of
	// a later one. A retirement certificate does not undo that: the
	// reference rule only adds the pool to psRetiring and leaves it in
	// psStakePools until POOLREAP runs at the epoch boundary.
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)
	// VRF key hashes claimed by earlier registrations in this transaction,
	// mirroring the psVRFKeyHashes insert that poolTransition performs.
	inTxVrfKeys := make(map[common.VrfKeyHash]common.PoolKeyHash)

	for _, cert := range certs {
		switch c := cert.(type) {
		case *common.PoolRegistrationCertificate:
			if err := validatePoolRegistration(
				c,
				ls,
				networkId,
				minPoolCost,
				checkNetworkId,
				checkVrfKeys,
				inTxVrfKeys,
			); err != nil {
				return err
			}
			inTxPoolRegs[c.Operator] = true
			if checkVrfKeys {
				inTxVrfKeys[c.VrfKeyHash] = c.Operator
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
	ls common.LedgerState,
	networkId uint,
	minPoolCost uint64,
	checkNetworkId bool,
	checkVrfKeys bool,
	inTxVrfKeys map[common.VrfKeyHash]common.PoolKeyHash,
) error {
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
	// One narrow case is not reproduced. psVRFKeyHashes also retains the
	// VRF key hash of an earlier same-epoch re-registration held in
	// psFutureStakePoolParams, which the reference rejects because it is
	// neither absent nor equal to the pool's current VRF key hash. This
	// package has no future-pool-parameter state, so a pool reverting to
	// such a key hash is accepted. That direction cannot reject a valid
	// registration.
	if owner, claimed := inTxVrfKeys[cert.VrfKeyHash]; claimed &&
		owner != cert.Operator {
		return VrfKeyHashAlreadyRegisteredError{
			PoolKeyHash:  cert.Operator,
			VrfKeyHash:   cert.VrfKeyHash,
			RegisteredBy: owner,
		}
	}
	inUse, owningPool, err := ls.IsVrfKeyInUse(cert.VrfKeyHash)
	if err != nil {
		return err
	}
	if inUse && owningPool != cert.Operator {
		return VrfKeyHashAlreadyRegisteredError{
			PoolKeyHash:  cert.Operator,
			VrfKeyHash:   cert.VrfKeyHash,
			RegisteredBy: owningPool,
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
	// The current epoch is only available from the optional EpochState
	// capability. Skip the bound rather than failing closed when the ledger
	// state does not provide it, so that a consumer which has not
	// implemented EpochForSlot keeps accepting valid retirements.
	epochState, ok := ls.(common.EpochState)
	if !ok {
		return nil
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
