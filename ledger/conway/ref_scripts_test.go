// Copyright 2026 Blink Labs Software
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

package conway_test

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func conwayRefScriptInput(
	t *testing.T,
	hashByte byte,
	index int,
	scriptSize int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input := shelley.NewShelleyTransactionInput(
		strings.Repeat(fmt.Sprintf("%02x", hashByte), 32),
		index,
	)
	output := &babbage.BabbageTransactionOutput{
		TxOutScriptRef: &common.ScriptRef{
			Script: make(common.PlutusV3Script, scriptSize),
		},
	}
	return input, common.Utxo{Id: input, Output: output}
}

func conwayRefScriptLedgerState(
	t *testing.T,
	utxos ...common.Utxo,
) common.LedgerState {
	t.Helper()
	byInput := make(map[string]common.Utxo, len(utxos))
	for _, utxo := range utxos {
		byInput[utxo.Id.String()] = utxo
	}
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			utxo, ok := byInput[input.String()]
			if !ok {
				return common.Utxo{}, fmt.Errorf("utxo not found: %s", input)
			}
			return utxo, nil
		}).
		Build()
}

func conwayTxWithInputs(
	inputs []shelley.ShelleyTransactionInput,
	referenceInputs []shelley.ShelleyTransactionInput,
) *conway.ConwayTransaction {
	return &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(inputs),
			TxReferenceInputs: cbor.NewSetType(
				referenceInputs,
				false,
			),
		},
		TxIsValid: true,
	}
}

func conwayBlockWithTransactions(
	txs ...*conway.ConwayTransaction,
) *conway.ConwayBlock {
	bodies := make([]conway.ConwayTransactionBody, len(txs))
	witnesses := make([]conway.ConwayTransactionWitnessSet, len(txs))
	for idx, tx := range txs {
		bodies[idx] = tx.Body
		witnesses[idx] = tx.WitnessSet
	}
	return &conway.ConwayBlock{
		TransactionBodies:      bodies,
		TransactionWitnessSets: witnesses,
	}
}

func TestConsumedReferenceScriptSizeInputUnion(t *testing.T) {
	inputA, utxoA := conwayRefScriptInput(t, 0x01, 0, 60)
	inputB, utxoB := conwayRefScriptInput(t, 0x02, 0, 60)
	ls := conwayRefScriptLedgerState(t, utxoA, utxoB)

	t.Run("regular input", func(t *testing.T) {
		size, err := common.ConsumedReferenceScriptSize(
			conwayTxWithInputs(
				[]shelley.ShelleyTransactionInput{inputA},
				nil,
			),
			ls,
		)
		require.NoError(t, err)
		require.Equal(t, uint64(60), size)
	})

	t.Run("reference input", func(t *testing.T) {
		size, err := common.ConsumedReferenceScriptSize(
			conwayTxWithInputs(
				nil,
				[]shelley.ShelleyTransactionInput{inputA},
			),
			ls,
		)
		require.NoError(t, err)
		require.Equal(t, uint64(60), size)
	})

	t.Run("overlap counted once", func(t *testing.T) {
		size, err := common.ConsumedReferenceScriptSize(
			conwayTxWithInputs(
				[]shelley.ShelleyTransactionInput{inputA},
				[]shelley.ShelleyTransactionInput{inputA},
			),
			ls,
		)
		require.NoError(t, err)
		require.Equal(t, uint64(60), size)
	})

	t.Run("distinct identical scripts counted twice", func(t *testing.T) {
		size, err := common.ConsumedReferenceScriptSize(
			conwayTxWithInputs(
				[]shelley.ShelleyTransactionInput{inputA},
				[]shelley.ShelleyTransactionInput{inputB},
			),
			ls,
		)
		require.NoError(t, err)
		require.Equal(t, uint64(120), size)
	})

	t.Run("publishing only", func(t *testing.T) {
		tx := conwayTxWithInputs(nil, nil)
		tx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
			{
				TxOutScriptRef: &common.ScriptRef{
					Script: make(common.PlutusV3Script, 60),
				},
			},
		}
		size, err := common.ConsumedReferenceScriptSize(tx, ls)
		require.NoError(t, err)
		require.Zero(t, size)
	})

	t.Run("unresolved regular input is left to bad-input validation", func(t *testing.T) {
		tx := conwayTxWithInputs(
			[]shelley.ShelleyTransactionInput{inputA},
			nil,
		)
		tx.SetCbor([]byte{0x84, 0xa0, 0xa0, 0xf5, 0xf6})
		missingState := conwayRefScriptLedgerState(t)
		size, err := common.ConsumedReferenceScriptSize(tx, missingState)
		require.NoError(t, err)
		require.Zero(t, size)
		require.NoError(t, conway.UtxoValidateFeeTooSmallUtxo(
			tx,
			0,
			missingState,
			&conway.ConwayProtocolParameters{},
		))
	})

	t.Run("unresolved reference input remains an error", func(t *testing.T) {
		tx := conwayTxWithInputs(
			nil,
			[]shelley.ShelleyTransactionInput{inputA},
		)
		_, err := common.ConsumedReferenceScriptSize(
			tx,
			conwayRefScriptLedgerState(t),
		)
		require.ErrorContains(t, err, "resolve consumed reference-script input")
	})

	t.Run("unresolved overlapping input remains an error", func(t *testing.T) {
		tx := conwayTxWithInputs(
			[]shelley.ShelleyTransactionInput{inputA},
			[]shelley.ShelleyTransactionInput{inputA},
		)
		_, err := common.ConsumedReferenceScriptSize(
			tx,
			conwayRefScriptLedgerState(t),
		)
		require.ErrorContains(t, err, "resolve consumed reference-script input")
	})
}

