package byron_test

import (
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// Unit test for ByronTransactionInput.Utxorpc()
func TestByronTransactionInput_Utxorpc(t *testing.T) {
	input := byron.NewByronTransactionInput("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1)

	got := input.Utxorpc()
	want := &utxorpc.TxInput{
		TxHash:      input.Id().Bytes(),
		OutputIndex: input.Index(),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ByronTransactionInput.Utxorpc() mismatch\nGot: %+v\nWant: %+v", got, want)
	}
}

// Unit test for ByronTransactionOutput.Utxorpc()
func TestByronTransactionOutput_Utxorpc(t *testing.T) {
	address := common.Address{}
	output := byron.ByronTransactionOutput{
		OutputAddress: address,
		OutputAmount:  5000,
	}

	got := output.Utxorpc()
	want := &utxorpc.TxOutput{
		Address: address.Bytes(),
		Coin:    output.OutputAmount,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ByronTransactionOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v", got, want)
	}
}

// Unit test for ByronTransaction.Utxorpc()
func TestByronTransaction_Utxorpc_Empty(t *testing.T) {
	// Create a dummy ByronTransaction
	tx := &byron.ByronTransaction{}

	// Run Utxorpc conversion
	result := tx.Utxorpc()

	// Validate it's not nil
	if result == nil {
		t.Fatal("ByronTransaction.Utxorpc() returned nil; expected empty utxorpc.Tx object")
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
