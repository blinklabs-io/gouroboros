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
	"math/big"
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

const (
	dijkstraSubUtxoInputAmount = uint64(2_400_000)
	dijkstraSubUtxoOverAmount  = uint64(1_000_200_000)
)

func dijkstraSubUtxoInput(index int) (
	shelley.ShelleyTransactionInput,
	common.Utxo,
) {
	input := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		index,
	)
	return input, common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: dijkstraSubUtxoInputAmount,
			},
		},
	}
}

func dijkstraSubUtxoOutput(
	t *testing.T,
	amount uint64,
) DijkstraTransactionOutput {
	t.Helper()
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x44}, common.AddressHashSize),
		bytes.Repeat([]byte{0x55}, common.AddressHashSize),
	)
	require.NoError(t, err)
	return DijkstraTransactionOutput{
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: address,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: amount},
		},
	}
}

// dijkstraSubUtxoTopLevelTx spends inputs and creates outputs at the top level.
func dijkstraSubUtxoTopLevelTx(
	inputs []shelley.ShelleyTransactionInput,
	outputs []DijkstraTransactionOutput,
) *DijkstraTransaction {
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs:  conway.NewConwayTransactionInputSet(inputs),
			TxOutputs: outputs,
		},
		TxIsValid: true,
	}
}

// dijkstraSubUtxoSubTx moves the same inputs and outputs into a
// sub-transaction, leaving the top-level body empty.
func dijkstraSubUtxoSubTx(
	inputs []shelley.ShelleyTransactionInput,
	outputs []DijkstraTransactionOutput,
) *DijkstraTransaction {
	return dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxInputs:  conway.NewConwayTransactionInputSet(inputs),
			TxOutputs: outputs,
		},
	})
}

// TestDijkstraValueConservationCoversSubTransactions pins the equivalence
// between a value-creating body at the top level and the identical body in a
// sub-transaction: both are rejected, with the same consumed and produced
// totals.
func TestDijkstraValueConservationCoversSubTransactions(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	inputs := []shelley.ShelleyTransactionInput{input}
	outputs := []DijkstraTransactionOutput{
		dijkstraSubUtxoOutput(t, dijkstraSubUtxoOverAmount),
	}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)

	var topErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(
		t,
		rule(dijkstraSubUtxoTopLevelTx(inputs, outputs), 0, ls, pp),
		&topErr,
	)
	require.Equal(
		t,
		new(big.Int).SetUint64(dijkstraSubUtxoInputAmount),
		topErr.Consumed,
	)
	require.Equal(
		t,
		new(big.Int).SetUint64(dijkstraSubUtxoOverAmount),
		topErr.Produced,
	)

	var subErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(
		t,
		rule(dijkstraSubUtxoSubTx(inputs, outputs), 0, ls, pp),
		&subErr,
	)
	require.Equal(t, topErr, subErr)

	conserving := []DijkstraTransactionOutput{
		dijkstraSubUtxoOutput(t, dijkstraSubUtxoInputAmount),
	}
	require.NoError(
		t,
		rule(dijkstraSubUtxoSubTx(inputs, conserving), 0, ls, pp),
	)
}

// TestDijkstraValueConservationSpansTransactionLevels covers a batch whose
// levels only balance when taken together: the top level consumes the input
// and the sub-transaction creates the output.
func TestDijkstraValueConservationSpansTransactionLevels(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)

	newTx := func(subOutput uint64) *DijkstraTransaction {
		tx := dijkstraSubUtxoSubTx(
			nil,
			[]DijkstraTransactionOutput{dijkstraSubUtxoOutput(t, subOutput)},
		)
		tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
			[]shelley.ShelleyTransactionInput{input},
		)
		return tx
	}

	require.NoError(t, rule(newTx(dijkstraSubUtxoInputAmount), 0, ls, pp))

	var err shelley.ValueNotConservedUtxoError
	require.ErrorAs(
		t,
		rule(newTx(dijkstraSubUtxoInputAmount+1), 0, ls, pp),
		&err,
	)
}

