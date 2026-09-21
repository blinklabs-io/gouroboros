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

package consensus

import (
	"container/list"
	"math/big"
	"sync"
)

// natThresholdCacheMaxEntries bounds the number of distinct
// (poolStake, totalStake, activeSlotCoeff, mode) tuples natThresholdMemo
// retains at once.
//
// CertifiedNatThresholdWithMode is a pure function of these four inputs, so
// memoizing it needs no invalidation logic at all: correctness follows
// directly from exact-input equality (unlike this codebase's epoch-scoped
// caches elsewhere, which do need generation-counter invalidation because
// their inputs change meaning across an epoch boundary). But an unbounded
// cache would still grow without limit: poolStake/totalStake vary with the
// pool set and stake snapshot, so a genesis-to-tip sync or a long-running
// mainnet node lives through many epochs' worth of distinct combinations.
// A few thousand entries comfortably covers every distinct-stake
// block-producing pool active in a single epoch (mainnet currently has on
// the order of 2-3k pools), so 4096 keeps the common case entirely
// resident while bounding worst-case memory via least-recently-used
// eviction, rather than requiring this pure-function package to know
// anything about epoch rollover.
const natThresholdCacheMaxEntries = 4096

// natThresholdCacheKey is the exact literal input tuple
// CertifiedNatThresholdWithMode is memoized by. activeSlotCoeff is a
// *big.Rat (a pointer), so it is keyed by its canonical string
// representation (RatString) rather than by pointer identity: two
// distinct *big.Rat values representing the same rational number (e.g. two
// separate reads of the same per-network genesis constant) must hit the
// same cache entry.
type natThresholdCacheKey struct {
	poolStake       uint64
	totalStake      uint64
	activeSlotCoeff string
	mode            ConsensusMode
}

// natThresholdCacheKeyFor builds the cache key for a
// CertifiedNatThresholdWithMode call. nil and non-nil activeSlotCoeff
// values are always distinguishable: big.Rat.RatString() never produces an
// empty string (a zero rational renders as "0"), so the empty string is a
// safe sentinel for nil.
func natThresholdCacheKeyFor(
	poolStake, totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) natThresholdCacheKey {
	coeffKey := ""
	if activeSlotCoeff != nil {
		coeffKey = activeSlotCoeff.RatString()
	}
	return natThresholdCacheKey{
		poolStake:       poolStake,
		totalStake:      totalStake,
		activeSlotCoeff: coeffKey,
		mode:            mode,
	}
}

// natThresholdCacheEntry pairs a cached result with the error (if any)
// CertifiedNatThresholdWithMode returned for the same key, so error
// results (e.g. an out-of-domain activeSlotCoeff, or an unresolved
// precision escalation) are memoized too, rather than being recomputed on
// every repeat call.
type natThresholdCacheEntry struct {
	key    natThresholdCacheKey
	result *big.Int
	err    error
}

// natThresholdCache is a bounded, least-recently-used memoization cache
// keyed by natThresholdCacheKey. Lookups and insertions are both cheap
// relative to the arbitrary-precision computation they replace (a map
// operation plus a doubly-linked-list move under a single mutex), so lock
// contention is not expected to be significant next to that computation
// even under concurrent header verification.
type natThresholdCache struct {
	mu      sync.Mutex
	entries map[natThresholdCacheKey]*list.Element
	order   *list.List // front = most recently used
	maxSize int
}

func newNatThresholdCache(maxSize int) *natThresholdCache {
	return &natThresholdCache{
		entries: make(map[natThresholdCacheKey]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
	}
}

func (c *natThresholdCache) get(
	key natThresholdCacheKey,
) (result *big.Int, err error, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, nil, false
	}
	c.order.MoveToFront(elem)
	entry, _ := elem.Value.(*natThresholdCacheEntry)
	return entry.result, entry.err, true
}

func (c *natThresholdCache) put(
	key natThresholdCacheKey,
	result *big.Int,
	err error,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.order.MoveToFront(elem)
		entry, _ := elem.Value.(*natThresholdCacheEntry)
		entry.result = result
		entry.err = err
		return
	}
	entry := &natThresholdCacheEntry{key: key, result: result, err: err}
	elem := c.order.PushFront(entry)
	c.entries[key] = elem
	if c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		c.order.Remove(oldest)
		if oldestEntry, ok := oldest.Value.(*natThresholdCacheEntry); ok {
			delete(c.entries, oldestEntry.key)
		}
	}
}

// natThresholdMemo is the package-level memoization cache used by
// CertifiedNatThresholdWithMode.
var natThresholdMemo = newNatThresholdCache(natThresholdCacheMaxEntries)

// natThresholdOnCompute, when non-nil, is invoked each time
// CertifiedNatThresholdWithMode actually executes the underlying
// arbitrary-precision computation rather than returning a memoized result
// (i.e. on a cache miss). Production code leaves this nil; it exists
// solely so tests can observe cache hit/miss behavior without reaching
// into natThresholdMemo's internals.
var natThresholdOnCompute func()
