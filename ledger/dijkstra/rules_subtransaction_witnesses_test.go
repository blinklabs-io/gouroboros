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
	"bytes"
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
	value := cbor.RawMessage{0xf6}
	if datum != nil {
		var err error
		value, err = cbor.Encode(*datum)
		require.NoError(t, err)
	}
	return dijkstraRequiredGuardsRawDatum(t, guard, value)
}

func dijkstraRequiredGuardsRawDatum(
	t *testing.T,
	guard common.Credential,
	datum cbor.RawMessage,
) *DijkstraRawCbor {
	t.Helper()
	raw, err := cbor.Encode(map[dijkstraV4TestCredentialKey]cbor.RawMessage{
		{Type: guard.CredType, Hash: guard.Credential}: datum,
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

func TestDijkstraRequiredTopLevelGuardDatumCBOR(t *testing.T) {
	guard := testGuardCredential()
	tests := []struct {
		name     string
		datum    cbor.RawMessage
		trailing bool
		wantErr  string
	}{
		{
			name:  "null optional datum",
			datum: cbor.RawMessage{0xf6},
		},
		{
			name:  "non-canonical integer datum preserves bytes",
			datum: cbor.RawMessage{0x18, 0x01},
		},
		{
			name:    "CBOR true is not Plutus data",
			datum:   cbor.RawMessage{0xf5},
			wantErr: "decode required guard datum",
		},
		{
			name:    "CBOR undefined is not optional data",
			datum:   cbor.RawMessage{0xf7},
			wantErr: "decode required guard datum",
		},
		{
			name:     "trailing top-level bytes",
			datum:    cbor.RawMessage{0x01},
			trailing: true,
			wantErr:  "decode required top-level guards",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			required := dijkstraRequiredGuardsRawDatum(t, guard, test.datum)
			if test.trailing {
				raw := append([]byte(nil), required.Cbor()...)
				required.SetCbor(append(raw, 0xf6))
			}
			decoded, err := dijkstraRequiredTopLevelGuards(
				&DijkstraSubTransactionBody{
					TxRequiredTopLevelGuards: required,
				},
			)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(
				t,
				test.datum,
				decoded[dijkstraCredentialKey{
					Type: guard.CredType,
					Hash: guard.Credential,
				}],
			)
		})
	}
}

func TestDijkstraInvalidTransactionRejectsNonPlutusGuardDatum(t *testing.T) {
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(plutus)
	tx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxRequiredTopLevelGuards: dijkstraRequiredGuardsRawDatum(
				t,
				guard,
				cbor.RawMessage{0xf5},
			),
		},
	})
	tx.TxIsValid = false
	tx.Body.TxGuards = &DijkstraGuards{
		Credentials: []common.Credential{guard},
	}
	tx.WitnessSet = testDijkstraWitnessSet(t, plutus)
	tx.WitnessSet.WsRedeemers = dijkstraGuardRedeemers()

	err := common.VerifyTransaction(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
		[]common.UtxoValidationRuleFunc{
			dijkstraRule(
				t,
				common.UtxoValidationRuleRedeemerAndScriptWitnesses,
			),
		},
	)
	require.ErrorContains(t, err, "decode required guard datum")
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

func TestDijkstraRejectsNativePurposeRedeemersWithGlobalScripts(t *testing.T) {
	native := testRequireGuardNativeScript(t, testGuardCredential())
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	mint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			native.Hash(): {
				cbor.NewByteString(nil): big.NewInt(1),
			},
			plutus.Hash(): {
				cbor.NewByteString(nil): big.NewInt(1),
			},
		},
	)
	nativeIndex := uint32(0)
	plutusIndex := uint32(1)
	if bytes.Compare(plutus.Hash().Bytes(), native.Hash().Bytes()) < 0 {
		nativeIndex, plutusIndex = plutusIndex, nativeIndex
	}
	nativeKey := common.RedeemerKey{
		Tag:   common.RedeemerTagMint,
		Index: nativeIndex,
	}
	plutusKey := common.RedeemerKey{
		Tag:   common.RedeemerTagMint,
		Index: plutusIndex,
	}

	for _, source := range []string{"top", "sibling"} {
		for _, txIsValid := range []bool{true, false} {
			validity := "valid"
			if !txIsValid {
				validity = "phase-2 invalid"
			}
			for _, extraNative := range []bool{false, true} {
				redeemerSet := "exact Plutus set"
				if extraNative {
					redeemerSet = "extra native redeemer"
				}
				t.Run(
					source+"/"+validity+"/"+redeemerSet,
					func(t *testing.T) {
						redeemers := map[common.RedeemerKey]common.RedeemerValue{
							plutusKey: {
								ExUnits: common.ExUnits{
									Steps:  10_000_000,
									Memory: 10_000_000,
								},
							},
						}
						if extraNative {
							redeemers[nativeKey] = common.RedeemerValue{}
						}
						witnesses := testDijkstraWitnessSet(t, plutus)
						witnesses.WsRedeemers = DijkstraRedeemers{
							Redeemers: redeemers,
						}
						target := DijkstraSubTransaction{
							Body: DijkstraSubTransactionBody{
								TxMint: &mint,
							},
							WitnessSet: witnesses,
						}
						input, utxo := dijkstraReferenceScriptInput(native, 931)
						tx := &DijkstraTransaction{TxIsValid: txIsValid}
						if source == "top" {
							tx.Body.TxReferenceInputs = dijkstraReferenceInputSet(
								input,
							)
							tx.Body.TxSubTransactions = cbor.NewSetType(
								[]DijkstraSubTransaction{target},
								true,
							)
						} else {
							tx.Body.TxSubTransactions = cbor.NewSetType(
								[]DijkstraSubTransaction{
									{Body: DijkstraSubTransactionBody{
										TxReferenceInputs: dijkstraReferenceInputSet(input),
									}},
									target,
								},
								true,
							)
						}
						ls := mockledger.NewLedgerStateBuilder().WithUtxos(
							[]common.Utxo{utxo},
						).Build()
						for _, validator := range []struct {
							name string
							rule common.UtxoValidationRuleFunc
						}{
							{
								name: "witness rule",
								rule: UtxoValidateRedeemerAndScriptWitnesses,
							},
							{
								name: "Plutus rule",
								rule: UtxoValidatePlutusScripts,
							},
						} {
							t.Run(validator.name, func(t *testing.T) {
								err := common.VerifyTransaction(
									tx,
									0,
									ls,
									dijkstraGuardTestPParams(),
									[]common.UtxoValidationRuleFunc{
										validator.rule,
									},
								)
								if !extraNative {
									require.NoError(t, err)
									return
								}
								var extra conway.ExtraRedeemerError
								require.ErrorAs(t, err, &extra)
								require.Equal(t, nativeKey, extra.RedeemerKey)
							})
						}
					},
				)
			}
		}
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
