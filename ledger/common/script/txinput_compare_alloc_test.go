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
	"testing"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// txInputCompareAllocFixtureSize matches the shape that made input
// resolution expensive in production: a transaction with dozens of inputs,
// each resolved against a UTxO set of comparable size, scanned once per
// block-apply during ledger validation.
const txInputCompareAllocFixtureSize = 40

// newTxInputCompareAllocInputs builds n distinct plain-key-address UTxOs and
// returns both their TransactionInput IDs (for the transaction) and the
// resolved lcommon.Utxo set (as would be supplied by ledger state lookups).
func newTxInputCompareAllocInputs(
	t *testing.T,
	n int,
) ([]lcommon.TransactionInput, []lcommon.Utxo) {
	t.Helper()
	inputs := make([]lcommon.TransactionInput, n)
	resolved := make([]lcommon.Utxo, n)
	for i := range n {
		// Give each input a distinct transaction ID so resolution actually
		// scans rather than matching the first candidate every time.
		txId := bytes.Repeat([]byte{byte(i + 1)}, lcommon.Blake2b256Size)
		utxo, err := mockledger.NewUtxoBuilder().
			WithTxId(txId).
			WithIndex(0).
			WithAddress(benchmarkKeyAddress).
			WithLovelace(2_000_000).
			Build()
		require.NoError(t, err)
		inputs[i] = utxo.Id
		resolved[i] = utxo
	}
	return inputs, resolved
}

// TestNewTxInfoV2InputResolutionAllocationBound guards against expandInputs
// (context.go, used by NewTxInfoV2FromTransaction/V3 to build Inputs and
// ReferenceInputs) reintroducing its former String()-based equality check.
// That check formatted both operands (hex-encode + Sprintf) on every one of
// the O(inputs x resolvedInputs) comparisons in this loop.
//
// A live CPU/heap profile taken from a from-genesis Preview sync
// (dingo-perf-no-koios, profile/heap endpoints) attributed roughly a quarter
// of all sampled allocated objects to ShelleyTransactionInput.String, ~60%
// of which was reached from expandInputs; the CPU profile's top consumer
// (runtime.spanClass.sizeclass, ~26% flat) was GC background-mark scanning
// driven by this allocation volume. Comparing by (TxId, Index) directly is
// allocation-free, since both are compared by value.
//
// This drives the fix through the same public entry point ledger validation
// uses (NewTxInfoV2FromTransaction), rather than calling the unexported
// expandInputs directly, so the fixture is a real mockledger-built
// transaction rather than an inline mock (see AGENTS.md: mock fixtures come
// from ouroboros-mock).
func TestNewTxInfoV2InputResolutionAllocationBound(t *testing.T) {
	inputs, resolved := newTxInputCompareAllocInputs(
		t,
		txInputCompareAllocFixtureSize,
	)
	tx := mockledger.NewTransactionBuilder()
	tx.WithInputs(inputs...)
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(benchmarkKeyAddress).
		WithLovelace(1_000_000).
		Build()
	require.NoError(t, err)
	tx.WithOutputs(output)
	builtTx, err := tx.Build()
	require.NoError(t, err)

	// Sanity check: every input actually resolves, so the allocation count
	// below reflects a full, successful scan rather than an early bail-out.
	txInfo, err := script.NewTxInfoV2FromTransaction(
		mockSlotState{}, builtTx, resolved, false, lcommon.ProtocolVersionDijkstra,
	)
	require.NoError(t, err)
	require.Len(t, txInfo.Inputs, txInputCompareAllocFixtureSize)

	allocs := testing.AllocsPerRun(20, func() {
		_, err := script.NewTxInfoV2FromTransaction(
			mockSlotState{}, builtTx, resolved, false, lcommon.ProtocolVersionDijkstra,
		)
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Lessf(
		t,
		allocs,
		float64(2500),
		"NewTxInfoV2FromTransaction allocated %.1f objects/op resolving %d "+
			"inputs; expected a bound well under the O(n^2) String()-based "+
			"comparison cost",
		allocs,
		txInputCompareAllocFixtureSize,
	)
}

// TestNewTxInfoV2SpendRedeemerAllocationBound covers the sibling
// String()-based comparison in scriptPurposeBuilder's spend-redeemer
// resolution (purpose.go), exercised through the same public
// NewTxInfoV2FromTransaction entry point by adding one spend redeemer.
func TestNewTxInfoV2SpendRedeemerAllocationBound(t *testing.T) {
	inputs, resolved := newTxInputCompareAllocInputs(
		t,
		txInputCompareAllocFixtureSize,
	)
	tx := mockledger.NewTransactionBuilder()
	tx.WithInputs(inputs...)
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(benchmarkKeyAddress).
		WithLovelace(1_000_000).
		Build()
	require.NoError(t, err)
	tx.WithOutputs(output)
	// Redeem the last input so the resolution scan runs to the end of
	// resolvedInputs on every call, matching the worst case measured live.
	lastIndex := uint32(txInputCompareAllocFixtureSize - 1)
	redeemers := conway.ConwayRedeemers{
		Redeemers: map[lcommon.RedeemerKey]lcommon.RedeemerValue{
			{Tag: lcommon.RedeemerTagSpend, Index: lastIndex}: {},
		},
	}
	tx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithRedeemers(redeemers),
	)
	builtTx, err := tx.Build()
	require.NoError(t, err)

	txInfo, err := script.NewTxInfoV2FromTransaction(
		mockSlotState{}, builtTx, resolved, false, lcommon.ProtocolVersionDijkstra,
	)
	require.NoError(t, err)
	require.Len(t, txInfo.Redeemers, 1)

	allocs := testing.AllocsPerRun(20, func() {
		_, err := script.NewTxInfoV2FromTransaction(
			mockSlotState{}, builtTx, resolved, false, lcommon.ProtocolVersionDijkstra,
		)
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Lessf(
		t,
		allocs,
		float64(2500),
		"NewTxInfoV2FromTransaction allocated %.1f objects/op resolving one "+
			"spend redeemer against %d candidates; expected a bound well "+
			"under the O(n^2) String()-based comparison cost",
		allocs,
		txInputCompareAllocFixtureSize,
	)
}

// TestResolvedInputEquals covers script.ResolvedInput.Equals directly: the
// single definition of the (TxId, Index) comparison expandInputs and
// scriptPurposeBuilder both call through, so a future change to what
// "the same input" means only needs to change in one place.
func TestResolvedInputEquals(t *testing.T) {
	txIdA := bytes.Repeat([]byte{0xaa}, lcommon.Blake2b256Size)
	txIdB := bytes.Repeat([]byte{0xbb}, lcommon.Blake2b256Size)

	utxoA0, err := mockledger.NewUtxoBuilder().
		WithTxId(txIdA).
		WithIndex(0).
		WithAddress(benchmarkKeyAddress).
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)
	utxoA1, err := mockledger.NewUtxoBuilder().
		WithTxId(txIdA).
		WithIndex(1).
		WithAddress(benchmarkKeyAddress).
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)
	utxoB0, err := mockledger.NewUtxoBuilder().
		WithTxId(txIdB).
		WithIndex(0).
		WithAddress(benchmarkKeyAddress).
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)

	resolvedA0 := script.ResolvedInput(utxoA0)

	require.True(
		t,
		resolvedA0.Equals(utxoA0.Id),
		"same TxId and Index must match",
	)
	require.False(
		t,
		resolvedA0.Equals(utxoA1.Id),
		"same TxId but different Index must not match",
	)
	require.False(
		t,
		resolvedA0.Equals(utxoB0.Id),
		"different TxId but same Index must not match",
	)
}
