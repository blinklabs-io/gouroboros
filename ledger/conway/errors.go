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

// ConwayCertificateWithPlutusV1V2Error indicates Conway-era certificates cannot be used with V1/V2 scripts
type ConwayCertificateWithPlutusV1V2Error struct {
	PlutusVersion   string
	CertificateType string
}

func (e ConwayCertificateWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf(
		"%s certificate cannot be used with %s scripts - only available for PlutusV3",
		e.CertificateType,
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

// BootstrapDisallowedGovActionError indicates a governance action type that is
// not permitted during the Conway bootstrap phase (PV9, before the Plomin hard fork).
type BootstrapDisallowedGovActionError struct {
	ActionType common.GovActionType
}

func (e BootstrapDisallowedGovActionError) Error() string {
	return fmt.Sprintf(
		"governance action type %d is not allowed during the Conway bootstrap phase (PV9); requires PV10 (Plomin) or later",
		e.ActionType,
	)
}

// BootstrapDisallowedParameterChangeError indicates a ParameterChange proposal
// that updates one or more bootstrap-restricted fields, which is not permitted
// during the Conway bootstrap phase (PV9). The Plomin hard fork (PV10) lifts
// the restriction.
type BootstrapDisallowedParameterChangeError struct {
	Fields []string
}

func (e BootstrapDisallowedParameterChangeError) Error() string {
	return fmt.Sprintf(
		"ParameterChange proposal updates bootstrap-restricted fields %v which are not allowed during the Conway bootstrap phase (PV9); requires PV10 (Plomin) or later",
		e.Fields,
	)
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

// InvalidDRepTypeError indicates a delegation certificate referenced a DRep
// with a type other than key hash, script hash, Abstain, or NoConfidence.
// This guards against programmatically constructed (non-CBOR-decoded)
// transactions carrying a common.Drep with an out-of-range Type value.
type InvalidDRepTypeError struct {
	DrepType int
}

func (e InvalidDRepTypeError) Error() string {
	return fmt.Sprintf("invalid DRep type: %d", e.DrepType)
}

// WithdrawalFromUnregisteredRewardAccountError indicates withdrawal from an unregistered reward account
type WithdrawalFromUnregisteredRewardAccountError = shelley.WithdrawalFromUnregisteredRewardAccountError

// WithdrawalNotDelegatedToDRepError indicates a PV10/PV11 reward withdrawal
// whose key-hash stake credential has no governance vote delegation.
type WithdrawalNotDelegatedToDRepError struct {
	RewardAddress common.Address
}

func (e WithdrawalNotDelegatedToDRepError) Error() string {
	return "reward withdrawal is not delegated to a DRep: " +
		e.RewardAddress.String()
}

// DRepDelegationStateUnavailableError indicates that a ledger state cannot
// answer the DRep-delegation query required for PV10/PV11 withdrawals.
type DRepDelegationStateUnavailableError struct{}

func (DRepDelegationStateUnavailableError) Error() string {
	return "ledger state does not support DRep delegation lookups"
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

// CommitteeTermLimitUnavailableError indicates that protocol parameters do
// not expose the constitutional committee maximum term length.
type CommitteeTermLimitUnavailableError struct{}

func (CommitteeTermLimitUnavailableError) Error() string {
	return "constitutional committee maximum term length is unavailable"
}

// CurrentEpochStateUnavailableError indicates that committee term validation
// cannot determine the current epoch.
type CurrentEpochStateUnavailableError struct{}

func (CurrentEpochStateUnavailableError) Error() string {
	return "ledger state does not expose the current epoch"
}

// CommitteeTermTooLongError indicates that a committee member's expiry is
// beyond the configured maximum term measured from the current epoch.
type CommitteeTermTooLongError struct {
	Credential    common.Blake2b224
	CurrentEpoch  uint64
	ExpiryEpoch   uint64
	MaxTermLength uint64
}

func (e CommitteeTermTooLongError) Error() string {
	return fmt.Sprintf(
		"CC member %x expires at epoch %d, beyond current epoch %d plus maximum term length %d",
		e.Credential[:],
		e.ExpiryEpoch,
		e.CurrentEpoch,
		e.MaxTermLength,
	)
}

// DuplicateVrfKeyError indicates a pool registration attempted to use a VRF key
// already registered by another pool. Introduced in Protocol Version 11.
type DuplicateVrfKeyError struct {
	VrfKeyHash     common.Blake2b256
	NewPoolId      common.PoolKeyHash
	ExistingPoolId common.PoolKeyHash
}

func (e DuplicateVrfKeyError) Error() string {
	return fmt.Sprintf(
		"duplicate VRF key: pool %x attempted to register VRF key %x already in use by pool %x",
		e.NewPoolId[:8],
		e.VrfKeyHash[:8],
		e.ExistingPoolId[:8],
	)
}

// CCVotingRestrictionError indicates a Constitutional Committee member violated
// voting restrictions. In PV11+, this is a ledger predicate failure.
type CCVotingRestrictionError struct {
	VoterId     common.Blake2b224
	ActionId    common.GovActionId
	Restriction string
}

func (e CCVotingRestrictionError) Error() string {
	return fmt.Sprintf(
		"constitutional committee voting restriction: voter %x on action %x#%d - %s",
		e.VoterId[:8],
		e.ActionId.TransactionId[:8],
		e.ActionId.GovActionIdx,
		e.Restriction,
	)
}

// StakePoolVotingRestrictionError indicates a stake pool (SPO) voter violated
// voting restrictions per the Conway ledger's isStakePoolVotingAllowed. SPOs
// may never vote on NewConstitution or TreasuryWithdrawal actions.
type StakePoolVotingRestrictionError struct {
	VoterId     common.PoolKeyHash
	ActionId    common.GovActionId
	Restriction string
}

func (e StakePoolVotingRestrictionError) Error() string {
	return fmt.Sprintf(
		"stake pool voting restriction: voter %x on action %x#%d - %s",
		e.VoterId[:8],
		e.ActionId.TransactionId[:8],
		e.ActionId.GovActionIdx,
		e.Restriction,
	)
}

// BootstrapVotingRestrictionError indicates a vote cast during the Conway
// bootstrap phase (PV9) that is not permitted: DReps may only vote on
// InfoAction, and all other voter types may only vote on bootstrap-eligible
// actions (ParameterChange, HardForkInitiation, InfoAction).
type BootstrapVotingRestrictionError struct {
	VoterId     common.Blake2b224
	ActionId    common.GovActionId
	Restriction string
}

func (e BootstrapVotingRestrictionError) Error() string {
	return fmt.Sprintf(
		"bootstrap-phase voting restriction: voter %x on action %x#%d - %s",
		e.VoterId[:8],
		e.ActionId.TransactionId[:8],
		e.ActionId.GovActionIdx,
		e.Restriction,
	)
}

// UnknownGovActionIdError indicates that a voting procedure referenced one or
// more governance action IDs that do not exist in the ledger state
// (ConwayGovPredFailure.GovActionsDoNotExist).
type UnknownGovActionIdError struct {
	ActionIds []common.GovActionId
}

func (e UnknownGovActionIdError) Error() string {
	ids := make([]string, len(e.ActionIds))
	for i, id := range e.ActionIds {
		ids[i] = fmt.Sprintf("%x#%d", id.TransactionId[:8], id.GovActionIdx)
	}
	return "vote references unknown governance action id(s): " +
		strings.Join(ids, ", ")
}

// UnknownVoterError indicates that a voting procedure was cast by a voter
// that does not exist in the ledger state (e.g. an unregistered DRep, an
// unregistered stake pool, or a credential not authorized as a committee hot
// key). Corresponds to ConwayGovPredFailure.VotersDoNotExist.
type UnknownVoterError struct {
	Voter common.Voter
}

func (e UnknownVoterError) Error() string {
	return fmt.Sprintf(
		"vote cast by unknown voter: type=%d hash=%x",
		e.Voter.Type,
		e.Voter.Hash[:8],
	)
}

// VotingOnExpiredGovActionError indicates a vote was cast on a governance
// action whose expiry slot has already passed
// (ConwayGovPredFailure.VotingOnExpiredGovAction).
type VotingOnExpiredGovActionError struct {
	Voter      common.Voter
	ActionId   common.GovActionId
	ExpirySlot uint64
	Slot       uint64
}

func (e VotingOnExpiredGovActionError) Error() string {
	return fmt.Sprintf(
		"vote cast on expired governance action %x#%d (expired at slot %d, current slot %d)",
		e.ActionId.TransactionId[:8],
		e.ActionId.GovActionIdx,
		e.ExpirySlot,
		e.Slot,
	)
}

// ProposalDepositIncorrectError indicates a proposal procedure's deposit does
// not equal the protocol's GovActionDeposit parameter
// (ConwayGovPredFailure.ProposalDepositIncorrect).
type ProposalDepositIncorrectError struct {
	Supplied uint64
	Expected uint64
}

func (e ProposalDepositIncorrectError) Error() string {
	return fmt.Sprintf(
		"proposal deposit incorrect: supplied %d, expected %d",
		e.Supplied,
		e.Expected,
	)
}

// ProposalReturnAccountDoesNotExistError indicates a proposal's return
// (refund) address is not a registered reward account
// (ConwayGovPredFailure.ProposalReturnAccountDoesNotExist).
type ProposalReturnAccountDoesNotExistError struct {
	Address common.Address
}

func (e ProposalReturnAccountDoesNotExistError) Error() string {
	return "proposal return account does not exist: " + e.Address.String()
}

// TreasuryWithdrawalReturnAccountsDoNotExistError indicates one or more
// treasury withdrawal destination addresses are not registered reward
// accounts (ConwayGovPredFailure.TreasuryWithdrawalReturnAccountsDoNotExist).
type TreasuryWithdrawalReturnAccountsDoNotExistError struct {
	Addresses []common.Address
}

func (e TreasuryWithdrawalReturnAccountsDoNotExistError) Error() string {
	addrs := make([]string, len(e.Addresses))
	for i, addr := range e.Addresses {
		addrs[i] = addr.String()
	}
	return "treasury withdrawal return account(s) do not exist: " +
		strings.Join(addrs, ", ")
}

// ConflictingCommitteeUpdateError indicates an UpdateCommittee governance
// action lists the same cold credential in both its removed-members set and
// its added-members map (ConwayGovPredFailure.ConflictingCommitteeUpdate).
type ConflictingCommitteeUpdateError struct {
	Credentials []common.Credential
}

func (e ConflictingCommitteeUpdateError) Error() string {
	creds := make([]string, len(e.Credentials))
	for i, cred := range e.Credentials {
		creds[i] = hex.EncodeToString(cred.Credential[:8])
	}
	return "update committee action lists credential(s) as both added and removed: " +
		strings.Join(
			creds,
			", ",
		)
}

// MalformedGovActionError indicates a governance action failed a structural
// well-formedness check (ConwayGovPredFailure.MalformedProposal).
type MalformedGovActionError struct {
	Reason string
}

func (e MalformedGovActionError) Error() string {
	return "malformed governance action: " + e.Reason
}

// BadHardForkProtocolVersionError indicates a HardForkInitiation governance
// action proposes a protocol version that cannot legally follow the current
// (or referenced ancestor) protocol version
// (ConwayGovPredFailure.ProposalCantFollow).
type BadHardForkProtocolVersionError struct {
	Supplied common.ProtocolParametersProtocolVersion
	Expected common.ProtocolParametersProtocolVersion
}

func (e BadHardForkProtocolVersionError) Error() string {
	return fmt.Sprintf(
		"hard fork protocol version %d.%d cannot follow current protocol version %d.%d",
		e.Supplied.Major,
		e.Supplied.Minor,
		e.Expected.Major,
		e.Expected.Minor,
	)
}

// InvalidGovActionAncestorError indicates a governance action's referenced
// ancestor (PrevGovActionId) does not exist, or exists but belongs to a
// different governance-action "purpose" chain
// (ConwayGovPredFailure.InvalidPrevGovActionId).
type InvalidGovActionAncestorError struct {
	ActionId common.GovActionId
	Reason   string
}

func (e InvalidGovActionAncestorError) Error() string {
	return fmt.Sprintf(
		"invalid governance action ancestor %x#%d: %s",
		e.ActionId.TransactionId[:8],
		e.ActionId.GovActionIdx,
		e.Reason,
	)
}
