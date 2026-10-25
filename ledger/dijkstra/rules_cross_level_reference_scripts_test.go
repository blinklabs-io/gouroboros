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

package dijkstra

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func dijkstraInlineDatumOutput(t *testing.T) DijkstraTransactionOutput {
	t.Helper()
	datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	datumCbor, err := cbor.Encode(datum)
	require.NoError(t, err)
	optionCbor, err := cbor.Encode([]any{
		uint64(babbage.DatumOptionTypeData),
		cbor.Tag{Number: cbor.CborTagCbor, Content: datumCbor},
	})
	require.NoError(t, err)
	var option babbage.BabbageTransactionOutputDatumOption
	_, err = cbor.Decode(optionCbor, &option)
	require.NoError(t, err)
	return DijkstraTransactionOutput{Output: &babbage.BabbageTransactionOutput{
		OutputAmount: mary.MaryTransactionOutputValue{Amount: 1_000_000},
		DatumOption:  &option,
	}}
}

func dijkstraDatumHashScriptInput(
	t *testing.T,
	script common.Script,
	datum common.Datum,
	index int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input, utxo := dijkstraScriptLockedInput(t, script, index)
	output := utxo.Output.(babbage.BabbageTransactionOutput)
	datumHash := datum.Hash()
	utxo.Output = &alonzo.AlonzoTransactionOutput{
		OutputAddress:   output.OutputAddress,
		OutputAmount:    output.OutputAmount,
		OutputDatumHash: &datumHash,
	}
	return input, utxo
}

func verifyDijkstraRule(
	t *testing.T,
	id common.UtxoValidationRuleId,
	tx *DijkstraTransaction,
	ls common.LedgerState,
) error {
	t.Helper()
	return common.VerifyTransaction(
		tx,
		0,
		ls,
		&DijkstraProtocolParameters{},
		[]common.UtxoValidationRuleFunc{dijkstraRule(t, id)},
	)
}

func TestDijkstraInlineDatumRuleUsesSubtransactionReferenceScripts(
	t *testing.T,
) {
	v1 := common.PlutusV1Script{0x01}
	v2 := common.PlutusV2Script{0x02}
	topInput, topUtxo := dijkstraScriptLockedInput(t, v1, 960)
	childInput, childUtxo := dijkstraScriptLockedInput(t, v2, 961)
	referenceInput, referenceUtxo := dijkstraReferenceScriptInput(v1, 962)
	child := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{childInput},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{referenceInput},
				true,
			),
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusV2Scripts: cbor.NewSetType(
				[]common.PlutusV2Script{v2},
				true,
			),
		},
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{topInput},
			),
			TxOutputs: []DijkstraTransactionOutput{
				dijkstraInlineDatumOutput(t),
			},
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{child},
				true,
			),
		},
		TxIsValid: true,
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		topUtxo,
		childUtxo,
		referenceUtxo,
	}).Build()

	err := verifyDijkstraRule(
		t,
		common.UtxoValidationRuleInlineDatumsWithPlutusV1,
		tx,
		state,
	)
	var inline common.InlineDatumsNotSupportedError
	require.ErrorAs(t, err, &inline)
	require.Equal(t, "PlutusV1", inline.PlutusVersion)

	child.Body.TxOutputs = tx.Body.TxOutputs
	tx.Body.TxOutputs = nil
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{child},
		true,
	)
	require.NoError(t, verifyDijkstraRule(
		t,
		common.UtxoValidationRuleInlineDatumsWithPlutusV1,
		tx,
		state,
	))
}

func TestDijkstraDatumRuleUsesSubtransactionReferenceScripts(
	t *testing.T,
) {
	v1 := common.PlutusV1Script{0x11}
	v2 := common.PlutusV2Script{0x12}
	topDatum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	childDatum := common.Datum{Data: data.NewInteger(big.NewInt(2))}
	topInput, topUtxo := dijkstraDatumHashScriptInput(
		t,
		v1,
		topDatum,
		963,
	)
	childInput, childUtxo := dijkstraDatumHashScriptInput(
		t,
		v2,
		childDatum,
		964,
	)
	referenceInput, referenceUtxo := dijkstraReferenceScriptInput(v1, 965)
	child := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{childInput},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{referenceInput},
				true,
			),
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusV2Scripts: cbor.NewSetType(
				[]common.PlutusV2Script{v2},
				true,
			),
			WsPlutusData: cbor.NewSetType(
				[]common.Datum{childDatum},
				true,
			),
		},
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{topInput},
			),
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{child},
				true,
			),
		},
		TxIsValid: true,
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		topUtxo,
		childUtxo,
		referenceUtxo,
	}).Build()

	err := verifyDijkstraRule(
		t,
		common.UtxoValidationRuleSupplementalDatums,
		tx,
		state,
	)
	var missing common.MissingDatumForSpendingScriptError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, v1.Hash(), missing.ScriptHash)
	require.Equal(t, topInput, missing.Input)

	tx.WitnessSet.WsPlutusData = cbor.NewSetType(
		[]common.Datum{topDatum},
		true,
	)
	require.NoError(t, verifyDijkstraRule(
		t,
		common.UtxoValidationRuleSupplementalDatums,
		tx,
		state,
	))
}
