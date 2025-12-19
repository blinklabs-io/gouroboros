// Copyright 2025 Blink Labs Software

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
	"errors"
	"time"

	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// MockLedgerStateRefFail is a shared test helper that fails UtxoById lookups
// This file is intentionally non-_test.go so other package tests can import it.
type MockLedgerStateRefFail struct{}

func (m *MockLedgerStateRefFail) UtxoById(
	input TransactionInput,
) (Utxo, error) {
	return Utxo{}, errors.New("utxo not found")
}

func (m *MockLedgerStateRefFail) StakeRegistration(
	key []byte,
) ([]StakeRegistrationCertificate, error) {
	return nil, nil
}

func (m *MockLedgerStateRefFail) SlotToTime(
	slot uint64,
) (time.Time, error) {
	return time.Time{}, nil
}

func (m *MockLedgerStateRefFail) TimeToSlot(
	t time.Time,
) (uint64, error) {
	return 0, nil
}

func (m *MockLedgerStateRefFail) PoolCurrentState(
	poolKeyHash PoolKeyHash,
) (*PoolRegistrationCertificate, *uint64, error) {
	return nil, nil, nil
}

func (m *MockLedgerStateRefFail) CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	return nil, errors.New("not implemented")
}

func (m *MockLedgerStateRefFail) GetAdaPots() AdaPots { return AdaPots{} }

func (m *MockLedgerStateRefFail) UpdateAdaPots(
	pots AdaPots,
) error {
	return nil
}

func (m *MockLedgerStateRefFail) GetRewardSnapshot(
	epoch uint64,
) (RewardSnapshot, error) {
	return RewardSnapshot{}, nil
}
func (m *MockLedgerStateRefFail) NetworkId() uint { return 0 }

// MockLedgerStateRules is a lightweight mock of LedgerState used by some
// unit tests (previously defined locally in verify_rules_test.go).
type MockLedgerStateRules struct{}

func (m *MockLedgerStateRules) UtxoById(input TransactionInput) (Utxo, error) {
	return Utxo{}, nil
}

func (m *MockLedgerStateRules) StakeRegistration(
	key []byte,
) ([]StakeRegistrationCertificate, error) {
	return nil, nil
}

func (m *MockLedgerStateRules) SlotToTime(
	slot uint64,
) (time.Time, error) {
	return time.Time{}, nil
}

func (m *MockLedgerStateRules) TimeToSlot(
	t time.Time,
) (uint64, error) {
	return 0, nil
}

func (m *MockLedgerStateRules) PoolCurrentState(
	poolKeyHash PoolKeyHash,
) (*PoolRegistrationCertificate, *uint64, error) {
	return nil, nil, nil
}

func (m *MockLedgerStateRules) CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	return nil, errors.New("not implemented")
}

func (m *MockLedgerStateRules) GetAdaPots() AdaPots              { return AdaPots{} }
func (m *MockLedgerStateRules) UpdateAdaPots(pots AdaPots) error { return nil }

func (m *MockLedgerStateRules) GetRewardSnapshot(
	epoch uint64,
) (RewardSnapshot, error) {
	return RewardSnapshot{}, nil
}
func (m *MockLedgerStateRules) NetworkId() uint { return 0 }

// MockProtocolParamsRules is a simple protocol params provider used in tests.
type MockProtocolParamsRules struct{}

func (m *MockProtocolParamsRules) Utxorpc() (*cardano.PParams, error) { return nil, nil }
