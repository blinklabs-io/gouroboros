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

// TestUtxoValidateRequiredRedeemersRegistered pins that Dijkstra's rule list
// reuses conway.UtxoValidateRequiredRedeemers directly, rather than each era
// re-deriving the reference-script-implies-redeemer check on its own (issue
// #2147's "avoid duplicating era-specific guards" requirement).
func TestUtxoValidateRequiredRedeemersRegistered(t *testing.T) {
	_, requiredRedeemersIdx := dijkstraValidationRule(
		t,
		"ledger/conway.UtxoValidateRequiredRedeemers",
	)
	_, badInputsIdx := dijkstraValidationRule(
		t,
		"ledger/conway.UtxoValidateBadInputsUtxo",
	)
	// Dijkstra registers its own ScriptWitnesses rule, which extends Conway's
	// to sub-transaction witness sets; the ordering requirement is the same.
	_, scriptWitnessesIdx := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateScriptWitnesses",
	)

	require.Greater(
		t,
		requiredRedeemersIdx,
		badInputsIdx,
		"UtxoValidateRequiredRedeemers must run after UtxoValidateBadInputsUtxo "+
			"so an unresolvable input surfaces as BadInputsUtxo, not a raw "+
			"input-resolution error",
	)
	require.Greater(
		t,
		requiredRedeemersIdx,
		scriptWitnessesIdx,
		"UtxoValidateRequiredRedeemers must run after UtxoValidateScriptWitnesses "+
			"so a missing script is reported by the latter, not masked as a "+
			"missing redeemer",
	)
}

// TestUtxoValidateRequiredRedeemersDijkstra exercises issue #2147's scenario
// through the Dijkstra entry point: a script-address input satisfied by a
// CIP-33 reference script, spent with no redeemer at all. Conway and
// Dijkstra must behave consistently here since they share one function.
func TestUtxoValidateRequiredRedeemersDijkstra(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		v1.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)

	input := shelley.NewShelleyTransactionInput(
		"6666666666666666666666666666666666666666666666666666666666666666",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			if id.String() == input.String() {
				return utxo, nil
			}
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	newTx := func() *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				),
			},
			TxIsValid: true,
		}
	}

	// No redeemer at all: the reference script satisfies script-presence
	// checks, but the spend is left completely unexecuted -- exactly the
	// gap issue #2147 describes.
	missingTx := newTx()
	err = conway.UtxoValidateRequiredRedeemers(
		missingTx,
		0,
		ls,
		&DijkstraProtocolParameters{},
	)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v1.Hash(), missingErr.ScriptHash)
	require.Equal(t, uint32(0), missingErr.Index)

	// With the matching spend redeemer present, the same input passes.
	validTx := newTx()
	validTx.WitnessSet = DijkstraTransactionWitnessSet{
		WsRedeemers: DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		},
	}
	require.NoError(t, conway.UtxoValidateRequiredRedeemers(
		validTx,
		0,
		ls,
		&DijkstraProtocolParameters{},
	))
}

