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
	"fmt"
	"math/big"
	"testing"
	"time"

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

func TestDijkstraOutsideForecastChecksChildForBothValidityOutcomes(
	t *testing.T,
) {
	const upperBound = uint64(12_345)
	for _, level := range []string{"child", "top-level"} {
		for _, valid := range []bool{true, false} {
			t.Run(level+"/"+map[bool]string{true: "valid", false: "invalid"}[valid], func(t *testing.T) {
				calls := 0
				ls := mockledger.NewLedgerStateBuilder().WithSlotToTime(
					func(slot uint64) (time.Time, error) {
						calls++
						if slot == 0 {
							return time.Unix(0, 0), nil
						}
						require.Equal(t, upperBound, slot)
						return time.Time{}, errors.New("slot is outside forecast")
					},
				).Build()
				redeemers := DijkstraRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagSpend, Index: 0}: {},
				}}
				var tx *DijkstraTransaction
				if level == "child" {
					tx = dijkstraSingleSubTx(DijkstraSubTransaction{
						Body: DijkstraSubTransactionBody{Ttl: upperBound},
						WitnessSet: DijkstraTransactionWitnessSet{
							WsRedeemers: redeemers,
						},
					})
				} else {
					tx = &DijkstraTransaction{
						Body:       DijkstraTransactionBody{Ttl: upperBound},
						WitnessSet: DijkstraTransactionWitnessSet{WsRedeemers: redeemers},
					}
				}
				tx.TxIsValid = valid
				var outsideForecast *common.OutsideForecastError
				err := UtxoValidateOutsideForecast(tx, 0, ls, nil)
				require.ErrorAs(t, err, &outsideForecast)
				require.NotNil(t, outsideForecast)
				require.Equal(t, upperBound, outsideForecast.Slot)
				require.Equal(t, uint8(16), outsideForecast.Type)
				require.Equal(t, 1, calls)
			})
		}
	}
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

	for _, level := range []string{"top-level", "child"} {
		t.Run(level+" reference input", func(t *testing.T) {
			body := DijkstraSubTransactionBody{
				TxReferenceInputs: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{missing}, true,
				),
			}
			tx := &DijkstraTransaction{TxIsValid: true}
			if level == "child" {
				tx = dijkstraSingleSubTx(DijkstraSubTransaction{Body: body})
			} else {
				tx.Body.TxReferenceInputs = body.TxReferenceInputs
			}
			var referenceErr common.ReferenceInputResolutionError
			require.ErrorAs(t, rule(tx, 0, ls, pp), &referenceErr)
			require.Equal(t, missing.String(), referenceErr.Input.String())

			validBody := DijkstraSubTransactionBody{
				TxReferenceInputs: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{input}, true,
				),
			}
			validTx := &DijkstraTransaction{TxIsValid: true}
			if level == "child" {
				validTx = dijkstraSingleSubTx(DijkstraSubTransaction{Body: validBody})
			} else {
				validTx.Body.TxReferenceInputs = validBody.TxReferenceInputs
			}
			require.NoError(t, rule(validTx, 0, ls, pp))
		})
	}
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
	rule := dijkstraRule(t, common.UtxoValidationRuleMetadata)
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprintf("is_valid=%t", valid), func(t *testing.T) {
			tx := dijkstraSingleSubTx(DijkstraSubTransaction{
				Body: DijkstraSubTransactionBody{TxAuxDataHash: &auxHash},
			})
			// The parent has matching auxiliary data. It must not satisfy the
			// child's hash, because SUBUTXOW validates its own aux-data bytes.
			tx.TxMetadata = metadata
			tx.auxData = auxData
			tx.Body.TxAuxDataHash = &auxHash
			tx.TxIsValid = valid
			wire, err := tx.MarshalCBOR()
			require.NoError(t, err)
			decoded, err := NewDijkstraTransactionFromCbor(wire)
			require.NoError(t, err)
			var missingMetadata common.MissingTransactionMetadataError
			require.ErrorAs(
				t,
				rule(decoded, 0, mockledger.NewLedgerStateBuilder().Build(),
					&DijkstraProtocolParameters{}),
				&missingMetadata,
			)
		})
	}
}

