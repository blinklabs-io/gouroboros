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
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

func dijkstraGuardTestPlutus(
	t *testing.T,
	version [3]uint32,
	fail bool,
) common.Script {
	t.Helper()
	var body syn.Term[syn.DeBruijn] = &syn.Constant{
		Con: &syn.Integer{Inner: big.NewInt(1)},
	}
	if fail {
		body = &syn.Error{}
	}
	argumentCount := 1
	if version == lang.LanguageVersionV1 || version == lang.LanguageVersionV2 {
		argumentCount = 3
	}
	for range argumentCount {
		body = &syn.Lambda[syn.DeBruijn]{Body: body}
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: version,
		Term:    body,
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	switch version {
	case lang.LanguageVersionV1:
		return common.PlutusV1Script(wrapper)
	case lang.LanguageVersionV2:
		return common.PlutusV2Script(wrapper)
	case lang.LanguageVersionV3:
		return common.PlutusV3Script(wrapper)
	case lang.LanguageVersionV4:
		return common.PlutusV4Script(wrapper)
	default:
		t.Fatalf("unsupported Plutus language version %v", version)
		return nil
	}
}

func dijkstraGuardTestPParams() *DijkstraProtocolParameters {
	costModels := make(map[uint][]int64, 4)
	for version, names := range [][]string{
		lang.CostModelParamNamesV1,
		lang.CostModelParamNamesV2,
		lang.CostModelParamNamesV3,
		lang.CostModelParamNamesV3,
	} {
		costModels[uint(version)] = make([]int64, len(names))
		for idx := range costModels[uint(version)] {
			costModels[uint(version)][idx] = 1
		}
	}
	return &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 12,
			},
			CostModels: costModels,
		},
	}
}

func dijkstraReferenceInputSet(
	input shelley.ShelleyTransactionInput,
) cbor.SetType[shelley.ShelleyTransactionInput] {
	return cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		true,
	)
}

func dijkstraSubtransactionPlutusGuardTx(
	t *testing.T,
	script common.Script,
	crossLevel bool,
) (*DijkstraTransaction, common.LedgerState) {
	t.Helper()
	guard := dijkstraGuardCredentialForScript(script)
	subTx := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guard},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{
							Steps:  10_000_000,
							Memory: 10_000_000,
						},
					},
				},
			},
		},
	}
	var utxos []common.Utxo
	if crossLevel {
		input, utxo := dijkstraReferenceScriptInput(script, 901)
		utxos = append(utxos, utxo)
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxReferenceInputs: dijkstraReferenceInputSet(input),
				TxSubTransactions: cbor.NewSetType(
					[]DijkstraSubTransaction{subTx},
					true,
				),
			},
			TxIsValid: true,
		}, mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	}
	witnesses := testDijkstraWitnessSet(t, script)
	witnesses.WsRedeemers = subTx.WitnessSet.WsRedeemers
	subTx.WitnessSet = witnesses
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			),
		},
		TxIsValid: true,
	}, mockledger.NewLedgerStateBuilder().Build()
}

func TestVerifyTransactionExecutesSubtransactionPlutusGuards(t *testing.T) {
	for _, crossLevel := range []bool{false, true} {
		name := "same-level witness"
		if crossLevel {
			name = "cross-level reference"
		}
		t.Run(name, func(t *testing.T) {
			tx, ls := dijkstraSubtransactionPlutusGuardTx(
				t,
				dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, true),
				crossLevel,
			)
			err := common.VerifyTransaction(
				tx,
				0,
				ls,
				dijkstraGuardTestPParams(),
				[]common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts},
			)
			var scriptErr conway.PlutusScriptFailedError
			require.ErrorAs(t, err, &scriptErr)
			require.Equal(t, common.RedeemerTagGuarding, scriptErr.Tag)
			require.Zero(t, scriptErr.Index)
		})
	}
}

func TestVerifyTransactionExecutesSubtransactionNativeGuards(t *testing.T) {
	missingGuard := testGuardCredential()
	native := testRequireGuardNativeScript(t, missingGuard)
	guard := dijkstraGuardCredentialForScript(native)
	for _, crossLevel := range []bool{false, true} {
		name := "same-level witness"
		if crossLevel {
			name = "cross-level reference"
		}
		t.Run(name, func(t *testing.T) {
			subTx := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
				TxGuards: &DijkstraGuards{
					Credentials: []common.Credential{guard},
				},
			}}
			var utxos []common.Utxo
			tx := &DijkstraTransaction{TxIsValid: true}
			if crossLevel {
				input, utxo := dijkstraReferenceScriptInput(native, 902)
				utxos = append(utxos, utxo)
				tx.Body.TxReferenceInputs = dijkstraReferenceInputSet(input)
			} else {
				subTx.WitnessSet.WsNativeScripts = cbor.NewSetType(
					[]common.NativeScript{native},
					true,
				)
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			)
			err := common.VerifyTransaction(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
				&DijkstraProtocolParameters{},
				[]common.UtxoValidationRuleFunc{UtxoValidateNativeScripts},
			)
			var scriptErr conway.NativeScriptFailedError
			require.ErrorAs(t, err, &scriptErr)
			require.Equal(t, native.Hash(), scriptErr.ScriptHash)
		})
	}
}

func TestVerifyTransactionSubtransactionGuardControls(t *testing.T) {
	for _, test := range []struct {
		name    string
		version [3]uint32
	}{
		{name: "passing PlutusV1 guard without treasury", version: lang.LanguageVersionV1},
		{name: "passing PlutusV2 guard without treasury", version: lang.LanguageVersionV2},
		{name: "passing PlutusV3 guard", version: lang.LanguageVersionV3},
		{name: "passing PlutusV4 guard", version: lang.LanguageVersionV4},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, ls := dijkstraSubtransactionPlutusGuardTx(
				t,
				dijkstraGuardTestPlutus(t, test.version, false),
				false,
			)
			require.Nil(t, tx.CurrentTreasuryValue())
			require.NoError(t, common.VerifyTransaction(
				tx,
				0,
				ls,
				dijkstraGuardTestPParams(),
				[]common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts},
			))
		})
	}

	t.Run("no guards or scripts", func(t *testing.T) {
		tx := &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxSubTransactions: cbor.NewSetType(
					[]DijkstraSubTransaction{{}},
					true,
				),
			},
			TxIsValid: true,
		}
		require.NoError(t, common.VerifyTransaction(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			dijkstraGuardTestPParams(),
			[]common.UtxoValidationRuleFunc{
				UtxoValidatePlutusScripts,
				UtxoValidateNativeScripts,
			},
		))
	})

	t.Run("unrelated scripts are not executed", func(t *testing.T) {
		subTx := DijkstraSubTransaction{WitnessSet: testDijkstraWitnessSet(
			t,
			dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, true),
		)}
		tx := &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxSubTransactions: cbor.NewSetType(
					[]DijkstraSubTransaction{subTx},
					true,
				),
			},
			TxIsValid: true,
		}
		require.NoError(t, common.VerifyTransaction(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			dijkstraGuardTestPParams(),
			[]common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts},
		))
	})
}
