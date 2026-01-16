package babbage

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestCostModelsPresent_UnresolvedReferenceInputReturnsError(t *testing.T) {
	// Create a BabbageTransaction with a single reference input so the lookup is attempted
	var slot uint64 = 0
	ls := &test_ledger.MockLedgerState{
		UtxoByIdFunc: func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		},
	}
	var pp common.ProtocolParameters = &BabbageProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &BabbageTransaction{}
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

func TestCostModelsPresent_UnresolvedReferenceInputUnwraps(t *testing.T) {
	// Create a BabbageTransaction with a single reference input so the lookup is attempted
	var slot uint64 = 0
	ls := &test_ledger.MockLedgerState{
		UtxoByIdFunc: func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		},
	}
	var pp common.ProtocolParameters = &BabbageProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &BabbageTransaction{}
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

func TestCostModelsPresent_ResolvedReferenceInputChecksCostModels(
	t *testing.T,
) {
	// Create a BabbageTransaction with a single reference input so the lookup is attempted
	var slot uint64 = 0

	// construct an output that contains a script reference (PlutusV1)
	addr := common.Address{}
	amount := mary.MaryTransactionOutputValue{Amount: 1000}

	// create a PlutusV1 script (PlutusV1Script is []byte)
	plutus := &common.PlutusV1Script{0x01, 0x02}
	scriptRef := &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV1,
		Script: plutus,
	}

	output := BabbageTransactionOutput{
		OutputAddress:  addr,
		OutputAmount:   amount,
		TxOutScriptRef: scriptRef,
	}

	// craft the UTxO that will be returned by the mock ledger state
	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	utxo := common.Utxo{
		Id:     input,
		Output: &output,
	}

	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{utxo})

	var pp common.ProtocolParameters = &BabbageProtocolParameters{}

	tmpTx := &BabbageTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	// First: missing cost models should return a MissingCostModelError
	err := UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	// Now provide the cost model for PlutusV1 and expect success
	if bp, ok := pp.(*BabbageProtocolParameters); ok {
		if bp.CostModels == nil {
			bp.CostModels = make(map[uint][]int64)
		}
		// populate a dummy non-empty cost model for Plutus V1 (version 0)
		bp.CostModels[0] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not BabbageProtocolParameters: %T", pp)
	}

	err = UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("expected no error after providing cost model, got %v", err)
	}
}
