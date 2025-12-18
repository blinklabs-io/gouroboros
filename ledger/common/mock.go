package common

import (
	"errors"
	"time"
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
