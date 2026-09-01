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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

func dijkstraResourceLimitWitnesses(
	exUnits common.ExUnits,
) DijkstraTransactionWitnessSet {
	return DijkstraTransactionWitnessSet{
		WsRedeemers: DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: {
					ExUnits: exUnits,
				},
			},
		},
	}
}

func TestVerifyTransactionDijkstraExUnitsIncludesEveryLevel(t *testing.T) {
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{
					{WitnessSet: dijkstraResourceLimitWitnesses(
						common.ExUnits{Memory: 13, Steps: 23},
					)},
					{WitnessSet: dijkstraResourceLimitWitnesses(
						common.ExUnits{Memory: 17, Steps: 29},
					)},
				},
				true,
			),
		},
		WitnessSet: dijkstraResourceLimitWitnesses(
			common.ExUnits{Memory: 11, Steps: 19},
		),
		TxIsValid: true,
	}
	wantTotal := common.ExUnits{Memory: 41, Steps: 71}
	tests := []struct {
		name      string
		max       common.ExUnits
		wantError bool
	}{
		{
			name: "below limit",
			max:  common.ExUnits{Memory: 42, Steps: 72},
		},
		{
			name: "at limit",
			max:  wantTotal,
		},
		{
			name:      "over limit",
			max:       common.ExUnits{Memory: 40, Steps: 70},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					MaxTxExUnits: test.max,
				},
			}
			err := common.VerifyTransaction(
				tx,
				0,
				nil,
				pp,
				[]common.UtxoValidationRuleFunc{
					UtxoValidateExUnitsTooBigUtxo,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				return
			}
			var target alonzo.ExUnitsTooBigUtxoError
			require.ErrorAs(t, err, &target)
			require.Equal(t, wantTotal, target.TotalExUnits)
		})
	}
}

func dijkstraResourceLimitOutput(
	script common.Script,
) DijkstraTransactionOutput {
	return DijkstraTransactionOutput{
		Output: babbage.BabbageTransactionOutput{
			TxOutScriptRef: &common.ScriptRef{Script: script},
		},
	}
}

func dijkstraResourceLimitTransaction(
	topLevelScript common.Script,
	collateralScript common.Script,
	subtransactionScripts ...common.Script,
) *DijkstraTransaction {
	tx := &DijkstraTransaction{TxIsValid: true}
	if topLevelScript != nil {
		tx.Body.TxOutputs = []DijkstraTransactionOutput{
			dijkstraResourceLimitOutput(topLevelScript),
		}
	}
	if collateralScript != nil {
		output := dijkstraResourceLimitOutput(collateralScript)
		tx.Body.TxCollateralReturn = &output
	}
	if len(subtransactionScripts) > 0 {
		subtransactions := make(
			[]DijkstraSubTransaction,
			len(subtransactionScripts),
		)
		for idx, script := range subtransactionScripts {
			subtransactions[idx].Body.TxOutputs = []DijkstraTransactionOutput{
				dijkstraResourceLimitOutput(script),
			}
		}
		tx.Body.TxSubTransactions = cbor.NewSetType(subtransactions, true)
	}
	return tx
}

// Plutus well-formedness is contextual on the protocol major, so these
// fixtures must carry the Dijkstra version rather than the zero value.
func dijkstraResourceLimitPparams() *DijkstraProtocolParameters {
	return &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
		},
	}
}

func TestVerifyTransactionDijkstraMalformedReferenceScriptOutputs(
	t *testing.T,
) {
	valid := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	malformed := common.PlutusV4Script{0xff}
	tests := []struct {
		name      string
		tx        *DijkstraTransaction
		wantError bool
	}{
		{
			name: "valid scripts at every output source",
			tx:   dijkstraResourceLimitTransaction(valid, valid, valid, valid),
		},
		{
			name:      "top-level output",
			tx:        dijkstraResourceLimitTransaction(malformed, nil),
			wantError: true,
		},
		{
			name:      "collateral return output",
			tx:        dijkstraResourceLimitTransaction(nil, malformed),
			wantError: true,
		},
		{
			name:      "subtransaction output",
			tx:        dijkstraResourceLimitTransaction(nil, nil, malformed),
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := common.VerifyTransaction(
				test.tx,
				0,
				nil,
				dijkstraResourceLimitPparams(),
				[]common.UtxoValidationRuleFunc{
					UtxoValidateMalformedReferenceScripts,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				return
			}
			var target common.MalformedReferenceScriptsError
			require.ErrorAs(t, err, &target)
			require.Equal(t, []common.ScriptHash{malformed.Hash()}, target.ScriptHashes)
		})
	}
}

func TestVerifyTransactionDijkstraReferenceScriptSizeIncludesEveryOutput(
	t *testing.T,
) {
	script := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	tx := dijkstraResourceLimitTransaction(script, script, script, script)
	wantTotal := uint64(4 * len(script.RawScriptBytes()))
	tests := []struct {
		name      string
		max       uint32
		wantError bool
	}{
		{name: "below limit", max: uint32(wantTotal + 1)},
		{name: "at limit", max: uint32(wantTotal)},
		{name: "over limit", max: uint32(wantTotal - 1), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := common.VerifyTransaction(
				tx,
				0,
				nil,
				&DijkstraProtocolParameters{MaxRefScriptSizePerTx: test.max},
				[]common.UtxoValidationRuleFunc{
					UtxoValidateRefScriptSizePerTx,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				return
			}
			var target common.RefScriptSizePerTxTooLargeError
			require.ErrorAs(t, err, &target)
			require.Equal(t, wantTotal, target.TxSize)
		})
	}
}
