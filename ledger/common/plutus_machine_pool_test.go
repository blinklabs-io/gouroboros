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

package common

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

// machineConstructionCount reports how many times cek.NewMachine has actually
// run for (version, evalContext), or 0 if no checkout has happened yet for
// that tuple. Test-only introspection into the pool's internal counter.
func machineConstructionCount(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) int64 {
	key := machineCheckoutKey{version: version, evalContext: evalContext}
	if entryIface, ok := machinePools.Load(key); ok {
		return entryIface.(*machinePoolEntry).constructed.Load()
	}
	return 0
}

// buildMachinePoolTestV3Script builds a minimal PlutusV3 script -- a single
// lambda returning Unit, ignoring its scriptContext argument -- that always
// succeeds. It exists so the machine-pool tests below can drive real
// PlutusV3Script.Evaluate calls without depending on the encode helpers
// defined in the external common_test package.
func buildMachinePoolTestV3Script(t *testing.T) PlutusV3Script {
	t.Helper()
	term := &syn.Lambda[syn.DeBruijn]{
		Body: &syn.Constant{Con: &syn.Unit{}},
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		// The UPLC wire version (distinct from the Plutus ledger language
		// this script targets): 1.0.0 is accepted unconditionally, unlike
		// 1.2.0 which the van Rossem gate requires a high protocol major
		// version for. See syn.ValidateTermVersionForExecution.
		Version: lang.LanguageVersion{1, 0, 0},
		Term:    term,
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return PlutusV3Script(wrapper)
}

func machinePoolTestBudget() ExUnits {
	return ExUnits{
		Memory: cek.DefaultExBudget.Mem,
		Steps:  cek.DefaultExBudget.Cpu,
	}
}

func machinePoolTestScriptContext() data.PlutusData {
	return &data.Constr{Tag: big.NewInt(0)}
}

// TestPooledEvalContextReusesPointerForSameTuple proves PooledEvalContext
// caches by (version, protoMajor, cost model params): repeated calls with an
// identical tuple return the identical *cek.EvalContext pointer, and a
// changed cost model parameter -- the shape a governance-driven cost model
// update takes -- produces a distinct one rather than a stale hit.
func TestPooledEvalContextReusesPointerForSameTuple(t *testing.T) {
	t.Parallel()

	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = int64(i + 1)
	}
	// protoMajor is part of the test's own cache key, so distinct subtests
	// sharing the package-level evalContextCache do not collide with each
	// other or with other parallel tests in this package.
	const protoMajor = 900001

	first, err := PooledEvalContext(lang.LanguageVersionV3, protoMajor, params)
	require.NoError(t, err)
	second, err := PooledEvalContext(lang.LanguageVersionV3, protoMajor, params)
	require.NoError(t, err)
	require.Same(t, first, second,
		"PooledEvalContext must return the same pointer for an identical tuple")

	changedParams := append([]int64(nil), params...)
	changedParams[0]++
	third, err := PooledEvalContext(
		lang.LanguageVersionV3,
		protoMajor,
		changedParams,
	)
	require.NoError(t, err)
	require.NotSame(
		t,
		first,
		third,
		"a changed cost model parameter must miss the cache, not reuse"+
			" a stale EvalContext",
	)
}

// TestMachinePoolReusesCheckedInMachine is the direct proof for gouroboros#2554
// acceptance criterion 1: cek.NewMachine is called at most once per distinct
// (version, evalContext) tuple live in the pool.
//
// It does not assert strict Machine pointer reuse across a checkout/release
// pair: sync.Pool.Put is allowed to drop a released item instead of retaining
// it, and does so at random whenever the race detector is enabled (see
// machinePoolEntry's doc comment), which is exactly how this repository's
// own `make test` runs every package. Asserting "the very next checkout
// returns the same pointer" would therefore be flaky under -race. Instead it
// asserts the weaker, race-safe claim the sync.Pool contract actually
// supports: across many sequential checkout/release cycles against one
// tuple, cek.NewMachine runs far fewer times than there are checkouts,
// proving reuse is happening rather than a fresh Machine being constructed
// on every single call the way the pre-fix code always did.
//
// A different tuple must also never be handed the other tuple's Machine --
// unlike the reuse claim, this holds deterministically regardless of -race,
// because sync.Pool.Get can only ever return an item previously Put into that
// same Pool, and every tuple owns its own Pool.
func TestMachinePoolReusesCheckedInMachine(t *testing.T) {
	t.Parallel()

	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	evalContextA, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		params,
	)
	require.NoError(t, err)
	evalContextB, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		params,
	)
	require.NoError(t, err)
	require.NotSame(t, evalContextA, evalContextB,
		"test fixture bug: two independent NewEvalContext calls must not alias")

	// Distinct tuples must never share a pooled Machine.
	mA := checkoutMachine(lang.LanguageVersionV3, evalContextA)
	mB := checkoutMachine(lang.LanguageVersionV3, evalContextB)
	require.NotSame(t, mA, mB,
		"two distinct EvalContext tuples must never share a pooled Machine")
	releaseMachine(lang.LanguageVersionV3, evalContextA, mA)
	releaseMachine(lang.LanguageVersionV3, evalContextB, mB)
	require.Equal(
		t,
		int64(1),
		machineConstructionCount(lang.LanguageVersionV3, evalContextA),
	)
	require.Equal(
		t,
		int64(1),
		machineConstructionCount(lang.LanguageVersionV3, evalContextB),
	)

	// Repeated checkout/release against one tuple (evalContextA, which
	// already has one Machine released into its pool above) must construct
	// far fewer Machines than the number of checkouts.
	const iterations = 200
	for range iterations {
		m := checkoutMachine(lang.LanguageVersionV3, evalContextA)
		releaseMachine(lang.LanguageVersionV3, evalContextA, m)
	}
	constructed := machineConstructionCount(
		lang.LanguageVersionV3,
		evalContextA,
	)
	require.Less(t, constructed, int64(iterations),
		"repeated checkouts against one tuple must reuse the released Machine"+
			" most of the time, not construct fresh cek.NewMachine on every checkout")
}

