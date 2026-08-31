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
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func dijkstraConwayFeaturesRule(
	t *testing.T,
) common.UtxoValidationRuleFunc {
	t.Helper()
	var rule common.UtxoValidationRuleFunc
	for _, descriptor := range UtxoValidationRuleDescriptors() {
		if descriptor.Id != common.UtxoValidationRuleConwayFeaturesWithPlutusV1V2 {
			continue
		}
		require.Nil(t, rule, "duplicate Conway-feature descriptor")
		rule = descriptor.Validator
	}
	require.NotNil(t, rule, "missing Conway-feature descriptor")
	return rule
}

func verifyDijkstraConwayFeatures(
	t *testing.T,
	tx *DijkstraTransaction,
	ls common.LedgerState,
) error {
	t.Helper()
	require.True(
		t,
		tx.IsValid(),
		"compatibility rule applies to a phase-2-valid transaction",
	)
	return common.VerifyTransaction(
		tx,
		0,
		ls,
		&DijkstraProtocolParameters{},
		[]common.UtxoValidationRuleFunc{dijkstraConwayFeaturesRule(t)},
	)
}

func requireDijkstraCurrentTreasuryPlutusError(
	t *testing.T,
	err error,
	wantVersion string,
) {
	t.Helper()
	var target conway.CurrentTreasuryValueWithPlutusV1V2Error
	require.ErrorAs(t, err, &target)
	require.Equal(t, wantVersion, target.PlutusVersion)
}

func TestDijkstraConwayFeaturesCurrentTreasuryWitnessScripts(
	t *testing.T,
) {
	tests := []struct {
		name     string
		treasury uint64
		script   common.Script
		version  string
		topLevel bool
	}{
		{
			name:     "top-level explicit zero with PlutusV1",
			script:   common.PlutusV1Script{0x01},
			version:  "PlutusV1",
			topLevel: true,
		},
		{
			name:     "top-level nonzero with PlutusV2",
			treasury: 42,
			script:   common.PlutusV2Script{0x01},
			version:  "PlutusV2",
			topLevel: true,
		},
		{
			name:    "subtransaction explicit zero with PlutusV1",
			script:  common.PlutusV1Script{0x01},
			version: "PlutusV1",
		},
		{
			name:     "subtransaction nonzero with PlutusV2",
			treasury: 42,
			script:   common.PlutusV2Script{0x01},
			version:  "PlutusV2",
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, utxo := dijkstraScriptLockedInput(t, test.script, idx)
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			if test.topLevel {
				tx.Body = decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(test.treasury),
				)
				tx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(test.treasury),
				)
				subTx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
				tx.Body.TxSubTransactions = cbor.NewSetType(
					[]DijkstraSubTransaction{subTx},
					true,
				)
			}

			requireDijkstraCurrentTreasuryPlutusError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{utxo},
					).Build(),
				),
				test.version,
			)
		})
	}
}

func TestDijkstraConwayFeaturesCurrentTreasuryPermittedCases(
	t *testing.T,
) {
	tests := []struct {
		name     string
		treasury *uint64
		script   common.Script
		topLevel bool
	}{
		{
			name:     "top-level absent with PlutusV1",
			script:   common.PlutusV1Script{0x01},
			topLevel: true,
		},
		{
			name:   "subtransaction absent with PlutusV2",
			script: common.PlutusV2Script{0x01},
		},
		{
			name:     "top-level explicit zero with PlutusV3",
			treasury: dijkstraTreasuryValue(0),
			script:   common.PlutusV3Script{0x01},
			topLevel: true,
		},
		{
			name:     "subtransaction nonzero with PlutusV3",
			treasury: dijkstraTreasuryValue(42),
			script:   common.PlutusV3Script{0x01},
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, utxo := dijkstraScriptLockedInput(t, test.script, idx+10)
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			if test.topLevel {
				tx.Body = decodeDijkstraTreasuryBody(t, test.treasury)
				tx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(t, test.treasury)
				subTx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
				tx.Body.TxSubTransactions = cbor.NewSetType(
					[]DijkstraSubTransaction{subTx},
					true,
				)
			}
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{utxo},
					).Build(),
				),
			)
		})
	}
}

func dijkstraScriptRef(
	script common.Script,
) *common.ScriptRef {
	var scriptType uint
	switch script.(type) {
	case common.NativeScript:
		scriptType = common.ScriptRefTypeNativeScript
	case common.PlutusV1Script:
		scriptType = common.ScriptRefTypePlutusV1
	case common.PlutusV2Script:
		scriptType = common.ScriptRefTypePlutusV2
	case common.PlutusV3Script:
		scriptType = common.ScriptRefTypePlutusV3
	case common.PlutusV4Script:
		scriptType = common.ScriptRefTypePlutusV4
	}
	return &common.ScriptRef{Type: scriptType, Script: script}
}

