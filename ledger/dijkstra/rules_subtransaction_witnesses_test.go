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
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

func dijkstraRule(
	t *testing.T,
	id common.UtxoValidationRuleId,
) common.UtxoValidationRuleFunc {
	t.Helper()
	for _, descriptor := range utxoValidationRuleDescriptors {
		if descriptor.Id == id {
			return descriptor.Validator
		}
	}
	t.Fatalf("Dijkstra validation rule %q is not registered", id)
	return nil
}

func dijkstraGuardRedeemers() DijkstraRedeemers {
	return DijkstraRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagGuarding}: {},
		},
	}
}

func dijkstraRequiredGuardsRaw(
	t *testing.T,
	guard common.Credential,
	datum *common.Datum,
) *DijkstraRawCbor {
	t.Helper()
	value := any(nil)
	if datum != nil {
		value = *datum
	}
	raw, err := cbor.Encode(map[dijkstraV4TestCredentialKey]any{
		{Type: guard.CredType, Hash: guard.Credential}: value,
	})
	require.NoError(t, err)
	ret := &DijkstraRawCbor{}
	ret.SetCbor(raw)
	return ret
}

func dijkstraSingleSubTx(sub DijkstraSubTransaction) *DijkstraTransaction {
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{TxSubTransactions: cbor.NewSetType(
			[]DijkstraSubTransaction{sub},
			true,
		)},
		TxIsValid: true,
	}
}

func TestDijkstraScriptWitnessesUseAllTransactionLevels(t *testing.T) {
	script := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(script)
	tests := []struct {
		name string
		tx   *DijkstraTransaction
		ls   common.LedgerState
	}{
		{
			name: "top witness satisfies subtransaction guard",
			tx: func() *DijkstraTransaction {
				tx := dijkstraSingleSubTx(DijkstraSubTransaction{
					Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{guard},
					}},
					WitnessSet: DijkstraTransactionWitnessSet{
						WsRedeemers: dijkstraGuardRedeemers(),
					},
				})
				tx.WitnessSet = testDijkstraWitnessSet(t, script)
				return tx
			}(),
		},
		{
			name: "subtransaction witness satisfies top guard",
			tx: func() *DijkstraTransaction {
				subWitnesses := testDijkstraWitnessSet(t, script)
				tx := dijkstraSingleSubTx(DijkstraSubTransaction{
					WitnessSet: subWitnesses,
				})
				tx.Body.TxGuards = &DijkstraGuards{
					Credentials: []common.Credential{guard},
				}
				tx.WitnessSet.WsRedeemers = dijkstraGuardRedeemers()
				return tx
			}(),
		},
		{
			name: "top reference satisfies subtransaction guard",
			tx: func() *DijkstraTransaction {
				input, _ := dijkstraReferenceScriptInput(script, 920)
				tx := dijkstraSingleSubTx(DijkstraSubTransaction{
					Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{guard},
					}},
				})
				tx.Body.TxReferenceInputs = dijkstraReferenceInputSet(input)
				return tx
			}(),
			ls: func() common.LedgerState {
				_, utxo := dijkstraReferenceScriptInput(script, 920)
				return mockledger.NewLedgerStateBuilder().WithUtxos(
					[]common.Utxo{utxo},
				).Build()
			}(),
		},
		{
			name: "sibling reference satisfies subtransaction guard",
			tx: func() *DijkstraTransaction {
				input, _ := dijkstraReferenceScriptInput(script, 921)
				return &DijkstraTransaction{
					Body: DijkstraTransactionBody{
						TxSubTransactions: cbor.NewSetType(
							[]DijkstraSubTransaction{
								{Body: DijkstraSubTransactionBody{
									TxReferenceInputs: dijkstraReferenceInputSet(
										input,
									),
								}},
								{
									Body: DijkstraSubTransactionBody{
										TxGuards: &DijkstraGuards{
											Credentials: []common.Credential{
												guard,
											},
										},
									},
								},
							},
							true,
						),
					},
					TxIsValid: true,
				}
			}(),
			ls: func() common.LedgerState {
				_, utxo := dijkstraReferenceScriptInput(script, 921)
				return mockledger.NewLedgerStateBuilder().WithUtxos(
					[]common.Utxo{utxo},
				).Build()
			}(),
		},
	}
	validator := dijkstraRule(t, common.UtxoValidationRuleScriptWitnesses)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ls := test.ls
			if ls == nil {
				ls = mockledger.NewLedgerStateBuilder().Build()
			}
			require.NoError(t, common.VerifyTransaction(
				test.tx,
				0,
				ls,
				dijkstraGuardTestPParams(),
				[]common.UtxoValidationRuleFunc{validator},
			))
		})
	}
}

