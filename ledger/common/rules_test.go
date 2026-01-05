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

package common_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/plutigo/data"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// mockTxEmpty implements the minimal Transaction interface used by the
// helpers. It represents a transaction with no required signers and no
// witnesses.
type mockTxEmpty struct {
	common.TransactionBodyBase
}

func (m *mockTxEmpty) Type() int    { return 0 }
func (m *mockTxEmpty) Cbor() []byte { return nil }

func (m *mockTxEmpty) Hash() common.Blake2b256 { return common.Blake2b256{} }

func (m *mockTxEmpty) LeiosHash() common.Blake2b256            { return common.Blake2b256{} }
func (m *mockTxEmpty) Metadata() common.TransactionMetadatum   { return nil }
func (m *mockTxEmpty) AuxiliaryData() common.AuxiliaryData     { return nil }
func (m *mockTxEmpty) IsValid() bool                           { return true }
func (m *mockTxEmpty) Consumed() []common.TransactionInput     { return nil }
func (m *mockTxEmpty) Produced() []common.Utxo                 { return nil }
func (m *mockTxEmpty) Witnesses() common.TransactionWitnessSet { return nil }

func (m *mockTxEmpty) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return 0, nil
}

func TestValidateRequiredVKeyWitnesses_Common(t *testing.T) {
	tx := &mockTxEmpty{}
	if err := common.ValidateRequiredVKeyWitnesses(tx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRedeemerAndScriptWitnesses_Common(t *testing.T) {
	tx := &mockTxEmpty{}
	if err := common.ValidateRedeemerAndScriptWitnesses(tx, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Tests for VerifyTransaction moved from verify_rules_test.go
func TestVerifyTransaction(t *testing.T) {
	var tx common.Transaction

	slot := uint64(1000)
	ledgerState := &test_ledger.MockLedgerState{
		UtxoByIdFunc: func(input common.TransactionInput) (common.Utxo, error) { return common.Utxo{}, nil },
	}
	protocolParams := &test_ledger.MockProtocolParamsRules{}

	t.Run("all_rules_pass", func(t *testing.T) {
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("first_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("first rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("middle_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("middle rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("last_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("last rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("empty_rules", func(t *testing.T) {
		rules := []common.UtxoValidationRuleFunc{}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err != nil {
			t.Errorf("expected no error with empty rules, got %v", err)
		}
	})
}

// Use centralized mocks from ledger/common/mock.go

// mockTxInput implements TransactionInput minimally for constructing
// ReferenceInputResolutionError in tests.
type mockTxInput struct{ id common.Blake2b256 }

func (m *mockTxInput) Id() common.Blake2b256 { return m.id }
func (m *mockTxInput) Index() uint32         { return 0 }

func (m *mockTxInput) String() string                     { return m.id.String() }
func (m *mockTxInput) Utxorpc() (*cardano.TxInput, error) { return nil, nil }
func (m *mockTxInput) ToPlutusData() data.PlutusData      { return nil }

func TestReferenceInputResolutionSentinel(t *testing.T) {
	// Construct ReferenceInputResolutionError using the concrete type defined
	// in common/errors.go. Provide a minimal mock input and inner error.
	inner := errors.New("utxo not found")
	rie := common.ReferenceInputResolutionError{
		Input: &mockTxInput{id: common.Blake2b256{}},
		Err:   inner,
	}
	err := rie

	if !errors.Is(err, common.ErrReferenceInputResolution) {
		t.Fatalf("expected errors.Is to match ErrReferenceInputResolution")
	}

	var out common.ReferenceInputResolutionError
	if !errors.As(err, &out) {
		t.Fatalf(
			"expected errors.As to unwrap to ReferenceInputResolutionError",
		)
	}

	if out.Err == nil || out.Err.Error() != "utxo not found" {
		t.Fatalf("expected inner message 'utxo not found', got %q", out.Err)
	}
}