func dijkstraScriptLockedInput(
	t *testing.T,
	script common.Script,
	index int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		index,
	)
	address, err := common.NewAddressFromParts(
		common.AddressTypeScriptKey,
		common.AddressNetworkTestnet,
		script.Hash().Bytes(),
		bytes.Repeat([]byte{0x55}, common.AddressHashSize),
	)
	require.NoError(t, err)
	return input, common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAddress: address,
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: 2_000_000,
			},
		},
	}
}

func dijkstraReferenceScriptInput(
	script common.Script,
	index int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	input := shelley.NewShelleyTransactionInput(
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		index,
	)
	return input, common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: 2_000_000,
			},
			TxOutScriptRef: dijkstraScriptRef(script),
		},
	}
}

func TestDijkstraConwayFeaturesUsesSharedRequiredScripts(t *testing.T) {
	tests := []struct {
		name              string
		featureTopLevel   bool
		treasury          uint64
		script            common.Script
		providedByWitness bool
		wantVersion       string
	}{
		{
			name:              "top-level explicit zero supplied by subtransaction witness",
			featureTopLevel:   true,
			script:            common.PlutusV1Script{0x01},
			providedByWitness: true,
			wantVersion:       "PlutusV1",
		},
		{
			name:            "top-level nonzero supplied by subtransaction reference input",
			featureTopLevel: true,
			treasury:        42,
			script:          common.PlutusV2Script{0x02},
			wantVersion:     "PlutusV2",
		},
		{
			name:              "subtransaction explicit zero supplied by top-level witness",
			script:            common.PlutusV1Script{0x03},
			providedByWitness: true,
			wantVersion:       "PlutusV1",
		},
		{
			name:        "subtransaction nonzero supplied by top-level reference input",
			treasury:    42,
			script:      common.PlutusV2Script{0x04},
			wantVersion: "PlutusV2",
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spendInput, spendUtxo := dijkstraScriptLockedInput(
				t,
				test.script,
				idx,
			)
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			subTx := decodeDijkstraTreasurySubTransaction(t, nil)
			if test.featureTopLevel {
				tx.Body = decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(test.treasury),
				)
				tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{spendInput},
				)
			} else {
				subTx = decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(test.treasury),
				)
				subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{spendInput},
				)
			}

			utxos := []common.Utxo{spendUtxo}
			if test.providedByWitness {
				if test.featureTopLevel {
					subTx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				} else {
					tx.WitnessSet = testDijkstraWitnessSet(t, test.script)
				}
			} else {
				refInput, refUtxo := dijkstraReferenceScriptInput(
					test.script,
					idx,
				)
				utxos = append(utxos, refUtxo)
				if test.featureTopLevel {
					subTx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{refInput},
						true,
					)
				} else {
					tx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{refInput},
						true,
					)
				}
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			)

			err := verifyDijkstraConwayFeatures(
				t,
				tx,
				mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
			)
			requireDijkstraCurrentTreasuryPlutusError(
				t,
				err,
				test.wantVersion,
			)
		})
	}
}

func TestDijkstraConwayFeaturesSharedRequiredPlutusV3Permitted(t *testing.T) {
	for _, featureTopLevel := range []bool{true, false} {
		name := "subtransaction feature"
		if featureTopLevel {
			name = "top-level feature"
		}
		t.Run(name, func(t *testing.T) {
			script := common.PlutusV3Script{0x01}
			spendInput, spendUtxo := dijkstraScriptLockedInput(t, script, 50)
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			subTx := decodeDijkstraTreasurySubTransaction(t, nil)
			if featureTopLevel {
				tx.Body = decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				)
				tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{spendInput},
				)
				subTx.WitnessSet = testDijkstraWitnessSet(t, script)
			} else {
				subTx = decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(42),
				)
				subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{spendInput},
				)
				tx.WitnessSet = testDijkstraWitnessSet(t, script)
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			)
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{spendUtxo},
					).Build(),
				),
			)
		})
	}
}