func TestDijkstraRequiresGuardScriptsAndRedeemersPerLevel(t *testing.T) {
	script := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(script)
	for _, subtransaction := range []bool{false, true} {
		level := "top"
		if subtransaction {
			level = "subtransaction"
		}
		t.Run(level+"/missing script", func(t *testing.T) {
			tx := &DijkstraTransaction{TxIsValid: true}
			if subtransaction {
				tx = dijkstraSingleSubTx(
					DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
						TxGuards: &DijkstraGuards{
							Credentials: []common.Credential{guard},
						},
					}},
				)
			} else {
				tx.Body.TxGuards = &DijkstraGuards{Credentials: []common.Credential{guard}}
			}
			err := UtxoValidateRedeemerAndScriptWitnesses(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				dijkstraGuardTestPParams(),
			)
			var missing common.MissingScriptWitnessesError
			require.ErrorAs(t, err, &missing)
			require.Equal(t, script.Hash(), missing.ScriptHash)
		})
		t.Run(level+"/missing redeemer", func(t *testing.T) {
			witnesses := testDijkstraWitnessSet(t, script)
			tx := &DijkstraTransaction{TxIsValid: true}
			if subtransaction {
				tx = dijkstraSingleSubTx(DijkstraSubTransaction{
					Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{guard},
					}},
					WitnessSet: witnesses,
				})
			} else {
				tx.Body.TxGuards = &DijkstraGuards{Credentials: []common.Credential{guard}}
				tx.WitnessSet = witnesses
			}
			err := UtxoValidateRedeemerAndScriptWitnesses(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				dijkstraGuardTestPParams(),
			)
			var missing conway.MissingRedeemerForScriptError
			require.ErrorAs(t, err, &missing)
			require.Equal(t, script.Hash(), missing.ScriptHash)
			require.Equal(t, common.RedeemerTagGuarding, missing.Tag)
		})
	}

	t.Run("cross-level valid control", func(t *testing.T) {
		tx := dijkstraSingleSubTx(DijkstraSubTransaction{
			Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guard},
			}},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsRedeemers: dijkstraGuardRedeemers(),
			},
		})
		tx.WitnessSet = testDijkstraWitnessSet(t, script)
		require.NoError(t, UtxoValidateRedeemerAndScriptWitnesses(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			dijkstraGuardTestPParams(),
		))
	})
}

func TestDijkstraRequiredTopLevelGuards(t *testing.T) {
	required := testGuardCredential()
	sub := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
		TxRequiredTopLevelGuards: dijkstraRequiredGuardsRaw(t, required, nil),
	}}
	t.Run("missing", func(t *testing.T) {
		err := UtxoValidateRedeemerAndScriptWitnesses(
			dijkstraSingleSubTx(sub),
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			dijkstraGuardTestPParams(),
		)
		var missing *MissingRequiredGuards
		require.ErrorAs(t, err, &missing)
		require.Equal(t, []common.Credential{required}, missing.Guards)
	})
	t.Run("present", func(t *testing.T) {
		tx := dijkstraSingleSubTx(sub)
		tx.Body.TxGuards = &DijkstraGuards{
			Credentials: []common.Credential{required},
		}
		require.NoError(t, UtxoValidateRedeemerAndScriptWitnesses(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			dijkstraGuardTestPParams(),
		))
	})
}