func TestConwayRefScriptSizePerTxBoundsAndPublishing(t *testing.T) {
	atInput, atUtxo := conwayRefScriptInput(
		t,
		0x01,
		0,
		int(conway.MaxRefScriptSizePerTx),
	)
	overInput, overUtxo := conwayRefScriptInput(
		t,
		0x02,
		0,
		int(conway.MaxRefScriptSizePerTx+1),
	)
	pp := &conway.ConwayProtocolParameters{}
	require.NoError(t, conway.UtxoValidateRefScriptSizePerTx(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{atInput}),
		0,
		conwayRefScriptLedgerState(t, atUtxo),
		pp,
	))
	err := conway.UtxoValidateRefScriptSizePerTx(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{overInput}),
		0,
		conwayRefScriptLedgerState(t, overUtxo),
		pp,
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})

	publishingTx := conwayTxWithInputs(nil, nil)
	publishingTx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(
					common.PlutusV3Script,
					conway.MaxRefScriptSizePerTx+1,
				),
			},
		},
	}
	require.NoError(t, conway.UtxoValidateRefScriptSizePerTx(
		publishingTx,
		0,
		conwayRefScriptLedgerState(t),
		pp,
	))
}

func TestConwayInvalidTxSkipsPerTxRefScriptLimitButCountsForBlock(
	t *testing.T,
) {
	input, utxo := conwayRefScriptInput(
		t,
		0x01,
		0,
		int(conway.MaxRefScriptSizePerBlock+1),
	)
	tx := conwayTxWithInputs(
		nil,
		[]shelley.ShelleyTransactionInput{input},
	)
	tx.TxIsValid = false
	pp := &conway.ConwayProtocolParameters{}
	ls := conwayRefScriptLedgerState(t, utxo)

	t.Run("per-tx limit is skipped", func(t *testing.T) {
		require.NoError(t, conway.UtxoValidateRefScriptSizePerTx(
			tx,
			0,
			ls,
			pp,
		))
	})

	t.Run("block limit still counts invalid transaction", func(t *testing.T) {
		block := conwayBlockWithTransactions(tx)
		block.InvalidTransactions = []uint{0}
		err := conway.ValidateRefScriptSizePerBlock(block, pp, ls)
		require.ErrorAs(
			t,
			err,
			&common.RefScriptSizePerBlockTooLargeError{},
		)
	})
}

func TestConwayRefScriptFeeAndLimitUseSameConsumedSet(t *testing.T) {
	input, utxo := conwayRefScriptInput(
		t,
		0x01,
		0,
		int(conway.RefScriptCostStride+1),
	)
	tx := conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{input})
	tx.SetCbor([]byte{0x84, 0xa0, 0xa0, 0xf5, 0xf6})
	pp := &conway.ConwayProtocolParameters{
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(1, 1)},
	}
	ls := conwayRefScriptLedgerState(t, utxo)
	minFee, err := conway.MinFeeTxWithUtxo(tx, pp, ls)
	require.NoError(t, err)
	require.Equal(t, uint64(25_601), minFee)

	tx.Body.TxFee = minFee - 1
	err = conway.UtxoValidateFeeTooSmallUtxo(tx, 0, ls, pp)
	require.ErrorAs(t, err, &shelley.FeeTooSmallUtxoError{})
	require.NoError(t, conway.UtxoValidateRefScriptSizePerTx(tx, 0, ls, pp))

	publishingTx := conwayTxWithInputs(nil, nil)
	publishingTx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(
					common.PlutusV3Script,
					conway.RefScriptCostStride+1,
				),
			},
		},
	}
	publishingTx.SetCbor([]byte{0x84, 0xa0, 0xa0, 0xf5, 0xf6})
	publishingFee, err := conway.MinFeeTxWithUtxo(
		publishingTx,
		pp,
		conwayRefScriptLedgerState(t),
	)
	require.NoError(t, err)
	require.Zero(t, publishingFee)
}

