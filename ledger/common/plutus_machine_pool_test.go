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
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"

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
	key := machineCheckoutKey{
		version:     version,
		evalContext: weak.Make(evalContext),
	}
	if entryIface, ok := machinePools.Load(key); ok {
		return entryIface.(*machinePoolEntry).constructed.Load()
	}
	return 0
}

// checkoutMachine is checkoutMachineReused without the reuse flag.
func checkoutMachine(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) *cek.Machine[syn.DeBruijn] {
	machine, _ := checkoutMachineReused(version, evalContext)
	return machine
}

// buildMachinePoolTestV3Script builds a minimal PlutusV3 script -- a single
// lambda returning Unit, ignoring its scriptContext argument -- that always
// succeeds. It exists so the machine-pool tests below can drive real
// PlutusV3Script.Evaluate calls without depending on the encode helpers
// defined in the external common_test package.
func buildMachinePoolTestV3Script(t testing.TB) PlutusV3Script {
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

// pooledTestEvalContext returns a PooledEvalContext for PlutusV3 with
// non-zero costs. Each test passes its own protoMajor so parallel tests never
// share a cached EvalContext or its machine pool.
func pooledTestEvalContext(t *testing.T, protoMajor uint) *cek.EvalContext {
	t.Helper()
	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = int64(i + 1)
	}
	evalContext, err := PooledEvalContext(
		lang.LanguageVersionV3,
		protoMajor,
		params,
	)
	require.NoError(t, err)
	return evalContext
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

	evalContextA := pooledTestEvalContext(t, 900002)
	evalContextB := pooledTestEvalContext(t, 900003)
	require.NotSame(t, evalContextA, evalContextB,
		"test fixture bug: two distinct tuples must not alias")

	// The pooled EvalContexts outlive one test run (-count > 1), so the
	// construction counters are compared as deltas.
	beforeA := machineConstructionCount(lang.LanguageVersionV3, evalContextA)
	beforeB := machineConstructionCount(lang.LanguageVersionV3, evalContextB)

	// Distinct tuples must never share a pooled Machine.
	mA := checkoutMachine(lang.LanguageVersionV3, evalContextA)
	mB := checkoutMachine(lang.LanguageVersionV3, evalContextB)
	require.NotSame(t, mA, mB,
		"two distinct EvalContext tuples must never share a pooled Machine")
	releaseMachine(lang.LanguageVersionV3, evalContextA, mA)
	releaseMachine(lang.LanguageVersionV3, evalContextB, mB)
	require.LessOrEqual(
		t,
		machineConstructionCount(lang.LanguageVersionV3, evalContextA)-beforeA,
		int64(1),
	)
	require.LessOrEqual(
		t,
		machineConstructionCount(lang.LanguageVersionV3, evalContextB)-beforeB,
		int64(1),
	)
	beforeLoop := machineConstructionCount(
		lang.LanguageVersionV3,
		evalContextA,
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
	) - beforeLoop
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
	evalContext := pooledTestEvalContext(t, 900004)

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
	evalContext := pooledTestEvalContext(t, 900005)
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

	evalContext := pooledTestEvalContext(t, 900006)

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

// TestMachinePoolEnforcesBudgetEqualToPreviousRemaining pins a reuse hazard in
// plutigo's Run: on a Machine that has already run, an ExBudget equal to the
// previous Run's remaining budget is read as "unchanged" and replaced with the
// previous Run's starting budget. A pooled Machine must therefore never run a
// redeemer whose declared budget equals the remaining budget the Machine was
// released with, or that redeemer executes against another redeemer's budget.
func TestMachinePoolEnforcesBudgetEqualToPreviousRemaining(t *testing.T) {
	t.Parallel()

	script := buildMachinePoolTestV3Script(t)
	evalContext := pooledTestEvalContext(t, 900007)
	used, err := script.Evaluate(
		machinePoolTestScriptContext(),
		machinePoolTestBudget(),
		evalContext,
	)
	require.NoError(t, err)
	require.Positive(t, used.Memory)
	require.Positive(t, used.Steps)

	// first leaves exactly short remaining; short cannot cover the script.
	first := ExUnits{Memory: 2*used.Memory - 1, Steps: 2*used.Steps - 1}
	short := ExUnits{Memory: used.Memory - 1, Steps: used.Steps - 1}
	for i := range 50 {
		_, err := script.Evaluate(
			machinePoolTestScriptContext(),
			first,
			evalContext,
		)
		require.NoError(t, err, "iteration %d", i)
		_, err = script.Evaluate(
			machinePoolTestScriptContext(),
			short,
			evalContext,
		)
		require.Error(t, err,
			"iteration %d: budget %+v below the script's cost %+v must fail",
			i, short, used)
	}
}

// callerBuiltTestEvalContext builds a PlutusV3 *cek.EvalContext directly with
// cek.NewEvalContext, the way a caller outside this package does, rather than
// through PooledEvalContext.
func callerBuiltTestEvalContext(t *testing.T) *cek.EvalContext {
	t.Helper()
	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = int64(i + 1)
	}
	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV3,
		cek.ProtoVersion{Major: 10},
		params,
	)
	require.NoError(t, err)
	return evalContext
}

