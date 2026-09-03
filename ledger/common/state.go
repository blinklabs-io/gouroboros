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

package common

// Related files:
//   - tx.go: Transaction interface that validation rules operate on
//   - rules.go: Validation rules that use LedgerState
//   - github.com/blinklabs-io/ouroboros-mock/ledger: MockLedgerState for testing
//   - ledger/{era}/rules.go: Era-specific validation using these interfaces

import (
	"time"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// UtxoState defines the interface for querying the UTxO state
type UtxoState interface {
	UtxoById(TransactionInput) (Utxo, error)
}

// CertState defines the interface for querying the certificate state
type CertState interface {
	StakeRegistration([]byte) ([]StakeRegistrationCertificate, error)
	// IsStakeCredentialRegistered checks if a stake credential is currently registered.
	// Returns true if the credential has an active registration (not deregistered).
	// This is the authoritative check for stake credential presence in the ledger state.
	IsStakeCredentialRegistered(Credential) bool
}

// StakeCredentialDepositState is the optional ledger-state capability used by
// Conway certificate validation to retrieve the deposit held for a registered
// stake credential. The returned pointer is nil when the credential is not
// registered.
type StakeCredentialDepositState interface {
	StakeCredentialDeposit(Credential) (*uint64, error)
}

// EpochState is the optional ledger-state capability that maps a slot to the
// epoch containing it. The Shelley POOL rule's retirement bound
// (StakePoolRetirementWrongEpochPOOL) is expressed relative to the current
// epoch, which the (transaction, slot, state, params) validation contract does
// not otherwise carry.
//
// It is deliberately optional and degrading: a ledger state that does not
// implement it keeps every other POOL predicate and simply does not get the
// retirement-epoch bound enforced. Validation must not fail closed when it is
// absent, so that adopting a gouroboros release containing this rule cannot
// reject otherwise-valid pool retirements in a consumer that has not
// implemented the method yet.
type EpochState interface {
	// EpochForSlot returns the epoch number containing the given slot.
	EpochForSlot(slot uint64) (uint64, error)
}

// PoolState defines the interface for querying the current pool state
type PoolState interface {
	// PoolCurrentState returns the latest active registration certificate for the given pool key hash.
	// It also returns the epoch of a pending retirement certificate, if one exists.
	// If the pool is not registered, the registration certificate will be nil.
	PoolCurrentState(PoolKeyHash) (*PoolRegistrationCertificate, *uint64, error)
	// IsPoolRegistered checks if a pool is currently registered
	IsPoolRegistered(PoolKeyHash) bool
	// IsVrfKeyInUse checks if a VRF key hash is registered by another pool.
	// Returns (inUse, owningPoolId, error). Used for PV11+ VRF uniqueness validation.
	IsVrfKeyInUse(vrfKeyHash Blake2b256) (bool, PoolKeyHash, error)
}

// RewardState defines the interface for reward calculation and querying
type RewardState interface {
	// CalculateRewards calculates rewards for the given epoch based on stake snapshot
	CalculateRewards(
		pots AdaPots,
		snapshot RewardSnapshot,
		params RewardParameters,
	) (*RewardCalculationResult, error)

	// GetAdaPots returns the current ADA pots
	GetAdaPots() AdaPots

	// UpdateAdaPots updates the ADA pots (typically called after reward calculation)
	UpdateAdaPots(pots AdaPots) error

	// GetRewardSnapshot returns the stake snapshot for reward calculation
	GetRewardSnapshot(epoch uint64) (RewardSnapshot, error)

	// IsRewardAccountRegistered checks if a reward account (by stake credential) is registered.
	// Returns true if the stake credential has an active registration, meaning the reward
	// account exists and can receive rewards or be withdrawn from.
	// This is typically equivalent to IsStakeCredentialRegistered but exists on RewardState
	// to allow checking registration without requiring full CertState access.
	IsRewardAccountRegistered(Credential) bool

	// RewardAccountBalance returns the current reward balance for a stake credential.
	//
	// Return value semantics:
	//   - nil, nil: The reward account is not registered (credential never registered or
	//     has been deregistered). Callers should use IsRewardAccountRegistered to
	//     distinguish this from an error condition if needed.
	//   - *uint64 (including 0), nil: The reward account is registered. The pointed-to
	//     value is the current balance, which may be zero if no rewards have accrued
	//     or if all rewards have been withdrawn.
	//   - nil, error: An error occurred while querying the balance.
	//
	// Callers needing to distinguish "unregistered" from "registered with zero balance"
	// should check for nil before examining the value.
	RewardAccountBalance(Credential) (*uint64, error)
}

// LedgerState defines the interface for querying the ledger
type LedgerState interface {
	UtxoState
	CertState
	SlotState
	PoolState
	RewardState
	GovState
	NetworkId() uint

	// Plutus cost models
	CostModels() map[PlutusLanguage]CostModel
}

// TipState defines the interface for querying the current tip
type TipState interface {
	Tip() (pcommon.Tip, error)
}

// SlotState defines the interface for querying slots
type SlotState interface {
	SlotToTime(uint64) (time.Time, error)
	TimeToSlot(time.Time) (uint64, error)
}

// Constitution is the current enacted constitution. ScriptHash is the
// optional guardrails script hash: nil means that the constitution has no
// guardrails script.
type Constitution struct {
	Anchor     GovAnchor
	ScriptHash []byte
}

// Minimal placeholder types used by the extended interface. These are
// intentionally lightweight so tests and era packages can compile while we
// wire real parsing.
type (
	PlutusLanguage uint8
	CostModel      struct{}
)

// Governance-related types required by the extended LedgerState.
type CommitteeMember struct {
	ColdKey     Blake2b224
	HotKey      *Blake2b224 // nil if not authorized
	ExpiryEpoch uint64
	Resigned    bool
}

// CommitteeCredentialState is the optional authoritative committee-state
// capability used by Conway certificate and voter validation. Credentials are
// passed with their key/script tag intact so providers cannot alias identities
// that share the same hash.
//
// CommitteeStateAvailable distinguishes an authoritative empty committee from
// a provider that cannot answer committee queries for the validation snapshot.
// Validation fails closed when this capability is absent or reports false.
type CommitteeCredentialState interface {
	CommitteeStateAvailable() (bool, error)
	CommitteeCredentialMember(Credential) (*CommitteeMember, error)
	CommitteeHotCredentialMember(Credential) (*CommitteeMember, error)
}

type DRepRegistration struct {
	Credential Blake2b224
	Anchor     *GovAnchor
	Deposit    uint64
}

// DRepDelegationState is the optional ledger-state capability used to query
// governance vote delegations. Ledger states used to validate PV10 or PV11
// key-hash reward withdrawals must implement this interface.
type DRepDelegationState interface {
	DRepDelegation(Credential) (*Drep, error)
}

// GenesisDelegationState is the optional ledger-state capability used to
// authorize move-instantaneous-rewards certificates. MIR certificates carry no
// field-level author, so Shelley through Babbage authorize them with signatures
// from a quorum of the currently delegated genesis keys. Ledger states used to
// validate those eras must implement this interface.
type GenesisDelegationState interface {
	// GenesisDelegateKeyHashes returns the key hash of every currently
	// delegated genesis key.
	GenesisDelegateKeyHashes() ([]Blake2b224, error)
	// GenesisUpdateQuorum returns the number of distinct genesis delegate
	// signatures required to authorize an MIR certificate.
	GenesisUpdateQuorum() (uint, error)
}

type PoolDelegation struct {
	Pool Blake2b224
}

// GovActionState holds the state of a governance proposal
type GovActionState struct {
	ActionId   GovActionId
	ActionType GovActionType
	ExpirySlot uint64
	// Action is the governance action itself, as proposed. It is optional
	// in the LedgerState contract: a state provider that only records the
	// action type leaves it nil. Rules that need the proposal's contents
	// (a hard-fork proposal's proposed protocol version, a parameter
	// change's modified parameters) skip their content-dependent check
	// when it is nil rather than guessing.
	Action GovAction
	// Add more fields as needed for validation
}

// GovPurposeRoots holds the current root of each governance-action purpose
// chain: the most recently enacted action of that purpose, or nil when
// nothing of that purpose has been enacted yet. It mirrors the GovRelation
// record reachable through pRootsL/toPrevGovActionIds in cardano-ledger's
// Cardano.Ledger.Conway.Governance.Proposals.
type GovPurposeRoots struct {
	PParamUpdate *GovActionId
	HardFork     *GovActionId
	Committee    *GovActionId
	Constitution *GovActionId
}

// GovPurposeRootsState is the optional ledger-state capability exposing the
// current root of each governance-action purpose chain. A ledger state that
// implements it gets the full Conway ancestry rule enforced (a proposal's
// predecessor must be the purpose root or a pending proposal of the same
// purpose); one that does not is limited to ancestor existence and purpose
// matching.
type GovPurposeRootsState interface {
	GovPurposeRoots() (*GovPurposeRoots, error)
}

// GovState defines the interface for querying governance state
type GovState interface {
	// Committee queries
	// CommitteeMember resolves a cold credential against both the current
	// committee and every pending UpdateCommittee proposal. It returns nil only
	// when the credential is neither a current nor a potential future member.
	CommitteeMember(coldKey Blake2b224) (*CommitteeMember, error)
	// CommitteeMembers returns the current committee members.
	CommitteeMembers() ([]CommitteeMember, error)

	// DRep queries
	DRepRegistration(credential Blake2b224) (*DRepRegistration, error)
	DRepRegistrations() ([]DRepRegistration, error)

	// Constitution
	Constitution() (*Constitution, error)

	// Treasury value
	TreasuryValue() (uint64, error)

	// Governance action queries
	GovActionById(GovActionId) (*GovActionState, error)
	GovActionExists(GovActionId) bool
}
