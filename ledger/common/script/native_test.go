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
	"bytes"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func nativeScriptHashes(scripts []common.NativeScript) []common.ScriptHash {
	out := make([]common.ScriptHash, 0, len(scripts))
	for _, nativeScript := range scripts {
		out = append(out, nativeScript.Hash())
	}
	return out
}

// The rules report the first failing script's hash, so the order has to be
// the same on every node and every run. Needed is a map, whose iteration order
// Go randomizes, so the reference-only tail is sorted rather than taken as it
// comes.
func TestNativeScriptsToEvaluateOrderIsStable(t *testing.T) {
	witnessFirst := testNativeScript(t, 1)
	witnessSecond := testNativeScript(t, 2)
	referenceOne := testNativeScript(t, 3)
	referenceTwo := testNativeScript(t, 4)

	tx := mockledger.NewTransactionBuilder().WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithNativeScripts(witnessSecond, witnessFirst),
	)
	view := script.TxScriptView{
		Needed: map[common.ScriptHash]common.Script{
			witnessFirst.Hash(): witnessFirst,
			referenceOne.Hash(): referenceOne,
			referenceTwo.Hash(): &referenceTwo,
		},
	}

	referenceTail := []common.ScriptHash{
		referenceOne.Hash(),
		referenceTwo.Hash(),
	}
	slices.SortFunc(referenceTail, func(a, b common.ScriptHash) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})
	want := append(
		[]common.ScriptHash{witnessSecond.Hash(), witnessFirst.Hash()},
		referenceTail...,
	)

	for range 20 {
		require.Equal(
			t,
			want,
			nativeScriptHashes(script.NativeScriptsToEvaluate(tx, view)),
		)
	}
}

// A reference script the transaction merely reaches, and a needed script that
// is not native, are both left out: the first because spending a UTxO
// carrying an unrelated reference script must not drag it into validation,
// the second because a Plutus script runs under the phase-2 rules instead.
func TestNativeScriptsToEvaluateSelection(t *testing.T) {
	needed := testNativeScript(t, 1)
	availableOnly := testNativeScript(t, 2)
	plutus := common.PlutusV2Script([]byte{0x01})

	tx := mockledger.NewTransactionBuilder()
	view := script.TxScriptView{
		Available: map[common.ScriptHash]common.Script{
			needed.Hash():        needed,
			availableOnly.Hash(): availableOnly,
			plutus.Hash():        plutus,
		},
		Needed: map[common.ScriptHash]common.Script{
			needed.Hash(): needed,
			plutus.Hash(): plutus,
		},
	}

	require.Equal(
		t,
		[]common.ScriptHash{needed.Hash()},
		nativeScriptHashes(script.NativeScriptsToEvaluate(tx, view)),
	)
}

// A script provided both as a witness and as a reference script is one
// script, and must be evaluated once.
func TestNativeScriptsToEvaluateDeduplicates(t *testing.T) {
	nativeScript := testNativeScript(t, 1)
	tx := mockledger.NewTransactionBuilder().WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithNativeScripts(nativeScript, nativeScript),
	)
	view := script.TxScriptView{
		Needed: map[common.ScriptHash]common.Script{
			nativeScript.Hash(): nativeScript,
		},
	}
	require.Equal(
		t,
		[]common.ScriptHash{nativeScript.Hash()},
		nativeScriptHashes(script.NativeScriptsToEvaluate(tx, view)),
	)
}

// An era before Babbage has no reference scripts and passes the zero view, so
// the witness set alone has to survive it.
func TestNativeScriptsToEvaluateZeroView(t *testing.T) {
	nativeScript := testNativeScript(t, 1)
	tx := mockledger.NewTransactionBuilder().WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithNativeScripts(nativeScript),
	)
	require.Equal(
		t,
		[]common.ScriptHash{nativeScript.Hash()},
		nativeScriptHashes(
			script.NativeScriptsToEvaluate(tx, script.TxScriptView{}),
		),
	)
	require.Empty(
		t,
		script.NativeScriptsToEvaluate(nil, script.TxScriptView{}),
	)
}

func TestNewNativeScriptEnv(t *testing.T) {
	vkey := bytes.Repeat([]byte{0x71}, 32)
	bootstrapKey := bytes.Repeat([]byte{0x72}, 32)

	t.Run("no transaction", func(t *testing.T) {
		env := script.NewNativeScriptEnv(nil, 7)
		require.Equal(t, uint64(7), env.Slot)
		require.Equal(t, uint64(0), env.ValidityStart)
		require.Equal(t, ^uint64(0), env.ValidityEnd)
		require.Empty(t, env.KeyHashes)
	})

	t.Run("no upper bound", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		require.Equal(
			t,
			^uint64(0),
			script.NewNativeScriptEnv(tx, 0).ValidityEnd,
			"an absent upper bound must not read as slot zero",
		)
	})

	t.Run("witness key hashes", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder().WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().
				WithVkeyWitnesses(common.VkeyWitness{Vkey: vkey}).
				WithBootstrapWitnesses(
					common.BootstrapWitness{PublicKey: bootstrapKey},
				),
		)
		tx.WithTTL(42)
		env := script.NewNativeScriptEnv(tx, 0)
		require.Equal(t, uint64(42), env.ValidityEnd)
		require.True(t, env.KeyHashes[common.Blake2b224Hash(vkey)])
		require.True(
			t,
			env.KeyHashes[common.Blake2b224Hash(bootstrapKey)],
		)
	})
}