func TestMinFeeTxIncludesExecutionAndReferenceScriptCosts(t *testing.T) {
	tx := &conway.ConwayTransaction{
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsRedeemers: conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{
						Tag:   common.RedeemerTagSpend,
						Index: 0,
					}: {
						ExUnits: common.ExUnits{Memory: 3, Steps: 4},
					},
				},
			},
		},
	}
	tx.SetCbor([]byte{0x84, 0xa0, 0xa0, 0xf5, 0xf6})
	pp := &conway.ConwayProtocolParameters{
		MinFeeA: 2,
		MinFeeB: 3,
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(1, 1)},
	}

	byteFee, err := common.CalculateMinFee(4, pp.MinFeeA, pp.MinFeeB)
	require.NoError(t, err)
	executionFee, err := conway.CalculateExecutionUnitsFee(tx, pp.ExecutionCosts)
	require.NoError(t, err)
	require.Equal(t, uint64(5), executionFee)
	minFee, err := conway.MinFeeTx(tx, pp)
	require.NoError(t, err)
	require.Equal(t, byteFee+executionFee, minFee)

	combinedFee, err := conway.MinFeeTxWithRefScriptSize(tx, pp, 2)
	require.NoError(t, err)
	require.Equal(t, minFee+2, combinedFee)
}

func TestCalculateRefScriptFeeFloorsFractionalTotal(t *testing.T) {
	fee, err := conway.CalculateRefScriptFee(
		51_201,
		&cbor.Rat{Rat: big.NewRat(1, 1)},
		25_600,
		&cbor.Rat{Rat: big.NewRat(6, 5)},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(56_321), fee)
}

func TestConwayRefScriptSizePerBlockBounds(t *testing.T) {
	inputA, utxoA := conwayRefScriptInput(t, 0x01, 0, 512*1024)
	inputB, utxoB := conwayRefScriptInput(t, 0x02, 0, 512*1024)
	pp := &conway.ConwayProtocolParameters{}
	ls := conwayRefScriptLedgerState(t, utxoA, utxoB)
	atLimit := conwayBlockWithTransactions(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputA}),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputB}),
	)
	require.NoError(t, conway.ValidateRefScriptSizePerBlock(atLimit, pp, ls))

	overInput, overUtxo := conwayRefScriptInput(t, 0x03, 0, 1)
	overLimit := conwayBlockWithTransactions(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputA}),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputB}),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{overInput}),
	)
	err := conway.ValidateRefScriptSizePerBlock(
		overLimit,
		pp,
		conwayRefScriptLedgerState(t, utxoA, utxoB, overUtxo),
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerBlockTooLargeError{})
}

func TestConwayRefScriptSizePerBlockPublishingOnly(t *testing.T) {
	publishingTx := conwayTxWithInputs(nil, nil)
	publishingTx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(
					common.PlutusV3Script,
					conway.MaxRefScriptSizePerBlock+1,
				),
			},
		},
	}
	require.NoError(t, conway.ValidateRefScriptSizePerBlock(
		conwayBlockWithTransactions(publishingTx),
		&conway.ConwayProtocolParameters{},
	))
}

func TestConwayRefScriptSizePerBlockUsesPriorOutputsAtPV11(t *testing.T) {
	publishingTx := conwayTxWithInputs(nil, nil)
	publishingTx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(
					common.PlutusV3Script,
					conway.MaxRefScriptSizePerBlock+1,
				),
			},
		},
	}
	consumedInput := shelley.NewShelleyTransactionInput(
		publishingTx.Hash().String(),
		0,
	)
	consumingTx := conwayTxWithInputs(
		nil,
		[]shelley.ShelleyTransactionInput{consumedInput},
	)
	block := conwayBlockWithTransactions(publishingTx, consumingTx)
	pp := &conway.ConwayProtocolParameters{
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: common.ProtocolVersionVanRossem,
		},
	}
	err := conway.ValidateRefScriptSizePerBlock(
		block,
		pp,
		conwayRefScriptLedgerState(t),
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerBlockTooLargeError{})
}
