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
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

var evaluateTestBudget = common.ExUnits{Steps: 10_000_000, Memory: 10_000_000}

func evaluateTestRedeemers(
	keys ...common.RedeemerKey,
) DijkstraRedeemers {
	redeemers := make(map[common.RedeemerKey]common.RedeemerValue, len(keys))
	for _, key := range keys {
		redeemers[key] = common.RedeemerValue{ExUnits: evaluateTestBudget}
	}
	return DijkstraRedeemers{Redeemers: redeemers}
}

func evaluateTestMintTx(
	t *testing.T,
	plutusScript common.Script,
) *DijkstraTransaction {
	t.Helper()
	mint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			plutusScript.Hash(): {cbor.NewByteString(nil): big.NewInt(1)},
		},
	)
	witnesses := testDijkstraWitnessSet(t, plutusScript)
	witnesses.WsRedeemers = evaluateTestRedeemers(
		common.RedeemerKey{Tag: common.RedeemerTagMint},
	)
	return &DijkstraTransaction{
		Body:       DijkstraTransactionBody{TxMint: &mint},
		WitnessSet: witnesses,
		TxIsValid:  true,
	}
}

func evaluateTestTopLevelGuardTx(
	t *testing.T,
	plutusScript common.Script,
) *DijkstraTransaction {
	t.Helper()
	witnesses := testDijkstraWitnessSet(t, plutusScript)
	witnesses.WsRedeemers = evaluateTestRedeemers(
		common.RedeemerKey{Tag: common.RedeemerTagGuarding},
	)
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{
					dijkstraGuardCredentialForScript(plutusScript),
				},
			},
		},
		WitnessSet: witnesses,
		TxIsValid:  true,
	}
}

// evaluateTestContextTx guards both sub-transactions and the top level with
// a reference V4 script that fails unless its context omits txInfoSubTxIx,
// so it succeeds only against the context validation builds.
func evaluateTestContextTx(
	t *testing.T,
) (*DijkstraTransaction, common.LedgerState) {
	t.Helper()
	plutusScript := dijkstraV4SubTxIndexAbsentScript(t)
	guard := dijkstraGuardCredentialForScript(plutusScript)
	referenceInput, referenceUtxo := dijkstraReferenceScriptInput(
		plutusScript,
		905,
	)
	referenceAddress, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		plutusScript.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	referenceOutput := referenceUtxo.Output.(babbage.BabbageTransactionOutput)
	referenceOutput.OutputAddress = referenceAddress
	referenceUtxo.Output = referenceOutput
	guardSet := &DijkstraGuards{Credentials: []common.Credential{guard}}
	key := common.RedeemerKey{Tag: common.RedeemerTagGuarding}
	children := []DijkstraSubTransaction{
		{
			Body: DijkstraSubTransactionBody{TxGuards: guardSet},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsRedeemers: evaluateTestRedeemers(key),
			},
		},
		{
			Body: DijkstraSubTransactionBody{TxGuards: guardSet},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsRedeemers: evaluateTestRedeemers(key),
			},
		},
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards:          guardSet,
			TxReferenceInputs: dijkstraReferenceInputSet(referenceInput),
			TxSubTransactions: cbor.NewSetType(children, true),
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: evaluateTestRedeemers(key),
		},
		TxIsValid: true,
	}
	return tx, mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{referenceUtxo}).
		Build()
}

// evaluateTestDeclare sets every redeemer of tx to the units evaluation
// reported for it, less shortfall steps for the redeemer at position short.
func evaluateTestDeclare(
	t *testing.T,
	tx *DijkstraTransaction,
	results []PlutusRedeemerEvaluation,
	short int,
) {
	t.Helper()
	subTxs := tx.Body.TxSubTransactions.Items()
	for idx, result := range results {
		units := result.ExUnits
		if idx == short {
			units.Steps--
		}
		redeemers := tx.WitnessSet.WsRedeemers.Redeemers
		if result.SubTransactionIndex != nil {
			redeemers = subTxs[*result.SubTransactionIndex].WitnessSet.WsRedeemers.Redeemers
		}
		value, ok := redeemers[result.Key]
		require.True(t, ok, "result names an absent redeemer %v", result.Key)
		value.ExUnits = units
		redeemers[result.Key] = value
	}
}

