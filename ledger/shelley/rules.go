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

	"github.com/blinklabs-io/gouroboros/cbor"
	common "github.com/blinklabs-io/gouroboros/ledger/common"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
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
	if tx.Fee() >= minFee {
		return nil
	}
	return FeeTooSmallUtxoError{
		Provided: tx.Fee(),
		Min:      minFee,
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
		switch cert.(type) {
		case *common.StakeDeregistrationCertificate:
			consumedValue += uint64(tmpPparams.KeyDeposit)
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
		case *common.StakeRegistrationCertificate:
			producedValue += uint64(tmpPparams.KeyDeposit)
		}
	}
	if consumedValue == producedValue {
		return nil
	}
	return ValueNotConservedUtxoError{
		Consumed: consumedValue,
		Produced: producedValue,
	}
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
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
		if tmpOutput.Amount() < minCoin {
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

// MinFeeTx calculates the minimum required fee for a transaction based on protocol parameters
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ShelleyProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txBytes := tx.Cbor()
	if len(txBytes) == 0 {
		var err error
		txBytes, err = cbor.Encode(tx)
		if err != nil {
			return 0, err
		}
	}
	minFee := uint64(
		(tmpPparams.MinFeeA * uint(len(txBytes))) + tmpPparams.MinFeeB,
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
