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

package common_test

import (
	"math/big"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/builtin"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

const plutusContextTestAddress = "addr_test1qz2fxv2umyhttkxyxp8x0dlpdt3" +
	"k6cwng5pxj3jhsydzer3jcu5d8ps7zex2k2xt3uqxgjqnnj83w" +
	"s8lhrn648jjxtwq2ytjqp"

func encodePlutusContextTestScript(
	t *testing.T,
	version lang.LanguageVersion,
	arity int,
	deadTerm syn.Term[syn.DeBruijn],
) []byte {
	t.Helper()
	var term syn.Term[syn.DeBruijn] = &syn.Constant{
		Con: &syn.Unit{},
	}
	if deadTerm != nil {
		term = &syn.Apply[syn.DeBruijn]{
			Function: &syn.Lambda[syn.DeBruijn]{Body: term},
			Argument: &syn.Delay[syn.DeBruijn]{Term: deadTerm},
		}
	}
	for range arity {
		term = &syn.Lambda[syn.DeBruijn]{Body: term}
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: version,
		Term:    term,
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return wrapper
}

func testPlutusData() data.PlutusData {
	return &data.Constr{Tag: big.NewInt(0)}
}

func TestPlutusEvaluateContextValidation(t *testing.T) {
	t.Run("Plutus V1 builtin protocol boundary", func(t *testing.T) {
		script := common.PlutusV1Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 0, 0},
			3,
			&syn.Builtin{DefaultFunction: builtin.SerialiseData},
		))
		_, err := script.Evaluate(
			testPlutusData(),
			testPlutusData(),
			testPlutusData(),
			common.ExUnits{},
			cek.NewDefaultEvalContext(
				lang.LanguageVersionV1,
				cek.ProtoVersion{Major: 10},
			),
		)
		require.ErrorContains(t, err, "builtin serialiseData is not available")

		_, err = script.Evaluate(
			testPlutusData(),
			testPlutusData(),
			testPlutusData(),
			common.ExUnits{},
			cek.NewDefaultEvalContext(
				lang.LanguageVersionV1,
				cek.ProtoVersion{Major: 11},
			),
		)
		require.NoError(t, err)
	})

	t.Run("Plutus V2 UPLC protocol boundary", func(t *testing.T) {
		script := common.PlutusV2Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 1, 0},
			3,
			nil,
		))
		_, err := script.Evaluate(
			testPlutusData(),
			testPlutusData(),
			testPlutusData(),
			common.ExUnits{},
			cek.NewDefaultEvalContext(
				lang.LanguageVersionV2,
				cek.ProtoVersion{Major: 10},
			),
		)
		require.ErrorContains(
			t,
			err,
			"UPLC version 1.1.0 is not available",
		)

		_, err = script.Evaluate(
			testPlutusData(),
			testPlutusData(),
			testPlutusData(),
			common.ExUnits{},
			cek.NewDefaultEvalContext(
				lang.LanguageVersionV2,
				cek.ProtoVersion{Major: 11},
			),
		)
		require.NoError(t, err)
	})

	t.Run("Plutus V3 constructor syntax", func(t *testing.T) {
		invalidScript := common.PlutusV3Script(
			encodePlutusContextTestScript(
				t,
				lang.LanguageVersion{1, 0, 0},
				1,
				&syn.Constr[syn.DeBruijn]{Tag: 0},
			),
		)
		evalContext := cek.NewDefaultEvalContext(
			lang.LanguageVersionV3,
			cek.ProtoVersion{Major: 9},
		)
		_, err := invalidScript.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			evalContext,
		)
		require.ErrorContains(t, err, "constr is not available")

		validScript := common.PlutusV3Script(
			encodePlutusContextTestScript(
				t,
				lang.LanguageVersion{1, 1, 0},
				1,
				&syn.Constr[syn.DeBruijn]{Tag: 0},
			),
		)
		_, err = validScript.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			evalContext,
		)
		require.NoError(t, err)
	})

	t.Run("Plutus V4 constructor field limit", func(t *testing.T) {
		fields := make([]syn.Term[syn.DeBruijn], 1025)
		for i := range fields {
			fields[i] = &syn.Constant{Con: &syn.Unit{}}
		}
		invalidScript := common.PlutusV4Script(
			encodePlutusContextTestScript(
				t,
				lang.LanguageVersion{1, 1, 0},
				1,
				&syn.Constr[syn.DeBruijn]{Tag: 0, Fields: fields},
			),
		)
		evalContext := cek.NewDefaultEvalContext(
			lang.LanguageVersionV4,
			cek.ProtoVersion{Major: 12},
		)
		_, err := invalidScript.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			evalContext,
		)
		require.ErrorContains(t, err, "constr with 1025 fields")

		validScript := common.PlutusV4Script(
			encodePlutusContextTestScript(
				t,
				lang.LanguageVersion{1, 1, 0},
				1,
				&syn.Constr[syn.DeBruijn]{Tag: 0, Fields: fields[:1024]},
			),
		)
		_, err = validScript.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			evalContext,
		)
		require.NoError(t, err)
	})

	t.Run("unsupported UPLC version", func(t *testing.T) {
		script := common.PlutusV4Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 2, 0},
			1,
			nil,
		))
		_, err := script.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			cek.NewDefaultEvalContext(
				lang.LanguageVersionV4,
				cek.ProtoVersion{Major: 12},
			),
		)
		require.ErrorContains(t, err, "unsupported UPLC program version")
	})

	t.Run("evaluation context is required", func(t *testing.T) {
		script := common.PlutusV4Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 1, 0},
			1,
			nil,
		))
		_, err := script.Evaluate(
			testPlutusData(),
			common.ExUnits{},
			nil,
		)
		require.ErrorContains(t, err, "evaluation context is required")
	})
}

