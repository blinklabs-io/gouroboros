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
	"errors"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// conwayAbsenceLedgerState reports an unknown input by wrapping
// common.ErrUtxoNotFound and any input in failing with lookupErr.
func conwayAbsenceLedgerState(
	failing map[string]error,
	utxos ...common.Utxo,
) common.LedgerState {
	byInput := make(map[string]common.Utxo, len(utxos))
	for _, utxo := range utxos {
		byInput[utxo.Id.String()] = utxo
	}
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			if err, ok := failing[input.String()]; ok {
				return common.Utxo{}, err
			}
			utxo, ok := byInput[input.String()]
			if !ok {
				return common.Utxo{}, fmt.Errorf(
					"%w: %s",
					common.ErrUtxoNotFound,
					input,
				)
			}
			return utxo, nil
		}).
		Build()
}

func conwayBlockRefScriptTotal(
	t *testing.T,
	block *conway.ConwayBlock,
	ls common.UtxoState,
	includeProduced bool,
) uint64 {
	t.Helper()
	total, err := common.ConsumedReferenceScriptSizePerBlock(
		block,
		ls,
		includeProduced,
		nil,
	)
	require.NoError(t, err)
	return total
}

func conwayPParamsAtVersion(major uint) *conway.ConwayProtocolParameters {
	pp := &conway.ConwayProtocolParameters{}
	pp.ProtocolVersion.Major = major
	return pp
}

func conwayTxWithFee(
	fee uint64,
	inputs []shelley.ShelleyTransactionInput,
	referenceInputs []shelley.ShelleyTransactionInput,
) *conway.ConwayTransaction {
	tx := conwayTxWithInputs(inputs, referenceInputs)
	tx.Body.TxFee = fee
	return tx
}

// From protocol version 11 the block rule only adds outputs to the UTxO, so
// an input spent by an earlier transaction still resolves.
func TestConwayRefScriptSizePerBlockSpentInputStillResolvesAtPV11(
	t *testing.T,
) {
	t.Parallel()
	inputX, utxoX := conwayRefScriptInput(t, 0x01, 0, 100)
	ls := conwayAbsenceLedgerState(nil, utxoX)
	block := conwayBlockWithTransactions(
		conwayTxWithInputs([]shelley.ShelleyTransactionInput{inputX}, nil),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputX}),
	)
	require.Equal(t, uint64(200), conwayBlockRefScriptTotal(t, block, ls, true))
}

func TestConwayRefScriptSizePerBlockDoubleSpendCountsEachTx(t *testing.T) {
	t.Parallel()
	inputX, utxoX := conwayRefScriptInput(t, 0x01, 0, 100)
	ls := conwayAbsenceLedgerState(nil, utxoX)
	block := conwayBlockWithTransactions(
		conwayTxWithFee(1, []shelley.ShelleyTransactionInput{inputX}, nil),
		conwayTxWithFee(2, []shelley.ShelleyTransactionInput{inputX}, nil),
	)
	require.Equal(t, uint64(200), conwayBlockRefScriptTotal(t, block, ls, true))
}

// An input absent from the starting UTxO and from every earlier output
// contributes nothing, as an unmatched key does in the reference lookup.
func TestConwayRefScriptSizePerBlockMissingInputCountsZero(t *testing.T) {
	t.Parallel()
	inputX, utxoX := conwayRefScriptInput(t, 0x01, 0, 100)
	missing, _ := conwayRefScriptInput(t, 0x02, 0, 100)
	ls := conwayAbsenceLedgerState(nil, utxoX)
	block := conwayBlockWithTransactions(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{missing}),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputX}),
	)
	for _, includeProduced := range []bool{true, false} {
		require.Equal(
			t,
			uint64(100),
			conwayBlockRefScriptTotal(t, block, ls, includeProduced),
			"includeProduced=%v",
			includeProduced,
		)
	}
}

// An output stays in the accumulated UTxO after a later transaction spends it.
func TestConwayRefScriptSizePerBlockSpentOutputStillResolvesAtPV11(
	t *testing.T,
) {
	t.Parallel()
	producer := conwayTxWithInputs(nil, nil)
	producer.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(common.PlutusV3Script, 100),
			},
		},
	}
	produced := shelley.MustNewShelleyTransactionInput(
		producer.Hash().String(),
		0,
	)
	block := conwayBlockWithTransactions(
		producer,
		conwayTxWithFee(1, []shelley.ShelleyTransactionInput{produced}, nil),
		conwayTxWithFee(2, nil, []shelley.ShelleyTransactionInput{produced}),
	)
	require.Equal(
		t,
		uint64(200),
		conwayBlockRefScriptTotal(
			t,
			block,
			conwayAbsenceLedgerState(nil),
			true,
		),
	)
}

