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
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withNatThresholdComputeCounter installs a test hook on
// natThresholdOnCompute that counts how many times
// CertifiedNatThresholdWithMode actually executed the underlying
// arbitrary-precision computation (i.e. missed the memoization cache),
// restoring the previous hook on cleanup. Callers must not run in
// parallel with any other test relying on this hook or on the shared
// package-level cache's hit/miss behavior, since both are process-global.
func withNatThresholdComputeCounter(t *testing.T) *atomic.Int64 {
	t.Helper()
	var count atomic.Int64
	prev := natThresholdOnCompute
	natThresholdOnCompute = func() { count.Add(1) }
	t.Cleanup(func() { natThresholdOnCompute = prev })
	return &count
}

// TestCertifiedNatThresholdWithModeMemoizesIdenticalInputs proves that
// repeated calls with the exact same (poolStake, totalStake,
// activeSlotCoeff, mode) tuple compute the underlying arbitrary-precision
// threshold exactly once, and every subsequent call returns an equal
// result without recomputing.
//
// Not t.Parallel: relies on the package-level natThresholdOnCompute hook
// and the shared natThresholdMemo cache, both process-global state that a
// concurrent sibling test's cache traffic would corrupt the count of.
func TestCertifiedNatThresholdWithModeMemoizesIdenticalInputs(t *testing.T) {
	count := withNatThresholdComputeCounter(t)

	poolStake := uint64(482719301)
	totalStake := uint64(9184756223)
	activeSlotCoeff := big.NewRat(37, 811) // distinctive, unused elsewhere
	mode := ConsensusModeCPraos

	first, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		activeSlotCoeff,
		mode,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), count.Load(),
		"first call for a new key must compute exactly once")

	for i := 0; i < 4; i++ {
		again, err := CertifiedNatThresholdWithMode(
			poolStake,
			totalStake,
			activeSlotCoeff,
			mode,
		)
		require.NoError(t, err)
		require.Equal(t, 0, first.Cmp(again),
			"memoized result must equal the original computation")
	}

	require.Equal(t, int64(1), count.Load(),
		"repeated calls with identical inputs must not recompute")
}

// TestCertifiedNatThresholdWithModeMemoizesByValueNotIdentity proves that
// the cache key compares activeSlotCoeff by rational value, not by
// pointer identity: two distinct *big.Rat instances holding the same
// value must hit the same cache entry.
//
// Not t.Parallel: same process-global-state reason as above.
func TestCertifiedNatThresholdWithModeMemoizesByValueNotIdentity(t *testing.T) {
	count := withNatThresholdComputeCounter(t)

	poolStake := uint64(650211987)
	totalStake := uint64(7719340021)
	mode := ConsensusModeTPraos

	coeffA := big.NewRat(11, 271) // distinctive, unused elsewhere
	first, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		coeffA,
		mode,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), count.Load())

	// A second, distinct *big.Rat pointer with the identical value.
	coeffB := big.NewRat(11, 271)
	require.NotSame(t, coeffA, coeffB,
		"test setup requires two distinct pointer instances")

	second, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		coeffB,
		mode,
	)
	require.NoError(t, err)
	require.Equal(t, 0, first.Cmp(second),
		"equal-by-value activeSlotCoeff pointers must produce equal results")
	require.Equal(t, int64(1), count.Load(),
		"an equal-by-value but distinct-pointer activeSlotCoeff must still "+
			"hit the cache, not recompute")
}