func TestDijkstraRequiredGuardDatumShapes(t *testing.T) {
	datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	native := testRequireGuardNativeScript(t, testGuardCredential())
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	tests := []struct {
		name      string
		guard     common.Credential
		script    common.Script
		datum     *common.Datum
		redeemers DijkstraRedeemers
		malformed bool
	}{
		{
			name:      "key with datum",
			guard:     testGuardCredential(),
			datum:     &datum,
			malformed: true,
		},
		{name: "key without datum", guard: testGuardCredential()},
		{
			name:      "native with datum",
			guard:     dijkstraGuardCredentialForScript(native),
			script:    native,
			datum:     &datum,
			malformed: true,
		},
		{
			name:   "native without datum",
			guard:  dijkstraGuardCredentialForScript(native),
			script: native,
		},
		{
			name:      "Plutus without datum",
			guard:     dijkstraGuardCredentialForScript(plutus),
			script:    plutus,
			redeemers: dijkstraGuardRedeemers(),
			malformed: true,
		},
		{
			name:      "Plutus with datum",
			guard:     dijkstraGuardCredentialForScript(plutus),
			script:    plutus,
			datum:     &datum,
			redeemers: dijkstraGuardRedeemers(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := dijkstraSingleSubTx(
				DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
					TxRequiredTopLevelGuards: dijkstraRequiredGuardsRaw(
						t,
						test.guard,
						test.datum,
					),
				}},
			)
			tx.Body.TxGuards = &DijkstraGuards{
				Credentials: []common.Credential{test.guard},
			}
			tx.WitnessSet = testDijkstraWitnessSet(t, test.script)
			tx.WitnessSet.WsRedeemers = test.redeemers
			err := UtxoValidateRedeemerAndScriptWitnesses(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				dijkstraGuardTestPParams(),
			)
			if !test.malformed {
				require.NoError(t, err)
				return
			}
			var malformed *MalformedGuardDatums
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, []common.Credential{test.guard}, malformed.Guards)
		})
	}
}

func TestDijkstraSubtransactionKeyGuardAuthorization(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	guard := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash(publicKey),
	}
	sub := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
		TxGuards: &DijkstraGuards{Credentials: []common.Credential{guard}},
	}}
	requiredValidator := dijkstraRule(
		t,
		common.UtxoValidationRuleRequiredVKeyWitnesses,
	)
	signatureValidator := dijkstraRule(t, common.UtxoValidationRuleSignatures)

	t.Run("missing witness", func(t *testing.T) {
		err := requiredValidator(dijkstraSingleSubTx(sub), 0, nil, nil)
		require.ErrorAs(t, err, &common.MissingVKeyWitnessesError{})
	})
	t.Run("signature is scoped to subtransaction body", func(t *testing.T) {
		tx := dijkstraSingleSubTx(sub)
		subTxs := tx.Body.TxSubTransactions.Items()
		subBodyCbor, err := cbor.Encode(subTxs[0].Body)
		require.NoError(t, err)
		subTxs[0].Body.SetCbor(subBodyCbor)
		txHash := tx.Hash()
		subTxs[0].WitnessSet.VkeyWitnesses = cbor.NewSetType(
			[]common.VkeyWitness{{
				Vkey:      publicKey,
				Signature: ed25519.Sign(privateKey, txHash[:]),
			}},
			true,
		)
		tx.Body.TxSubTransactions = cbor.NewSetType(subTxs, true)
		require.NoError(t, requiredValidator(tx, 0, nil, nil))
		require.Error(t, signatureValidator(tx, 0, nil, nil))
	})
	t.Run("valid", func(t *testing.T) {
		tx := dijkstraSingleSubTx(sub)
		subTxs := tx.Body.TxSubTransactions.Items()
		subBodyCbor, err := cbor.Encode(subTxs[0].Body)
		require.NoError(t, err)
		subTxs[0].Body.SetCbor(subBodyCbor)
		subHash := subTxs[0].Body.Id()
		subTxs[0].WitnessSet.VkeyWitnesses = cbor.NewSetType(
			[]common.VkeyWitness{{
				Vkey:      publicKey,
				Signature: ed25519.Sign(privateKey, subHash[:]),
			}},
			true,
		)
		tx.Body.TxSubTransactions = cbor.NewSetType(subTxs, true)
		require.NoError(t, requiredValidator(tx, 0, nil, nil))
		require.NoError(t, signatureValidator(tx, 0, nil, nil))
	})
}