// TestDijkstraDuplicateInputAcrossTransactionLevels rejects an input spent by
// two levels of the same transaction, which the batch-wide consumed total
// would otherwise count twice.
func TestDijkstraDuplicateInputAcrossTransactionLevels(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	inputs := []shelley.ShelleyTransactionInput{input}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleNoDuplicateInputs)

	tx := dijkstraSubUtxoSubTx(inputs, nil)
	require.NoError(t, rule(tx, 0, ls, pp))

	tx.Body.TxInputs = conway.NewConwayTransactionInputSet(inputs)
	var duplicateErr shelley.DuplicateInputError
	require.ErrorAs(t, rule(tx, 0, ls, pp), &duplicateErr)
	require.Equal(t, "regular", duplicateErr.InputType)

	secondLevel := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{
					{Body: DijkstraSubTransactionBody{
						TxInputs: conway.NewConwayTransactionInputSet(inputs),
					}},
					{Body: DijkstraSubTransactionBody{
						TxInputs: conway.NewConwayTransactionInputSet(inputs),
					}},
				},
				true,
			),
		},
		TxIsValid: true,
	}
	var acrossSubs shelley.DuplicateInputError
	require.ErrorAs(t, rule(secondLevel, 0, ls, pp), &acrossSubs)
}

func TestDijkstraDuplicateInputRulePrecedesValueConservation(t *testing.T) {
	index := func(id common.UtxoValidationRuleId) int {
		for idx, descriptor := range utxoValidationRuleDescriptors {
			if descriptor.Id == id {
				return idx
			}
		}
		return -1
	}
	duplicateIndex := index(common.UtxoValidationRuleNoDuplicateInputs)
	valueIndex := index(common.UtxoValidationRuleValueNotConserved)
	require.GreaterOrEqual(t, duplicateIndex, 0)
	require.Greater(t, valueIndex, duplicateIndex)
}

func TestDijkstraAllowsSpendReferenceInputOverlap(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	topLevelTx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input}, true,
			),
		},
		TxIsValid: true,
	}
	subTransactionTx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input}, true,
			),
		},
	})
	require.NoError(t, UtxoValidateDisjointRefInputs(topLevelTx, 0, ls, pp))
	require.NoError(t, UtxoValidateDisjointRefInputs(subTransactionTx, 0, ls, pp))
	for _, descriptor := range utxoValidationRuleDescriptors {
		require.NotEqual(t, common.UtxoValidationRuleDisjointRefInputs, descriptor.Id)
	}
}

func TestDijkstraBootstrapOutputAttributesCoverSubTransactions(t *testing.T) {
	address, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		bytes.Repeat([]byte{0x11}, common.AddressHashSize),
		common.ByronAddressAttributes{Payload: bytes.Repeat([]byte{0x22}, 100)},
	)
	require.NoError(t, err)
	output := DijkstraTransactionOutput{Output: &babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000},
	}}
	tx := dijkstraSubUtxoSubTx(nil, []DijkstraTransactionOutput{output})
	var attrsErr shelley.OutputBootAddrAttrsTooBigError
	require.ErrorAs(
		t,
		UtxoValidateOutputBootAddrAttrsTooBig(
			tx, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{},
		),
		&attrsErr,
	)
}

