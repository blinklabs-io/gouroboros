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
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
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

func TestVerifyTransactionRequiresReferencePlutusV4GuardRedeemer(t *testing.T) {
	v4 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(v4)
	referenceInput, referenceUtxo := dijkstraReferenceScriptInput(v4, 903)
	referenceAddress, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		v4.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	referenceOutput := referenceUtxo.Output.(babbage.BabbageTransactionOutput)
	referenceOutput.OutputAddress = referenceAddress
	referenceUtxo.Output = referenceOutput
	redeemerKey := common.RedeemerKey{
		Tag: common.RedeemerTagGuarding,
	}

	for _, subtransaction := range []bool{false, true} {
		level := "top-level"
		if subtransaction {
			level = "subtransaction"
		}
		for _, withRedeemer := range []bool{false, true} {
			witnessState := "missing redeemer"
			if withRedeemer {
				witnessState = "valid redeemer"
			}
			t.Run(level+"/"+witnessState, func(t *testing.T) {
				witnesses := DijkstraTransactionWitnessSet{}
				if withRedeemer {
					witnesses.WsRedeemers = DijkstraRedeemers{
						Redeemers: map[common.RedeemerKey]common.RedeemerValue{
							redeemerKey: {
								ExUnits: common.ExUnits{
									Steps:  10_000_000,
									Memory: 10_000_000,
								},
							},
						},
					}
				}
				tx := &DijkstraTransaction{
					Body: DijkstraTransactionBody{
						TxReferenceInputs: dijkstraReferenceInputSet(
							referenceInput,
						),
					},
					TxIsValid: true,
				}
				guards := &DijkstraGuards{
					Credentials: []common.Credential{guard},
				}
				if subtransaction {
					tx.Body.TxSubTransactions = cbor.NewSetType(
						[]DijkstraSubTransaction{{
							Body: DijkstraSubTransactionBody{
								TxGuards: guards,
							},
							WitnessSet: witnesses,
						}},
						true,
					)
				} else {
					tx.Body.TxGuards = guards
					tx.WitnessSet = witnesses
				}

				err := common.VerifyTransaction(
					tx,
					0,
					mockledger.NewLedgerStateBuilder().
						WithUtxos([]common.Utxo{referenceUtxo}).
						Build(),
					dijkstraGuardTestPParams(),
					[]common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts},
				)
				if withRedeemer {
					require.NoError(t, err)
					return
				}
				var missing conway.MissingRedeemerForScriptError
				require.ErrorAs(t, err, &missing)
				require.Equal(t, v4.Hash(), missing.ScriptHash)
				require.Equal(t, redeemerKey.Tag, missing.Tag)
				require.Equal(t, redeemerKey.Index, missing.Index)
			})
		}
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
	tests := []struct {
		name        string
		version     [3]uint32
		unsupported bool
	}{
		{
			name:        "PlutusV1",
			version:     lang.LanguageVersionV1,
			unsupported: true,
		},
		{
			name:        "PlutusV2",
			version:     lang.LanguageVersionV2,
			unsupported: true,
		},
		{
			name:        "PlutusV3",
			version:     lang.LanguageVersionV3,
			unsupported: true,
		},
		{name: "PlutusV4", version: lang.LanguageVersionV4},
	}
	for _, test := range tests {
		for _, txIsValid := range []bool{true, false} {
			validity := "valid"
			if !txIsValid {
				validity = "phase-2 invalid"
			}
			t.Run(test.name+"/"+validity, func(t *testing.T) {
				candidate := dijkstraGuardTestPlutus(t, test.version, false)
				tx, ls := dijkstraSubtransactionPlutusGuardTx(
					t,
					candidate,
					false,
				)
				tx.TxIsValid = txIsValid
				require.Nil(t, tx.CurrentTreasuryValue())
				err := common.VerifyTransaction(
					tx,
					0,
					ls,
					dijkstraGuardTestPParams(),
					[]common.UtxoValidationRuleFunc{UtxoValidatePlutusScripts},
				)
				if !test.unsupported {
					require.NoError(t, err)
					return
				}
				var unsupported UnsupportedScriptInSubtransactionError
				require.ErrorAs(t, err, &unsupported)
				version, ok := common.PlutusScriptVersion(candidate)
				require.True(t, ok)
				require.Equal(t, version, unsupported.Version)
				require.Zero(t, unsupported.SubtransactionIndex)
				require.Equal(
					t,
					tx.Body.TxSubTransactions.Items()[0].Body.Id(),
					unsupported.TransactionId,
				)
			})
		}
	}

	for _, test := range tests {
		for _, txIsValid := range []bool{true, false} {
			validity := "valid"
			if !txIsValid {
				validity = "phase-2 invalid"
			}
			t.Run("top-level/"+test.name+"/"+validity, func(t *testing.T) {
				candidate := dijkstraGuardTestPlutus(t, test.version, false)
				witnesses := testDijkstraWitnessSet(t, candidate)
				witnesses.WsRedeemers = DijkstraRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagGuarding}: {
							ExUnits: common.ExUnits{
								Steps:  10_000_000,
								Memory: 10_000_000,
							},
						},
					},
				}
				tx := &DijkstraTransaction{
					Body: DijkstraTransactionBody{
						TxGuards: &DijkstraGuards{
							Credentials: []common.Credential{
								dijkstraGuardCredentialForScript(candidate),
							},
						},
					},
					WitnessSet: witnesses,
					TxIsValid:  txIsValid,
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

func TestVerifyTransactionChecksSubtransactionWitnesses(t *testing.T) {
	script := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	guard := dijkstraGuardCredentialForScript(script)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{
					Body: DijkstraSubTransactionBody{
						TxGuards: &DijkstraGuards{
							Credentials: []common.Credential{guard},
						},
					},
					WitnessSet: DijkstraTransactionWitnessSet{
						WsRedeemers: DijkstraRedeemers{
							Redeemers: map[common.RedeemerKey]common.RedeemerValue{
								{Tag: common.RedeemerTagGuarding}: {},
							},
						},
					},
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	err := common.VerifyTransaction(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
		[]common.UtxoValidationRuleFunc{UtxoValidateRedeemerAndScriptWitnesses},
	)
	require.ErrorAs(t, err, &common.MissingPlutusScriptWitnessesError{})
}

func TestUtxoValidateExtraneousRedeemersPerTransactionLevel(t *testing.T) {
	inputs := []shelley.ShelleyTransactionInput{
		shelley.NewShelleyTransactionInput(
			"0000000000000000000000000000000000000000000000000000000000000001",
			0,
		),
		shelley.NewShelleyTransactionInput(
			"0000000000000000000000000000000000000000000000000000000000000002",
			0,
		),
	}
	inputSet := func(count int) conway.ConwayTransactionInputSet {
		return conway.NewConwayTransactionInputSet(inputs[:count])
	}
	witnesses := func(
		key *common.RedeemerKey,
	) DijkstraTransactionWitnessSet {
		if key == nil {
			return DijkstraTransactionWitnessSet{}
		}
		return DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					*key: {},
				},
			},
		}
	}
	unknown := common.RedeemerKey{Tag: common.RedeemerTag(99), Index: 7}
	spendZero := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 0}
	spendOne := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 1}
	tests := []struct {
		name      string
		topInputs int
		subInputs int
		topKey    *common.RedeemerKey
		subKey    *common.RedeemerKey
		expected  *common.RedeemerKey
	}{
		{
			name:      "subtransaction unknown tag",
			topInputs: 2,
			subKey:    &unknown,
			expected:  &unknown,
		},
		{
			name:      "subtransaction out-of-range index",
			topInputs: 2,
			subInputs: 1,
			subKey:    &spendOne,
			expected:  &spendOne,
		},
		{
			name:      "subtransaction in-range index",
			subInputs: 1,
			subKey:    &spendZero,
		},
		{
			name:     "top-level unknown tag",
			topKey:   &unknown,
			expected: &unknown,
		},
		{
			name:      "top-level out-of-range index",
			topInputs: 1,
			subInputs: 2,
			topKey:    &spendOne,
			expected:  &spendOne,
		},
		{
			name:      "top-level in-range index",
			topInputs: 1,
			topKey:    &spendZero,
		},
	}
	for _, test := range tests {
		for _, txIsValid := range []bool{true, false} {
			validity := "valid"
			if !txIsValid {
				validity = "phase-2 invalid"
			}
			t.Run(test.name+"/"+validity, func(t *testing.T) {
				tx := &DijkstraTransaction{
					Body: DijkstraTransactionBody{
						TxInputs: inputSet(test.topInputs),
						TxSubTransactions: cbor.NewSetType(
							[]DijkstraSubTransaction{{
								Body: DijkstraSubTransactionBody{
									TxInputs: inputSet(test.subInputs),
								},
								WitnessSet: witnesses(test.subKey),
							}},
							true,
						),
					},
					WitnessSet: witnesses(test.topKey),
					TxIsValid:  txIsValid,
				}
				err := UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
				if test.expected == nil {
					require.NoError(t, err)
					return
				}
				var extra conway.ExtraRedeemerError
				require.ErrorAs(t, err, &extra)
				require.Equal(t, *test.expected, extra.RedeemerKey)
			})
		}
	}
}

