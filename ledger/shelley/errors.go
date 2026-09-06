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
//   - rules.go: Validation rules that return these errors
//   - ledger/common/errors.go: Shared error types across eras
//   - Later eras (allegra, alonzo, etc.) have their own errors.go

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ExpiredUtxoError struct {
	Ttl  uint64
	Slot uint64
}

func (e ExpiredUtxoError) Error() string {
	return fmt.Sprintf(
		"expired UTxO: TTL %d, slot %d",
		e.Ttl,
		e.Slot,
	)
}

type InputSetEmptyUtxoError struct{}

func (InputSetEmptyUtxoError) Error() string {
	return "input set empty"
}

// DuplicateInputError indicates a duplicate input was found in the transaction
type DuplicateInputError struct {
	Input     common.TransactionInput
	InputType string // "regular", "collateral", or "reference"
}

func (e DuplicateInputError) Error() string {
	return fmt.Sprintf("duplicate %s input: %s", e.InputType, e.Input.String())
}

type FeeTooSmallUtxoError struct {
	Provided *big.Int
	Min      *big.Int
}

func (e FeeTooSmallUtxoError) Error() string {
	provided := "<nil>"
	min := "<nil>"
	if e.Provided != nil {
		provided = e.Provided.String()
	}
	if e.Min != nil {
		min = e.Min.String()
	}
	return fmt.Sprintf("fee too small: provided %s, minimum %s", provided, min)
}

type BadInputsUtxoError struct {
	Inputs []common.TransactionInput
}

func (e BadInputsUtxoError) Error() string {
	tmpInputs := make([]string, len(e.Inputs))
	for idx, tmpInput := range e.Inputs {
		tmpInputs[idx] = tmpInput.String()
	}
	return "bad input(s): " + strings.Join(tmpInputs, ", ")
}

type NativeScriptFailedError struct {
	ScriptHash common.ScriptHash
}

func (e NativeScriptFailedError) Error() string {
	return fmt.Sprintf("native script failed (hash=%x)", e.ScriptHash[:])
}

type WrongNetworkError struct {
	NetId uint
	Addrs []common.Address
}

func (e WrongNetworkError) Error() string {
	tmpAddrs := make([]string, len(e.Addrs))
	for idx, tmpAddr := range e.Addrs {
		tmpAddrs[idx] = tmpAddr.String()
	}
	return "wrong network: " + strings.Join(tmpAddrs, ", ")
}

type WrongNetworkWithdrawalError struct {
	NetId uint
	Addrs []common.Address
}

func (e WrongNetworkWithdrawalError) Error() string {
	tmpAddrs := make([]string, len(e.Addrs))
	for idx, tmpAddr := range e.Addrs {
		tmpAddrs[idx] = tmpAddr.String()
	}
	return "wrong network withdrawals: " + strings.Join(tmpAddrs, ", ")
}

type ValueNotConservedUtxoError struct {
	Consumed *big.Int
	Produced *big.Int
}

func (e ValueNotConservedUtxoError) Error() string {
	consumed := "<nil>"
	produced := "<nil>"
	if e.Consumed != nil {
		consumed = e.Consumed.String()
	}
	if e.Produced != nil {
		produced = e.Produced.String()
	}
	return fmt.Sprintf(
		"value not conserved: consumed %s, produced %s",
		consumed,
		produced,
	)
}

type OutputTooSmallUtxoError struct {
	Outputs []common.TransactionOutput
}

func (e OutputTooSmallUtxoError) Error() string {
	tmpOutputs := make([]string, len(e.Outputs))
	for idx, tmpOutput := range e.Outputs {
		tmpOutputs[idx] = tmpOutput.String()
	}
	return "output too small: " + strings.Join(tmpOutputs, ", ")
}

type OutputBootAddrAttrsTooBigError struct {
	Outputs []common.TransactionOutput
}

func (e OutputBootAddrAttrsTooBigError) Error() string {
	tmpOutputs := make([]string, len(e.Outputs))
	for idx, tmpOutput := range e.Outputs {
		tmpOutputs[idx] = tmpOutput.String()
	}
	return "output bootstrap address attributes too big: " + strings.Join(
		tmpOutputs,
		", ",
	)
}

type MaxTxSizeUtxoError struct {
	TxSize    uint
	MaxTxSize uint
}

func (e MaxTxSizeUtxoError) Error() string {
	return fmt.Sprintf(
		"transaction size too large: size %d, max %d",
		e.TxSize,
		e.MaxTxSize,
	)
}

type InvalidCertificateDepositError struct {
	CertificateType common.CertificateType
	Amount          int64
}

func (e InvalidCertificateDepositError) Error() string {
	return fmt.Sprintf(
		"invalid certificate deposit amount: type %d, amount %d",
		e.CertificateType,
		e.Amount,
	)
}

// Type aliases for backward compatibility
type (
	MissingTransactionMetadataError          = common.MissingTransactionMetadataError
	MissingTransactionAuxiliaryDataHashError = common.MissingTransactionAuxiliaryDataHashError
	ConflictingMetadataHashError             = common.ConflictingMetadataHashError
)

// Witness validation errors (alias to common types)
type MissingVKeyWitnessesError = common.MissingVKeyWitnessesError

type MissingRequiredVKeyWitnessForSignerError = common.MissingRequiredVKeyWitnessForSignerError

// DelegateToUnregisteredPoolError indicates delegation to a pool that is not registered
type DelegateToUnregisteredPoolError struct {
	PoolKeyHash common.PoolKeyHash
}