func plutusContextTestOutput(t *testing.T) common.TransactionOutput {
	t.Helper()
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(plutusContextTestAddress).
		WithLovelace(1_000_000).
		Build()
	require.NoError(t, err)
	return output
}

func plutusContextReferenceOutput(
	t *testing.T,
	typeId uint,
	script common.Script,
) common.TransactionOutput {
	t.Helper()
	scriptRef, err := cbor.Encode(common.ScriptRef{
		Type:   typeId,
		Script: script,
	})
	require.NoError(t, err)
	utxo, err := mockledger.NewUtxoBuilder().
		WithTxId([]byte{2}).
		WithIndex(0).
		WithAddress(plutusContextTestAddress).
		WithLovelace(1_000_000).
		WithScriptRef(scriptRef).
		Build()
	require.NoError(t, err)
	return utxo.Output
}

func plutusContextMockTransaction(
	t *testing.T,
	witnesses *mockledger.MockTransactionWitnessSet,
	outputs ...common.TransactionOutput,
) common.Transaction {
	t.Helper()
	input, err := mockledger.NewTransactionInputBuilder().
		WithTxId([]byte{1}).
		WithIndex(0).
		Build()
	require.NoError(t, err)
	if len(outputs) == 0 {
		outputs = []common.TransactionOutput{plutusContextTestOutput(t)}
	}
	builder := mockledger.NewTransactionBuilder()
	builder.WithInputs(input)
	builder.WithOutputs(outputs...)
	builder.WithValid(false)
	builder.WithWitnesses(witnesses)
	tx, err := builder.Build()
	require.NoError(t, err)
	require.False(t, tx.IsValid())
	return tx
}

func plutusContextValidationRuleIndex(
	t *testing.T,
	rules []common.UtxoValidationRuleFunc,
	want string,
) int {
	t.Helper()
	for idx, rule := range rules {
		fn := runtime.FuncForPC(reflect.ValueOf(rule).Pointer())
		if fn != nil && strings.HasSuffix(fn.Name(), want) {
			return idx
		}
	}
	t.Fatalf("validation rule %s is not registered", want)
	return -1
}

