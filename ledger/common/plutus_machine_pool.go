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
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

// evalContextKey identifies the (language version, protocol major version,
// cost model parameter list) tuple a *cek.EvalContext -- and therefore any
// *cek.Machine built from it -- is derived from. ProtoVersion.Minor is
// excluded: cek.GetSemantics (and every other reachable use of ProtoVersion)
// switches only on Major, per the reuse guarantee documented on
// cek.EvalContext.
type evalContextKey struct {
	version    lang.LanguageVersion
	protoMajor uint
	costModels string
}

// encodeCostModelParams turns a cost model parameter list into a comparable
// map-key string. The exact parameter values must distinguish an
// otherwise-identical (version, protoMajor) tuple across a governance-driven
// cost model update, so the full list is encoded rather than hashed or
// summarized.
func encodeCostModelParams(params []int64) string {
	buf := make([]byte, len(params)*8)
	for i, p := range params {
		// Bit-pattern reinterpretation for a map key, not an arithmetic
		// conversion: every int64 value, negative or not, must round-trip
		// through its 8 bytes unchanged so distinct parameter lists never
		// collide.
		binary.LittleEndian.PutUint64(buf[i*8:], uint64(p)) //nolint:gosec
	}
	return string(buf)
}

// evalContextCache holds one *cek.EvalContext per evalContextKey, process-wide
// and never evicted: the number of distinct (version, protoMajor,
// cost-model-list) tuples live on a running chain is small and each entry is
// small, the same tradeoff already made for the package's cached CBOR
// EncMode/DecMode.
var evalContextCache sync.Map // evalContextKey -> *cek.EvalContext

// PooledEvalContext returns a *cek.EvalContext for the given (language
// version, protocol major version, cost model parameter list) tuple,
// building and caching one on first use so every caller sharing the tuple
// receives the identical *cek.EvalContext pointer.
//
// Reusing the pointer is what lets the machine pool below (see
// checkoutMachine) recognize repeat evaluations of the same tuple and check
// out a pooled *cek.Machine instead of constructing and discarding one per
// redeemer (gouroboros#2554). Machines are pooled only for EvalContexts
// returned here. A caller that instead builds its own *cek.EvalContext -- via
// cek.NewEvalContext directly, or by passing nil -- gets a fresh Machine per
// evaluation and leaves nothing retained, which is the pre-existing behavior.
func PooledEvalContext(
	version lang.LanguageVersion,
	protoMajor uint,
	costModelParams []int64,
) (*cek.EvalContext, error) {
	key := evalContextKey{
		version:    version,
		protoMajor: protoMajor,
		costModels: encodeCostModelParams(costModelParams),
	}
	if cached, ok := evalContextCache.Load(key); ok {
		return cached.(*cek.EvalContext), nil
	}
	built, err := cek.NewEvalContext(
		version,
		cek.ProtoVersion{Major: protoMajor},
		costModelParams,
	)
	if err != nil {
		return nil, err
	}
	actualIface, _ := evalContextCache.LoadOrStore(key, built)
	actual := actualIface.(*cek.EvalContext)
	registerMachinePool(version, actual)
	return actual, nil
}

// machineCheckoutKey pairs the language version passed to cek.NewMachine with
// the *cek.EvalContext pointer used to build it. A Machine's builtins,
// per-step costs, and available-builtin table (cek.NewMachine) are derived
// only from that (version, evalContext) pair, so two checkouts sharing both
// are interchangeable. Keying on the pair rather than the evalContext pointer
// alone means an inconsistent caller -- reusing one *cek.EvalContext across
// two different language versions, which nothing in this package does but
// nothing prevents either -- still gets a correct Machine for each, rather
// than silently reusing whichever was pooled first.
type machineCheckoutKey struct {
	version     lang.LanguageVersion
	evalContext *cek.EvalContext
}

