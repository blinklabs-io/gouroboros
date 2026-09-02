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

package script_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// scriptAddress builds a script-only (no staking part) address whose payment
// part is s's hash, so an output at that address requires s as its spending
// script.
func scriptAddress(t *testing.T, s common.Script) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		s.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	return addr
}

func ledgerStateWithUtxos(utxos ...common.Utxo) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			for _, u := range utxos {
				if u.Id != nil && u.Id.String() == id.String() {
					return u, nil
				}
			}
			return common.Utxo{}, errors.New("utxo not found")
		}).
		Build()
}

func withSpendRedeemer(index uint32) conway.ConwayRedeemers {
	return conway.ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: index}: {
				ExUnits: common.ExUnits{Steps: 1, Memory: 1},
			},
		},
	}
}

// TestValidateRequiredRedeemersMissingReferenceScript pins the exact bug
// #2147 describes: a script-address input satisfied entirely by a CIP-33
// reference script, with no redeemer at all. ValidateScriptWitnesses accepts
// this (the script is reachable via the reference), so nothing but this
// check catches the missing redeemer.
func TestValidateRequiredRedeemersMissingReferenceScript(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	input := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: true,
	}

	err := script.ValidateRequiredRedeemers(tx, ls)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v1.Hash(), missingErr.ScriptHash)
	require.Equal(t, common.RedeemerTagSpend, missingErr.Tag)
	require.Equal(t, uint32(0), missingErr.Index)
}

// TestValidateRequiredRedeemersValidReferenceScript is the corresponding
// happy path: the same reference-script-backed input, but with its spend
// redeemer present.
func TestValidateRequiredRedeemersValidReferenceScript(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	input := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsRedeemers: withSpendRedeemer(0),
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersScriptViaSeparateReferenceInput covers the
// canonical CIP-33 shape: the Plutus script lives on a dedicated reference
// input (TxReferenceInputs), not on the output actually being spent. The
// spent input's own output carries no script at all.
func TestValidateRequiredRedeemersScriptViaSeparateReferenceInput(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	spentInput := shelley.NewShelleyTransactionInput(
		"9999999999999999999999999999999999999999999999999999999999999999",
		0,
	)
	refInput := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	spentUtxo := common.Utxo{
		Id: spentInput,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	refUtxo := common.Utxo{
		Id: refInput,
		Output: &babbage.BabbageTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := ledgerStateWithUtxos(spentUtxo, refUtxo)

	newTx := func() *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{spentInput},
				),
				TxReferenceInputs: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{refInput},
					false,
				),
			},
			TxIsValid: true,
		}
	}

	t.Run("missing redeemer rejected", func(t *testing.T) {
		err := script.ValidateRequiredRedeemers(newTx(), ls)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, v1.Hash(), missingErr.ScriptHash)
		require.Equal(t, uint32(0), missingErr.Index)
	})

	t.Run("valid redeemer accepted", func(t *testing.T) {
		tx := newTx()
		tx.WitnessSet.WsRedeemers = withSpendRedeemer(0)
		require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
	})
}

// TestValidateRequiredRedeemersMissingWitnessScript shows the gap isn't
// unique to reference scripts: an explicit Plutus witness script on a
// script-address input still needs its own redeemer even when other
// redeemers exist in the transaction.
func TestValidateRequiredRedeemersMissingWitnessScript(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	input := shelley.NewShelleyTransactionInput(
		"2222222222222222222222222222222222222222222222222222222222222222",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType(
				[]common.PlutusV1Script{v1},
				false,
			),
			// A redeemer at some other purpose's index -- not this input's
			// spend index -- must not satisfy this input's requirement.
			WsRedeemers: withSpendRedeemer(1),
		},
		TxIsValid: true,
	}

	err := script.ValidateRequiredRedeemers(tx, ls)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, uint32(0), missingErr.Index)
}

// TestValidateRequiredRedeemersMixedInputs spends two script-address inputs
// sorted (TxId, Index) so the second input comes first; only the second has
// a redeemer. The first (sorted index 1) must be reported missing, and the
// reference-script/witness-script distinction between the two inputs must
// not matter.
func TestValidateRequiredRedeemersMixedInputs(t *testing.T) {
	v1 := common.PlutusV1Script{0x01}
	v2 := common.PlutusV2Script{0x02}

	// Sorts after "aaaa...", so v1's input lands at sorted index 1.
	inputWitness := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	inputRef := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)

	utxoWitness := common.Utxo{
		Id: inputWitness,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	utxoRef := common.Utxo{
		Id: inputRef,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v2),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV2,
				Script: v2,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxoWitness, utxoRef)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{inputWitness, inputRef},
			),
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType(
				[]common.PlutusV1Script{v1},
				false,
			),
			// Only the reference-script input (sorted index 0) gets a redeemer.
			WsRedeemers: withSpendRedeemer(0),
		},
		TxIsValid: true,
	}

	err := script.ValidateRequiredRedeemers(tx, ls)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v1.Hash(), missingErr.ScriptHash)
	require.Equal(t, uint32(1), missingErr.Index)
}