func TestDijkstraSubTransactionMetadataChecksChildHashAndData(t *testing.T) {
	childAuxCBOR := []byte{0xa1, 0x00, 0x01}
	parentAuxCBOR := []byte{0xa1, 0x00, 0x02}
	childAux, err := common.DecodeAuxiliaryData(childAuxCBOR)
	require.NoError(t, err)
	childMetadata, err := common.DecodeAuxiliaryDataToMetadata(childAuxCBOR)
	require.NoError(t, err)
	parentAux, err := common.DecodeAuxiliaryData(parentAuxCBOR)
	require.NoError(t, err)
	parentMetadata, err := common.DecodeAuxiliaryDataToMetadata(parentAuxCBOR)
	require.NoError(t, err)
	childHash := common.Blake2b256Hash(childAuxCBOR)
	secondChildAuxCBOR := []byte{0xa1, 0x00, 0x03}
	secondChildAux, err := common.DecodeAuxiliaryData(secondChildAuxCBOR)
	require.NoError(t, err)
	secondChildMetadata, err := common.DecodeAuxiliaryDataToMetadata(
		secondChildAuxCBOR,
	)
	require.NoError(t, err)
	secondChildHash := common.Blake2b256Hash(secondChildAuxCBOR)
	parentHash := common.Blake2b256Hash(parentAuxCBOR)

	child := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
		TxAuxDataHash: &childHash,
	}}
	child.TxMetadata = childMetadata
	child.auxData = childAux
	tx := dijkstraSingleSubTx(child)
	secondChild := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
		TxAuxDataHash: &secondChildHash,
	}}
	secondChild.TxMetadata = secondChildMetadata
	secondChild.auxData = secondChildAux
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{child, secondChild},
		true,
	)
	tx.Body.TxAuxDataHash = &parentHash
	tx.TxMetadata = parentMetadata
	tx.auxData = parentAux
	rule := dijkstraRule(t, common.UtxoValidationRuleMetadata)
	wire, err := tx.MarshalCBOR()
	require.NoError(t, err)
	decoded, err := NewDijkstraTransactionFromCbor(wire)
	require.NoError(t, err)
	require.NoError(
		t,
		rule(decoded, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
	)

	wrongHash := common.Blake2b256{0xff}
	mismatchedChild := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{TxAuxDataHash: &wrongHash},
	}
	mismatchedChild.TxMetadata = childMetadata
	mismatchedChild.auxData = childAux
	mismatchedTx := dijkstraSingleSubTx(mismatchedChild)
	var mismatch common.ConflictingMetadataHashError
	mismatchWire, err := mismatchedTx.MarshalCBOR()
	require.NoError(t, err)
	mismatchedDecoded, err := NewDijkstraTransactionFromCbor(mismatchWire)
	require.NoError(t, err)
	require.ErrorAs(
		t,
		rule(mismatchedDecoded, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
		&mismatch,
	)
}

func TestDijkstraSubTransactionMetadataRequiresChildHash(t *testing.T) {
	auxCBOR := []byte{0xa1, 0x00, 0x01}
	auxData, err := common.DecodeAuxiliaryData(auxCBOR)
	require.NoError(t, err)
	metadata, err := common.DecodeAuxiliaryDataToMetadata(auxCBOR)
	require.NoError(t, err)
	child := DijkstraSubTransaction{}
	child.TxMetadata = metadata
	child.auxData = auxData
	tx := dijkstraSingleSubTx(child)
	wire, err := tx.MarshalCBOR()
	require.NoError(t, err)
	decoded, err := NewDijkstraTransactionFromCbor(wire)
	require.NoError(t, err)
	rule := dijkstraRule(t, common.UtxoValidationRuleMetadata)
	var missingHash common.MissingTransactionAuxiliaryDataHashError
	require.ErrorAs(
		t,
		rule(decoded, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
		&missingHash,
	)
}

func TestDijkstraSubTransactionMetadataRejectsInvalidChildMetadata(t *testing.T) {
	// Label 0 maps to a 65-byte text value, beyond the Cardano metadata limit.
	auxCBOR := append([]byte{0xa1, 0x00, 0x78, 0x41}, bytes.Repeat([]byte{'x'}, 65)...)
	auxData, err := common.DecodeAuxiliaryData(auxCBOR)
	require.NoError(t, err)
	metadata, err := common.DecodeAuxiliaryDataToMetadata(auxCBOR)
	require.NoError(t, err)
	auxHash := common.Blake2b256Hash(auxCBOR)
	child := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{TxAuxDataHash: &auxHash},
	}
	child.TxMetadata = metadata
	child.auxData = auxData
	tx := dijkstraSingleSubTx(child)
	wire, err := tx.MarshalCBOR()
	require.NoError(t, err)
	decoded, err := NewDijkstraTransactionFromCbor(wire)
	require.NoError(t, err)

	err = dijkstraRule(t, common.UtxoValidationRuleMetadata)(
		decoded, 0, mockledger.NewLedgerStateBuilder().Build(),
		&DijkstraProtocolParameters{},
	)
	require.ErrorContains(t, err, "metadata text exceeds 64 byte limit")
}

