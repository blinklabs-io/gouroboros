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

package common

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
}

// PoolState defines the interface for querying the current pool state
type PoolState interface {
	// PoolCurrentState returns the latest active registration certificate for the given pool key hash.
	// It also returns the epoch of a pending retirement certificate, if one exists.
	// If the pool is not registered, the registration certificate will be nil.
	PoolCurrentState(PoolKeyHash) (*PoolRegistrationCertificate, *uint64, error)
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

// Minimal placeholder types used by the extended interface. These are intentionally
// lightweight so tests and era packages can compile while we wire real parsing.
type (
	Constitution   struct{}
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

type DRepRegistration struct {
	Credential Blake2b224
	Anchor     *GovAnchor
	Deposit    uint64
}

type DRepDelegation struct {
	DRep Blake2b224
}

type PoolDelegation struct {
	Pool Blake2b224
}

// GovState defines the interface for querying governance state
type GovState interface {
	// Committee queries
	CommitteeMember(coldKey Blake2b224) (*CommitteeMember, error)
	CommitteeMembers() ([]CommitteeMember, error)

	// DRep queries
	DRepRegistration(credential Blake2b224) (*DRepRegistration, error)
	DRepRegistrations() ([]DRepRegistration, error)

	// Constitution
	Constitution() (*Constitution, error)

	// Treasury value
	TreasuryValue() (uint64, error)
}
