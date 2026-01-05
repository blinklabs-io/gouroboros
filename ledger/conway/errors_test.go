package conway

import (
"math/big"
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestConway_CostModelsPresent_UnresolvedReferenceInputReturnsError(
	t *testing.T,
) {
	var slot uint64 = 0
	ls := &test_ledger.MockLedgerState{
		UtxoByIdFunc: func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		},
	}
	var pp common.ProtocolParameters = &ConwayProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, common.ErrReferenceInputResolution) {
		t.Fatalf("expected ErrReferenceInputResolution, got %v", err)
	}
}

func TestConway_CostModelsPresent_UnresolvedReferenceInputUnwraps(
	t *testing.T,
) {
	var slot uint64 = 0
	ls := &test_ledger.MockLedgerState{
		UtxoByIdFunc: func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		},
	}
	var pp common.ProtocolParameters = &ConwayProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var refErr common.ReferenceInputResolutionError
	if !errors.As(err, &refErr) {
		t.Fatalf(
			"expected ReferenceInputResolutionError via errors.As, got %T",
			err,
		)
	}
	if refErr.Err == nil || refErr.Err.Error() != "utxo not found" {
		t.Fatalf("expected inner error 'utxo not found', got %v", refErr.Err)
	}
}
