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

package byron_test

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// Unit test for ByronTransactionInput.Utxorpc()
func TestByronTransactionInput_Utxorpc(t *testing.T) {
	input := byron.NewByronTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		1,
	)

	got, err := input.Utxorpc()
	if err != nil {
		t.Fatal("Could not get transaction input")
	}
	want := &cardano.TxInput{
		TxHash:      input.Id().Bytes(),
		OutputIndex: input.Index(),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"ByronTransactionInput.Utxorpc() mismatch\nGot: %+v\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for ByronTransactionOutput.Utxorpc()
func TestByronTransactionOutput_Utxorpc(t *testing.T) {
	address := common.Address{}
	output := byron.ByronTransactionOutput{
		OutputAddress: address,
		OutputAmount:  5000,
	}

	got, err := output.Utxorpc()
	assert.NoError(t, err)
	addr, err := address.Bytes()
	assert.NoError(t, err)
	want := &cardano.TxOutput{
		Address: addr,
		Coin:    output.OutputAmount,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"ByronTransactionOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for ByronTransaction.Utxorpc()
func TestByronTransaction_Utxorpc_Empty(t *testing.T) {
	// Create a dummy ByronTransaction
	tx := &byron.ByronTransaction{}

	// Run Utxorpc conversion
	result, err := tx.Utxorpc()
	if err != nil {
		t.Fatal("Could not convert Utxorpc")
	}

	// Validate it's not nil
	if result == nil {
		t.Fatal(
			"ByronTransaction.Utxorpc() returned nil; expected empty cardano.Tx object",
		)
	}

	// Validate that it's the zero value
	if len(result.Inputs) != 0 {
		t.Errorf("Expected zero inputs, got %d", len(result.Inputs))
	}
	if len(result.Outputs) != 0 {
		t.Errorf("Expected zero outputs, got %d", len(result.Outputs))
	}
	if result.Fee != 0 {
		t.Errorf("Expected fee = 0, got %d", result.Fee)
	}
}

func TestByronTransactionOutputString(t *testing.T) {
	addr, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	out := byron.ByronTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	s := out.String()
	re := regexp.MustCompile(`^\(ByronTransactionOutput address=[1-9A-HJ-NP-Za-km-z]+ amount=456\)$`)
	if !re.MatchString(s) {
		t.Fatalf("unexpected string: %s", s)
	}
}