// TestMachinePoolConcurrentEvaluationIsRaceFree is the proof for acceptance
// criterion 3: many goroutines evaluating the same PlutusV3 script under the
// same shared *cek.EvalContext -- as every redeemer of one script version
// within a transaction now does, via PooledEvalContext -- must never observe
// a corrupted or shared in-flight Machine. Every goroutine's result and
// consumed budget must match what a single-threaded evaluation produces. Run
// with -race.
func TestMachinePoolConcurrentEvaluationIsRaceFree(t *testing.T) {
	t.Parallel()

	script := buildMachinePoolTestV3Script(t)
	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		make(
			[]int64,
			len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
		),
	)
	require.NoError(t, err)

	wantUnits, err := script.Evaluate(
		machinePoolTestScriptContext(),
		machinePoolTestBudget(),
		evalContext,
	)
	require.NoError(t, err)

	const goroutines = 16
	const iterations = 25
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				gotUnits, err := script.Evaluate(
					machinePoolTestScriptContext(),
					machinePoolTestBudget(),
					evalContext,
				)
				if err != nil {
					errs <- err
					continue
				}
				if gotUnits != wantUnits {
					errs <- fmt.Errorf(
						"concurrent evaluation diverged: got %+v, want %+v",
						gotUnits,
						wantUnits,
					)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestMachinePoolReducesAllocationsVsUnpooled is the control-comparison proof
// for gouroboros#2554 acceptance criterion 2: repeated evaluation against one
// (version, evalContext) tuple must allocate less through the pooled
// checkout/release path than through a construct-and-discard control built
// the way Evaluate always worked before this pool existed. Both sides run the
// identical decoded program and budget in the same process, differing only
// in whether the Machine is reused, rather than asserting an absolute
// allocation count that would vary by Go version or platform.
//
// Not t.Parallel: testing.AllocsPerRun measures allocations across many
// sequential sub-calls made by the calling goroutine; a concurrently running
// sibling test would add its own allocations to the same process-wide
// accounting and corrupt the comparison.
func TestMachinePoolReducesAllocationsVsUnpooled(t *testing.T) {
	script := buildMachinePoolTestV3Script(t)
	innerScript, err := decodePlutusScript([]byte(script), true)
	require.NoError(t, err)
	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		make(
			[]int64,
			len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
		),
	)
	require.NoError(t, err)
	program, err := decodePlutusProgram(
		innerScript,
		lang.LanguageVersionV3,
		evalContext,
	)
	require.NoError(t, err)
	contextTerm := &syn.Constant{
		Con: &syn.Data{Inner: machinePoolTestScriptContext()},
	}
	wrappedProgram := &syn.Apply[syn.DeBruijn]{
		Function: program.Term,
		Argument: contextTerm,
	}
	budget := exBudgetFromUnits(machinePoolTestBudget())

	// Warm the pool so the pooled measurement below is not charged for its
	// own one-time first cek.NewMachine call.
	warm := checkoutMachine(lang.LanguageVersionV3, evalContext)
	releaseMachine(lang.LanguageVersionV3, evalContext, warm)

	pooledAllocs := testing.AllocsPerRun(200, func() {
		m := checkoutMachine(lang.LanguageVersionV3, evalContext)
		m.ExBudget = budget
		if _, err := m.Run(wrappedProgram); err != nil {
			t.Fatal(err)
		}
		releaseMachine(lang.LanguageVersionV3, evalContext, m)
	})

	unpooledAllocs := testing.AllocsPerRun(200, func() {
		m := cek.NewMachine[syn.DeBruijn](
			cek.LanguageVersionV3,
			200,
			evalContext,
		)
		m.ExBudget = budget
		if _, err := m.Run(wrappedProgram); err != nil {
			t.Fatal(err)
		}
	})

	t.Logf(
		"pooled: %.1f allocs/op, unpooled control: %.1f allocs/op",
		pooledAllocs,
		unpooledAllocs,
	)
	require.Less(t, pooledAllocs, unpooledAllocs,
		"reusing a pooled Machine across repeated evaluations of one tuple"+
			" must allocate less than constructing a fresh cek.Machine on"+
			" every evaluation and discarding it, the pre-fix behavior")
}

// TestMachinePoolChecksOutFreshMachineWhenPoolEmpty documents the
// no-shared-singleton correctness constraint: two concurrently *outstanding*
// checkouts against the same tuple (neither released yet) must never be the
// same instance, since the caller runs Run() against each independently.
// sync.Pool.Get only ever returns a pooled item to one caller at a time, so
// this must build a second Machine rather than reuse the first.
func TestMachinePoolChecksOutFreshMachineWhenPoolEmpty(t *testing.T) {
	t.Parallel()

	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		make(
			[]int64,
			len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
		),
	)
	require.NoError(t, err)

	m1 := checkoutMachine(lang.LanguageVersionV3, evalContext)
	m2 := checkoutMachine(lang.LanguageVersionV3, evalContext)
	require.NotSame(
		t,
		m1,
		m2,
		"two simultaneously outstanding checkouts must never alias the same Machine",
	)
	releaseMachine(lang.LanguageVersionV3, evalContext, m1)
	releaseMachine(lang.LanguageVersionV3, evalContext, m2)
}