func TestVerifyTransactionChecksSubtransactionSupplementalDatums(t *testing.T) {
	datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{
					WitnessSet: DijkstraTransactionWitnessSet{
						WsPlutusData: cbor.NewSetType(
							[]common.Datum{datum},
							true,
						),
					},
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	var validator common.UtxoValidationRuleFunc
	for _, descriptor := range utxoValidationRuleDescriptors {
		if descriptor.Id == common.UtxoValidationRuleSupplementalDatums {
			validator = descriptor.Validator
			break
		}
	}
	require.NotNil(t, validator)
	err := common.VerifyTransaction(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
		[]common.UtxoValidationRuleFunc{validator},
	)
	require.ErrorAs(t, err, &conway.NotAllowedSupplementalDatumsError{})
}

func TestVerifyTransactionChecksSubtransactionScriptIntegrity(t *testing.T) {
	redeemers := DijkstraRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagMint}: {},
		},
	}
	redeemersCbor, err := cbor.Encode(redeemers.Redeemers)
	require.NoError(t, err)
	redeemers.SetCbor(redeemersCbor)
	declared := common.Blake2b256{}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{
					Body: DijkstraSubTransactionBody{
						TxScriptDataHash: &declared,
					},
					WitnessSet: DijkstraTransactionWitnessSet{
						WsRedeemers: redeemers,
					},
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	err = common.VerifyTransaction(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
		[]common.UtxoValidationRuleFunc{UtxoValidateScriptDataHash},
	)
	require.ErrorAs(t, err, &common.ScriptDataHashMismatchError{})
}

