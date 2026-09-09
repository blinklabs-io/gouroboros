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

// selectRules picks named rules out of an era's production rule set, keeping
// the order the era registered them in. A case therefore runs the rules the
// era actually wired in, in the order it runs them, rather than functions
// called directly.
func selectRules(
	t *testing.T,
	rules []common.UtxoValidationRuleFunc,
	suffixes ...string,
) []common.UtxoValidationRuleFunc {
	t.Helper()
	out := make([]common.UtxoValidationRuleFunc, 0, len(suffixes))
	for _, rule := range rules {
		name := runtime.FuncForPC(reflect.ValueOf(rule).Pointer()).Name()
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				out = append(out, rule)
				break
			}
		}
	}
	require.Len(
		t,
		out,
		len(suffixes),
		"expected rules are not registered: %v",
		suffixes,
	)
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
			rules := selectRules(
				t,
				era.rules,
				".UtxoValidateScriptWitnesses",
				".UtxoValidateNativeScripts",
			)
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

// UTXOW witness requirements apply before phase-2 validity is interpreted.
// An invalid transaction may therefore use a native script carried by a
// reference input, but it may not omit the required script entirely.
func TestInvalidTransactionsRetainReferenceScriptRequirements(t *testing.T) {
	vkey := bytes.Repeat([]byte{0x64}, 32)
	nativeScript := testPubkeyNativeScript(t, vkey)
	unusedScript := testPubkeyNativeScript(t, bytes.Repeat([]byte{0x65}, 32))
	scriptAddr := testScriptPaymentAddress(t, nativeScript.Hash())
	keyAddr := testKeyPaymentAddress(t, vkey)

	spendInput := shelley.NewShelleyTransactionInput(
		"6666666666666666666666666666666666666666666666666666666666666666",
		0,
	)
	refInput := shelley.NewShelleyTransactionInput(
		"7777777777777777777777777777777777777777777777777777777777777777",
		0,
	)

	eras := []struct {
		name  string
		rules []common.UtxoValidationRuleFunc
	}{
		{name: "Babbage", rules: babbage.UtxoValidationRules},
		{name: "Conway", rules: conway.UtxoValidationRules},
		{name: "Dijkstra", rules: dijkstra.UtxoValidationRules},
	}
	tests := []struct {
		name      string
		reference common.Script
		wantError bool
	}{
		{
			name:      "matching reference script satisfies the spend",
			reference: nativeScript,
		},
		{
			name:      "unrelated reference script does not satisfy the spend",
			reference: unusedScript,
			wantError: true,
		},
	}

	for _, era := range eras {
		t.Run(era.name, func(t *testing.T) {
			rules := selectRules(
				t,
				era.rules,
				".UtxoValidateScriptWitnesses",
				".UtxoValidateNativeScripts",
			)
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					tx := mockledger.NewTransactionBuilder()
					tx.WithValid(false)
					tx.WithInputs(spendInput)
					tx.WithReferenceInputs(refInput)
					tx.WithWitnesses(
						mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
							common.VkeyWitness{Vkey: vkey},
						),
					)
					ls := testLedgerState(map[string]common.TransactionOutput{
						spendInput.String(): testOutput(scriptAddr, nil),
						refInput.String():   testOutput(keyAddr, test.reference),
					})

					err := common.VerifyTransaction(tx, 0, ls, nil, rules)
					if !test.wantError {
						require.NoError(t, err)
						return
					}
					var missing common.MissingScriptWitnessesError
					require.ErrorAs(t, err, &missing)
					require.Equal(t, common.ScriptHash(nativeScript.Hash()), missing.ScriptHash)
				})
			}
		})
	}
}

// The native-script rule reads an empty script view when an input cannot be
// resolved, so it evaluates the witness set alone and contributes no
// reference-provided script. That is safe only because a transaction with an
// unresolvable input is rejected anyway, by a rule every era registers ahead
// of the native-script rule: UtxoValidateBadInputsUtxo for a consumed input,
// UtxoValidateScriptWitnesses for a reference input, which
// UtxoValidateBadInputsUtxo does not cover. Resolving the reachable inputs
// into a partial view instead would make the needed-script set depend on
// which lookups happened to succeed, which is worse for a rule that has to
// reach the same verdict on every node.
func TestUnresolvableInputsAreRejectedBeforeNativeScripts(t *testing.T) {
	vkey := bytes.Repeat([]byte{0x63}, 32)
	nativeScript := testPubkeyNativeScript(t, vkey)
	scriptAddr := testScriptPaymentAddress(t, nativeScript.Hash())
	keyAddr := testKeyPaymentAddress(t, vkey)

	scriptInput := shelley.NewShelleyTransactionInput(
		"3333333333333333333333333333333333333333333333333333333333333333",
		0,
	)
	refInput := shelley.NewShelleyTransactionInput(
		"4444444444444444444444444444444444444444444444444444444444444444",
		0,
	)
	missingInput := shelley.NewShelleyTransactionInput(
		"5555555555555555555555555555555555555555555555555555555555555555",
		0,
	)

	tests := []struct {
		name   string
		build  func() (*mockledger.MockTransaction, common.LedgerState)
		assert func(t *testing.T, err error)
	}{
		{
			name: "unresolvable consumed input",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := mockledger.NewTransactionBuilder()
				tx.WithInputs(scriptInput, missingInput)
				tx.WithReferenceInputs(refInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						scriptInput.String(): testOutput(scriptAddr, nil),
						refInput.String(): testOutput(
							keyAddr,
							nativeScript,
						),
					},
				)
			},
			assert: func(t *testing.T, err error) {
				var badInputs shelley.BadInputsUtxoError
				require.ErrorAs(t, err, &badInputs)
			},
		},
		{
			name: "unresolvable reference input",
			build: func() (*mockledger.MockTransaction, common.LedgerState) {
				tx := mockledger.NewTransactionBuilder()
				tx.WithInputs(scriptInput)
				tx.WithReferenceInputs(missingInput)
				return tx, testLedgerState(
					map[string]common.TransactionOutput{
						scriptInput.String(): testOutput(scriptAddr, nil),
					},
				)
			},
			assert: func(t *testing.T, err error) {
				require.ErrorIs(
					t,
					err,
					common.ErrReferenceInputResolution,
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
			rules := selectRules(
				t,
				era.rules,
				".UtxoValidateBadInputsUtxo",
				".UtxoValidateScriptWitnesses",
				".UtxoValidateNativeScripts",
			)
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					tx, ls := test.build()
					err := common.VerifyTransaction(tx, 0, ls, nil, rules)
					require.Error(t, err)
					test.assert(t, err)
				})
			}
		})
	}
}