// TestValidateRequiredRedeemersMixedInputsBothRedeemed is
// TestValidateRequiredRedeemersMixedInputs's positive counterpart: the same
// two script-address inputs (one witness-backed, one reference-backed),
// each correctly redeemed at its own sorted-input index, must pass.
func TestValidateRequiredRedeemersMixedInputsBothRedeemed(t *testing.T) {
	v1 := common.PlutusV1Script{0x01}
	v2 := common.PlutusV2Script{0x02}

	// Sorts after "aaaa...", so v1's input lands at sorted index 1.
	inputWitness := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	inputRef := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)

	utxoWitness := common.Utxo{
		Id: inputWitness,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	utxoRef := common.Utxo{
		Id: inputRef,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v2),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV2,
				Script: v2,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxoWitness, utxoRef)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{inputWitness, inputRef},
			),
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType(
				[]common.PlutusV1Script{v1},
				false,
			),
			WsRedeemers: conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					// inputRef (v2, reference-backed) at sorted index 0.
					{Tag: common.RedeemerTagSpend, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
					// inputWitness (v1, witness-backed) at sorted index 1.
					{Tag: common.RedeemerTagSpend, Index: 1}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersSkipsNonScriptAddress ensures a key-address
// input is never treated as requiring a redeemer.
func TestValidateRequiredRedeemersSkipsNonScriptAddress(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"3333333333333333333333333333333333333333333333333333333333333333",
		0,
	)
	keyAddr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		make([]byte, 28),
		nil,
	)
	require.NoError(t, err)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: keyAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersSkipsNativeScript ensures a script-address
// input backed by a native (non-Plutus) reference script is not required to
// carry a redeemer.
func TestValidateRequiredRedeemersSkipsNativeScript(t *testing.T) {
	nsCbor, err := cbor.Encode(
		common.NativeScriptInvalidBefore{Type: 4, Slot: 0},
	)
	require.NoError(t, err)
	var ns common.NativeScript
	require.NoError(t, ns.UnmarshalCBOR(nsCbor))

	input := shelley.NewShelleyTransactionInput(
		"4444444444444444444444444444444444444444444444444444444444444444",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, ns),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypeNativeScript,
				Script: ns,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersSkipsNativeWitnessScript is
// TestValidateRequiredRedeemersSkipsNativeScript's explicit-witness
// counterpart: a native script provided in the witness set, not as a
// reference script, must not be treated as requiring a redeemer either.
func TestValidateRequiredRedeemersSkipsNativeWitnessScript(t *testing.T) {
	nsCbor, err := cbor.Encode(
		common.NativeScriptInvalidBefore{Type: 4, Slot: 0},
	)
	require.NoError(t, err)
	var ns common.NativeScript
	require.NoError(t, ns.UnmarshalCBOR(nsCbor))

	input := shelley.NewShelleyTransactionInput(
		"6666666666666666666666666666666666666666666666666666666666666666",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, ns),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsNativeScripts: cbor.NewSetType(
				[]common.NativeScript{ns},
				false,
			),
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersSkipsScriptEntirelyMissing ensures a
// script-address input whose script is reachable via neither an explicit
// witness nor a reference script is not reported by this check. That
// absence is ValidateScriptWitnesses's error to report (MissingScriptWitnessesError);
// this check must not also fire, or must not fire a *different* error, for
// the same missing script.
func TestValidateRequiredRedeemersSkipsScriptEntirelyMissing(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	input := shelley.NewShelleyTransactionInput(
		"7777777777777777777777777777777777777777777777777777777777777777",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			// scriptAddress requires v1's hash, but v1 is never provided as
			// a witness or a reference script anywhere in this transaction.
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: true,
	}

	require.NoError(t, script.ValidateRequiredRedeemers(tx, ls))
}

// TestValidateRequiredRedeemersPropagatesInputResolutionError pins the
// current resolution-error contract: an unresolvable consumed input
// surfaces as common.InputResolutionError, the same error
// NewTxScriptView/ResolveTxInputs report, even though
// shelley.UtxoValidateBadInputsUtxo already reports the same failure more
// specifically and is registered ahead of this rule in every era's
// production rule list. This test pins today's actual behavior so a future
// switch to NewTxScriptViewSkippingUnresolved (#2162) changes it
// deliberately rather than silently.
func TestValidateRequiredRedeemersPropagatesInputResolutionError(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"8888888888888888888888888888888888888888888888888888888888888888",
		0,
	)
	// No UTxOs registered, so every UtxoById call fails.
	ls := ledgerStateWithUtxos()

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: true,
	}

	err := script.ValidateRequiredRedeemers(tx, ls)
	require.ErrorAs(t, err, &common.InputResolutionError{})
}

