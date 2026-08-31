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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(test.treasury),
				)
				subTx.WitnessSet = testDijkstraWitnessSet(t, test.script)
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
					mockledger.NewLedgerStateBuilder().Build(),
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			if test.topLevel {
				tx.Body = decodeDijkstraTreasuryBody(t, test.treasury)
				tx.WitnessSet = testDijkstraWitnessSet(t, test.script)
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(t, test.treasury)
				subTx.WitnessSet = testDijkstraWitnessSet(t, test.script)
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
					mockledger.NewLedgerStateBuilder().Build(),
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
	case common.PlutusV1Script:
		scriptType = common.ScriptRefTypePlutusV1
	case common.PlutusV2Script:
		scriptType = common.ScriptRefTypePlutusV2
	case common.PlutusV3Script:
		scriptType = common.ScriptRefTypePlutusV3
	}
	return &common.ScriptRef{Type: scriptType, Script: script}
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
			input := shelley.NewShelleyTransactionInput(
				"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				idx,
			)
			state := mockledger.NewLedgerStateBuilder().WithUtxos(
				[]common.Utxo{{
					Id: input,
					Output: babbage.BabbageTransactionOutput{
						OutputAmount: mary.MaryTransactionOutputValue{
							Amount: 1,
						},
						TxOutScriptRef: dijkstraScriptRef(test.script),
					},
				}},
			).Build()
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, nil),
				TxIsValid: true,
			}
			if test.topLevel {
				tx.Body = decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(42),
				)
				if test.referenceInput {
					tx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{input},
						true,
					)
				} else {
					tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
						[]shelley.ShelleyTransactionInput{input},
					)
				}
			} else {
				subTx := decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(0),
				)
				if test.referenceInput {
					subTx.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{input},
						true,
					)
				} else {
					subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
						[]shelley.ShelleyTransactionInput{input},
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

func TestDijkstraConwayFeaturesUsesPerLevelScope(t *testing.T) {
	t.Run(
		"top-level feature does not pair with subtransaction witness",
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
		"subtransaction feature does not pair with top-level witness",
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
			first := decodeDijkstraTreasurySubTransaction(
				t,
				dijkstraTreasuryValue(42),
			)
			first.WitnessSet = testDijkstraWitnessSet(
				t,
				common.PlutusV2Script{0x01},
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
					mockledger.NewLedgerStateBuilder().Build(),
				),
				"PlutusV2",
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
			subTx := decodeDijkstraTreasurySubTransaction(t, nil)
			test.configure(&subTx.Body)
			subTx.WitnessSet = testDijkstraWitnessSet(
				t,
				common.PlutusV1Script{0x01},
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
					mockledger.NewLedgerStateBuilder().Build(),
				),
			)
		})
	}
}

func TestDijkstraConwayFeaturesPhase2ValidProductionRules(t *testing.T) {
	subTx := decodeDijkstraTreasurySubTransaction(
		t,
		dijkstraTreasuryValue(42),
	)
	subTx.WitnessSet = testDijkstraWitnessSet(
		t,
		common.PlutusV1Script{0x01},
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
