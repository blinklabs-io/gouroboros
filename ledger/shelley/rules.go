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

package shelley

import (
	"errors"
	"fmt"
	"math/big"
	"unicode/utf8"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateSignatures,
	UtxoValidateTimeToLive,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateBadInputsUtxo,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
}

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
		return nil
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
	required := tx.RequiredSigners()
	if len(required) == 0 {
		return nil
	}

	w := tx.Witnesses()
	if w == nil || len(w.Vkey()) == 0 {
		return MissingVKeyWitnessesError{}
	}

	// Build a set of hashes from the provided vkey witnesses for quick lookup
	vkeyHashes := make(map[common.Blake2b224]struct{})
	for _, vw := range w.Vkey() {
		h := common.Blake2b224Hash(vw.Vkey)
		vkeyHashes[h] = struct{}{}
	}

	// Ensure each required signer hash has a matching vkey witness
	for _, req := range required {
		if _, ok := vkeyHashes[req]; !ok {
			return MissingRequiredVKeyWitnessForSignerError{Signer: req}
		}
	}
	return nil
}

// MinFeeTx calculates the minimum required fee for a transaction based on protocol parameters
// Fee is calculated using the transaction body CBOR size as per Cardano protocol
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ShelleyProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ShelleyTransaction)
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
	case *common.MetaText:
		if !utf8.ValidString(m.Value) {
			return errors.New("metadata contains invalid UTF-8 text")
		}
		// Cardano spec: metadata text strings must not exceed 64 bytes
		if len(m.Value) > 64 {
			return fmt.Errorf("metadata text exceeds 64 byte limit: %d bytes", len(m.Value))
		}
	case *common.MetaBytes:
		// Cardano spec: metadata byte strings must not exceed 64 bytes
		if len(m.Value) > 64 {
			return fmt.Errorf("metadata byte string exceeds 64 byte limit: %d bytes", len(m.Value))
		}
	case *common.MetaInt:
		if m.Value == nil {
			return errors.New("metadata contains nil integer value")
		}
	case *common.MetaList:
		for _, item := range m.Items {
			if err := validateMetadatumContent(item, depth+1); err != nil {
				return err
			}
		}
	case *common.MetaMap:
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
// It checks that reward accounts are registered.
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

	for addr := range withdrawals {
		// Extract credential from reward address staking payload
		stakingPayload := addr.StakingPayload()
		if stakingPayload == nil {
			continue
		}

		var cred common.Credential
		switch p := stakingPayload.(type) {
		case *common.AddressPayloadKeyHash:
			cred = common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.NewBlake2b224(p.Hash.Bytes()),
			}
		case *common.AddressPayloadScriptHash:
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
	}
	return nil
}