func TestDijkstraConwayFeaturesCurrentTreasuryReferenceScripts(
	t *testing.T,
) {
	tests := []struct {
		name           string
		script         common.Script
		version        string
		topLevel       bool
		referenceInput bool
	}{
		{
			name:           "top-level reference input PlutusV1",
			script:         common.PlutusV1Script{0x01},
			version:        "PlutusV1",
			topLevel:       true,
			referenceInput: true,
		},
		{
			name:     "top-level regular input PlutusV2",
			script:   common.PlutusV2Script{0x01},
			version:  "PlutusV2",
			topLevel: true,
		},
		{
			name:           "subtransaction reference input PlutusV2",
			script:         common.PlutusV2Script{0x01},
			version:        "PlutusV2",
			referenceInput: true,
		},
		{
			name:    "subtransaction regular input PlutusV1",
			script:  common.PlutusV1Script{0x01},
			version: "PlutusV1",
		},
		{
			name:           "subtransaction reference input PlutusV3",
			script:         common.PlutusV3Script{0x01},
			referenceInput: true,
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, inputUtxo := dijkstraScriptLockedInput(
				t,
				test.script,
				idx+20,
			)
			utxos := []common.Utxo{inputUtxo}
			var referenceInput shelley.ShelleyTransactionInput
			if test.referenceInput {
				var referenceUtxo common.Utxo
				referenceInput, referenceUtxo = dijkstraReferenceScriptInput(
					test.script,
					idx+20,
				)
				utxos = append(utxos, referenceUtxo)
			} else {
				output := inputUtxo.Output.(babbage.BabbageTransactionOutput)
				output.TxOutScriptRef = dijkstraScriptRef(test.script)
				utxos[0].Output = output
			}
			state := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			if test.topLevel {
				tx.Body = decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				)
				tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
				if test.referenceInput {
					tx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{referenceInput},
						true,
					)
				}
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(0),
				)
				subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				)
				if test.referenceInput {
					subTx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{referenceInput},
						true,
					)
				}
				tx.Body.TxSubTransactions = cbor.NewSetType(
					[]DijkstraSubTransaction{subTx},
					true,
				)
			}

			err := verifyDijkstraConwayFeatures(t, tx, state)
			if test.version == "" {
				require.NoError(t, err)
			} else {
				requireDijkstraCurrentTreasuryPlutusError(
					t,
					err,
					test.version,
				)
			}
		})
	}
}

func TestDijkstraConwayFeaturesIgnoresUnneededCrossLevelScripts(t *testing.T) {
	t.Run(
		"top-level feature ignores unneeded subtransaction witness",
		func(t *testing.T) {
			tx := &DijkstraTransaction{
				Body: decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				),
				TxIsValid: true,
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{{
					WitnessSet: testDijkstraWitnessSet(
						t,
						common.PlutusV1Script{0x01},
					),
				}},
				true,
			)
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().Build(),
				),
			)
		},
	)

	t.Run(
		"subtransaction feature ignores unneeded top-level witness",
		func(t *testing.T) {
			tx := &DijkstraTransaction{
				Body: decodeDijkstraTreasuryBody(t, nil),
				WitnessSet: testDijkstraWitnessSet(
					t,
					common.PlutusV1Script{0x01},
				),
				TxIsValid: true,
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{
					decodeDijkstraTreasurySubTransaction(
						t,
						dijkstraTreasuryValue(42),
					),
				},
				true,
			)
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().Build(),
				),
			)
		},
	)

	t.Run(
		"multiple subtransactions preserve transition order",
		func(t *testing.T) {
			plutusV2 := common.PlutusV2Script{0x01}
			input, utxo := dijkstraScriptLockedInput(t, plutusV2, 40)
			first := decodeDijkstraTreasurySubTransaction(
				t,
				dijkstraTreasuryValue(42),
			)
			first.WitnessSet = testDijkstraWitnessSet(
				t,
				plutusV2,
			)
			first.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			)
			second := decodeDijkstraTreasurySubTransaction(t, nil)
			second.WitnessSet = testDijkstraWitnessSet(
				t,
				common.PlutusV1Script{0x01},
			)
			tx := &DijkstraTransaction{
				Body: decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				),
				WitnessSet: testDijkstraWitnessSet(
					t,
					common.PlutusV1Script{0x01},
				),
				TxIsValid: true,
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{first, second},
				true,
			)
			requireDijkstraCurrentTreasuryPlutusError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{utxo},
					).Build(),
				),
				"PlutusV2",
			)
		},
	)
}

func dijkstraGuardCredentialForScript(
	script common.Script,
) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(script.Hash()),
	}
}