// TestValidateRequiredRedeemersMissingRedeemerPlutusV3AndV4 extends the
// missing-redeemer coverage beyond PlutusV1/V2 to PlutusV3 and PlutusV4
// reference scripts.
func TestValidateRequiredRedeemersMissingRedeemerPlutusV3AndV4(t *testing.T) {
	for _, tc := range []struct {
		name    string
		script  common.Script
		refType uint
	}{
		{"PlutusV3", common.PlutusV3Script{0x01}, common.ScriptRefTypePlutusV3},
		{"PlutusV4", common.PlutusV4Script{0x01}, common.ScriptRefTypePlutusV4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := shelley.NewShelleyTransactionInput(
				"9999999999999999999999999999999999999999999999999999999999999999",
				0,
			)
			utxo := common.Utxo{
				Id: input,
				Output: &babbage.BabbageTransactionOutput{
					OutputAddress: scriptAddress(t, tc.script),
					OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
					TxOutScriptRef: &common.ScriptRef{
						Type:   tc.refType,
						Script: tc.script,
					},
				},
			}
			ls := ledgerStateWithUtxos(utxo)

			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxInputs: conway.NewConwayTransactionInputSet(
						[]shelley.ShelleyTransactionInput{input},
					),
				},
				TxIsValid: true,
			}

			err := script.ValidateRequiredRedeemers(tx, ls)
			var missingErr common.MissingRedeemerForScriptError
			require.ErrorAs(t, err, &missingErr)
			require.Equal(t, tc.script.Hash(), missingErr.ScriptHash)
			require.Equal(t, uint32(0), missingErr.Index)
		})
	}
}

// TestValidateRequiredRedeemersAppliesRegardlessOfIsValid pins that this
// check is NOT gated on the transaction's IsValid flag, unlike
// ValidateScriptWitnesses. AlonzoUTXOW's hasExactSetOfRedeemers (the source
// of AlonzoUtxowPredFailure.MissingRedeemers) is a witnessing-completeness
// check that cardano-ledger enforces unconditionally; IsValid is read only
// by UTXOS, never by UTXOW, to decide which witnesses are required. A
// submitter marking a transaction invalid does not excuse it from carrying
// every redeemer its script-locked inputs require.
func TestValidateRequiredRedeemersAppliesRegardlessOfIsValid(t *testing.T) {
	v1 := common.PlutusV1Script{0x01}
	input := shelley.NewShelleyTransactionInput(
		"5555555555555555555555555555555555555555555555555555555555555555",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddress(t, v1),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := ledgerStateWithUtxos(utxo)

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
		},
		TxIsValid: false,
	}

	err := script.ValidateRequiredRedeemers(tx, ls)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v1.Hash(), missingErr.ScriptHash)
	require.Equal(t, uint32(0), missingErr.Index)
}

// TestValidateRequiredRedeemersNilLedgerState mirrors
// ValidateScriptWitnesses's contract: without a LedgerState, script
// resolution isn't possible, so the check must not report a false positive.
func TestValidateRequiredRedeemersNilLedgerState(t *testing.T) {
	tx := &conway.ConwayTransaction{}
	require.NoError(t, script.ValidateRequiredRedeemers(tx, nil))
}

// TestValidateRequiredRedeemersNilTransaction pins that a nil Transaction
// interface value is rejected before tx.IsValid() is called on it -- calling
// a method on a nil interface panics, so the nil check must short-circuit
// first (matching NewTxScriptView's own "tx == nil || ls == nil" guard).
func TestValidateRequiredRedeemersNilTransaction(t *testing.T) {
	require.NotPanics(t, func() {
		err := script.ValidateRequiredRedeemers(nil, nil)
		require.NoError(t, err)
	})
}