// TestUtxoValidateRequiredRedeemersSubTransaction covers issue #2250's second
// half: a Plutus script-address input inside a sub-transaction. The
// sub-transaction's witness set puts the script into the aggregated
// ScriptsProvided, but the spend walk only ever visited the top-level
// transaction's inputs, so this purpose was never checked.
//
// cardano-ledger applies hasExactSetOfRedeemers once per transaction level --
// DijkstraUTXOW over the top-level scriptsNeeded
// (eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxow.hs) and
// DijkstraSUBUTXOW over each sub-transaction's own
// (.../Rules/SubUtxow.hs) -- because a redeemer pointer indexes its own
// level's inputs. A redeemer on the top-level transaction therefore does not
// satisfy a sub-transaction's purpose, which the third subtest pins.
func TestUtxoValidateRequiredRedeemersSubTransaction(t *testing.T) {
	v3 := common.PlutusV3Script{0x0a, 0x0b, 0x0c}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		v3.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	input := shelley.NewShelleyTransactionInput(
		"7777777777777777777777777777777777777777777777777777777777777777",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			if id.String() == input.String() {
				return utxo, nil
			}
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	newTx := func(
		subRedeemers, topRedeemers DijkstraRedeemers,
	) *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxSubTransactions: cbor.NewSetType(
					[]DijkstraSubTransaction{
						{
							Body: DijkstraSubTransactionBody{
								TxInputs: conway.NewConwayTransactionInputSet(
									[]shelley.ShelleyTransactionInput{input},
								),
							},
							WitnessSet: DijkstraTransactionWitnessSet{
								WsPlutusV3Scripts: cbor.NewSetType(
									[]common.PlutusV3Script{v3},
									true,
								),
								WsRedeemers: subRedeemers,
							},
						},
					},
					false,
				),
			},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsRedeemers: topRedeemers,
			},
			TxIsValid: true,
		}
	}
	spendRedeemer := DijkstraRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: 0}: {
				ExUnits: common.ExUnits{Steps: 1, Memory: 1},
			},
		},
	}
	pp := &DijkstraProtocolParameters{}

	t.Run("missing sub-transaction redeemer rejected", func(t *testing.T) {
		err := conway.UtxoValidateRequiredRedeemers(
			newTx(DijkstraRedeemers{}, DijkstraRedeemers{}),
			0,
			ls,
			pp,
		)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, v3.Hash(), missingErr.ScriptHash)
		require.Equal(t, common.RedeemerTagSpend, missingErr.Tag)
		require.Equal(t, uint32(0), missingErr.Index)
	})

	t.Run("sub-transaction redeemer accepted", func(t *testing.T) {
		require.NoError(t, conway.UtxoValidateRequiredRedeemers(
			newTx(spendRedeemer, DijkstraRedeemers{}),
			0,
			ls,
			pp,
		))
	})

	t.Run("top-level redeemer does not satisfy sub-transaction", func(t *testing.T) {
		err := conway.UtxoValidateRequiredRedeemers(
			newTx(DijkstraRedeemers{}, spendRedeemer),
			0,
			ls,
			pp,
		)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, v3.Hash(), missingErr.ScriptHash)
	})
}

// TestUtxoValidateRequiredRedeemersGuardingPurpose pins that a Dijkstra
// guarding purpose requires a redeemer.
//
// getDijkstraScriptsNeeded is getConwayScriptsNeeded plus guardingScriptsNeeded
// (eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/UTxO.hs), and
// hasExactSetOfRedeemers derives its pointers from the whole of scriptsNeeded,
// so a guard whose credential is a script hash and whose script is Plutus is
// required exactly like any other purpose. The guard's index is its position
// in the guards list, from zipAsIxItem over guardsTxBodyL.
func TestUtxoValidateRequiredRedeemersGuardingPurpose(t *testing.T) {
	v3 := common.PlutusV3Script{0x0d, 0x0e, 0x0f}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	newTx := func(redeemers DijkstraRedeemers) *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxGuards: &DijkstraGuards{
					Credentials: []common.Credential{
						{
							CredType:   common.CredentialTypeScriptHash,
							Credential: common.Blake2b224(v3.Hash()),
						},
					},
				},
			},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV3Scripts: cbor.NewSetType(
					[]common.PlutusV3Script{v3},
					true,
				),
				WsRedeemers: redeemers,
			},
			TxIsValid: true,
		}
	}
	pp := &DijkstraProtocolParameters{}

	err := conway.UtxoValidateRequiredRedeemers(
		newTx(DijkstraRedeemers{}),
		0,
		ls,
		pp,
	)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v3.Hash(), missingErr.ScriptHash)
	require.Equal(t, common.RedeemerTagGuarding, missingErr.Tag)
	require.Equal(t, uint32(0), missingErr.Index)

	require.NoError(t, conway.UtxoValidateRequiredRedeemers(
		newTx(DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}),
		0,
		ls,
		pp,
	))
}