func dijkstraGuardedTreasuryTransaction(
	t *testing.T,
	script common.Script,
	treasury *uint64,
	guardTopLevel bool,
	providerTopLevel bool,
	referenceProvider bool,
	index int,
) (*DijkstraTransaction, []common.Utxo) {
	t.Helper()
	tx := &DijkstraTransaction{
		Body:      decodeDijkstraTreasuryBody(t, nil),
		TxIsValid: true,
	}
	subTx := decodeDijkstraTreasurySubTransaction(t, nil)
	guard := dijkstraGuardCredentialForScript(script)
	if guardTopLevel {
		tx.Body = decodeDijkstraTreasuryBody(t, treasury)
		tx.Body.TxGuards = &DijkstraGuards{
			Credentials: []common.Credential{guard},
		}
	} else {
		subTx = decodeDijkstraTreasurySubTransaction(t, treasury)
		subTx.Body.TxGuards = &DijkstraGuards{
			Credentials: []common.Credential{guard},
		}
	}

	var utxos []common.Utxo
	if referenceProvider {
		input, utxo := dijkstraReferenceScriptInput(script, index)
		utxos = append(utxos, utxo)
		if providerTopLevel {
			tx.Body.TxReferenceInputs = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input},
				true,
			)
		} else {
			subTx.Body.TxReferenceInputs = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input},
				true,
			)
		}
	} else if providerTopLevel {
		tx.WitnessSet = testDijkstraWitnessSet(t, script)
	} else {
		subTx.WitnessSet = testDijkstraWitnessSet(t, script)
	}

	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{subTx},
		true,
	)
	return tx, utxos
}

func TestDijkstraConwayFeaturesCurrentTreasuryGuardingScripts(
	t *testing.T,
) {
	tests := []struct {
		name              string
		script            common.Script
		treasury          uint64
		guardTopLevel     bool
		providerTopLevel  bool
		referenceProvider bool
		wantPlutusVersion string
	}{
		{
			name:              "top-level explicit zero same-level witness PlutusV1",
			script:            common.PlutusV1Script{0x11},
			guardTopLevel:     true,
			providerTopLevel:  true,
			wantPlutusVersion: "PlutusV1",
		},
		{
			name:              "subtransaction nonzero same-level witness PlutusV2",
			script:            common.PlutusV2Script{0x12},
			treasury:          42,
			wantPlutusVersion: "PlutusV2",
		},
		{
			name:              "top-level nonzero cross-level witness PlutusV2",
			script:            common.PlutusV2Script{0x13},
			treasury:          42,
			guardTopLevel:     true,
			wantPlutusVersion: "PlutusV2",
		},
		{
			name:              "subtransaction explicit zero cross-level witness PlutusV1",
			script:            common.PlutusV1Script{0x14},
			providerTopLevel:  true,
			wantPlutusVersion: "PlutusV1",
		},
		{
			name:              "top-level nonzero same-level reference PlutusV1",
			script:            common.PlutusV1Script{0x15},
			treasury:          42,
			guardTopLevel:     true,
			providerTopLevel:  true,
			referenceProvider: true,
			wantPlutusVersion: "PlutusV1",
		},
		{
			name:              "subtransaction explicit zero same-level reference PlutusV2",
			script:            common.PlutusV2Script{0x16},
			referenceProvider: true,
			wantPlutusVersion: "PlutusV2",
		},
		{
			name:              "top-level explicit zero cross-level reference PlutusV2",
			script:            common.PlutusV2Script{0x17},
			guardTopLevel:     true,
			referenceProvider: true,
			wantPlutusVersion: "PlutusV2",
		},
		{
			name:              "subtransaction nonzero cross-level reference PlutusV1",
			script:            common.PlutusV1Script{0x18},
			treasury:          42,
			providerTopLevel:  true,
			referenceProvider: true,
			wantPlutusVersion: "PlutusV1",
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, utxos := dijkstraGuardedTreasuryTransaction(
				t,
				test.script,
				dijkstraTreasuryValue(test.treasury),
				test.guardTopLevel,
				test.providerTopLevel,
				test.referenceProvider,
				idx+100,
			)
			err := verifyDijkstraConwayFeatures(
				t,
				tx,
				mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
			)
			requireDijkstraCurrentTreasuryPlutusError(
				t,
				err,
				test.wantPlutusVersion,
			)
		})
	}
}

