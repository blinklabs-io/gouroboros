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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// A needed script supplied by a reference script is removed from the set the
// witness set may carry: babbageMissingScripts computes
// extra = sReceived \ (sNeeded \ sRefs), so a witness duplicating it is
// ExtraneousScriptWitnessesUTXOW.
func TestReferenceSuppliedScriptWitnessIsExtraneous(t *testing.T) {
	t.Parallel()
	vkey := bytes.Repeat([]byte{0x71}, 32)
	keyAddr := testKeyPaymentAddress(t, vkey)
	spentInput := shelley.MustNewShelleyTransactionInput(
		"3333333333333333333333333333333333333333333333333333333333333333", 0,
	)
	refInput := shelley.MustNewShelleyTransactionInput(
		"4444444444444444444444444444444444444444444444444444444444444444", 0,
	)
	nativeScript := testPubkeyNativeScript(t, vkey)
	plutusScript := common.PlutusV2Script([]byte{0x41, 0x01})
	unrelatedScript := common.PlutusV2Script([]byte{0x41, 0x02})
	spendRedeemer := conway.ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: 0}: {},
		},
	}

	scripts := []struct {
		name      string
		script    common.Script
		refType   uint
		hash      common.ScriptHash
		redeemers bool
		witness   func(
			*mockledger.MockTransactionWitnessSet,
		) *mockledger.MockTransactionWitnessSet
	}{
		{
			name:    "native",
			script:  nativeScript,
			refType: common.ScriptRefTypeNativeScript,
			hash:    nativeScript.Hash(),
			witness: func(w *mockledger.MockTransactionWitnessSet) *mockledger.MockTransactionWitnessSet {
				return w.WithNativeScripts(nativeScript).
					WithVkeyWitnesses(common.VkeyWitness{Vkey: vkey})
			},
		},
		{
			name:      "plutus",
			script:    plutusScript,
			refType:   common.ScriptRefTypePlutusV2,
			hash:      plutusScript.Hash(),
			redeemers: true,
			witness: func(w *mockledger.MockTransactionWitnessSet) *mockledger.MockTransactionWitnessSet {
				return w.WithPlutusV2Scripts(plutusScript)
			},
		},
	}
	sources := []string{"reference input", "spent input"}
	modes := []struct {
		name      string
		ref       bool
		unrelated bool
		explicit  bool
		wantErr   bool
	}{
		{name: "reference and witness", ref: true, explicit: true, wantErr: true},
		{name: "reference only", ref: true},
		{name: "witness only", explicit: true},
		// A reference script for a different hash leaves the witness needed.
		{name: "unrelated reference and witness", ref: true, unrelated: true, explicit: true},
	}
	eras := []struct {
		name  string
		rules []common.UtxoValidationRuleFunc
	}{
		{name: "Babbage", rules: babbage.UtxoValidationRules},
		{name: "Conway", rules: conway.UtxoValidationRules},
		{name: "Dijkstra", rules: dijkstra.UtxoValidationRules},
	}
	for _, era := range eras {
		rules := selectRules(t, era.rules, ".UtxoValidateScriptWitnesses")
		for _, s := range scripts {
			for _, source := range sources {
				for _, mode := range modes {
					name := era.name + "/" + s.name + "/" + source + "/" + mode.name
					t.Run(name, func(t *testing.T) {
						t.Parallel()
						scriptAddr := testScriptPaymentAddress(t, s.hash)
						var withRef common.TransactionOutput = &babbage.BabbageTransactionOutput{
							OutputAddress: scriptAddr,
						}
						refOut := &babbage.BabbageTransactionOutput{
							OutputAddress: keyAddr,
						}
						if mode.ref {
							ref := &common.ScriptRef{
								Type:   s.refType,
								Script: s.script,
							}
							if mode.unrelated {
								ref = &common.ScriptRef{
									Type:   common.ScriptRefTypePlutusV2,
									Script: unrelatedScript,
								}
							}
							if source == "spent input" {
								withRef = &babbage.BabbageTransactionOutput{
									OutputAddress:  scriptAddr,
									TxOutScriptRef: ref,
								}
							} else {
								refOut.TxOutScriptRef = ref
							}
						}
						wits := mockledger.NewMockTransactionWitnessSet()
						if mode.explicit {
							wits = s.witness(wits)
						}
						if s.redeemers {
							wits = wits.WithRedeemers(spendRedeemer)
						}
						tx := mockledger.NewTransactionBuilder()
						tx.WithInputs(spentInput)
						tx.WithWitnesses(wits)
						outputs := map[string]common.TransactionOutput{
							spentInput.String(): withRef,
						}
						if source == "reference input" {
							tx.WithReferenceInputs(refInput)
							outputs[refInput.String()] = refOut
						}
						err := common.VerifyTransaction(
							tx, 0, testLedgerState(outputs), nil, rules,
						)
						if !mode.wantErr {
							require.NoError(t, err)
							return
						}
						var extra common.ExtraneousScriptWitnessesError
						require.ErrorAs(t, err, &extra)
						require.Equal(t, s.hash, extra.ScriptHash)
					})
				}
			}
		}
	}
}