func TestPlutusWellFormednessRunsBeforePhase2(t *testing.T) {
	for _, test := range []struct {
		name               string
		rules              []common.UtxoValidationRuleFunc
		wellFormedRuleName string
		phase2RuleName     string
	}{
		{
			name:               "Babbage",
			rules:              babbage.UtxoValidationRules,
			wellFormedRuleName: "ledger/babbage.UtxoValidateMalformedReferenceScripts",
			phase2RuleName:     "ledger/babbage.UtxoValidatePlutusScripts",
		},
		{
			name:               "Conway",
			rules:              conway.UtxoValidationRules,
			wellFormedRuleName: "ledger/conway.UtxoValidateMalformedReferenceScripts",
			phase2RuleName:     "ledger/conway.UtxoValidatePlutusScripts",
		},
		{
			name:               "Dijkstra",
			rules:              dijkstra.UtxoValidationRules,
			wellFormedRuleName: "ledger/dijkstra.UtxoValidateMalformedReferenceScripts",
			phase2RuleName:     "ledger/dijkstra.UtxoValidatePlutusScripts",
		},
	} {
		t.Run(test.name+" registration", func(t *testing.T) {
			wellFormedIdx := plutusContextValidationRuleIndex(
				t,
				test.rules,
				test.wellFormedRuleName,
			)
			phase2Idx := plutusContextValidationRuleIndex(
				t,
				test.rules,
				test.phase2RuleName,
			)
			require.Less(t, wellFormedIdx, phase2Idx)
		})
	}

	t.Run(
		"VerifyTransaction rejects malformed script before execution",
		func(t *testing.T) {
			tx := &babbage.BabbageTransaction{TxIsValid: true}
			tx.WitnessSet.WsPlutusV2Scripts = []common.PlutusV2Script{{0xff}}
			tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{{
					Tag:     common.RedeemerTagSpend,
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				}},
			}
			wellFormedIdx := plutusContextValidationRuleIndex(
				t,
				babbage.UtxoValidationRules,
				"ledger/babbage.UtxoValidateMalformedReferenceScripts",
			)
			phase2Idx := plutusContextValidationRuleIndex(
				t,
				babbage.UtxoValidationRules,
				"ledger/babbage.UtxoValidatePlutusScripts",
			)
			firstIdx, lastIdx := wellFormedIdx, phase2Idx
			if firstIdx > lastIdx {
				firstIdx, lastIdx = lastIdx, firstIdx
			}
			err := common.VerifyTransaction(
				tx,
				0,
				nil,
				&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
				babbage.UtxoValidationRules[firstIdx:lastIdx+1],
			)
			require.ErrorIs(t, err, common.ErrMalformedScriptWitnesses)
			var unsupported common.PlutusScriptValidationUnsupportedError
			require.NotErrorAs(t, err, &unsupported)
		},
	)
}

type plutusContextScriptRefOutput struct {
	common.TransactionOutput
	script common.Script
}

func (o plutusContextScriptRefOutput) ScriptRef() common.Script {
	return o.script
}

type plutusContextOutputsTransaction struct {
	common.Transaction
	outputs []common.TransactionOutput
}

func (tx plutusContextOutputsTransaction) Outputs() []common.TransactionOutput {
	return tx.outputs
}

func TestPlutusTypedNilScriptValidation(t *testing.T) {
	var v1 *common.PlutusV1Script
	var v2 *common.PlutusV2Script
	var v3 *common.PlutusV3Script
	var v4 *common.PlutusV4Script

	for _, test := range []struct {
		name   string
		script common.Script
	}{
		{name: "Plutus V1", script: v1},
		{name: "Plutus V2", script: v2},
		{name: "Plutus V3", script: v3},
		{name: "Plutus V4", script: v4},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseTx := plutusContextMockTransaction(
				t,
				mockledger.NewMockTransactionWitnessSet(),
			)
			tx := plutusContextOutputsTransaction{
				Transaction: baseTx,
				outputs: []common.TransactionOutput{
					plutusContextScriptRefOutput{
						TransactionOutput: plutusContextTestOutput(t),
						script:            test.script,
					},
				},
			}

			var err error
			require.NotPanics(t, func() {
				err = common.VerifyTransaction(
					tx,
					0,
					nil,
					dijkstraContextParams(12),
					[]common.UtxoValidationRuleFunc{
						dijkstra.UtxoValidateMalformedReferenceScripts,
					},
				)
			})
			require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)
			var malformed common.MalformedReferenceScriptsError
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, []common.ScriptHash{{}}, malformed.ScriptHashes)
		})
	}
}

