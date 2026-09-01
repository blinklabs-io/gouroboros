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
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// testPubkeyNativeScript builds a native script that is satisfied only when
// vkey has signed the transaction, so a case toggles satisfied/unsatisfied by
// adding or omitting the vkey witness rather than by moving the slot.
func testPubkeyNativeScript(
	t *testing.T,
	vkey []byte,
) common.NativeScript {
	t.Helper()
	scriptCbor, err := cbor.Encode(common.NativeScriptPubkey{
		Type: 0,
		Hash: common.Blake2b224Hash(vkey).Bytes(),
	})
	require.NoError(t, err)
	var nativeScript common.NativeScript
	require.NoError(t, nativeScript.UnmarshalCBOR(scriptCbor))
	return nativeScript
}

func testScriptPaymentAddress(
	t *testing.T,
	scriptHash common.ScriptHash,
) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkMainnet,
		scriptHash.Bytes(),
		nil,
	)
	require.NoError(t, err)
	return addr
}

func testKeyPaymentAddress(t *testing.T, vkey []byte) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkMainnet,
		common.Blake2b224Hash(vkey).Bytes(),
		nil,
	)
	require.NoError(t, err)
	return addr
}

// testOutput builds a Babbage output, optionally carrying scriptRef as its
// reference script. Babbage is the first era with reference scripts and the
// output type every later era wraps, so the same output serves all three
// eras under test.
func testOutput(
	addr common.Address,
	scriptRef common.Script,
) common.TransactionOutput {
	out := &babbage.BabbageTransactionOutput{OutputAddress: addr}
	if scriptRef != nil {
		out.TxOutScriptRef = &common.ScriptRef{
			Type:   common.ScriptRefTypeNativeScript,
			Script: scriptRef,
		}
	}
	return out
}

// testLedgerState resolves exactly the inputs the case set up. An input the
// case did not register fails to resolve, so a rule that reaches for one the
// test did not intend is caught rather than handed a zero value.
func testLedgerState(
	outputs map[string]common.TransactionOutput,
) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(
			func(input common.TransactionInput) (common.Utxo, error) {
				output, ok := outputs[input.String()]
				if !ok {
					return common.Utxo{}, errors.New("unknown input")
				}
				return common.Utxo{Id: input, Output: output}, nil
			},
		).
		Build()
}

// scriptAuthorizationRules picks the two rules that decide whether a needed
// script is provided and whether it is satisfied out of an era's production
// rule set, so a case runs the registered rules rather than a function the
// era might not have wired in.
func scriptAuthorizationRules(
	t *testing.T,
	rules []common.UtxoValidationRuleFunc,
) []common.UtxoValidationRuleFunc {
	t.Helper()
	out := make([]common.UtxoValidationRuleFunc, 0, 2)
	for _, rule := range rules {
		name := runtime.FuncForPC(reflect.ValueOf(rule).Pointer()).Name()
		if strings.HasSuffix(name, ".UtxoValidateScriptWitnesses") ||
			strings.HasSuffix(name, ".UtxoValidateNativeScripts") {
			out = append(out, rule)
		}
	}
	require.Len(t, out, 2, "authorization rules are not registered")
	return out
}

// A native script some purpose of the transaction requires must be evaluated
// wherever the transaction supplies it. The rule used to read only
// witnesses.NativeScripts(), so a script delivered as a reference script --
// on a reference input or on the spent input itself, both permitted by
// CIP-33 -- was never evaluated and an unsatisfied one passed phase-1.
// cardano-ledger evaluates the needed native scripts out of the scripts the
// whole transaction provides (Cardano.Ledger.Babbage.Rules.Utxow,
// validateFailedBabbageScripts).
func TestReferenceProvidedNativeScriptsAreValidated(t *testing.T) {
	vkey := bytes.Repeat([]byte{0x61}, 32)
	nativeScript := testPubkeyNativeScript(t, vkey)
	scriptHash := nativeScript.Hash()
	scriptAddr := testScriptPaymentAddress(t, scriptHash)

	// An unrelated script parked on a reference input the transaction names
	// but no purpose requires. It can never be satisfied, so evaluating it
	// would reject a transaction the ledger accepts.
	unusedVkey := bytes.Repeat([]byte{0x62}, 32)
	unusedScript := testPubkeyNativeScript(t, unusedVkey)
	keyAddr := testKeyPaymentAddress(t, vkey)

	spentInput := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		0,
	)
	refInput := shelley.NewShelleyTransactionInput(
		"2222222222222222222222222222222222222222222222222222222222222222",
		0,
	)

	witnessedTx := func(
		signed bool,
	) *mockledger.MockTransaction {
		wits := mockledger.NewMockTransactionWitnessSet()
		if signed {
			wits = wits.WithVkeyWitnesses(common.VkeyWitness{Vkey: vkey})
		}
		return mockledger.NewTransactionBuilder().WithWitnesses(wits)
	}

	tests := []struct {
		name    string
		build   func() (*mockledger.MockTransaction, common.LedgerState)
		wantErr bool
	}{
		{
			name: "reference input provides unsatisfied script",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := witnessedTx(false)
				tx.WithInputs(spentInput)
				tx.WithReferenceInputs(refInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						spentInput.String(): testOutput(scriptAddr, nil),
						refInput.String(): testOutput(
							keyAddr,
							nativeScript,
						),
					},
				)
			},
			wantErr: true,
		},
		{
			name: "spent input provides unsatisfied script",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := witnessedTx(false)
				tx.WithInputs(spentInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						spentInput.String(): testOutput(
							scriptAddr,
							nativeScript,
						),
					},
				)
			},
			wantErr: true,
		},
		{
			name: "reference input provides satisfied script",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := witnessedTx(true)
				tx.WithInputs(spentInput)
				tx.WithReferenceInputs(refInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						spentInput.String(): testOutput(scriptAddr, nil),
						refInput.String(): testOutput(
							keyAddr,
							nativeScript,
						),
					},
				)
			},
		},
		{
			name: "spent input provides satisfied script",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := witnessedTx(true)
				tx.WithInputs(spentInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						spentInput.String(): testOutput(
							scriptAddr,
							nativeScript,
						),
					},
				)
			},
		},
		{
			name: "unused reference script is not evaluated",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := witnessedTx(false)
				tx.WithInputs(spentInput)
				tx.WithReferenceInputs(refInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						spentInput.String(): testOutput(keyAddr, nil),
						refInput.String(): testOutput(
							keyAddr,
							unusedScript,
						),
					},
				)
			},
		},
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
		t.Run(era.name, func(t *testing.T) {
			rules := scriptAuthorizationRules(t, era.rules)
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					// Both rules run together on purpose: an
					// unsatisfied case must fail as a script
					// failure, not as a missing witness, which
					// pins that the two rules agree the script
					// was provided.
					tx, ls := test.build()
					err := common.VerifyTransaction(tx, 0, ls, nil, rules)
					if !test.wantErr {
						require.NoError(t, err)
						return
					}
					var failed allegra.NativeScriptFailedError
					require.ErrorAs(t, err, &failed)
					require.Equal(t, scriptHash, failed.ScriptHash)
				})
			}
		})
	}
}