// machinePoolEntry is the sync.Pool of *cek.Machine[syn.DeBruijn] instances
// built for one machineCheckoutKey, plus a count of how many times its New
// has actually run. The count exists so a test (or an operator) can observe
// how effective reuse is being for a tuple without relying on Machine pointer
// identity, which sync.Pool does not guarantee to preserve: Put is allowed to
// drop an item instead of retaining it (the runtime does exactly this, at
// random, whenever the race detector is enabled -- see runtime/race branches
// in sync.Pool's Put/Get -- specifically to stop callers from assuming
// otherwise), and a GC cycle can clear the pool entirely at any time.
type machinePoolEntry struct {
	pool        *sync.Pool
	constructed atomic.Int64
}

// machinePools maps a machineCheckoutKey to its machinePoolEntry. Entries are
// created only by registerMachinePool, for EvalContexts held in
// evalContextCache, so the map is bounded by that cache. Creating an entry on
// checkout instead would pin every caller-built *cek.EvalContext forever.
var machinePools sync.Map // machineCheckoutKey -> *machinePoolEntry

// checkoutMachine returns a *cek.Machine[syn.DeBruijn] built for (version,
// evalContext), reusing a previously released one when the pool for that key
// has one available, or a fresh unpooled one when evalContext was not issued
// by PooledEvalContext. The caller owns the returned Machine exclusively until
// it calls releaseMachine; sync.Pool.Get never hands out the same instance to
// two concurrent callers, so two evaluations sharing the tuple never observe
// each other's in-progress Run.
func checkoutMachine(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) *cek.Machine[syn.DeBruijn] {
	entry := loadMachinePool(version, evalContext)
	if entry == nil {
		return cek.NewMachine[syn.DeBruijn](version, 200, evalContext)
	}
	machine, _ := entry.pool.Get().(*cek.Machine[syn.DeBruijn])
	return machine
}

// checkoutMachineWithBudget checks out a Machine as checkoutMachine does and
// sets its ExBudget to budget, ready for one Run.
//
// A Machine that has already run treats an ExBudget equal to its previous
// Run's remaining budget as unchanged and restores the previous Run's starting
// budget instead (plutigo cek.Machine.runContext). A pooled Machine is
// released holding exactly that remaining budget, so when the next redeemer
// declares the same value it would run against the previous redeemer's
// budget. Such a Machine is replaced with a fresh one, which has never run and
// so always honours the budget it is given.
func checkoutMachineWithBudget(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
	budget cek.ExBudget,
) *cek.Machine[syn.DeBruijn] {
	machine := checkoutMachine(version, evalContext)
	if machine.ExBudget == budget {
		machine = cek.NewMachine[syn.DeBruijn](version, 200, evalContext)
		if entry := loadMachinePool(version, evalContext); entry != nil {
			entry.constructed.Add(1)
		}
	}
	machine.ExBudget = budget
	return machine
}

func registerMachinePool(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) {
	key := machineCheckoutKey{version: version, evalContext: evalContext}
	if _, ok := machinePools.Load(key); ok {
		return
	}
	entry := &machinePoolEntry{}
	entry.pool = &sync.Pool{
		New: func() any {
			entry.constructed.Add(1)
			return cek.NewMachine[syn.DeBruijn](version, 200, evalContext)
		},
	}
	machinePools.LoadOrStore(key, entry)
}

func loadMachinePool(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) *machinePoolEntry {
	key := machineCheckoutKey{version: version, evalContext: evalContext}
	if entryIface, ok := machinePools.Load(key); ok {
		entry, _ := entryIface.(*machinePoolEntry)
		return entry
	}
	return nil
}

// releaseMachine returns a checked-out Machine to its pool, or drops it when
// evalContext has no pool. Only call it after the Machine's Run has fully
// returned: Run's per-call state (ExBudget, Logs, arena reuse via
// lazyPrepareValueArenas/lazyPrepareEnvArena) assumes exclusive ownership for
// the duration of one Run, and handing the same Machine to a second checkout
// before the first Run returns would let two evaluations mutate it
// concurrently.
func releaseMachine(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
	machine *cek.Machine[syn.DeBruijn],
) {
	if entry := loadMachinePool(version, evalContext); entry != nil {
		entry.pool.Put(machine)
	}
}