func TestBabbagePlutusWellFormednessAdmission(t *testing.T) {
	t.Run("Plutus V1 witness and valid control", func(t *testing.T) {
		invalid := common.PlutusV1Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 0, 0},
			3,
			&syn.Builtin{DefaultFunction: builtin.SerialiseData},
		))
		witnesses := mockledger.NewMockTransactionWitnessSet().
			WithPlutusV1Scripts(invalid)
		tx := plutusContextMockTransaction(t, witnesses)
		err := babbage.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.ErrorContains(t, err, "builtin serialiseData is not available")
		require.ErrorIs(t, err, common.ErrMalformedScriptWitnesses)

		valid := common.PlutusV1Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 0, 0},
			3,
			nil,
		))
		tx = plutusContextMockTransaction(
			t,
			mockledger.NewMockTransactionWitnessSet().
				WithPlutusV1Scripts(valid),
		)
		err = babbage.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.NoError(t, err)
	})

	t.Run("Plutus V2 reference script protocol boundary", func(t *testing.T) {
		script := common.PlutusV2Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 0, 0},
			3,
			&syn.Builtin{
				DefaultFunction: builtin.VerifyEcdsaSecp256k1Signature,
			},
		))
		tx := plutusContextMockTransaction(
			t,
			mockledger.NewMockTransactionWitnessSet(),
			plutusContextReferenceOutput(
				t,
				common.ScriptRefTypePlutusV2,
				script,
			),
		)
		err := babbage.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 7},
		)
		require.Error(t, err)
		require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)

		err = babbage.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.NoError(t, err)
	})

	t.Run("malformed collateral return on phase-2-invalid transaction", func(t *testing.T) {
		newTx := func(script common.Script) *babbage.BabbageTransaction {
			return &babbage.BabbageTransaction{
				Body: babbage.BabbageTransactionBody{
					TxCollateralReturn: &babbage.BabbageTransactionOutput{
						TxOutScriptRef: &common.ScriptRef{
							Type:   common.ScriptRefTypePlutusV2,
							Script: script,
						},
					},
				},
				TxIsValid: false,
			}
		}

		tx := newTx(common.PlutusV2Script{0xff})
		require.False(t, tx.IsValid())
		produced := tx.Produced()
		require.Len(t, produced, 1)
		require.Same(t, tx.CollateralReturn(), produced[0].Output)

		err := babbage.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)

		validScript := common.PlutusV2Script(encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 0, 0},
			3,
			nil,
		))
		err = babbage.UtxoValidateMalformedReferenceScripts(
			newTx(validScript),
			0,
			nil,
			&babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.NoError(t, err)
	})
}

func TestConwayPlutusWellFormednessAdmission(t *testing.T) {
	script := common.PlutusV3Script(encodePlutusContextTestScript(
		t,
		lang.LanguageVersion{1, 1, 0},
		1,
		&syn.Builtin{DefaultFunction: builtin.AndByteString},
	))
	params := func(major uint) *conway.ConwayProtocolParameters {
		return &conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: major,
			},
		}
	}

	t.Run("Plutus V3 witness protocol boundary", func(t *testing.T) {
		tx := plutusContextMockTransaction(
			t,
			mockledger.NewMockTransactionWitnessSet().
				WithPlutusV3Scripts(script),
		)
		err := conway.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			params(9),
		)
		require.ErrorContains(t, err, "builtin andByteString is not available")
		require.ErrorIs(t, err, common.ErrMalformedScriptWitnesses)

		err = conway.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			params(10),
		)
		require.NoError(t, err)
	})

	t.Run("Plutus V3 reference script protocol boundary", func(t *testing.T) {
		tx := plutusContextMockTransaction(
			t,
			mockledger.NewMockTransactionWitnessSet(),
			plutusContextReferenceOutput(
				t,
				common.ScriptRefTypePlutusV3,
				script,
			),
		)
		err := conway.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			params(9),
		)
		require.Error(t, err)
		require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)

		err = conway.UtxoValidateMalformedReferenceScripts(
			tx,
			0,
			nil,
			params(10),
		)
		require.NoError(t, err)
	})
}

func dijkstraContextParams(
	major uint,
) *dijkstra.DijkstraProtocolParameters {
	return &dijkstra.DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: major,
			},
		},
	}
}

func dijkstraContextWitnesses(
	script common.PlutusV4Script,
) dijkstra.DijkstraTransactionWitnessSet {
	return dijkstra.DijkstraTransactionWitnessSet{
		WsPlutusV4Scripts: cbor.NewSetType(
			[]common.PlutusV4Script{script},
			false,
		),
	}
}

