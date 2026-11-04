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

package common_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// inputCountingTransaction counts Inputs calls, which every build of a script
// view makes, to show whether a view was rebuilt.
type inputCountingTransaction struct {
	common.Transaction
	inputs atomic.Int32
}

func (t *inputCountingTransaction) Inputs() []common.TransactionInput {
	t.inputs.Add(1)
	return t.Transaction.Inputs()
}

type memoKeyA struct{}

type memoKeyB struct{}

// valueTransaction is a transaction that is not a pointer, so it has no
// object identity to memoize by.
type valueTransaction struct {
	common.Transaction
}

func TestTransactionScopedMemoIsPerTransactionObjectAndKey(t *testing.T) {
	scoped := common.NewTransactionScopedLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	memo, ok := scoped.(common.TransactionScopedMemoizer)
	require.True(t, ok)
	require.Same(t, scoped, common.NewTransactionScopedLedgerState(scoped))
	require.Nil(t, common.NewTransactionScopedLedgerState(nil))

	witnesses := mockledger.NewMockTransactionWitnessSet()
	first := plutusContextMockTransaction(t, witnesses)
	second := plutusContextMockTransaction(t, witnesses)
	builds := 0
	build := func() (any, error) {
		builds++
		return builds, nil
	}
	get := func(tx common.Transaction, key any) any {
		t.Helper()
		value, err := memo.TransactionScopedMemo(tx, key, build)
		require.NoError(t, err)
		return value
	}

	require.Equal(t, 1, get(first, memoKeyA{}))
	require.Equal(t, 1, get(first, memoKeyA{}))
	// An equal but distinct transaction object gets its own entry.
	require.Equal(t, 2, get(second, memoKeyA{}))
	require.Equal(t, 3, get(first, memoKeyB{}))
	require.Equal(t, 1, get(first, memoKeyA{}))
	// A transaction without object identity is never memoized.
	value := valueTransaction{Transaction: first}
	require.Equal(t, 4, get(value, memoKeyA{}))
	require.Equal(t, 5, get(value, memoKeyA{}))

	// A failure is shared like a value, so every rule sees the same one.
	failure := errors.New("build failed")
	failures := 0
	for range 2 {
		_, err := memo.TransactionScopedMemo(second, memoKeyB{},
			func() (any, error) {
				failures++
				return nil, failure
			})
		require.ErrorIs(t, err, failure)
	}
	require.Equal(t, 1, failures)
}

// TestTxScriptViewThroughScopedStateMatchesUnscoped checks that the shared
// view is the view each rule would otherwise build, including the partial
// view and the error reported when an input does not resolve.
func TestTxScriptViewThroughScopedStateMatchesUnscoped(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	tx := plutusContextMockTransaction(
		t,
		mockledger.NewMockTransactionWitnessSet(),
		plutusContextReferenceOutput(
			t,
			common.ScriptRefTypePlutusV3,
			common.PlutusV3Script{0x01},
		),
	)
	want, wantErr := script.NewTxScriptView(tx, ls)
	counted := &inputCountingTransaction{Transaction: tx}
	scoped := common.NewTransactionScopedLedgerState(ls)
	for range 2 {
		got, err := script.NewTxScriptView(counted, scoped)
		require.Equal(t, want, got)
		if wantErr == nil {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, wantErr.Error())
		}
	}
	// Building the view reads the inputs, so a memo hit reads them once.
	firstBuild := counted.inputs.Load()
	require.NotZero(t, firstBuild)
	_, _ = script.NewTxScriptView(counted, scoped)
	require.Equal(t, firstBuild, counted.inputs.Load())
}

func TestTransactionScopedMemoNonComparableKeyIsNotMemoized(t *testing.T) {
	scoped := common.NewTransactionScopedLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	memo := scoped.(common.TransactionScopedMemoizer)
	tx := plutusContextMockTransaction(
		t,
		mockledger.NewMockTransactionWitnessSet(),
	)
	builds := 0
	for range 3 {
		require.NotPanics(t, func() {
			_, err := memo.TransactionScopedMemo(
				tx,
				[]int{1},
				func() (any, error) { builds++; return nil, nil },
			)
			require.NoError(t, err)
		})
	}
	require.Equal(t, 3, builds)
}

func TestTransactionScopedMemoConcurrentCallersShareOneBuild(t *testing.T) {
	scoped := common.NewTransactionScopedLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	memo := scoped.(common.TransactionScopedMemoizer)
	tx := plutusContextMockTransaction(
		t,
		mockledger.NewMockTransactionWitnessSet(),
	)
	var builds atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := memo.TransactionScopedMemo(
				tx,
				memoKeyA{},
				func() (any, error) {
					builds.Add(1)
					<-release
					return "built", nil
				},
			)
			require.NoError(t, err)
			require.Equal(t, "built", value)
		}()
	}
	close(release)
	wg.Wait()
	require.Equal(t, int32(1), builds.Load())
}

type forwardingMemoizer struct {
	common.LedgerState
	common.TransactionScopedMemoizer
}

func TestIsTransactionScopedLedgerState(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	scoped := common.NewTransactionScopedLedgerState(ls)
	require.False(t, common.IsTransactionScopedLedgerState(ls))
	require.True(t, common.IsTransactionScopedLedgerState(scoped))
	adapter := forwardingMemoizer{
		LedgerState:               scoped,
		TransactionScopedMemoizer: scoped.(common.TransactionScopedMemoizer),
	}
	require.False(t, common.IsTransactionScopedLedgerState(adapter))
}