func (e DelegateToUnregisteredPoolError) Error() string {
	return fmt.Sprintf("delegation to unregistered pool: %x", e.PoolKeyHash[:])
}

// DelegateUnregisteredStakeCredentialError indicates delegation from an unregistered stake credential
type DelegateUnregisteredStakeCredentialError struct {
	Credential common.Credential
}

func (e DelegateUnregisteredStakeCredentialError) Error() string {
	return fmt.Sprintf(
		"delegation from unregistered stake credential: %x",
		e.Credential.Credential[:],
	)
}

// MIRNegativesNotCurrentlyAllowedError indicates a move instantaneous rewards
// certificate carrying a negative reward delta at a protocol version before
// the Alonzo hard fork, which is the first version that permits one.
//
// Reference: MIRNegativesNotCurrentlyAllowed in delegTransition,
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs.
type MIRNegativesNotCurrentlyAllowedError struct {
	Credential common.Credential
	Delta      *big.Int
}

func (e MIRNegativesNotCurrentlyAllowedError) Error() string {
	delta := "nil"
	if e.Delta != nil {
		delta = e.Delta.String()
	}
	return fmt.Sprintf(
		"negative instantaneous rewards delta not allowed at this protocol version: credential %x delta %s",
		e.Credential.Credential[:],
		delta,
	)
}

// WithdrawalFromUnregisteredRewardAccountError indicates withdrawal from an unregistered reward account
type WithdrawalFromUnregisteredRewardAccountError struct {
	RewardAddress common.Address
}

func (e WithdrawalFromUnregisteredRewardAccountError) Error() string {
	return "withdrawal from unregistered reward account: " + e.RewardAddress.String()
}

// IncorrectWithdrawalAmountError indicates that a withdrawal violates the
// active era's required relationship to the reward account's current balance.
type IncorrectWithdrawalAmountError struct {
	RewardAddress common.Address
	Provided      *big.Int
	Balance       uint64
}

func (e IncorrectWithdrawalAmountError) Error() string {
	provided := "<nil>"
	if e.Provided != nil {
		provided = e.Provided.String()
	}
	return fmt.Sprintf(
		"incorrect withdrawal amount for %s: provided %s, balance %d",
		e.RewardAddress.String(),
		provided,
		e.Balance,
	)
}

// StakePoolNotRegisteredOnKeyError indicates a pool retirement certificate for
// a stake pool that is not registered.
//
// Reference: StakePoolNotRegisteredOnKeyPOOL in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs
type StakePoolNotRegisteredOnKeyError struct {
	PoolKeyHash common.PoolKeyHash
}

func (e StakePoolNotRegisteredOnKeyError) Error() string {
	return "stake pool not registered: " + e.PoolKeyHash.String()
}

// StakePoolRetirementWrongEpochError indicates a pool retirement certificate
// whose retirement epoch is not after the current epoch and at most eMax
// epochs beyond it.
//
// Reference: StakePoolRetirementWrongEpochPOOL in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs
type StakePoolRetirementWrongEpochError struct {
	PoolKeyHash  common.PoolKeyHash
	Supplied     uint64
	CurrentEpoch uint64
	LimitEpoch   uint64
}

func (e StakePoolRetirementWrongEpochError) Error() string {
	return fmt.Sprintf(
		"stake pool %s retirement epoch %d outside allowed range (%d, %d]",
		e.PoolKeyHash.String(),
		e.Supplied,
		e.CurrentEpoch,
		e.LimitEpoch,
	)
}

// StakePoolCostTooLowError indicates a pool registration certificate whose
// declared cost is below the minPoolCost protocol parameter.
//
// Reference: StakePoolCostTooLowPOOL in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs
type StakePoolCostTooLowError struct {
	PoolKeyHash common.PoolKeyHash
	Supplied    uint64
	Min         uint64
}

func (e StakePoolCostTooLowError) Error() string {
	return fmt.Sprintf(
		"stake pool %s cost %d below minimum %d",
		e.PoolKeyHash.String(),
		e.Supplied,
		e.Min,
	)
}

// WrongNetworkPoolError indicates a pool registration certificate whose reward
// account is on a different network than the ledger state.
//
// Reference: WrongNetworkPOOL in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs
type WrongNetworkPoolError struct {
	PoolKeyHash common.PoolKeyHash
	Supplied    uint
	Expected    uint
}

func (e WrongNetworkPoolError) Error() string {
	return fmt.Sprintf(
		"stake pool %s reward account network %d, expected %d",
		e.PoolKeyHash.String(),
		e.Supplied,
		e.Expected,
	)
}

// VrfKeyHashAlreadyRegisteredError indicates a pool registration certificate
// claiming a VRF key hash that another stake pool already uses.
//
// Reference: VRFKeyHashAlreadyRegistered in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs
type VrfKeyHashAlreadyRegisteredError struct {
	PoolKeyHash common.PoolKeyHash
	VrfKeyHash  common.VrfKeyHash
	// RegisteredBy is the pool that already holds the VRF key hash. It is
	// reported for diagnostics only; the reference predicate is a set
	// membership test that does not name the holder.
	RegisteredBy common.PoolKeyHash
}

func (e VrfKeyHashAlreadyRegisteredError) Error() string {
	return fmt.Sprintf(
		"stake pool %s VRF key hash %s already registered by pool %s",
		e.PoolKeyHash.String(),
		e.VrfKeyHash.String(),
		e.RegisteredBy.String(),
	)
}
