package conway_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
)

func TestConway_CostModelsPresent_UnresolvedReferenceInputReturnsError(
	t *testing.T,
) {
	var slot uint64 = 0
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		}).
		Build()
	var pp common.ProtocolParameters = &conway.ConwayProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &conway.ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("utxo not found")
		}).
		Build()
	var pp common.ProtocolParameters = &conway.ConwayProtocolParameters{}

	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tmpTx := &conway.ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	plutus := common.PlutusV2Script{0x01, 0x02}
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
		Output: output,
	}

	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).Build()

	var pp common.ProtocolParameters = &conway.ConwayProtocolParameters{}

	tmpTx := &conway.ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	// First: missing cost models should return a MissingCostModelError
	err := conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	// Now provide a dummy cost model for PlutusV2 (version 1)
	if cp, ok := pp.(*conway.ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[1] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	plutus := common.PlutusV1Script{0x0A}
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
	utxo := common.Utxo{Id: input, Output: output}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).Build()
	var pp common.ProtocolParameters = &conway.ConwayProtocolParameters{}

	tmpTx := &conway.ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	if cp, ok := pp.(*conway.ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[0] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
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
	plutus := common.PlutusV3Script{0x0B}
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
	utxo := common.Utxo{Id: input, Output: output}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).Build()
	var pp common.ProtocolParameters = &conway.ConwayProtocolParameters{}

	tmpTx := &conway.ConwayTransaction{}
	tmpTx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err == nil {
		t.Fatal("expected error due to missing cost model, got nil")
	}
	var mErr common.MissingCostModelError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MissingCostModelError, got %T", err)
	}

	if cp, ok := pp.(*conway.ConwayProtocolParameters); ok {
		if cp.CostModels == nil {
			cp.CostModels = make(map[uint][]int64)
		}
		cp.CostModels[2] = []int64{1}
	} else {
		t.Fatalf("protocol parameters not ConwayProtocolParameters: %T", pp)
	}

	err = conway.UtxoValidateCostModelsPresent(tx, slot, ls, pp)
	if err != nil {
		t.Fatalf("expected no error after providing cost model, got %v", err)
	}
}

func TestConway_ScriptDataHash_MissingScriptDataHashError_Is(t *testing.T) {
	var slot uint64 = 0
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{}).Build()
	pp := &conway.ConwayProtocolParameters{}

	// Create a transaction with redeemers but no script data hash
	tmpTx := &conway.ConwayTransaction{}
	tmpTx.WitnessSet.WsRedeemers.Redeemers = map[common.RedeemerKey]common.RedeemerValue{
		{Tag: 0, Index: 0}: {},
	}
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateScriptDataHash(tx, slot, ls, pp)
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
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{}).Build()
	pp := &conway.ConwayProtocolParameters{}

	// Create a transaction with redeemers but no script data hash
	tmpTx := &conway.ConwayTransaction{}
	tmpTx.WitnessSet.WsRedeemers.Redeemers = map[common.RedeemerKey]common.RedeemerValue{
		{Tag: 0, Index: 0}: {},
	}
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateScriptDataHash(tx, slot, ls, pp)
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
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{}).Build()
	pp := &conway.ConwayProtocolParameters{}

	// Create a transaction with a script data hash but no Plutus scripts or redeemers
	tmpTx := &conway.ConwayTransaction{}
	dummyHash := common.Blake2b256{}
	tmpTx.Body.TxScriptDataHash = &dummyHash
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateScriptDataHash(tx, slot, ls, pp)
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
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{}).Build()
	pp := &conway.ConwayProtocolParameters{}

	// Create a transaction with a script data hash but no Plutus scripts or redeemers
	tmpTx := &conway.ConwayTransaction{}
	dummyHash := common.Blake2b256{}
	tmpTx.Body.TxScriptDataHash = &dummyHash
	var tx common.Transaction = tmpTx

	err := conway.UtxoValidateScriptDataHash(tx, slot, ls, pp)
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

func TestDuplicateVrfKeyError(t *testing.T) {
	err := conway.DuplicateVrfKeyError{
		VrfKeyHash:     common.Blake2b256{0x01},
		NewPoolId:      common.PoolKeyHash{0x02},
		ExistingPoolId: common.PoolKeyHash{0x03},
	}
	assert.Contains(t, err.Error(), "duplicate VRF key")
}

func TestCCVotingRestrictionError(t *testing.T) {
	err := conway.CCVotingRestrictionError{
		VoterId:     common.Blake2b224{0x01},
		ActionId:    common.GovActionId{TransactionId: common.Blake2b256{0x02}, GovActionIdx: 1},
		Restriction: "CC cannot vote on this action type",
	}
	assert.Contains(t, err.Error(), "voting restriction")
}

func TestNonMatchingWithdrawalError(t *testing.T) {
	err := conway.NonMatchingWithdrawalError{
		RewardAccount: common.Address{},
		Expected:      1000,
		Actual:        500,
	}
	assert.Contains(t, err.Error(), "non-matching withdrawal")
	assert.Contains(t, err.Error(), "expected 1000")
}

func TestPPViewHashesDontMatchError(t *testing.T) {
	err := conway.PPViewHashesDontMatchError{
		ProvidedHash:   common.Blake2b256{0x01},
		ComputedHash:   common.Blake2b256{0x02},
		ExpectedPPData: []byte{0x03, 0x04},
	}
	assert.Contains(t, err.Error(), "protocol parameter")
	assert.Contains(t, err.Error(), "provided")
	assert.Contains(t, err.Error(), "computed")
}