func TestDijkstraDonationScriptCheckIsPerLevel(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)
	v1 := DijkstraTransactionWitnessSet{
		WsPlutusV1Scripts: cbor.NewSetType(
			[]common.PlutusV1Script{{0x41, 0}}, false,
		),
	}

	newTx := func(
		subWitnesses DijkstraTransactionWitnessSet,
	) *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				),
				TxSubTransactions: cbor.NewSetType(
					[]DijkstraSubTransaction{{
						Body: DijkstraSubTransactionBody{
							TxDonation: dijkstraSubUtxoInputAmount,
						},
						WitnessSet: subWitnesses,
					}}, true,
				),
			},
			WitnessSet: v1,
			TxIsValid:  true,
		}
	}

	// The top-level witness does not make a sub-transaction donation invalid.
	require.NoError(t, rule(newTx(DijkstraTransactionWitnessSet{}), 0, ls, pp))

	// The same-level PlutusV1 witness and donation are rejected.
	var donationErr conway.TreasuryDonationWithPlutusV1V2Error
	require.ErrorAs(
		t,
		rule(newTx(v1), 0, ls, pp),
		&donationErr,
	)
}

// TestDijkstraBadInputsCoversSubTransactions pins that an unresolvable input
// is reported the same way at either transaction level.
func TestDijkstraBadInputsCoversSubTransactions(t *testing.T) {
	input, utxo := dijkstraSubUtxoInput(0)
	missing, _ := dijkstraSubUtxoInput(7)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleBadInputs)

	inputs := []shelley.ShelleyTransactionInput{input}
	require.NoError(t, rule(dijkstraSubUtxoSubTx(inputs, nil), 0, ls, pp))

	badInputs := []shelley.ShelleyTransactionInput{missing}
	var topErr shelley.BadInputsUtxoError
	require.ErrorAs(
		t,
		rule(dijkstraSubUtxoTopLevelTx(badInputs, nil), 0, ls, pp),
		&topErr,
	)
	var subErr shelley.BadInputsUtxoError
	require.ErrorAs(
		t,
		rule(dijkstraSubUtxoSubTx(badInputs, nil), 0, ls, pp),
		&subErr,
	)
	require.Equal(t, topErr, subErr)
}

func TestDijkstraSubTransactionRunsItsOwnUtxoPredicates(t *testing.T) {
	t.Run("empty input set", func(t *testing.T) {
		tx := dijkstraSubUtxoSubTx(nil, nil)
		rule := dijkstraRule(t, common.UtxoValidationRuleInputSetEmpty)
		var inputSetEmptyErr shelley.InputSetEmptyUtxoError
		require.ErrorAs(
			t,
			rule(tx, 0, mockledger.NewLedgerStateBuilder().Build(),
				&DijkstraProtocolParameters{}),
			&inputSetEmptyErr,
		)
	})

	t.Run("outside validity interval", func(t *testing.T) {
		tx := dijkstraSingleSubTx(DijkstraSubTransaction{
			Body: DijkstraSubTransactionBody{Ttl: 10},
		})
		rule := dijkstraRule(
			t,
			common.UtxoValidationRuleOutsideValidityInterval,
		)
		require.Error(
			t,
			rule(tx, 11, mockledger.NewLedgerStateBuilder().Build(),
				&DijkstraProtocolParameters{}),
		)
	})

	t.Run("body network id", func(t *testing.T) {
		wrongNetwork := uint8(common.AddressNetworkTestnet)
		tx := dijkstraSingleSubTx(DijkstraSubTransaction{
			Body: DijkstraSubTransactionBody{TxNetworkId: &wrongNetwork},
		})
		ls := mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			Build()
		rule := dijkstraRule(
			t,
			common.UtxoValidationRuleTransactionNetworkId,
		)
		var networkErr conway.WrongTransactionNetworkIdError
		require.ErrorAs(
			t,
			rule(tx, 0, ls, &DijkstraProtocolParameters{}),
			&networkErr,
		)
	})
}

func TestDijkstraSubTransactionMetadataUsesChildAuxiliaryData(t *testing.T) {
	auxCBOR := []byte{0xa1, 0x00, 0x01}
	auxData, err := common.DecodeAuxiliaryData(auxCBOR)
	require.NoError(t, err)
	metadata, err := common.DecodeAuxiliaryDataToMetadata(auxCBOR)
	require.NoError(t, err)
	auxHash := common.Blake2b256Hash(auxCBOR)
	tx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{TxAuxDataHash: &auxHash},
	})
	// The parent has matching auxiliary data. It must not satisfy the child's
	// hash, because each SUBUTXOW validates its own third transaction component.
	tx.TxMetadata = metadata
	tx.auxData = auxData
	rule := dijkstraRule(t, common.UtxoValidationRuleMetadata)
	var missingMetadata common.MissingTransactionMetadataError
	require.ErrorAs(
		t,
		rule(tx, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
		&missingMetadata,
	)
}