// TestCertifiedNatThresholdWithModeDistinctInputsNotConflated proves that
// memoization never conflates distinct input tuples: computing several
// different (poolStake, totalStake, activeSlotCoeff, mode) combinations,
// including ones that collide on some but not all fields, still returns
// each one's own correct, distinct result and does not leak a
// previously-cached value for an unrelated key.
func TestCertifiedNatThresholdWithModeDistinctInputsNotConflated(t *testing.T) {
	t.Parallel()

	type input struct {
		poolStake, totalStake uint64
		coeff                 *big.Rat
		mode                  ConsensusMode
	}
	base := input{
		poolStake:  1_000_000,
		totalStake: 10_000_000,
		coeff:      big.NewRat(1, 25),
		mode:       ConsensusModeCPraos,
	}
	inputs := []input{
		base,
		{
			poolStake:  2_000_000,
			totalStake: base.totalStake,
			coeff:      base.coeff,
			mode:       base.mode,
		},
		{
			poolStake:  base.poolStake,
			totalStake: 20_000_000,
			coeff:      base.coeff,
			mode:       base.mode,
		},
		{
			poolStake:  base.poolStake,
			totalStake: base.totalStake,
			coeff:      big.NewRat(1, 15),
			mode:       base.mode,
		},
		{
			poolStake:  base.poolStake,
			totalStake: base.totalStake,
			coeff:      base.coeff,
			mode:       ConsensusModeTPraos,
		},
	}

	results := make([]*big.Int, len(inputs))
	for i, in := range inputs {
		r, err := CertifiedNatThresholdWithMode(
			in.poolStake,
			in.totalStake,
			in.coeff,
			in.mode,
		)
		require.NoError(t, err)
		results[i] = r

		// Independently compute the expected value directly via the
		// uncached path and confirm they agree, so this is checking
		// against ground truth rather than merely against the cache's
		// own prior output.
		want, err := certifiedNatThresholdWithModeUncached(
			in.poolStake,
			in.totalStake,
			in.coeff,
			in.mode,
		)
		require.NoError(t, err)
		require.Equal(t, 0, want.Cmp(r),
			"cached result for input %d must match the uncached computation", i)
	}

	// Every pair of distinct inputs above differs in exactly one field
	// from the base case, so no two results should collide.
	for i := range results {
		for j := i + 1; j < len(results); j++ {
			require.NotEqual(t, 0, results[i].Cmp(results[j]),
				"distinct inputs %d and %d must not produce the same "+
					"threshold", i, j)
		}
	}

	// Re-fetch the base case last and confirm it still returns its own
	// value, not one clobbered by a later, different key.
	again, err := CertifiedNatThresholdWithMode(
		base.poolStake,
		base.totalStake,
		base.coeff,
		base.mode,
	)
	require.NoError(t, err)
	require.Equal(t, 0, results[0].Cmp(again),
		"the base case's cached entry must survive unrelated cache traffic")
}

// TestCertifiedNatThresholdWithModeCacheReturnsIndependentCopies proves
// that mutating a returned *big.Int does not corrupt the cached entry for
// the same key: CertifiedNatThresholdWithMode must hand back a value safe
// for the caller to treat as its own.
//
// Not t.Parallel: relies on the process-global natThresholdMemo cache.
func TestCertifiedNatThresholdWithModeReturnsIndependentCopies(
	t *testing.T,
) {
	poolStake := uint64(300112233)
	totalStake := uint64(4004008009)
	activeSlotCoeff := big.NewRat(3, 41) // distinctive, unused elsewhere
	mode := ConsensusModeCPraos

	first, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		activeSlotCoeff,
		mode,
	)
	require.NoError(t, err)
	untampered := new(big.Int).Set(first)

	// Mutate the caller's copy.
	first.Add(first, big.NewInt(12345))

	second, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		activeSlotCoeff,
		mode,
	)
	require.NoError(t, err)
	require.Equal(t, 0, untampered.Cmp(second),
		"mutating a previously returned threshold must not affect a later "+
			"cache hit for the same key")
}

// TestCertifiedNatThresholdWithModeCachesErrors proves that an
// error-producing input (an out-of-domain activeSlotCoeff) is also
// memoized: it is not silently recomputed forever, and it must never be
// confused with a successful, non-nil result for an unrelated key.
//
// Not t.Parallel: relies on the package-level natThresholdOnCompute hook.
func TestCertifiedNatThresholdWithModeCachesErrors(t *testing.T) {
	count := withNatThresholdComputeCounter(t)

	poolStake := uint64(120000)
	totalStake := uint64(340000)
	invalidCoeff := big.NewRat(9, 4) // 2.25, out of [0,1] domain
	mode := ConsensusModeCPraos

	_, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		invalidCoeff,
		mode,
	)
	require.Error(t, err)
	require.Equal(t, int64(1), count.Load())

	_, err = CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		invalidCoeff,
		mode,
	)
	require.Error(t, err)
	require.Equal(t, int64(1), count.Load(),
		"a memoized error result must not be recomputed on a repeat call")
}