func TestDijkstraConwayFeaturesCurrentTreasuryGuardingControls(
	t *testing.T,
) {
	t.Run("PlutusV3 guard is permitted", func(t *testing.T) {
		tx, utxos := dijkstraGuardedTreasuryTransaction(
			t,
			common.PlutusV3Script{0x21},
			dijkstraTreasuryValue(42),
			true,
			true,
			false,
			200,
		)
		require.NoError(
			t,
			verifyDijkstraConwayFeatures(
				t,
				tx,
				mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
			),
		)
	})

	t.Run(
		"unrelated PlutusV1 witness with key guard is permitted",
		func(t *testing.T) {
			tx := &DijkstraTransaction{
				Body: decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				),
				WitnessSet: testDijkstraWitnessSet(
					t,
					common.PlutusV1Script{0x22},
				),
				TxIsValid: true,
			}
			tx.Body.TxGuards = &DijkstraGuards{
				KeyHashes: []common.Blake2b224{{0x01}},
			}
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().Build(),
				),
			)
		},
	)

	t.Run("native script guard is permitted", func(t *testing.T) {
		native := testRequireGuardNativeScript(t, testGuardCredential())
		tx, utxos := dijkstraGuardedTreasuryTransaction(
			t,
			native,
			dijkstraTreasuryValue(0),
			true,
			true,
			false,
			201,
		)
		require.NoError(
			t,
			verifyDijkstraConwayFeatures(
				t,
				tx,
				mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
			),
		)
	})

	t.Run(
		"PlutusV2 guard without current treasury is permitted",
		func(t *testing.T) {
			tx, utxos := dijkstraGuardedTreasuryTransaction(
				t,
				common.PlutusV2Script{0x23},
				nil,
				false,
				true,
				false,
				202,
			)
			require.NoError(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build(),
				),
			)
		},
	)
}

func TestDijkstraConwayFeaturesChecksAllSubtransactionFeatures(
	t *testing.T,
) {
	voter := common.Voter{}
	actionId := common.GovActionId{}
	tests := []struct {
		name      string
		configure func(*DijkstraSubTransactionBody)
		check     func(*testing.T, error)
	}{
		{
			name: "proposal procedures",
			configure: func(body *DijkstraSubTransactionBody) {
				body.TxProposalProcedures = []DijkstraProposalProcedure{{}}
			},
			check: func(t *testing.T, err error) {
				var target conway.ProposalProceduresWithPlutusV1V2Error
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name: "voting procedures",
			configure: func(body *DijkstraSubTransactionBody) {
				body.TxVotingProcedures = common.VotingProcedures{
					&voter: {&actionId: {}},
				}
			},
			check: func(t *testing.T, err error) {
				var target conway.VotingProceduresWithPlutusV1V2Error
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name: "Conway certificates",
			configure: func(body *DijkstraSubTransactionBody) {
				body.TxCertificates = []common.CertificateWrapper{{
					Certificate: &common.RegistrationDrepCertificate{},
				}}
			},
			check: func(t *testing.T, err error) {
				var target conway.ConwayCertificateWithPlutusV1V2Error
				require.ErrorAs(t, err, &target)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plutusV1 := common.PlutusV1Script{0x01}
			input, utxo := dijkstraScriptLockedInput(t, plutusV1, 41)
			subTx := decodeDijkstraTreasurySubTransaction(t, nil)
			test.configure(&subTx.Body)
			subTx.WitnessSet = testDijkstraWitnessSet(
				t,
				plutusV1,
			)
			subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			)
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			)
			test.check(
				t,
				verifyDijkstraConwayFeatures(
					t,
					tx,
					mockledger.NewLedgerStateBuilder().WithUtxos(
						[]common.Utxo{utxo},
					).Build(),
				),
			)
		})
	}
}

func TestDijkstraConwayFeaturesPhase2ValidProductionRules(t *testing.T) {
	plutusV1 := common.PlutusV1Script{0x01}
	input, utxo := dijkstraScriptLockedInput(t, plutusV1, 42)
	subTx := decodeDijkstraTreasurySubTransaction(
		t,
		dijkstraTreasuryValue(42),
	)
	subTx.WitnessSet = testDijkstraWitnessSet(
		t,
		plutusV1,
	)
	subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)
	tx := &DijkstraTransaction{
		Body:      decodeDijkstraTreasuryBody(t, nil),
		TxIsValid: true,
	}
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{subTx},
		true,
	)
	state := mockledger.NewLedgerStateBuilder().
		WithTreasuryAmount(42).
		WithUtxos([]common.Utxo{utxo}).
		Build()
	err := common.VerifyTransaction(
		tx,
		0,
		state,
		&DijkstraProtocolParameters{
			ConwayProtocolParameters: conway.ConwayProtocolParameters{
				CostModels: map[uint][]int64{0: {1}},
			},
		},
		UtxoValidationRules,
	)
	requireDijkstraCurrentTreasuryPlutusError(t, err, "PlutusV1")
	var validationErr *common.ValidationError
	require.True(t, errors.As(err, &validationErr))
	require.Equal(t, 20, validationErr.Details["rule_index"])
}
