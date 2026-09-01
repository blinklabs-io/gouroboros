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
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// noUtxoLedgerState satisfies the LedgerState argument for a transaction with no
// inputs. Every method is nil on purpose, so a call this test did not intend
// panics rather than returning a zero value.
type noUtxoLedgerState struct {
	common.LedgerState
}

// ScriptPurposeVoting.ScriptHash returns Blake2b224(Voter.Hash) with no check on
// Voter.Type, so a key-hash voter yields its key hash typed as a script hash.
// These cases give the voter the same 28 bytes as an available PlutusV1 script,
// which is the shape that turns the missing type check into a false entry in
// Needed.
func TestNewTxScriptViewVotingProcedureVoterTypes(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x01, 0x02})
	scriptHash := v1.Hash()
	var voterHash [28]byte
	copy(voterHash[:], scriptHash.Bytes())

	for _, tc := range []struct {
		name       string
		voterType  uint8
		wantNeeded bool
	}{
		{"drep script hash", common.VoterTypeDRepScriptHash, true},
		{
			"committee hot script hash",
			common.VoterTypeConstitutionalCommitteeHotScriptHash,
			true,
		},
		{"drep key hash", common.VoterTypeDRepKeyHash, false},
		{
			"committee hot key hash",
			common.VoterTypeConstitutionalCommitteeHotKeyHash,
			false,
		},
		{
			"staking pool key hash",
			common.VoterTypeStakingPoolKeyHash,
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			voter := common.Voter{Type: tc.voterType, Hash: voterHash}
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxVotingProcedures: common.VotingProcedures{}.AddOrReplace(
						voter,
						common.GovActionId{},
						common.VotingProcedure{},
					),
				},
				WitnessSet: conway.ConwayTransactionWitnessSet{
					WsPlutusV1Scripts: cbor.NewSetType(
						[]common.PlutusV1Script{v1},
						false,
					),
				},
			}
			view, err := script.NewTxScriptView(tx, noUtxoLedgerState{})
			require.NoError(t, err)
			require.Contains(
				t,
				view.Available,
				scriptHash,
				"the witness script is reachable regardless of voter type",
			)
			if tc.wantNeeded {
				require.Contains(
					t,
					view.Needed,
					scriptHash,
					"a script voter requires the script it names",
				)
				return
			}
			require.NotContains(
				t,
				view.Needed,
				scriptHash,
				"a key-hash voter must not enter a key hash as a needed script",
			)
		})
	}
}

// A view assembled field-by-field rather than through NewTxScriptView has no
// cached concatenation, so AllResolvedInputs must still build one.
func TestTxScriptViewAllResolvedInputsWithoutCache(t *testing.T) {
	consumed := common.Utxo{}
	reference := common.Utxo{}
	view := script.TxScriptView{
		ResolvedInputs:          []common.Utxo{consumed},
		ResolvedReferenceInputs: []common.Utxo{reference},
	}
	require.Len(t, view.AllResolvedInputs(), 2)
	require.Empty(t, script.TxScriptView{}.AllResolvedInputs())
}

func testNativeScript(t *testing.T, slot uint64) common.NativeScript {
	t.Helper()
	scriptCbor, err := cbor.Encode(
		common.NativeScriptInvalidBefore{Type: 4, Slot: slot},
	)
	require.NoError(t, err)
	var ns common.NativeScript
	require.NoError(t, ns.UnmarshalCBOR(scriptCbor))
	return ns
}

func refOutput(s common.Script) common.TransactionOutput {
	refType := uint(common.ScriptRefTypeNativeScript)
	switch s.(type) {
	case common.PlutusV1Script:
		refType = common.ScriptRefTypePlutusV1
	case common.PlutusV2Script:
		refType = common.ScriptRefTypePlutusV2
	case common.PlutusV3Script:
		refType = common.ScriptRefTypePlutusV3
	case common.PlutusV4Script:
		refType = common.ScriptRefTypePlutusV4
	}
	return &babbage.BabbageTransactionOutput{
		TxOutScriptRef: &common.ScriptRef{Type: refType, Script: s},
	}
}

// AvailablePlutusScripts backs conway.UtxoValidatePlutusScripts' and
// dijkstra's guarding-redeemer execution's script-availability maps. Both
// key an unmatched redeemer's "no script" branch on a hash's absence here,
// and Dijkstra's native-script guard fallback depends on a native script --
// witness or reference -- staying absent rather than
// present-but-unexecutable. dijkstra/rules_test.go's
// TestUtxoValidateGuardingRedeemerRejectsNativeScriptGuard pins the witness
// case at the rule level;
// TestUtxoValidateGuardingRedeemerRejectsNativeReferenceScriptGuard pins the
// reference case there.
func TestAvailablePlutusScriptsExcludesNativeScripts(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x01})
	v2 := common.PlutusV2Script([]byte{0x02})
	nativeWitness := testNativeScript(t, 0)
	nativeRef := testNativeScript(t, 1000)

	tx := &conway.ConwayTransaction{
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType(
				[]common.PlutusV1Script{v1},
				false,
			),
			WsNativeScripts: cbor.NewSetType(
				[]common.NativeScript{nativeWitness},
				false,
			),
		},
	}
	resolved := []common.Utxo{
		{Output: refOutput(v2)},
		{Output: refOutput(nativeRef)},
	}

	available := script.AvailablePlutusScripts(tx, resolved)

	require.Contains(
		t,
		available,
		v1.Hash(),
		"witness PlutusV1 script must be available",
	)
	require.Contains(
		t,
		available,
		v2.Hash(),
		"Plutus reference script must be available",
	)
	require.NotContains(
		t,
		available,
		nativeWitness.Hash(),
		"witness native script must not be an available Plutus script",
	)
	require.NotContains(
		t,
		available,
		nativeRef.Hash(),
		"native reference script must not be an available Plutus script",
	)
}

