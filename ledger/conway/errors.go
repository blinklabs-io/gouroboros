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
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// NonDisjointRefInputsError is an alias to babbage.NonDisjointRefInputsError
type NonDisjointRefInputsError = babbage.NonDisjointRefInputsError

// Witness validation errors (alias to common types)
type MissingVKeyWitnessesError = common.MissingVKeyWitnessesError

type MissingRequiredVKeyWitnessForSignerError = common.MissingRequiredVKeyWitnessForSignerError

type MissingRedeemersForScriptDataHashError = common.MissingRedeemersForScriptDataHashError

type MissingPlutusScriptWitnessesError = common.MissingPlutusScriptWitnessesError

type ExtraneousPlutusScriptWitnessesError = common.ExtraneousPlutusScriptWitnessesError

// Metadata / cost model / IsValid aliases
type MissingTransactionMetadataError = common.MissingTransactionMetadataError

type (
	MissingTransactionAuxiliaryDataHashError = common.MissingTransactionAuxiliaryDataHashError
	ConflictingMetadataHashError             = common.ConflictingMetadataHashError
	MissingCostModelError                    = common.MissingCostModelError
	InvalidIsValidFlagError                  = common.InvalidIsValidFlagError
)

type WrongTransactionNetworkIdError struct {
	TxNetworkId     uint8
	LedgerNetworkId uint
}

func (e WrongTransactionNetworkIdError) Error() string {
	return fmt.Sprintf(
		"wrong transaction network ID: transaction has %d, ledger expects %d",
		e.TxNetworkId,
		e.LedgerNetworkId,
	)
}

type TreasuryDonationWithPlutusV1V2Error struct {
	Donation      uint64
	PlutusVersion string
}

func (e TreasuryDonationWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf(
		"treasury donation (%d lovelace) cannot be used with %s scripts - treasury donation is a Conway feature only available for PlutusV3",
		e.Donation,
		e.PlutusVersion,
	)
}

// CurrentTreasuryValueWithPlutusV1V2Error indicates CurrentTreasuryValue cannot be used with V1/V2 scripts
type CurrentTreasuryValueWithPlutusV1V2Error struct {
	PlutusVersion string
}

func (e CurrentTreasuryValueWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf(
		"current treasury value cannot be used with %s scripts - only available for PlutusV3",
		e.PlutusVersion,
	)
}

// ProposalProceduresWithPlutusV1V2Error indicates ProposalProcedures cannot be used with V1/V2 scripts
type ProposalProceduresWithPlutusV1V2Error struct {
	PlutusVersion string
}

func (e ProposalProceduresWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf(
		"proposal procedures cannot be used with %s scripts - only available for PlutusV3",
		e.PlutusVersion,
	)
}

// VotingProceduresWithPlutusV1V2Error indicates VotingProcedures cannot be used with V1/V2 scripts
type VotingProceduresWithPlutusV1V2Error struct {
	PlutusVersion string
}

func (e VotingProceduresWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf(
		"voting procedures cannot be used with %s scripts - only available for PlutusV3",
		e.PlutusVersion,
	)
}

// PlutusScriptFailedError indicates that a Plutus script execution failed
type PlutusScriptFailedError struct {
	ScriptHash common.ScriptHash
	Tag        common.RedeemerTag
	Index      uint32
	Err        error
}

func (e PlutusScriptFailedError) Error() string {
	return fmt.Sprintf(
		"plutus script failed (hash=%x, tag=%d, index=%d): %v",
		e.ScriptHash[:],
		e.Tag,
		e.Index,
		e.Err,
	)
}

func (e PlutusScriptFailedError) Unwrap() error {
	return e.Err
}

// NativeScriptFailedError indicates that a native (timelock) script evaluation failed
type NativeScriptFailedError = allegra.NativeScriptFailedError

// ErrNativeScriptFailed is the sentinel error for native script failures
var ErrNativeScriptFailed = allegra.ErrNativeScriptFailed

// ScriptContextConstructionError indicates that the script context could not be built
type ScriptContextConstructionError struct {
	Err error
}

func (e ScriptContextConstructionError) Error() string {
	return fmt.Sprintf("failed to construct script context: %v", e.Err)
}

func (e ScriptContextConstructionError) Unwrap() error {
	return e.Err
}

// MissingDatumForSpendingScriptError indicates that a spending script requires a datum but none was provided
type MissingDatumForSpendingScriptError struct {
	ScriptHash common.ScriptHash
	Input      common.TransactionInput
}

func (e MissingDatumForSpendingScriptError) Error() string {
	return fmt.Sprintf(
		"missing datum for spending script (hash=%x, input=%s)",
		e.ScriptHash[:],
		e.Input.String(),
	)
}

// NotAllowedSupplementalDatumsError indicates that datums in the witness set are not required by any script input
type NotAllowedSupplementalDatumsError struct {
	DatumHashes []common.Blake2b256
}

func (e NotAllowedSupplementalDatumsError) Error() string {
	hashes := make([]string, len(e.DatumHashes))
	for i, h := range e.DatumHashes {
		hashes[i] = hex.EncodeToString(h[:])
	}
	return "not allowed supplemental datums in witness set: " + strings.Join(
		hashes,
		", ",
	)
}

// ExtraRedeemerError indicates a redeemer exists that doesn't match any valid script purpose
// (e.g., redeemer index is out of bounds for the inputs/mints/etc.)
type ExtraRedeemerError struct {
	RedeemerKey common.RedeemerKey
}

func (e ExtraRedeemerError) Error() string {
	return fmt.Sprintf(
		"extra redeemer: tag=%d, index=%d doesn't match any valid script purpose",
		e.RedeemerKey.Tag,
		e.RedeemerKey.Index,
	)
}

