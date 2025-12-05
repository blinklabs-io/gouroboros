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
	"errors"
	"testing"
	"time"

	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestVerifyTransaction(t *testing.T) {
	// Mock transaction - we don't need a real one for this test
	var tx Transaction

	slot := uint64(1000)
	ledgerState := &mockLedgerStateRules{}
	protocolParams := &mockProtocolParamsRules{}

	t.Run("all_rules_pass", func(t *testing.T) {
		rules := []UtxoValidationRuleFunc{
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
		}

		err := VerifyTransaction(tx, slot, ledgerState, protocolParams, rules)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("first_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("first rule failed")
		rules := []UtxoValidationRuleFunc{
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return expectedErr },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
		}

		err := VerifyTransaction(tx, slot, ledgerState, protocolParams, rules)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("middle_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("middle rule failed")
		rules := []UtxoValidationRuleFunc{
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return expectedErr },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
		}

		err := VerifyTransaction(tx, slot, ledgerState, protocolParams, rules)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("last_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("last rule failed")
		rules := []UtxoValidationRuleFunc{
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return nil },
			func(Transaction, uint64, LedgerState, ProtocolParameters) error { return expectedErr },
		}

		err := VerifyTransaction(tx, slot, ledgerState, protocolParams, rules)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("empty_rules", func(t *testing.T) {
		rules := []UtxoValidationRuleFunc{}

		err := VerifyTransaction(tx, slot, ledgerState, protocolParams, rules)
		if err != nil {
			t.Errorf("expected no error with empty rules, got %v", err)
		}
	})
}

// Mock types for testing
type mockLedgerStateRules struct{}

func (m *mockLedgerStateRules) UtxoById(
	input TransactionInput,
) (Utxo, error) {
	return Utxo{}, nil
}

func (m *mockLedgerStateRules) StakeRegistration(
	key []byte,
) ([]StakeRegistrationCertificate, error) {
	return nil, nil
}

func (m *mockLedgerStateRules) SlotToTime(
	slot uint64,
) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockLedgerStateRules) TimeToSlot(
	t time.Time,
) (uint64, error) {
	return 0, nil
}

func (m *mockLedgerStateRules) PoolCurrentState(
	poolKeyHash PoolKeyHash,
) (*PoolRegistrationCertificate, *uint64, error) {
	return nil, nil, nil
}

func (m *mockLedgerStateRules) CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLedgerStateRules) GetAdaPots() AdaPots              { return AdaPots{} }
func (m *mockLedgerStateRules) UpdateAdaPots(pots AdaPots) error { return nil }

func (m *mockLedgerStateRules) GetRewardSnapshot(
	epoch uint64,
) (RewardSnapshot, error) {
	return RewardSnapshot{}, nil
}
func (m *mockLedgerStateRules) NetworkId() uint { return 0 }

type mockProtocolParamsRules struct{}

func (m *mockProtocolParamsRules) Utxorpc() (*cardano.PParams, error) { return nil, nil }