func TestEvaluatePlutusScriptsMatchesValidation(t *testing.T) {
	cases := []struct {
		name   string
		build  func(t *testing.T) (*DijkstraTransaction, common.LedgerState)
		subTxs []*uint32
		tags   []common.RedeemerTag
	}{
		{
			name: "top-level Plutus V3 mint",
			build: func(t *testing.T) (*DijkstraTransaction, common.LedgerState) {
				return evaluateTestMintTx(
					t,
					dijkstraGuardTestPlutus(t, lang.LanguageVersionV3, false),
				), mockledger.NewLedgerStateBuilder().Build()
			},
			subTxs: []*uint32{nil},
			tags:   []common.RedeemerTag{common.RedeemerTagMint},
		},
		{
			name: "top-level Plutus V4 mint",
			build: func(t *testing.T) (*DijkstraTransaction, common.LedgerState) {
				return evaluateTestMintTx(
					t,
					dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false),
				), mockledger.NewLedgerStateBuilder().Build()
			},
			subTxs: []*uint32{nil},
			tags:   []common.RedeemerTag{common.RedeemerTagMint},
		},
		{
			name: "top-level Plutus V3 guard",
			build: func(t *testing.T) (*DijkstraTransaction, common.LedgerState) {
				return evaluateTestTopLevelGuardTx(
					t,
					dijkstraGuardTestPlutus(t, lang.LanguageVersionV3, false),
				), mockledger.NewLedgerStateBuilder().Build()
			},
			subTxs: []*uint32{nil},
			tags:   []common.RedeemerTag{common.RedeemerTagGuarding},
		},
		{
			name: "sub-transaction Plutus V4 guard",
			build: func(t *testing.T) (*DijkstraTransaction, common.LedgerState) {
				return dijkstraSubtransactionPlutusGuardTx(
					t,
					dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false),
					false,
				)
			},
			subTxs: []*uint32{new(uint32)},
			tags:   []common.RedeemerTag{common.RedeemerTagGuarding},
		},
		{
			name: "sub-transaction guard from a top-level reference script",
			build: func(t *testing.T) (*DijkstraTransaction, common.LedgerState) {
				return dijkstraSubtransactionPlutusGuardTx(
					t,
					dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false),
					true,
				)
			},
			subTxs: []*uint32{new(uint32)},
			tags:   []common.RedeemerTag{common.RedeemerTagGuarding},
		},
		{
			name:  "context-sensitive V4 guards at every level",
			build: evaluateTestContextTx,
			subTxs: []*uint32{
				new(uint32),
				func() *uint32 { one := uint32(1); return &one }(),
				nil,
			},
			tags: []common.RedeemerTag{
				common.RedeemerTagGuarding,
				common.RedeemerTagGuarding,
				common.RedeemerTagGuarding,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pp := dijkstraGuardTestPParams()
			tx, ls := tc.build(t)
			results, err := EvaluatePlutusScripts(
				tx,
				ls,
				pp,
				evaluateTestBudget,
			)
			require.NoError(t, err)
			require.Len(t, results, len(tc.tags))
			for idx, result := range results {
				require.Equal(t, tc.subTxs[idx], result.SubTransactionIndex)
				require.Equal(t, tc.tags[idx], result.Key.Tag)
				require.Positive(t, result.ExUnits.Steps)
				require.Positive(t, result.ExUnits.Memory)
			}
			rules := []common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts}

			evaluateTestDeclare(t, tx, results, -1)
			require.NoError(t, common.VerifyTransaction(tx, 0, ls, pp, rules))

			for short := range results {
				evaluateTestDeclare(t, tx, results, short)
				err := common.VerifyTransaction(tx, 0, ls, pp, rules)
				var scriptErr conway.PlutusScriptFailedError
				require.ErrorAs(t, err, &scriptErr)
				require.Equal(t, results[short].Key.Tag, scriptErr.Tag)
			}
		})
	}
}

func TestEvaluatePlutusScriptsIgnoresDeclaredUnits(t *testing.T) {
	pp := dijkstraGuardTestPParams()
	ls := mockledger.NewLedgerStateBuilder().Build()
	v4 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	tx := evaluateTestMintTx(t, v4)
	want, err := EvaluatePlutusScripts(tx, ls, pp, evaluateTestBudget)
	require.NoError(t, err)
	tx.WitnessSet.WsRedeemers.Redeemers[common.RedeemerKey{
		Tag: common.RedeemerTagMint,
	}] = common.RedeemerValue{}
	tx.TxIsValid = false
	got, err := EvaluatePlutusScripts(tx, ls, pp, evaluateTestBudget)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestEvaluatePlutusScriptsReportsFailures(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	t.Run("failing script", func(t *testing.T) {
		tx := evaluateTestMintTx(
			t,
			dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, true),
		)
		_, err := EvaluatePlutusScripts(
			tx,
			ls,
			dijkstraGuardTestPParams(),
			evaluateTestBudget,
		)
		var scriptErr conway.PlutusScriptFailedError
		require.ErrorAs(t, err, &scriptErr)
		require.Equal(t, common.RedeemerTagMint, scriptErr.Tag)
	})
	t.Run("budget exhausted", func(t *testing.T) {
		tx := evaluateTestMintTx(
			t,
			dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false),
		)
		_, err := EvaluatePlutusScripts(
			tx,
			ls,
			dijkstraGuardTestPParams(),
			common.ExUnits{Steps: 1, Memory: 1},
		)
		var scriptErr conway.PlutusScriptFailedError
		require.ErrorAs(t, err, &scriptErr)
	})
	t.Run("redeemer without a script purpose", func(t *testing.T) {
		tx := evaluateTestMintTx(
			t,
			dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false),
		)
		tx.WitnessSet.WsRedeemers.Redeemers[common.RedeemerKey{
			Tag:   common.RedeemerTagMint,
			Index: 1,
		}] = common.RedeemerValue{ExUnits: evaluateTestBudget}
		_, err := EvaluatePlutusScripts(
			tx,
			ls,
			dijkstraGuardTestPParams(),
			evaluateTestBudget,
		)
		var extraErr conway.ExtraRedeemerError
		require.ErrorAs(t, err, &extraErr)
	})
}