// Through protocol version 10 every transaction is measured against the
// starting UTxO, so earlier outputs are invisible and spends do not matter.
func TestConwayRefScriptSizePerBlockPV10UsesOriginalUtxo(t *testing.T) {
	t.Parallel()
	inputX, utxoX := conwayRefScriptInput(t, 0x01, 0, 100)
	ls := conwayAbsenceLedgerState(nil, utxoX)
	block := conwayBlockWithTransactions(
		conwayTxWithInputs([]shelley.ShelleyTransactionInput{inputX}, nil),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputX}),
	)
	require.Equal(
		t,
		uint64(200),
		conwayBlockRefScriptTotal(t, block, ls, false),
	)

	producer := conwayTxWithInputs(nil, nil)
	producer.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			TxOutScriptRef: &common.ScriptRef{
				Script: make(common.PlutusV3Script, 100),
			},
		},
	}
	produced := shelley.MustNewShelleyTransactionInput(
		producer.Hash().String(),
		0,
	)
	block = conwayBlockWithTransactions(
		producer,
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{produced}),
	)
	require.Zero(
		t,
		conwayBlockRefScriptTotal(
			t,
			block,
			conwayAbsenceLedgerState(nil),
			false,
		),
	)
}

// Drives the three divergent cases through the limit check. Each script is
// over half the limit, so two measurements exceed it.
func TestConwayValidateRefScriptSizePerBlockAccumulation(t *testing.T) {
	t.Parallel()
	size := int(conway.MaxRefScriptSizePerBlock/2 + 1)
	inputX, utxoX := conwayRefScriptInput(t, 0x01, 0, size)
	ls := conwayAbsenceLedgerState(nil, utxoX)
	pp11 := conwayPParamsAtVersion(common.ProtocolVersionVanRossem)
	pp10 := conwayPParamsAtVersion(common.ProtocolVersionVanRossem - 1)

	spendThenReference := conwayBlockWithTransactions(
		conwayTxWithInputs([]shelley.ShelleyTransactionInput{inputX}, nil),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputX}),
	)
	doubleSpend := conwayBlockWithTransactions(
		conwayTxWithFee(1, []shelley.ShelleyTransactionInput{inputX}, nil),
		conwayTxWithFee(2, []shelley.ShelleyTransactionInput{inputX}, nil),
	)
	for name, block := range map[string]*conway.ConwayBlock{
		"spend then reference": spendThenReference,
		"double spend":         doubleSpend,
	} {
		for pvName, pp := range map[string]*conway.ConwayProtocolParameters{
			"pv11": pp11,
			"pv10": pp10,
		} {
			err := conway.ValidateRefScriptSizePerBlock(block, pp, ls)
			require.ErrorAs(
				t,
				err,
				&common.RefScriptSizePerBlockTooLargeError{},
				"%s %s",
				name,
				pvName,
			)
		}
	}

	missing, _ := conwayRefScriptInput(t, 0x02, 0, size)
	missingBlock := conwayBlockWithTransactions(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{missing}),
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{inputX}),
	)
	require.NoError(
		t,
		conway.ValidateRefScriptSizePerBlock(missingBlock, pp11, ls),
	)
}

// A lookup failure other than absence is not a missing input: it must reach
// the caller rather than count as zero.
func TestConwayRefScriptSizePerBlockPropagatesLookupFailure(t *testing.T) {
	t.Parallel()
	failing, _ := conwayRefScriptInput(t, 0x02, 0, 100)
	lookupErr := errors.New("storage unavailable")
	ls := conwayAbsenceLedgerState(
		map[string]error{failing.String(): lookupErr},
	)
	block := conwayBlockWithTransactions(
		conwayTxWithInputs(nil, []shelley.ShelleyTransactionInput{failing}),
	)
	for _, major := range []uint{
		common.ProtocolVersionVanRossem,
		common.ProtocolVersionVanRossem - 1,
	} {
		err := conway.ValidateRefScriptSizePerBlock(
			block,
			conwayPParamsAtVersion(major),
			ls,
		)
		require.ErrorIs(t, err, lookupErr, "pv%d", major)
	}
}