func TestDijkstraChildProposalIsVisibleToTopLevelVote(t *testing.T) {
	tx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPRewardAccount: testAccountAddress(t),
				PPGovAction: DijkstraGovAction{
					Action: &common.InfoGovAction{},
				},
			}},
		},
	})
	subTransactions := tx.Body.TxSubTransactions.Items()
	childActionID := common.GovActionId{
		TransactionId: subTransactions[0].Body.Id(),
	}
	voter := common.Voter{
		Type: common.VoterTypeStakingPoolKeyHash,
		Hash: common.Blake2b224{0x01},
	}
	tx.Body.TxVotingProcedures = common.VotingProcedures{
		&voter: {&childActionID: {Vote: common.GovVoteYes}},
	}
	rule := dijkstraRule(t, common.UtxoValidationRuleUnknownGovActionIds)
	require.NoError(
		t,
		rule(tx, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
	)
}

// TestDijkstraOutputRulesCoverSubTransactions pins the minimum-coin,
// maximum-value-size and network checks against a sub-transaction's outputs.
func TestDijkstraOutputRulesCoverSubTransactions(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	pp := &DijkstraProtocolParameters{}
	pp.AdaPerUtxoByte = 4310
	pp.MaxValueSize = 4

	tooSmall := []DijkstraTransactionOutput{dijkstraSubUtxoOutput(t, 1)}
	tooSmallRule := dijkstraRule(t, common.UtxoValidationRuleOutputTooSmall)
	var topSmallErr shelley.OutputTooSmallUtxoError
	require.ErrorAs(
		t,
		tooSmallRule(dijkstraSubUtxoTopLevelTx(nil, tooSmall), 0, ls, pp),
		&topSmallErr,
	)
	var subSmallErr shelley.OutputTooSmallUtxoError
	require.ErrorAs(
		t,
		tooSmallRule(dijkstraSubUtxoSubTx(nil, tooSmall), 0, ls, pp),
		&subSmallErr,
	)
	require.Equal(t, topSmallErr, subSmallErr)

	tooBig := []DijkstraTransactionOutput{
		dijkstraSubUtxoOutput(t, dijkstraSubUtxoOverAmount),
	}
	tooBigRule := dijkstraRule(t, common.UtxoValidationRuleOutputTooBig)
	var topBigErr mary.OutputTooBigUtxoError
	require.ErrorAs(
		t,
		tooBigRule(dijkstraSubUtxoTopLevelTx(nil, tooBig), 0, ls, pp),
		&topBigErr,
	)
	var subBigErr mary.OutputTooBigUtxoError
	require.ErrorAs(
		t,
		tooBigRule(dijkstraSubUtxoSubTx(nil, tooBig), 0, ls, pp),
		&subBigErr,
	)
	require.Equal(t, topBigErr, subBigErr)

	// The zero address in these outputs carries the testnet network ID.
	networkRule := dijkstraRule(t, common.UtxoValidationRuleWrongNetwork)
	var topNetworkErr shelley.WrongNetworkError
	require.ErrorAs(
		t,
		networkRule(dijkstraSubUtxoTopLevelTx(nil, tooBig), 0, ls, pp),
		&topNetworkErr,
	)
	var subNetworkErr shelley.WrongNetworkError
	require.ErrorAs(
		t,
		networkRule(dijkstraSubUtxoSubTx(nil, tooBig), 0, ls, pp),
		&subNetworkErr,
	)
	require.Equal(t, topNetworkErr, subNetworkErr)
}
