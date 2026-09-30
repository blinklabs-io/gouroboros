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
	"container/list"
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

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

// evalContextCacheLimit bounds evalContextCache. A running chain has at most
// one live cost model list per Plutus language version, plus the previous
// list around a governance update, so this leaves room for every tuple in
// use across a protocol version change while capping what a sequence of cost
// model updates can retain over a node's lifetime.
const evalContextCacheLimit = 16

// evalContextCacheEntry is one evalContextCache slot, linked into the cache's
// recency list.
type evalContextCacheEntry struct {
	key         evalContextKey
	evalContext *cek.EvalContext
}

// evalContextCache holds at most evalContextCacheLimit *cek.EvalContext
// values, evicting the least recently used. Eviction only drops the cache's
// reference: a caller still holding an evicted EvalContext keeps using it, and
// its machine pool entry is removed once that caller releases it (see
// machinePools).
var evalContextCache = struct {
	sync.Mutex
	entries map[evalContextKey]*list.Element // value: *evalContextCacheEntry
	recency list.List                        // front: most recently used
}{
	entries: make(map[evalContextKey]*list.Element),
}

func loadEvalContext(key evalContextKey) (*cek.EvalContext, bool) {
	evalContextCache.Lock()
	defer evalContextCache.Unlock()
	elem, ok := evalContextCache.entries[key]
	if !ok {
		return nil, false
	}
	evalContextCache.recency.MoveToFront(elem)
	return elem.Value.(*evalContextCacheEntry).evalContext, true
}

// loadOrStoreEvalContext stores built under key unless a concurrent caller
// stored one first, and returns whichever is cached, so every caller that
// raced on the same miss receives the same pointer.
func loadOrStoreEvalContext(
	key evalContextKey,
	built *cek.EvalContext,
) *cek.EvalContext {
	evalContextCache.Lock()
	defer evalContextCache.Unlock()
	if elem, ok := evalContextCache.entries[key]; ok {
		evalContextCache.recency.MoveToFront(elem)
		return elem.Value.(*evalContextCacheEntry).evalContext
	}
	evalContextCache.entries[key] = evalContextCache.recency.PushFront(
		&evalContextCacheEntry{key: key, evalContext: built},
	)
	for evalContextCache.recency.Len() > evalContextCacheLimit {
		oldest := evalContextCache.recency.Back()
		evalContextCache.recency.Remove(oldest)
		delete(
			evalContextCache.entries,
			oldest.Value.(*evalContextCacheEntry).key,
		)
	}
	return built
}

// PooledEvalContext returns a *cek.EvalContext for the given (language
// version, protocol major version, cost model parameter list) tuple,
// building and caching one on first use so every caller sharing the tuple
// receives the identical *cek.EvalContext pointer while it stays cached.
//
// The cache is process-wide and holds a bounded number of tuples, evicting the
// least recently used; a caller may keep using a returned EvalContext after it
// is evicted. The cache exists so that callers evaluating many redeemers, such
// as the Conway and Dijkstra ledger rules, share one EvalContext instead of
// building one per redeemer. Evaluate reuses cek.Machine instances for any
// *cek.EvalContext passed to it repeatedly, so a caller that caches its own
// EvalContexts gets the same Machine reuse without using this function.
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
	if cached, ok := loadEvalContext(key); ok {
		return cached, nil
	}
	built, err := cek.NewEvalContext(
		version,
		cek.ProtoVersion{Major: protoMajor},
		costModelParams,
	)
	if err != nil {
		return nil, err
	}
	return loadOrStoreEvalContext(key, built), nil
}

// machineCheckoutKey pairs the language version passed to cek.NewMachine with
// the *cek.EvalContext used to build it. A Machine's builtins, per-step costs
// and available-builtin table (cek.NewMachine) derive only from that pair, so
// Machines sharing a key are interchangeable. The version is part of the key
// because nothing stops a caller reusing one EvalContext across versions.
//
// The EvalContext is held as a weak pointer so that a pool entry never keeps
// its EvalContext alive. A weak.Pointer identifies one object for its whole
// lifetime and never compares equal to one made from a different object, even
// one later allocated at the same address, so a Machine is only ever reused
// with the EvalContext it was built from.
type machineCheckoutKey struct {
	version     lang.LanguageVersion
	evalContext weak.Pointer[cek.EvalContext]
}

