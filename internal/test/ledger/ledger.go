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

package test_ledger

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// Compile-time checks that MockLedgerState implements LedgerState and all child interfaces
var (
	_ common.LedgerState = (*MockLedgerState)(nil)
	_ common.UtxoState   = (*MockLedgerState)(nil)
	_ common.CertState   = (*MockLedgerState)(nil)
	_ common.SlotState   = (*MockLedgerState)(nil)
	_ common.PoolState   = (*MockLedgerState)(nil)
	_ common.RewardState = (*MockLedgerState)(nil)
	_ common.GovState    = (*MockLedgerState)(nil)
)

// MockLedgerState is the canonical internal mock used by tests. Tests should
// construct &test_ledger.MockLedgerState{} and configure fields (e.g.
// NetworkIdVal, UtxoByIdFunc) to control behavior. Keeping this in an
// internal package prevents external consumers from depending on test-only
// APIs while allowing in-repo tests to reuse the same mock.
type MockLedgerState struct {
	NetworkIdVal      uint
	AdaPotsVal        common.AdaPots
	RewardSnapshotVal common.RewardSnapshot
	PoolRegistrations []common.PoolRegistrationCertificate
	UtxoByIdFunc      func(common.TransactionInput) (common.Utxo, error)
	// CalculateRewardsFunc optionally overrides reward calculation behavior for tests.
	// If nil, the mock will call common.CalculateRewards by default.
	CalculateRewardsFunc func(common.AdaPots, common.RewardSnapshot, common.RewardParameters) (*common.RewardCalculationResult, error)
	// StakeRegistrationFunc optionally overrides stake registration lookup.
	// If nil, returns empty slice and nil error (no registrations found).
	StakeRegistrationFunc func([]byte) ([]common.StakeRegistrationCertificate, error)
	// SlotToTimeFunc optionally overrides slot-to-time conversion.
	// If nil, returns error indicating mock is not configured.
	SlotToTimeFunc func(uint64) (time.Time, error)
	// TimeToSlotFunc optionally overrides time-to-slot conversion.
	// If nil, returns error indicating mock is not configured.
	TimeToSlotFunc func(time.Time) (uint64, error)
	// CostModelsVal holds Plutus cost models for script execution validation.
	CostModelsVal map[common.PlutusLanguage]common.CostModel
	// Governance state
	CommitteeMembersVal  []common.CommitteeMember
	DRepRegistrationsVal []common.DRepRegistration
	ConstitutionVal      *common.Constitution
	TreasuryValueVal     uint64
	// Stake credential registration state (credential hash -> registered)
	StakeRegistrationsVal map[common.Blake2b224]bool
	// Reward account balances (credential hash -> balance in lovelace)
	RewardAccountsVal map[common.Blake2b224]uint64
	// Governance action state (action id string -> state)
	GovActionsVal map[string]*common.GovActionState
}

func (m *MockLedgerState) UtxoById(
	id common.TransactionInput,
) (common.Utxo, error) {
	if m.UtxoByIdFunc != nil {
		return m.UtxoByIdFunc(id)
	}
	return common.Utxo{}, errors.New("not found")
}

func (m *MockLedgerState) StakeRegistration(
	key []byte,
) ([]common.StakeRegistrationCertificate, error) {
	if m.StakeRegistrationFunc != nil {
		return m.StakeRegistrationFunc(key)
	}
	return []common.StakeRegistrationCertificate{}, nil
}

func (m *MockLedgerState) SlotToTime(
	slot uint64,
) (time.Time, error) {
	if m.SlotToTimeFunc != nil {
		return m.SlotToTimeFunc(slot)
	}
	return time.Time{}, fmt.Errorf(
		"MockLedgerState.SlotToTimeFunc not configured for slot %d",
		slot,
	)
}

func (m *MockLedgerState) TimeToSlot(
	t time.Time,
) (uint64, error) {
	if m.TimeToSlotFunc != nil {
		return m.TimeToSlotFunc(t)
	}
	return 0, errors.New("MockLedgerState.TimeToSlotFunc not configured")
}

func (m *MockLedgerState) PoolCurrentState(
	poolKeyHash common.PoolKeyHash,
) (*common.PoolRegistrationCertificate, *uint64, error) {
	for _, cert := range m.PoolRegistrations {
		if cert.Operator == poolKeyHash {
			c := cert
			return &c, nil, nil
		}
	}
	return nil, nil, nil
}

func (m *MockLedgerState) CalculateRewards(
	pots common.AdaPots,
	snapshot common.RewardSnapshot,
	params common.RewardParameters,
) (*common.RewardCalculationResult, error) {
	if m.CalculateRewardsFunc != nil {
		return m.CalculateRewardsFunc(pots, snapshot, params)
	}
	return common.CalculateRewards(pots, snapshot, params)
}

func (m *MockLedgerState) GetAdaPots() common.AdaPots { return m.AdaPotsVal }