func TestDijkstraChildProposalIsVisibleToTopLevelVote(t *testing.T) {
	child := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPRewardAccount: testAccountAddress(t),
				PPGovAction: DijkstraGovAction{
					Action: &common.InfoGovAction{},
				},
			}},
		},
	}
	childBodyCBOR, err := cbor.Encode(&child.Body)
	require.NoError(t, err)
	child.Body.SetCborReference(childBodyCBOR)
	tx := dijkstraSingleSubTx(child)
	childActionID := common.GovActionId{
		TransactionId: child.Body.Id(),
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

func TestDijkstraChildProposalIsVisibleToLaterChildVote(t *testing.T) {
	children := []DijkstraSubTransaction{
		{Body: DijkstraSubTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPRewardAccount: testAccountAddress(t),
				PPGovAction: DijkstraGovAction{
					Action: &common.InfoGovAction{},
				},
			}},
		}},
		{Body: DijkstraSubTransactionBody{}},
	}
	firstBodyCBOR, err := cbor.Encode(&children[0].Body)
	require.NoError(t, err)
	children[0].Body.SetCborReference(firstBodyCBOR)
	actionID := common.GovActionId{
		TransactionId: children[0].Body.Id(),
	}
	voter := common.Voter{
		Type: common.VoterTypeStakingPoolKeyHash,
		Hash: common.Blake2b224{0x02},
	}
	children[1].Body.TxVotingProcedures = common.VotingProcedures{
		&voter: {&actionID: {Vote: common.GovVoteYes}},
	}
	secondBodyCBOR, err := cbor.Encode(&children[1].Body)
	require.NoError(t, err)
	children[1].Body.SetCborReference(secondBodyCBOR)
	require.NotEqual(t, children[0].Body.Id(), children[1].Body.Id())
	tx := dijkstraSingleSubTx(children[0])
	tx.Body.TxSubTransactions = cbor.NewSetType(children, true)
	rule := dijkstraRule(t, common.UtxoValidationRuleUnknownGovActionIds)
	require.NoError(
		t,
		rule(tx, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
	)
}

func TestDijkstraChildProposalAncestryUsesEarlierChildState(t *testing.T) {
	children := []DijkstraSubTransaction{
		{Body: DijkstraSubTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPRewardAccount: testAccountAddress(t),
				PPGovAction: DijkstraGovAction{
					Action: &DijkstraParameterChangeGovAction{},
				},
			}},
		}},
		{Body: DijkstraSubTransactionBody{}},
	}
	firstBodyCBOR, err := cbor.Encode(&children[0].Body)
	require.NoError(t, err)
	children[0].Body.SetCborReference(firstBodyCBOR)
	earlierActionID := common.GovActionId{
		TransactionId: children[0].Body.Id(),
	}
	children[1].Body.TxProposalProcedures = []DijkstraProposalProcedure{{
		PPRewardAccount: testAccountAddress(t),
		PPGovAction: DijkstraGovAction{
			Action: &DijkstraParameterChangeGovAction{
				ActionId: &earlierActionID,
			},
		},
	}}
	secondBodyCBOR, err := cbor.Encode(&children[1].Body)
	require.NoError(t, err)
	children[1].Body.SetCborReference(secondBodyCBOR)
	require.NotEqual(t, children[0].Body.Id(), children[1].Body.Id())
	tx := dijkstraSingleSubTx(children[0])
	tx.Body.TxSubTransactions = cbor.NewSetType(children, true)
	rule := dijkstraRule(t, common.UtxoValidationRuleProposalAncestry)
	require.NoError(
		t,
		rule(tx, 0, mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{}),
	)
}