func TestDijkstraPlutusWellFormednessAdmission(t *testing.T) {
	validScript := common.PlutusV4Script(encodePlutusContextTestScript(
		t,
		lang.LanguageVersion{1, 1, 0},
		1,
		nil,
	))

	for _, test := range []struct {
		name string
		tx   *dijkstra.DijkstraTransaction
	}{
		{
			name: "top-level Plutus V4 witness",
			tx: &dijkstra.DijkstraTransaction{
				WitnessSet: dijkstraContextWitnesses(validScript),
				TxIsValid:  false,
			},
		},
		{
			name: "subtransaction Plutus V4 witness",
			tx: &dijkstra.DijkstraTransaction{
				Body: dijkstra.DijkstraTransactionBody{
					TxSubTransactions: cbor.NewSetType(
						[]dijkstra.DijkstraSubTransaction{{
							WitnessSet: dijkstraContextWitnesses(validScript),
						}},
						false,
					),
				},
				TxIsValid: false,
			},
		},
	} {
		t.Run(test.name+" protocol boundary", func(t *testing.T) {
			require.False(t, test.tx.IsValid())
			err := dijkstra.UtxoValidateMalformedReferenceScripts(
				test.tx,
				0,
				nil,
				dijkstraContextParams(11),
			)
			require.ErrorContains(
				t,
				err,
				"plutus ledger language V4 is not available",
			)
			require.ErrorIs(t, err, common.ErrMalformedScriptWitnesses)

			err = dijkstra.UtxoValidateMalformedReferenceScripts(
				test.tx,
				0,
				nil,
				dijkstraContextParams(12),
			)
			require.NoError(t, err)
		})
	}

	fields := make([]syn.Term[syn.DeBruijn], 1025)
	for i := range fields {
		fields[i] = &syn.Constant{Con: &syn.Unit{}}
	}
	invalidReference := common.PlutusV4Script(
		encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 1, 0},
			1,
			&syn.Constr[syn.DeBruijn]{Tag: 0, Fields: fields},
		),
	)
	validReference := common.PlutusV4Script(
		encodePlutusContextTestScript(
			t,
			lang.LanguageVersion{1, 1, 0},
			1,
			&syn.Constr[syn.DeBruijn]{Tag: 0, Fields: fields[:1024]},
		),
	)

	for _, test := range []struct {
		name  string
		newTx func(common.TransactionOutput) *dijkstra.DijkstraTransaction
	}{
		{
			name: "top-level Plutus V4 reference script",
			newTx: func(output common.TransactionOutput) *dijkstra.DijkstraTransaction {
				return &dijkstra.DijkstraTransaction{
					Body: dijkstra.DijkstraTransactionBody{
						TxOutputs: []dijkstra.DijkstraTransactionOutput{{
							Output: output,
						}},
					},
					TxIsValid: false,
				}
			},
		},
		{
			name: "subtransaction Plutus V4 reference script",
			newTx: func(output common.TransactionOutput) *dijkstra.DijkstraTransaction {
				return &dijkstra.DijkstraTransaction{
					Body: dijkstra.DijkstraTransactionBody{
						TxSubTransactions: cbor.NewSetType(
							[]dijkstra.DijkstraSubTransaction{{
								Body: dijkstra.DijkstraSubTransactionBody{
									TxOutputs: []dijkstra.DijkstraTransactionOutput{{
										Output: output,
									}},
								},
							}},
							false,
						),
					},
					TxIsValid: false,
				}
			},
		},
	} {
		t.Run(test.name+" constructor limit", func(t *testing.T) {
			invalidOutput := plutusContextReferenceOutput(
				t,
				common.ScriptRefTypePlutusV4,
				invalidReference,
			)
			tx := test.newTx(invalidOutput)
			require.False(t, tx.IsValid())
			err := dijkstra.UtxoValidateMalformedReferenceScripts(
				tx,
				0,
				nil,
				dijkstraContextParams(12),
			)
			require.Error(t, err)
			require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)

			validOutput := plutusContextReferenceOutput(
				t,
				common.ScriptRefTypePlutusV4,
				validReference,
			)
			err = dijkstra.UtxoValidateMalformedReferenceScripts(
				test.newTx(validOutput),
				0,
				nil,
				dijkstraContextParams(12),
			)
			require.NoError(t, err)
		})
	}
}
