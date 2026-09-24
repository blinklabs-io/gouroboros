package babbage_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

func TestCostModelsPresent_UnresolvedReferenceInputReturnsError(t *testing.T) {
	// Create a BabbageTransaction with a single reference input so the lookup is attempted
	var slot uint64 = 0
	ls := mockledger.NewLedgerStateBuilder().WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
		return common.Utxo{}, errors.New("utxo not found")
	}).Build()
	var pp common.ProtocolParameters = &babbage.BabbageProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &babbage.BabbageTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := babbage.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	ls := mockledger.NewLedgerStateBuilder().WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
		return common.Utxo{}, errors.New("utxo not found")
	}).Build()
	var pp common.ProtocolParameters = &babbage.BabbageProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &babbage.BabbageTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := babbage.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	plutus := common.PlutusV1Script{0x01, 0x02}
	scriptRef := &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV1,
		Script: plutus,
	}

	output := babbage.BabbageTransactionOutput{
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
		Output: output,
	}

	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).Build()

	var pp common.ProtocolParameters = &babbage.BabbageProtocolParameters{}

	tmpTx := &babbage.BabbageTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	// A reachable but unused reference script does not require a cost model.
	err := babbage.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("unused reference script should not require a cost model: %v", err)
	}
}
