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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// boEra is one era whose rule list the rule-order test walks.
type boEra struct {
	name  string
	rules []common.UtxoValidationRuleFunc
	desc  []common.UtxoValidationRuleDescriptor
	pp    common.ProtocolParameters
	refs  bool
	build func(ins, coll, refs []shelley.ShelleyTransactionInput) common.Transaction
}

func boInputSet(
	in []shelley.ShelleyTransactionInput,
) cbor.SetType[shelley.ShelleyTransactionInput] {
	return cbor.NewSetType(in, true)
}

func boSpendKey() common.RedeemerKey {
	return common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 0}
}

func boEras() []boEra {
	return []boEra{
		{
			name:  "alonzo",
			rules: alonzo.UtxoValidationRules,
			desc:  alonzo.UtxoValidationRuleDescriptors(),
			pp:    umPparams(umAlonzo),
			build: func(ins, coll, _ []shelley.ShelleyTransactionInput) common.Transaction {
				return &alonzo.AlonzoTransaction{
					Body: alonzo.AlonzoTransactionBody{
						TxInputs:     shelley.NewShelleyTransactionInputSet(ins),
						TxCollateral: boInputSet(coll),
					},
					WitnessSet: alonzo.AlonzoTransactionWitnessSet{
						VkeyWitnesses: umWitnesses(),
						WsRedeemers: alonzo.AlonzoRedeemers{
							Redeemers: []alonzo.AlonzoRedeemer{{}},
						},
					},
					TxIsValid: true,
				}
			},
		},
		{
			name:  "babbage",
			rules: babbage.UtxoValidationRules,
			desc:  babbage.UtxoValidationRuleDescriptors(),
			pp:    umPparams(umBabbage),
			refs:  true,
			build: func(ins, coll, refs []shelley.ShelleyTransactionInput) common.Transaction {
				return &babbage.BabbageTransaction{
					Body: babbage.BabbageTransactionBody{
						TxInputs:          shelley.NewShelleyTransactionInputSet(ins),
						TxCollateral:      boInputSet(coll),
						TxReferenceInputs: boInputSet(refs),
					},
					WitnessSet: babbage.BabbageTransactionWitnessSet{
						VkeyWitnesses: umWitnesses(),
						WsRedeemers: alonzo.AlonzoRedeemers{
							Redeemers: []alonzo.AlonzoRedeemer{{}},
						},
					},
					TxIsValid: true,
				}
			},
		},
		{
			name:  "conway",
			rules: conway.UtxoValidationRules,
			desc:  conway.UtxoValidationRuleDescriptors(),
			pp: &conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: common.ProtocolVersionPlomin,
				},
			},
			refs: true,
			build: func(ins, coll, refs []shelley.ShelleyTransactionInput) common.Transaction {
				return &conway.ConwayTransaction{
					Body: conway.ConwayTransactionBody{
						TxInputs:          conway.NewConwayTransactionInputSet(ins),
						TxCollateral:      boInputSet(coll),
						TxReferenceInputs: boInputSet(refs),
					},
					WitnessSet: conway.ConwayTransactionWitnessSet{
						WsRedeemers: conway.ConwayRedeemers{
							Redeemers: map[common.RedeemerKey]common.RedeemerValue{
								boSpendKey(): {},
							},
						},
					},
					TxIsValid: true,
				}
			},
		},
		{
			name:  "dijkstra",
			rules: dijkstra.UtxoValidationRules,
			desc:  dijkstra.UtxoValidationRuleDescriptors(),
			pp:    &dijkstra.DijkstraProtocolParameters{},
			refs:  true,
			build: func(ins, coll, refs []shelley.ShelleyTransactionInput) common.Transaction {
				return &dijkstra.DijkstraTransaction{
					Body: dijkstra.DijkstraTransactionBody{
						TxInputs:          conway.NewConwayTransactionInputSet(ins),
						TxCollateral:      boInputSet(coll),
						TxReferenceInputs: boInputSet(refs),
					},
					WitnessSet: dijkstra.DijkstraTransactionWitnessSet{
						WsRedeemers: dijkstra.DijkstraRedeemers{
							Redeemers: map[common.RedeemerKey]common.RedeemerValue{
								boSpendKey(): {},
							},
						},
					},
					TxIsValid: true,
				}
			},
		},
	}
}

// boValidators returns the validators of descs in order.
func boValidators(
	descs []common.UtxoValidationRuleDescriptor,
) []common.UtxoValidationRuleFunc {
	ret := make([]common.UtxoValidationRuleFunc, 0, len(descs))
	for _, d := range descs {
		ret = append(ret, d.Validator)
	}
	return ret
}

// boRequireBadInput requires err to be a BadInputsUtxoError naming only want.
func boRequireBadInput(
	t *testing.T,
	err error,
	want shelley.ShelleyTransactionInput,
) {
	t.Helper()
	var badErr shelley.BadInputsUtxoError
	require.ErrorAs(t, err, &badErr)
	require.Len(t, badErr.Inputs, 1)
	require.Equal(t, want.String(), badErr.Inputs[0].String())
}

// TestBadInputsReportsMissingInputsFirst runs each era's whole rule list,
// both UtxoValidationRules and the descriptor list, through
// VerifyTransaction on a transaction whose collateral or reference input
// is not in the UTxO. cardano-ledger accumulates every UTXO and UTXOW
// failure, so a missing input always yields BadInputsUTxO. gOuroboros
// stops at the first failing rule, and the witness, script and collateral
// rules resolve these inputs too, so the error must still be BadInputs
// rather than one of theirs.
func TestBadInputsReportsMissingInputsFirst(t *testing.T) {
	present := umInput(0)
	missing := umInput(1)
	one := func(in shelley.ShelleyTransactionInput) []shelley.ShelleyTransactionInput {
		return []shelley.ShelleyTransactionInput{in}
	}
	for _, era := range boEras() {
		ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
			Id:     present,
			Output: umEraOutput(t, umBabbage, 3, 0, false, false),
		}}).Build()
		cases := []struct {
			name string
			tx   common.Transaction
		}{
			{"missing collateral", era.build(one(present), one(missing), nil)},
		}
		if era.refs {
			cases = append(cases, struct {
				name string
				tx   common.Transaction
			}{"missing reference input", era.build(one(present), one(present), one(missing))})
		}
		for _, tc := range cases {
			t.Run(era.name+"/"+tc.name, func(t *testing.T) {
				boRequireBadInput(t, common.VerifyTransaction(
					tc.tx, 0, ls, era.pp, era.rules,
				), missing)
				boRequireBadInput(t, common.VerifyTransaction(
					tc.tx, 0, ls, era.pp, boValidators(era.desc),
				), missing)
			})
		}
	}
}

// TestDijkstraBadInputsNilLedgerState pins that the Dijkstra BadInputs rule
// reports an input it cannot resolve without a ledger state, rather than
// panicking in the Conway rule it delegates each level to.
func TestDijkstraBadInputsNilLedgerState(t *testing.T) {
	ref := umInput(1)
	tx := &dijkstra.DijkstraTransaction{
		Body: dijkstra.DijkstraTransactionBody{
			TxReferenceInputs: boInputSet([]shelley.ShelleyTransactionInput{ref}),
		},
		TxIsValid: true,
	}
	var err error
	require.NotPanics(t, func() {
		err = dijkstra.UtxoValidateBadInputsUtxo(
			tx, 0, nil, &dijkstra.DijkstraProtocolParameters{},
		)
	})
	boRequireBadInput(t, err, ref)
}