// TestCertifiedNatThresholdWithModeConcurrentAccess drives concurrent
// calls across a small set of overlapping keys (some goroutines share a
// key, some don't) to exercise natThresholdMemo's mutex-guarded map/list
// under contention. Run with -race, this proves the cache is safe for
// dingo's header-verification call pattern, which is not necessarily
// single-goroutine. It also cross-checks every result against the
// uncached computation for correctness under concurrent access.
func TestCertifiedNatThresholdWithModeConcurrentAccess(t *testing.T) {
	t.Parallel()

	type input struct {
		poolStake, totalStake uint64
		coeff                 *big.Rat
	}
	inputs := []input{
		{poolStake: 111, totalStake: 9973, coeff: big.NewRat(1, 20)},
		{poolStake: 222, totalStake: 9973, coeff: big.NewRat(1, 20)},
		{poolStake: 333, totalStake: 9973, coeff: big.NewRat(3, 100)},
	}

	const goroutinesPerInput = 20
	var wg sync.WaitGroup
	for _, in := range inputs {
		want, err := certifiedNatThresholdWithModeUncached(
			in.poolStake,
			in.totalStake,
			in.coeff,
			ConsensusModeCPraos,
		)
		require.NoError(t, err)

		for i := 0; i < goroutinesPerInput; i++ {
			wg.Add(1)
			go func(in input, want *big.Int) {
				defer wg.Done()
				// A fresh *big.Rat with the same value as in.coeff on
				// every call, to also exercise by-value key comparison
				// under concurrent access.
				got, err := CertifiedNatThresholdWithMode(
					in.poolStake,
					in.totalStake,
					new(big.Rat).Set(in.coeff),
					ConsensusModeCPraos,
				)
				assert.NoError(t, err)
				assert.Equal(t, 0, want.Cmp(got))
			}(in, want)
		}
	}
	wg.Wait()
}

// natThresholdCacheSizes reports a cache's map and intrusive-list lengths
// under its own lock. The two must always agree: a divergence means an
// eviction dropped a list element without dropping its map entry (or vice
// versa), which is exactly what turns this bounded cache into an unbounded
// leak on a long-running node.
func natThresholdCacheSizes(c *natThresholdCache) (mapLen, listLen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.order.Len()
}

// natThresholdTestKey builds a distinct cache key per i, varying only
// poolStake.
func natThresholdTestKey(i uint64) natThresholdCacheKey {
	return natThresholdCacheKeyFor(
		i,
		1_000_000,
		big.NewRat(1, 20),
		ConsensusModeCPraos,
	)
}