func (m *MockLedgerState) UpdateAdaPots(
	pots common.AdaPots,
) error {
	m.AdaPotsVal = pots
	return nil
}

func (m *MockLedgerState) GetRewardSnapshot(
	epoch uint64,
) (common.RewardSnapshot, error) {
	return m.RewardSnapshotVal, nil
}
func (m *MockLedgerState) NetworkId() uint { return m.NetworkIdVal }

func (m *MockLedgerState) CostModels() map[common.PlutusLanguage]common.CostModel {
	if m.CostModelsVal != nil {
		return m.CostModelsVal
	}
	return map[common.PlutusLanguage]common.CostModel{}
}

// GovState interface implementation

func (m *MockLedgerState) CommitteeMember(
	coldKey common.Blake2b224,
) (*common.CommitteeMember, error) {
	for _, member := range m.CommitteeMembersVal {
		if member.ColdKey == coldKey {
			return &member, nil
		}
	}
	return nil, nil
}

func (m *MockLedgerState) CommitteeMembers() ([]common.CommitteeMember, error) {
	return m.CommitteeMembersVal, nil
}

func (m *MockLedgerState) DRepRegistration(
	credential common.Blake2b224,
) (*common.DRepRegistration, error) {
	for _, drep := range m.DRepRegistrationsVal {
		if drep.Credential == credential {
			return &drep, nil
		}
	}
	return nil, nil
}

func (m *MockLedgerState) DRepRegistrations() ([]common.DRepRegistration, error) {
	return m.DRepRegistrationsVal, nil
}

func (m *MockLedgerState) Constitution() (*common.Constitution, error) {
	return m.ConstitutionVal, nil
}

func (m *MockLedgerState) TreasuryValue() (uint64, error) {
	return m.TreasuryValueVal, nil
}

// CertState interface - new methods

func (m *MockLedgerState) IsStakeCredentialRegistered(
	cred common.Credential,
) bool {
	if m.StakeRegistrationsVal == nil {
		return false
	}
	return m.StakeRegistrationsVal[cred.Credential]
}

// PoolState interface - new methods

func (m *MockLedgerState) IsPoolRegistered(
	poolKeyHash common.PoolKeyHash,
) bool {
	for _, cert := range m.PoolRegistrations {
		if cert.Operator == poolKeyHash {
			return true
		}
	}
	return false
}

// RewardState interface - new methods

func (m *MockLedgerState) IsRewardAccountRegistered(
	cred common.Credential,
) bool {
	// Reward account registration is tied to stake credential registration.
	// A reward account exists if and only if the stake credential is registered.
	return m.IsStakeCredentialRegistered(cred)
}

func (m *MockLedgerState) RewardAccountBalance(
	cred common.Credential,
) (*uint64, error) {
	// Returns nil for unregistered accounts (absent from map).
	// Returns a pointer to the balance (which may be 0) for registered accounts.
	if m.RewardAccountsVal == nil {
		return nil, nil
	}
	balance, exists := m.RewardAccountsVal[cred.Credential]
	if !exists {
		return nil, nil
	}
	return &balance, nil
}

// GovState interface - new methods

func (m *MockLedgerState) GovActionById(
	id common.GovActionId,
) (*common.GovActionState, error) {
	if m.GovActionsVal == nil {
		return nil, nil
	}
	// Create key from action id
	key := fmt.Sprintf("%x#%d", id.TransactionId[:], id.GovActionIdx)
	state, exists := m.GovActionsVal[key]
	if !exists {
		return nil, nil
	}
	return state, nil
}

func (m *MockLedgerState) GovActionExists(id common.GovActionId) bool {
	state, _ := m.GovActionById(id)
	return state != nil
}

// MockProtocolParamsRules is a simple protocol params provider used in tests.
// Utxorpc() returns a zero-value struct to prevent nil pointer dereferences.
// Tests needing specific params should set PParams field.
type MockProtocolParamsRules struct {
	PParams *cardano.PParams
}

func (m *MockProtocolParamsRules) Utxorpc() (*cardano.PParams, error) {
	if m.PParams != nil {
		return m.PParams, nil
	}
	return &cardano.PParams{}, nil
}

// NewMockLedgerStateWithUtxos creates a MockLedgerState with lookup behavior for provided UTXOs.
// This helper uses bytes.Equal for efficient byte array comparison.
func NewMockLedgerStateWithUtxos(utxos []common.Utxo) *MockLedgerState {
	return &MockLedgerState{
		UtxoByIdFunc: func(id common.TransactionInput) (common.Utxo, error) {
			for _, u := range utxos {
				if id.Index() == u.Id.Index() &&
					bytes.Equal(id.Id().Bytes(), u.Id.Id().Bytes()) {
					return u, nil
				}
			}
			return common.Utxo{}, errors.New("not found")
		},
	}
}