func TestDijkstraInvalidTransactionsStillRequireExactRedeemers(t *testing.T) {
	script := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(script)
	for _, subtransaction := range []bool{false, true} {
		level := "top"
		if subtransaction {
			level = "subtransaction"
		}
		t.Run(level, func(t *testing.T) {
			witnesses := testDijkstraWitnessSet(t, script)
			tx := &DijkstraTransaction{TxIsValid: false}
			if subtransaction {
				tx = dijkstraSingleSubTx(DijkstraSubTransaction{
					Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{guard},
					}},
					WitnessSet: witnesses,
				})
				tx.TxIsValid = false
			} else {
				tx.Body.TxGuards = &DijkstraGuards{Credentials: []common.Credential{guard}}
				tx.WitnessSet = witnesses
			}
			err := UtxoValidatePlutusScripts(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				dijkstraGuardTestPParams(),
			)
			var missing conway.MissingRedeemerForScriptError
			require.ErrorAs(t, err, &missing)
		})
	}
}

func TestDijkstraExtraneousGuardRedeemersUseGlobalScripts(t *testing.T) {
	native := testRequireGuardNativeScript(t, testGuardCredential())
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	for _, source := range []string{"top", "sibling"} {
		for _, test := range []struct {
			name       string
			script     common.Script
			extraneous bool
		}{
			{name: "native", script: native, extraneous: true},
			{name: "Plutus control", script: plutus},
		} {
			t.Run(source+"/"+test.name, func(t *testing.T) {
				input, utxo := dijkstraReferenceScriptInput(test.script, 930)
				guardSub := DijkstraSubTransaction{
					Body: DijkstraSubTransactionBody{TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{
							dijkstraGuardCredentialForScript(test.script),
						},
					}},
					WitnessSet: DijkstraTransactionWitnessSet{
						WsRedeemers: dijkstraGuardRedeemers(),
					},
				}
				tx := &DijkstraTransaction{TxIsValid: true}
				if source == "top" {
					tx.Body.TxReferenceInputs = dijkstraReferenceInputSet(input)
					tx.Body.TxSubTransactions = cbor.NewSetType(
						[]DijkstraSubTransaction{guardSub},
						true,
					)
				} else {
					referenceSub := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
						TxReferenceInputs: dijkstraReferenceInputSet(input),
					}}
					tx.Body.TxSubTransactions = cbor.NewSetType(
						[]DijkstraSubTransaction{referenceSub, guardSub},
						true,
					)
				}
				err := UtxoValidateExtraneousRedeemers(
					tx,
					0,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{utxo},
					).Build(),
					dijkstraGuardTestPParams(),
				)
				if !test.extraneous {
					require.NoError(t, err)
					return
				}
				var extra conway.ExtraRedeemerError
				require.ErrorAs(t, err, &extra)
				require.Equal(
					t,
					common.RedeemerTagGuarding,
					extra.RedeemerKey.Tag,
				)
			})
		}
	}
}

func TestDijkstraInvalidFlagUsesSubtransactionRedeemers(t *testing.T) {
	tx := dijkstraSingleSubTx(
		DijkstraSubTransaction{WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: dijkstraGuardRedeemers(),
		}},
	)
	tx.TxIsValid = false
	validator := dijkstraRule(t, common.UtxoValidationRuleIsValidFlag)
	require.NoError(t, validator(tx, 0, nil, nil))

	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{{}},
		true,
	)
	require.ErrorAs(
		t,
		validator(tx, 0, nil, nil),
		&common.InvalidIsValidFlagError{},
	)
}