// TestMachinePoolReusesMachineForCallerBuiltEvalContext pins that Machine
// reuse depends only on the caller passing the same *cek.EvalContext again,
// not on where it came from: a caller that builds and caches its own
// EvalContext must get pooled Machines from Evaluate. The bound is loose for
// the reason given on TestMachinePoolReusesCheckedInMachine.
func TestMachinePoolReusesMachineForCallerBuiltEvalContext(t *testing.T) {
	t.Parallel()

	script := buildMachinePoolTestV3Script(t)
	evalContext := callerBuiltTestEvalContext(t)
	const iterations = 200
	for i := range iterations {
		_, err := script.Evaluate(
			machinePoolTestScriptContext(),
			machinePoolTestBudget(),
			evalContext,
		)
		require.NoError(t, err, "iteration %d", i)
	}
	constructed := machineConstructionCount(
		lang.LanguageVersionV3,
		evalContext,
	)
	require.Positive(t, constructed,
		"a caller-built EvalContext must be pooled")
	require.Less(t, constructed, int64(iterations),
		"repeated Evaluate calls with one caller-built EvalContext must"+
			" reuse Machines rather than construct one per call")
}

// TestMachinePoolReleasesEntriesForCollectedEvalContexts pins the retention
// bound for a caller that builds a fresh *cek.EvalContext per evaluation: each
// evaluation stays correct, and every pool entry it creates is removed once
// its EvalContext is garbage collected, so entries do not accumulate over a
// long-running node's lifetime.
func TestMachinePoolReleasesEntriesForCollectedEvalContexts(t *testing.T) {
	t.Parallel()

	script := buildMachinePoolTestV3Script(t)
	want, err := script.Evaluate(
		machinePoolTestScriptContext(),
		machinePoolTestBudget(),
		callerBuiltTestEvalContext(t),
	)
	require.NoError(t, err)

	const contexts = 200
	keys := make([]machineCheckoutKey, 0, contexts)
	for i := range contexts {
		evalContext := callerBuiltTestEvalContext(t)
		got, err := script.Evaluate(
			machinePoolTestScriptContext(),
			machinePoolTestBudget(),
			evalContext,
		)
		require.NoError(t, err, "iteration %d", i)
		require.Equal(t, want, got, "iteration %d", i)
		key := machineCheckoutKey{
			version:     lang.LanguageVersionV3,
			evalContext: weak.Make(evalContext),
		}
		_, ok := machinePools.Load(key)
		require.True(t, ok,
			"iteration %d: a live EvalContext must have a pool entry", i)
		keys = append(keys, key)
	}

	retained := func() int {
		n := 0
		for _, key := range keys {
			if _, ok := machinePools.Load(key); ok {
				n++
			}
		}
		return n
	}
	deadline := time.Now().Add(5 * time.Second)
	for retained() > 0 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	require.Zero(t, retained(),
		"pool entries for collected EvalContexts must be removed")
}

// TestPooledEvalContextCacheIsBounded pins the retention bound on
// PooledEvalContext: a sequence of distinct cost model lists, the shape
// repeated governance updates take, evicts the least recently used tuple
// instead of retaining every list for the process lifetime, and an evicted
// EvalContext still held by a caller keeps evaluating correctly.
//
// It is not parallel: parallel tests in this package rely on their own cached
// tuples staying resident, and this test deliberately overflows the cache.
func TestPooledEvalContextCacheIsBounded(t *testing.T) {
	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = int64(i + 1)
	}
	const baseMajor = 910000
	pooled := func(protoMajor uint) *cek.EvalContext {
		evalContext, err := PooledEvalContext(
			lang.LanguageVersionV3,
			protoMajor,
			params,
		)
		require.NoError(t, err)
		return evalContext
	}

	evicted := pooled(baseMajor)
	hot := pooled(baseMajor + 1)
	for i := range uint(evalContextCacheLimit) {
		pooled(baseMajor + 2 + i)
		require.Same(t, hot, pooled(baseMajor+1),
			"a recently used tuple must stay cached")
	}

	require.NotSame(t, evicted, pooled(baseMajor),
		"the least recently used tuple must be evicted once the cache is full")

	script := buildMachinePoolTestV3Script(t)
	want, err := script.Evaluate(
		machinePoolTestScriptContext(),
		machinePoolTestBudget(),
		pooled(baseMajor),
	)
	require.NoError(t, err)
	got, err := script.Evaluate(
		machinePoolTestScriptContext(),
		machinePoolTestBudget(),
		evicted,
	)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"an evicted EvalContext held by a caller must still evaluate")
}

// unknownTerm satisfies syn.Term through its embedded Term but is a type
// plutigo's evaluator does not recognise, so Run panics on it.
type unknownTerm struct {
	syn.Term[syn.DeBruijn]
}

// TestRunPooledMachineDropsMachineWhenRunPanics pins that a Machine whose Run
// panics is never returned to its pool, so a caller that recovers the panic
// cannot hand an interrupted Machine to the next evaluation.
func TestRunPooledMachineDropsMachineWhenRunPanics(t *testing.T) {
	t.Parallel()

	evalContext := callerBuiltTestEvalContext(t)
	budget := cek.ExBudget{
		Mem: cek.DefaultExBudget.Mem,
		Cpu: cek.DefaultExBudget.Cpu,
	}
	require.Panics(t, func() {
		_, _ = runPooledMachine(
			lang.LanguageVersionV3,
			evalContext,
			budget,
			unknownTerm{Term: &syn.Error{}},
		)
	})

	machine, reused := checkoutMachineReused(
		lang.LanguageVersionV3,
		evalContext,
	)
	require.False(t, reused,
		"a Machine whose Run panicked must not be returned to its pool")
	releaseMachine(lang.LanguageVersionV3, evalContext, machine)
}