func TestDijkstraChildDRepRegistrationIsVisibleToLaterChildVote(t *testing.T) {
	drepCredential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x31},
	}
	actionID := common.GovActionId{TransactionId: common.Blake2b256{0x41}}
	children := []DijkstraSubTransaction{
		{Body: DijkstraSubTransactionBody{
			TxCertificates: []common.CertificateWrapper{{
				Type: uint(common.CertificateTypeRegistrationDrep),
				Certificate: &common.RegistrationDrepCertificate{
					CertType:       uint(common.CertificateTypeRegistrationDrep),
					DrepCredential: drepCredential,
					Amount:         1,
				},
			}},
		}},
		{Body: DijkstraSubTransactionBody{
			TxVotingProcedures: common.VotingProcedures{
				&common.Voter{
					Type: common.VoterTypeDRepKeyHash,
					Hash: drepCredential.Credential,
				}: {&actionID: {Vote: common.GovVoteYes}},
			},
		}},
	}
	tx := dijkstraSingleSubTx(children[0])
	tx.Body.TxSubTransactions = cbor.NewSetType(children, true)
	state := mockledger.NewLedgerStateBuilder().
		WithGovActionById(func(
			id common.GovActionId,
		) (*common.GovActionState, error) {
			if id == actionID {
				return &common.GovActionState{
					ActionId:   actionID,
					ActionType: common.GovActionTypeInfo,
				}, nil
			}
			return nil, nil
		}).Build()
	rule := dijkstraRule(t, common.UtxoValidationRuleUnknownVoters)
	require.NoError(t, rule(tx, 0, state, &DijkstraProtocolParameters{}))
}

func TestDijkstraChildDRepRegistrationIsVisibleToSameChildVote(t *testing.T) {
	drepCredential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x32},
	}
	actionID := common.GovActionId{TransactionId: common.Blake2b256{0x42}}
	child := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxCertificates: []common.CertificateWrapper{{
				Type: uint(common.CertificateTypeRegistrationDrep),
				Certificate: &common.RegistrationDrepCertificate{
					CertType:       uint(common.CertificateTypeRegistrationDrep),
					DrepCredential: drepCredential,
					Amount:         1,
				},
			}},
			TxVotingProcedures: common.VotingProcedures{
				&common.Voter{
					Type: common.VoterTypeDRepKeyHash,
					Hash: drepCredential.Credential,
				}: {&actionID: {Vote: common.GovVoteYes}},
			},
		},
	}
	tx := dijkstraSingleSubTx(child)
	state := mockledger.NewLedgerStateBuilder().
		WithGovActionById(func(
			id common.GovActionId,
		) (*common.GovActionState, error) {
			if id == actionID {
				return &common.GovActionState{
					ActionId:   actionID,
					ActionType: common.GovActionTypeInfo,
				}, nil
			}
			return nil, nil
		}).Build()
	rule := dijkstraRule(t, common.UtxoValidationRuleUnknownVoters)
	require.NoError(t, rule(tx, 0, state, &DijkstraProtocolParameters{}))
}

func TestDijkstraChildGovernanceRulesRespectBatchValidity(t *testing.T) {
	proposal := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPDeposit:       1,
				PPRewardAccount: testAccountAddress(t),
				PPGovAction: DijkstraGovAction{
					Action: &common.InfoGovAction{},
				},
			}},
		},
	}
	proposalTx := dijkstraSingleSubTx(proposal)
	proposalRule, _ := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateProposalDeposit",
	)
	var depositErr conway.ProposalDepositIncorrectError
	require.ErrorAs(
		t,
		proposalRule(proposalTx, 0, nil, &DijkstraProtocolParameters{}),
		&depositErr,
	)
	proposalTx.TxIsValid = false
	require.NoError(
		t,
		proposalRule(proposalTx, 0, nil, &DijkstraProtocolParameters{}),
	)

	unknownVoter := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxVotingProcedures: common.VotingProcedures{
				&common.Voter{
					Type: common.VoterTypeDRepKeyHash,
					Hash: common.Blake2b224{0x51},
				}: {
					&common.GovActionId{
						TransactionId: common.Blake2b256{0x52},
					}: {Vote: common.GovVoteYes},
				},
			},
		},
	}
	validVoteTx := dijkstraSingleSubTx(unknownVoter)
	var unknownErr conway.UnknownVoterError
	require.ErrorAs(
		t,
		dijkstraRule(t, common.UtxoValidationRuleUnknownVoters)(
			validVoteTx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{},
		),
		&unknownErr,
	)
	invalidVoteTx := dijkstraSingleSubTx(unknownVoter)
	invalidVoteTx.TxIsValid = false
	unknownVoterRule, _ := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateUnknownVoters",
	)
	require.NoError(
		t,
		unknownVoterRule(
			invalidVoteTx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			&DijkstraProtocolParameters{},
		),
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