func TestVerifyTransactionRequiresCollateralForSubtransactionPlutus(
	t *testing.T,
) {
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{
					WitnessSet: DijkstraTransactionWitnessSet{
						WsRedeemers: DijkstraRedeemers{
							Redeemers: map[common.RedeemerKey]common.RedeemerValue{
								{Tag: common.RedeemerTagMint}: {},
							},
						},
					},
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	err := common.VerifyTransaction(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
		[]common.UtxoValidationRuleFunc{UtxoValidateNoCollateralInputs},
	)
	require.ErrorAs(t, err, &alonzo.NoCollateralInputsError{})
}

func TestUtxoValidateCostModelsPresentUsesNeededScripts(t *testing.T) {
	v4 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	v1 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV1, false)
	witnesses := testDijkstraWitnessSet(t, v4)
	witnesses.WsPlutusV1Scripts = cbor.NewSetType(
		[]common.PlutusV1Script{v1.(common.PlutusV1Script)},
		true,
	)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{
					Body: DijkstraSubTransactionBody{
						TxGuards: &DijkstraGuards{
							Credentials: []common.Credential{
								dijkstraGuardCredentialForScript(v4),
							},
						},
					},
					WitnessSet: witnesses,
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	pp := dijkstraGuardTestPParams()
	delete(pp.CostModels, 0)
	require.NoError(t, UtxoValidateCostModelsPresent(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		pp,
	))
	delete(pp.CostModels, 3)
	err := UtxoValidateCostModelsPresent(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		pp,
	)
	var missing common.MissingCostModelError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, uint(3), missing.Version)
}

func TestVerifyTransactionAcceptsNonGuardPlutusV4(t *testing.T) {
	v4 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	mint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			v4.Hash(): {cbor.NewByteString(nil): big.NewInt(1)},
		},
	)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{TxMint: &mint},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusV4Scripts: cbor.NewSetType(
				[]common.PlutusV4Script{v4.(common.PlutusV4Script)},
				true,
			),
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagMint}: {
						ExUnits: common.ExUnits{
							Steps:  10_000_000,
							Memory: 10_000_000,
						},
					},
				},
			},
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
}