// TestNatThresholdCacheEvictsLeastRecentlyUsed proves the bounded LRU
// actually bounds. Inserting more distinct keys than maxSize must hold both
// the map and the list at exactly maxSize, must evict in least-recently-used
// order, and must keep an entry that a get() promoted. Re-putting an
// existing key must replace its value in place rather than growing the
// cache.
//
// Not t.Parallel: sequential ordering keeps the shared-cache assertions
// elsewhere in this file deterministic.
func TestNatThresholdCacheEvictsLeastRecentlyUsed(t *testing.T) {
	const maxSize = 4
	c := newNatThresholdCache(maxSize)

	for i := uint64(0); i < maxSize; i++ {
		c.put(natThresholdTestKey(i), big.NewInt(int64(i)), nil)
	}
	mapLen, listLen := natThresholdCacheSizes(c)
	require.Equal(t, maxSize, mapLen)
	require.Equal(t, maxSize, listLen)

	// Promote key 0, so key 1 becomes the least recently used.
	got, _, ok := c.get(natThresholdTestKey(0))
	require.True(t, ok)
	require.Equal(t, int64(0), got.Int64())

	// One more insertion must evict exactly one entry, and it must be
	// key 1 rather than the just-promoted key 0.
	c.put(natThresholdTestKey(maxSize), big.NewInt(maxSize), nil)
	mapLen, listLen = natThresholdCacheSizes(c)
	require.Equal(t, maxSize, mapLen, "cache must not grow past maxSize")
	require.Equal(t, maxSize, listLen, "list and map must stay in sync")

	_, _, ok = c.get(natThresholdTestKey(0))
	require.True(t, ok, "a get()-promoted key must survive the next eviction")
	_, _, ok = c.get(natThresholdTestKey(1))
	require.False(t, ok, "the least recently used key must be the evicted one")

	// Overwriting an existing key replaces its value without growing.
	c.put(natThresholdTestKey(maxSize), big.NewInt(99), nil)
	mapLen, listLen = natThresholdCacheSizes(c)
	require.Equal(t, maxSize, mapLen,
		"re-putting an existing key must not grow the cache")
	require.Equal(t, maxSize, listLen)
	got, _, ok = c.get(natThresholdTestKey(maxSize))
	require.True(t, ok)
	require.Equal(t, int64(99), got.Int64(),
		"re-putting an existing key must replace the stored value")
}

// TestNatThresholdMemoIsBounded proves the package-level cache that
// CertifiedNatThresholdWithMode actually uses is bounded at
// natThresholdCacheMaxEntries -- not merely that a separately constructed
// cache would be, which would still pass if the production instance were
// wired up without its cap. It drives more distinct keys than the cap
// through the public entry point, using poolStake==0 inputs so each call
// short-circuits before the arbitrary-precision pipeline while still
// occupying a cache entry of its own.
//
// Not t.Parallel: fills and evicts the shared package-level cache.
func TestNatThresholdMemoIsBounded(t *testing.T) {
	coeff := big.NewRat(1, 20)
	for i := uint64(0); i < natThresholdCacheMaxEntries+512; i++ {
		_, err := CertifiedNatThresholdWithMode(
			0,
			i+1,
			coeff,
			ConsensusModeCPraos,
		)
		require.NoError(t, err)
	}
	mapLen, listLen := natThresholdCacheSizes(natThresholdMemo)
	require.Equal(t, natThresholdCacheMaxEntries, mapLen,
		"the package cache must stay bounded at its configured cap")
	require.Equal(t, natThresholdCacheMaxEntries, listLen,
		"the package cache's list and map must stay in sync")
}

// benchmarkThresholdInputs holds a representative, moderately-precise
// input tuple shared by the cached/uncached benchmarks below, so both
// exercise the same escalating-precision ln/exp computation on a miss.
var (
	benchmarkPoolStake       = uint64(500_000_001)
	benchmarkTotalStake      = uint64(1_000_000_003)
	benchmarkActiveSlotCoeff = big.NewRat(1, 17)
)

// BenchmarkCertifiedNatThresholdWithModeUncachedRepeated measures the cost
// of repeatedly computing the same threshold with memoization bypassed
// entirely (calling the uncached computation directly), establishing the
// per-call allocation baseline this issue is about.
func BenchmarkCertifiedNatThresholdWithModeUncachedRepeated(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := certifiedNatThresholdWithModeUncached(
			benchmarkPoolStake,
			benchmarkTotalStake,
			benchmarkActiveSlotCoeff,
			ConsensusModeCPraos,
		); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCertifiedNatThresholdWithModeCachedRepeated measures the same
// repeated call through the public, memoized entry point. After the first
// iteration warms the cache, every subsequent call is a cache hit, so
// allocs/op should be dramatically lower than the uncached benchmark
// above -- proving the fix actually eliminates the repeated
// arbitrary-precision recomputation, not just reports doing so.
func BenchmarkCertifiedNatThresholdWithModeCachedRepeated(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := CertifiedNatThresholdWithMode(
			benchmarkPoolStake,
			benchmarkTotalStake,
			benchmarkActiveSlotCoeff,
			ConsensusModeCPraos,
		); err != nil {
			b.Fatal(err)
		}
	}
}
