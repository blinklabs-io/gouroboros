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

package babbage_test

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// preview1796036TxHex is preview transaction
// 557b728b5a8aaed825d07ac2f76de31e3fb3f3069e19d32294215d820f375821, in block
// height 83667 at slot 1796036 (epoch 20, Babbage). Koios reports it
// valid_contract: true.
//
// Its witness set is exactly three fields — vkey witnesses, one datum
// (Constr 0 []), and an empty redeemer list — and it spends a single input
// that carries a reference script. No script runs.
const preview1796036TxHex = "84a500818258203b063fb1ab831a4d03e628a30a072394fc9bb3d0278b6b7f355e623e5baa462e01018283581d70c0c671fba483641a71bb92d3a8b7c52c90bf1c01e2b83116ad7d45361a001e84805820923918e403bf43c34b4ef6b48eb2ee04babed17320d8d1b9ff9ad086e86f44ec825839005bb3534ddfca05e65461af994b952a9c50a59e9155e7ba57785392dda201bb51c680bf0065167a2c6d540396bda3f4ba3e35835ca197d0fa821a0091259fa1581c998afa2ac4fd62044bbb74b327b5436c2655261698873ec81d5afa2ba14643544c4e465401021a0002b0f10b582015dd0a3ac1244430aacc7e95c2734b51f1a8cf2aaf05e5d6e8124cb78ab54cc90f00a300818258208f5c2f5ad1a6b3b8a3ee45246f54c12901f45bfa9ffb2245d160a9cc009f255c58409a7ab043d9752234ccd2e95615e88c650974d73b2836c06ac485f5352482d5e787f9a7c98123feb0bc477fca43d89046d6d4db8e61029fcdebfc1d643f45b607049fd87980ff0580f5f6"

// TestUtxoValidateScriptDataHashIgnoresUnneededReferenceScript is the
// gouroboros #2188 regression, from the transaction that first exposed it.
//
// The rule used to fold the language of every reference script reachable
// through a resolved input into the language views. This transaction spends an
// output carrying a PlutusV2 reference script that no redeemer invokes, so the
// producer's language-view map was empty and ours was not:
//
//	declared 15dd0a3ac1244430aacc7e95c2734b51f1a8cf2aaf05e5d6e8124cb78ab54cc9
//	computed 5d1149cf583ac917368235b421a5bed45e86e9d2b592923b55b2dbf614d9628f
//
// blake2b256(80 || 9fd87980ff || a0) is the declared value, which is what pins
// the empty map as the correct input.
func TestUtxoValidateScriptDataHashIgnoresUnneededReferenceScript(t *testing.T) {
	txBytes, err := hex.DecodeString(preview1796036TxHex)
	require.NoError(t, err)
	tx, err := babbage.NewBabbageTransactionFromCbor(txBytes)
	require.NoError(t, err)

	inputs := tx.Inputs()
	require.Len(t, inputs, 1)

	// The spent output is at a key address — nothing about it needs a script —
	// and happens to carry a PlutusV2 reference script.
	keyAddr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		make([]byte, 28),
		nil,
	)
	require.NoError(t, err)
	refScript := common.PlutusV2Script{0x01, 0x02, 0x03}
	utxo := common.Utxo{
		Id: inputs[0],
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: keyAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 11688720},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV2,
				Script: refScript,
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			if id.String() == inputs[0].String() {
				return utxo, nil
			}
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	// Both cost models present, so a wrongly-included language fails on the
	// hash rather than short-circuiting as a missing cost model.
	pp := &babbage.BabbageProtocolParameters{
		CostModels: map[uint][]int64{
			0: {197209, 0},
			1: {205665, 812},
		},
	}

	require.NoError(
		t,
		babbage.UtxoValidateScriptDataHash(tx, 1796036, ls, pp),
		"a reference script no redeemer invokes must not contribute a language view",
	)
}

// TestUtxoValidateScriptDataHashCountsNeededReferenceScript is the control: the
// same reference script, on an input the transaction genuinely has to run a
// script for, does contribute its language — so the fix narrows the set rather
// than emptying it.
func TestUtxoValidateScriptDataHashCountsNeededReferenceScript(t *testing.T) {
	txBytes, err := hex.DecodeString(preview1796036TxHex)
	require.NoError(t, err)
	tx, err := babbage.NewBabbageTransactionFromCbor(txBytes)
	require.NoError(t, err)
	inputs := tx.Inputs()
	require.Len(t, inputs, 1)

	refScript := common.PlutusV2Script{0x01, 0x02, 0x03}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		refScript.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	utxo := common.Utxo{
		Id: inputs[0],
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 11688720},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV2,
				Script: refScript,
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			if id.String() == inputs[0].String() {
				return utxo, nil
			}
			return common.Utxo{}, errors.New("not found")
		}).
		Build()
	pp := &babbage.BabbageProtocolParameters{
		CostModels: map[uint][]int64{
			0: {197209, 0},
			1: {205665, 812},
		},
	}

	err = babbage.UtxoValidateScriptDataHash(tx, 1796036, ls, pp)
	require.Error(t, err,
		"a script the spend purpose requires must contribute its language view")
	require.ErrorIs(t, err, common.ErrScriptDataHashMismatch)
}

// TestUtxoValidateScriptDataHashDefersUnresolvableSpentInput pins the error
// taxonomy this rule must not disturb. Deriving the language set from the
// needed scripts requires resolving the transaction's inputs, which this rule
// did not do in Alonzo at all and did only partially in Babbage. An input that
// does not resolve is reported by UtxoValidateBadInputsUtxo, registered in the
// same rule list, so reporting it from here instead would change which error an
// invalid transaction produces.
//
// A reference input is deliberately not covered by that rule, so its resolution
// failure still surfaces here — the behaviour Babbage and Conway already had.
func TestUtxoValidateScriptDataHashDefersUnresolvableSpentInput(t *testing.T) {
	txBytes, err := hex.DecodeString(preview1796036TxHex)
	require.NoError(t, err)
	tx, err := babbage.NewBabbageTransactionFromCbor(txBytes)
	require.NoError(t, err)

	// Nothing resolves.
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("not found")
		}).
		Build()
	pp := &babbage.BabbageProtocolParameters{
		CostModels: map[uint][]int64{0: {197209, 0}, 1: {205665, 812}},
	}

	require.NoError(
		t,
		babbage.UtxoValidateScriptDataHash(tx, 1796036, ls, pp),
		"an unresolvable spent input belongs to UtxoValidateBadInputsUtxo",
	)

	// And that rule does reject it, with the error the deferral is deferring
	// to. Asserting merely that something failed would keep passing if that
	// rule started reporting the resolution failure some other way, which is
	// exactly the coupling this test exists to pin.
	err = babbage.UtxoValidateBadInputsUtxo(tx, 1796036, ls, pp)
	require.Error(t, err, "the dedicated rule still rejects the transaction")
	var badInputs shelley.BadInputsUtxoError
	require.ErrorAs(t, err, &badInputs)
	require.Len(t, badInputs.Inputs, 1)
	require.Equal(t, tx.Inputs()[0].String(), badInputs.Inputs[0].String())
}