// machinePoolEntry is the sync.Pool of *cek.Machine[syn.DeBruijn] instances
// built for one machineCheckoutKey, plus a count of how many Machines have
// been constructed for it. The count lets a test observe reuse without relying
// on Machine pointer identity, which sync.Pool does not preserve: Put may drop
// an item (it does so at random under the race detector) and a GC cycle may
// clear the pool.
//
// The entry holds no strong reference to its EvalContext: Machines are built
// by checkoutMachineReused from the caller's pointer, not by a pool New func
// that would capture it.
type machinePoolEntry struct {
	pool        sync.Pool
	constructed atomic.Int64
}

// machinePools maps a machineCheckoutKey to its *machinePoolEntry. An entry is
// created on the first checkout for a key and deleted by a runtime cleanup
// once its EvalContext becomes unreachable, so the map holds at most one entry
// per (version, EvalContext) pair still alive. A caller that builds a fresh
// EvalContext per evaluation therefore retains nothing beyond the
// EvalContexts it is itself still holding.
var machinePools sync.Map // machineCheckoutKey -> *machinePoolEntry

func machinePoolFor(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) *machinePoolEntry {
	key := machineCheckoutKey{
		version:     version,
		evalContext: weak.Make(evalContext),
	}
	if entry, ok := machinePools.Load(key); ok {
		return entry.(*machinePoolEntry)
	}
	fresh := &machinePoolEntry{}
	actual, loaded := machinePools.LoadOrStore(key, fresh)
	if !loaded {
		// The cleanup captures only the key, whose weak pointer does not keep
		// evalContext reachable; capturing evalContext would pin it forever.
		runtime.AddCleanup(
			evalContext,
			func(k machineCheckoutKey) { machinePools.Delete(k) },
			key,
		)
	}
	return actual.(*machinePoolEntry)
}

// checkoutMachineReused returns a *cek.Machine[syn.DeBruijn] built for
// (version, evalContext), reusing a previously released one when available,
// and reports whether it was reused. The caller owns the returned Machine
// exclusively until it calls releaseMachine; sync.Pool.Get never hands the
// same instance to two concurrent callers. A nil evalContext is not pooled
// and always gets a fresh Machine.
func checkoutMachineReused(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
) (*cek.Machine[syn.DeBruijn], bool) {
	if evalContext == nil {
		return cek.NewMachine[syn.DeBruijn](version, 200, nil), false
	}
	entry := machinePoolFor(version, evalContext)
	if machine, ok := entry.pool.Get().(*cek.Machine[syn.DeBruijn]); ok {
		return machine, true
	}
	entry.constructed.Add(1)
	return cek.NewMachine[syn.DeBruijn](version, 200, evalContext), false
}

// checkoutMachineWithBudget checks out a Machine as checkoutMachineReused does
// and sets its ExBudget to budget, ready for one Run.
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
	machine, reused := checkoutMachineReused(version, evalContext)
	if reused && machine.ExBudget == budget {
		machine = cek.NewMachine[syn.DeBruijn](version, 200, evalContext)
		machinePoolFor(version, evalContext).constructed.Add(1)
	}
	machine.ExBudget = budget
	return machine
}

// releaseMachine returns a checked-out Machine to its pool, or drops it when
// evalContext is nil. Only call it after the Machine's Run has fully
// returned: Run's per-call state (ExBudget, Logs, arena reuse) assumes
// exclusive ownership for the duration of one Run.
func releaseMachine(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
	machine *cek.Machine[syn.DeBruijn],
) {
	if evalContext == nil {
		return
	}
	machinePoolFor(version, evalContext).pool.Put(machine)
}

// runPooledMachine runs program on a Machine checked out for (version,
// evalContext) with the given budget and returns the budget consumed, which
// is computed even when Run fails.
//
// The Machine is released only after Run has returned and its ExBudget has
// been read, never from a defer: a Run that panics leaves the Machine out of
// its pool rather than handing a Machine interrupted mid-evaluation to the
// next caller, and releasing before the ExBudget read would let a concurrent
// checkout overwrite it.
func runPooledMachine(
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
	budget cek.ExBudget,
	program syn.Term[syn.DeBruijn],
) (cek.ExBudget, error) {
	machine := checkoutMachineWithBudget(version, evalContext, budget)
	_, runErr := machine.Run(program)
	consumed := budget.Sub(&machine.ExBudget)
	releaseMachine(version, evalContext, machine)
	return consumed, runErr
}