func TestPlutusWitnessScriptsNilWitnessSet(t *testing.T) {
	out := script.PlutusWitnessScripts(nil)
	require.NotNil(t, out)
	require.Empty(t, out)
}

// ResolveTxInputs is the single UtxoById caller NewTxScriptView,
// conway.UtxoValidatePlutusScripts, and Dijkstra's guarding-redeemer
// execution now all share. These pin its order and error-type contract.
func TestResolveTxInputsOrderAndErrors(t *testing.T) {
	in1 := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		0,
	)
	in2 := shelley.NewShelleyTransactionInput(
		"2222222222222222222222222222222222222222222222222222222222222222",
		1,
	)
	ref1 := shelley.NewShelleyTransactionInput(
		"3333333333333333333333333333333333333333333333333333333333333333",
		0,
	)
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{in1, in2},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{ref1},
				false,
			),
		},
	}

	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			switch input.String() {
			case in1.String(), in2.String(), ref1.String():
				return common.Utxo{Id: input}, nil
			default:
				return common.Utxo{}, errors.New("utxo not found")
			}
		}).
		Build()

	inputs, refInputs, err := script.ResolveTxInputs(tx, ls)
	require.NoError(t, err)
	require.Len(t, inputs, 2)
	require.Equal(
		t,
		in1.String(),
		inputs[0].Id.String(),
		"consumed inputs stay in tx.Inputs() order",
	)
	require.Equal(t, in2.String(), inputs[1].Id.String())
	require.Len(t, refInputs, 1)
	require.Equal(t, ref1.String(), refInputs[0].Id.String())

	concat := script.ConcatResolvedInputs(inputs, refInputs)
	require.Len(t, concat, 3)
	require.Equal(
		t,
		in1.String(),
		concat[0].Id.String(),
		"consumed inputs come first",
	)
	require.Equal(t, in2.String(), concat[1].Id.String())
	require.Equal(
		t,
		ref1.String(),
		concat[2].Id.String(),
		"reference inputs come last",
	)

	failLs := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("boom")
		}).
		Build()

	consumedOnlyTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{in1},
			),
		},
	}
	_, _, err = script.ResolveTxInputs(consumedOnlyTx, failLs)
	require.ErrorAs(t, err, &common.InputResolutionError{})

	refOnlyTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{ref1},
				false,
			),
		},
	}
	_, _, err = script.ResolveTxInputs(refOnlyTx, failLs)
	require.ErrorAs(t, err, &common.ReferenceInputResolutionError{})
}

// partialResolutionLedgerState resolves reference inputs but fails the spend
// input, which is the shape that previously produced a view with no reference
// scripts at all.
type partialResolutionLedgerState struct {
	common.LedgerState
	resolvable map[string]common.Utxo
}

func (s partialResolutionLedgerState) UtxoById(
	input common.TransactionInput,
) (common.Utxo, error) {
	key := string(input.Id().Bytes()) + string(rune(input.Index()))
	if utxo, ok := s.resolvable[key]; ok {
		return utxo, nil
	}
	return common.Utxo{}, errors.New("input not found")
}

// A script supplied only as a reference script must still reach Available when
// a spend input cannot be resolved, so a caller enforcing a language
// restriction sees it before deferring the failure to UtxoValidateBadInputsUtxo.
func TestNewTxScriptViewPartialViewKeepsReferenceScripts(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x01, 0x02})
	scriptHash := v1.Hash()

	spendInput := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000001",
		0,
	)
	refInput := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000002",
		0,
	)
	refUtxo := common.Utxo{
		Id: refInput,
		Output: babbage.BabbageTransactionOutput{
			TxOutScriptRef: &common.ScriptRef{Script: v1},
		},
	}
	ls := partialResolutionLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		resolvable: map[string]common.Utxo{
			string(refInput.Id().Bytes()) + string(rune(refInput.Index())): refUtxo,
		},
	}

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{spendInput},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{refInput},
				false,
			),
		},
	}

	view, err := script.NewTxScriptView(tx, ls)
	require.Error(t, err, "the unresolvable spend input is still reported")
	var resolutionErr common.InputResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	require.Contains(
		t,
		view.Available,
		scriptHash,
		"a reference script that resolved must survive the partial view",
	)
	require.Empty(
		t,
		view.ResolvedReferenceInputs,
		"the view stays partial",
	)
}

// A consumed input that resolved carries its scripts into the partial view
// too, not just reference inputs.
func TestNewTxScriptViewPartialViewKeepsResolvedSpendScripts(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x03, 0x04})
	scriptHash := v1.Hash()

	resolvableSpend := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000003",
		0,
	)
	unresolvableSpend := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000004",
		0,
	)
	spendUtxo := common.Utxo{
		Id: resolvableSpend,
		Output: babbage.BabbageTransactionOutput{
			TxOutScriptRef: &common.ScriptRef{Script: v1},
		},
	}
	ls := partialResolutionLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		resolvable: map[string]common.Utxo{
			string(resolvableSpend.Id().Bytes()) +
				string(rune(resolvableSpend.Index())): spendUtxo,
		},
	}

	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					resolvableSpend,
					unresolvableSpend,
				},
			),
		},
	}

	view, err := script.NewTxScriptView(tx, ls)
	require.Error(t, err, "the unresolvable spend input is still reported")
	require.Contains(
		t,
		view.Available,
		scriptHash,
		"a script on a consumed input that resolved must survive",
	)
	require.Empty(t, view.ResolvedInputs, "the view stays partial")
}