// MissingRedeemerForScriptError indicates a script purpose requires a redeemer but none was provided
type MissingRedeemerForScriptError struct {
	ScriptHash common.ScriptHash
	Tag        common.RedeemerTag
	Index      uint32
}

func (e MissingRedeemerForScriptError) Error() string {
	return fmt.Sprintf("missing redeemer for script %x: tag=%d, index=%d",
		e.ScriptHash[:], e.Tag, e.Index)
}

// ProtocolParameterUpdateEmptyError indicates that a PPU has no fields set
type ProtocolParameterUpdateEmptyError struct{}

func (e ProtocolParameterUpdateEmptyError) Error() string {
	return "protocol parameter update is empty (at least one field must be set)"
}

// ProtocolParameterUpdateFieldZeroError indicates that a PPU field cannot be zero
type ProtocolParameterUpdateFieldZeroError struct {
	FieldName string
	Value     uint
}

func (e ProtocolParameterUpdateFieldZeroError) Error() string {
	return fmt.Sprintf(
		"protocol parameter update field %s cannot be 0, got %d",
		e.FieldName,
		e.Value,
	)
}

// EmptyTreasuryWithdrawalsError indicates that a TreasuryWithdrawalGovAction has an empty withdrawals map
type EmptyTreasuryWithdrawalsError struct{}

func (e EmptyTreasuryWithdrawalsError) Error() string {
	return "treasury withdrawal governance action has empty withdrawals map"
}

// ZeroTreasuryWithdrawalAmountError indicates that a TreasuryWithdrawalGovAction has all zero amounts
type ZeroTreasuryWithdrawalAmountError struct{}

func (e ZeroTreasuryWithdrawalAmountError) Error() string {
	return "treasury withdrawal governance action has zero withdrawal amount"
}

// WrongNetworkProposalAddressError indicates that a proposal address has wrong network ID
type WrongNetworkProposalAddressError struct {
	NetId uint
	Addrs []common.Address
}

func (e WrongNetworkProposalAddressError) Error() string {
	tmpAddrs := make([]string, len(e.Addrs))
	for idx, addr := range e.Addrs {
		tmpAddrs[idx] = addr.String()
	}
	return fmt.Sprintf(
		"wrong network ID in proposal address(es): expected %d, got %s",
		e.NetId,
		strings.Join(tmpAddrs, ", "),
	)
}

// Delegation errors (alias to shelley types)
type (
	DelegateToUnregisteredPoolError          = shelley.DelegateToUnregisteredPoolError
	DelegateUnregisteredStakeCredentialError = shelley.DelegateUnregisteredStakeCredentialError
)

// DelegateVoteToUnregisteredDRepError indicates vote delegation to a DRep that is not registered
type DelegateVoteToUnregisteredDRepError struct {
	DRepCredential common.Credential
}

func (e DelegateVoteToUnregisteredDRepError) Error() string {
	return fmt.Sprintf(
		"vote delegation to unregistered DRep: %x",
		e.DRepCredential.Credential[:],
	)
}

// WithdrawalFromUnregisteredRewardAccountError indicates withdrawal from an unregistered reward account
type WithdrawalFromUnregisteredRewardAccountError struct {
	RewardAddress common.Address
}

func (e WithdrawalFromUnregisteredRewardAccountError) Error() string {
	return "withdrawal from unregistered reward account: " + e.RewardAddress.String()
}

// WithdrawalWrongAmountError indicates withdrawal of wrong amount from reward account
type WithdrawalWrongAmountError struct {
	RewardAddress   common.Address
	RequestedAmount uint64
	ActualBalance   uint64
}

func (e WithdrawalWrongAmountError) Error() string {
	return fmt.Sprintf(
		"withdrawal wrong amount from %s: requested %d, actual balance %d",
		e.RewardAddress.String(),
		e.RequestedAmount,
		e.ActualBalance,
	)
}

// StakeCredentialAlreadyRegisteredError indicates attempting to register an already registered stake credential
type StakeCredentialAlreadyRegisteredError struct {
	Credential common.Credential
}

func (e StakeCredentialAlreadyRegisteredError) Error() string {
	return fmt.Sprintf(
		"stake credential already registered: %x",
		e.Credential.Credential[:],
	)
}

// DRepAlreadyRegisteredError indicates attempting to register an already registered DRep
type DRepAlreadyRegisteredError struct {
	Credential common.Credential
}

func (e DRepAlreadyRegisteredError) Error() string {
	return fmt.Sprintf(
		"DRep already registered: %x",
		e.Credential.Credential[:],
	)
}

// NotCommitteeMemberError indicates an operation on a credential that is not a CC member
type NotCommitteeMemberError struct {
	Credential common.Blake2b224
	Operation  string
}

func (e NotCommitteeMemberError) Error() string {
	return fmt.Sprintf(
		"not a CC member, cannot %s: %x",
		e.Operation,
		e.Credential[:],
	)
}

// ResignedCommitteeMemberHotKeyError indicates trying to authorize hot key for resigned CC member
type ResignedCommitteeMemberHotKeyError struct {
	ColdKey common.Blake2b224
}

func (e ResignedCommitteeMemberHotKeyError) Error() string {
	return fmt.Sprintf(
		"cannot authorize hot key for resigned CC member: %x",
		e.ColdKey[:],
	)
}

// CommitteeMemberLookupError indicates a failure to look up a committee member
type CommitteeMemberLookupError struct {
	Credential common.Blake2b224
	Err        error
}

func (e CommitteeMemberLookupError) Error() string {
	return fmt.Sprintf(
		"failed to look up CC member %x: %v",
		e.Credential[:],
		e.Err,
	)
}

func (e CommitteeMemberLookupError) Unwrap() error {
	return e.Err
}
