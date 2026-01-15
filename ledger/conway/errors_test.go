package conway

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
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

func TestConway_CostModelsPresent_ResolvedReferenceInputChecksCostModels(
	t *testing.T,
) {
	var slot uint64 = 0

	// construct an output that contains a script reference (PlutusV2)
	addr := common.Address{}
	amount := uint64(1000)

	// PlutusV2Script is []byte
	plutus := &common.PlutusV2Script{0x01, 0x02}
	scriptRef := &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV2,
		Script: plutus,
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress:  addr,
		OutputAmount:   mary.MaryTransactionOutputValue{Amount: amount},
		TxOutScriptRef: scriptRef,
	}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	utxo := common.Utxo{
		Id:     input,
		Output: &output,
	}

	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{utxo})

	var pp common.ProtocolParameters = &ConwayProtocolParameters{}

	tmpTx := &ConwayTransaction{}
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

	// Now provide a dummy cost model for PlutusV2 (version 1)
	if cp, ok := pp.(*ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[1] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("expected no error after providing cost model, got %v", err)
	}
}

func TestConway_CostModelsPresent_ResolvedReferenceInput_PlutusV1(
	t *testing.T,
) {
	var slot uint64 = 0

	addr := common.Address{}
	amount := uint64(500)
	plutus := &common.PlutusV1Script{0x0A}
	scriptRef := &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV1,
		Script: plutus,
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress:  addr,
		OutputAmount:   mary.MaryTransactionOutputValue{Amount: amount},
		TxOutScriptRef: scriptRef,
	}

	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)
	utxo := common.Utxo{Id: input, Output: &output}
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{utxo})
	var pp common.ProtocolParameters = &ConwayProtocolParameters{}

	tmpTx := &ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	if cp, ok := pp.(*ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[0] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("expected no error after providing cost model, got %v", err)
	}
}

func TestConway_CostModelsPresent_ResolvedReferenceInput_PlutusV3(
	t *testing.T,
) {
	var slot uint64 = 0

	addr := common.Address{}
	amount := uint64(750)
	plutus := &common.PlutusV3Script{0x0B}
	scriptRef := &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV3,
		Script: plutus,
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress:  addr,
		OutputAmount:   mary.MaryTransactionOutputValue{Amount: amount},
		TxOutScriptRef: scriptRef,
	}

	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	utxo := common.Utxo{Id: input, Output: &output}
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{utxo})
	var pp common.ProtocolParameters = &ConwayProtocolParameters{}

	tmpTx := &ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	if cp, ok := pp.(*ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[2] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("expected no error after providing cost model, got %v", err)
	}
}

func TestConway_ScriptDataHash_MissingScriptDataHashError_Is(t *testing.T) {
	var slot uint64 = 0
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{})
	pp := &ConwayProtocolParameters{}

	// Create a transaction with redeemers but no script data hash
	tmpTx := &ConwayTransaction{}
	tmpTx.WitnessSet.WsRedeemers.Redeemers = map[common.RedeemerKey]common.RedeemerValue{
		{Tag: 0, Index: 0}: {},
	}
	var tx common.Transaction = tmpTx

	err := UtxoValidateScriptDataHash(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected MissingScriptDataHashError, got nil")
	}
	if !errors.Is(err, common.ErrMissingScriptDataHash) {
		t.Fatalf(
			"expected errors.Is(err, ErrMissingScriptDataHash) to be true, got false: %v",
			err,
		)
	}
}

func TestConway_ScriptDataHash_MissingScriptDataHashError_As(t *testing.T) {
	var slot uint64 = 0
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{})
	pp := &ConwayProtocolParameters{}

	// Create a transaction with redeemers but no script data hash
	tmpTx := &ConwayTransaction{}
	tmpTx.WitnessSet.WsRedeemers.Redeemers = map[common.RedeemerKey]common.RedeemerValue{
		{Tag: 0, Index: 0}: {},
	}
	var tx common.Transaction = tmpTx

	err := UtxoValidateScriptDataHash(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected MissingScriptDataHashError, got nil")
	}
	var mErr common.MissingScriptDataHashError
	if !errors.As(err, &mErr) {
		t.Fatalf(
			"expected MissingScriptDataHashError via errors.As, got %T",
			err,
		)
	}
}

func TestConway_ScriptDataHash_ExtraneousScriptDataHashError_Is(t *testing.T) {
	var slot uint64 = 0
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{})
	pp := &ConwayProtocolParameters{}

	// Create a transaction with a script data hash but no Plutus scripts or redeemers
	tmpTx := &ConwayTransaction{}
	dummyHash := common.Blake2b256{}
	tmpTx.Body.TxScriptDataHash = &dummyHash
	var tx common.Transaction = tmpTx

	err := UtxoValidateScriptDataHash(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected ExtraneousScriptDataHashError, got nil")
	}
	if !errors.Is(err, common.ErrExtraneousScriptDataHash) {
		t.Fatalf(
			"expected errors.Is(err, ErrExtraneousScriptDataHash) to be true, got false: %v",
			err,
		)
	}
}

func TestConway_ScriptDataHash_ExtraneousScriptDataHashError_As(t *testing.T) {
	var slot uint64 = 0
	ls := test_ledger.NewMockLedgerStateWithUtxos([]common.Utxo{})
	pp := &ConwayProtocolParameters{}

	// Create a transaction with a script data hash but no Plutus scripts or redeemers
	tmpTx := &ConwayTransaction{}
	dummyHash := common.Blake2b256{}
	tmpTx.Body.TxScriptDataHash = &dummyHash
	var tx common.Transaction = tmpTx

	err := UtxoValidateScriptDataHash(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected ExtraneousScriptDataHashError, got nil")
	}
	var eErr common.ExtraneousScriptDataHashError
	if !errors.As(err, &eErr) {
		t.Fatalf(
			"expected ExtraneousScriptDataHashError via errors.As, got %T",
			err,
		)
	}
}
