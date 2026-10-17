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
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLeaderEligibilityCacheLRU(t *testing.T) {
	t.Parallel()
	cache := newLeaderEligibilityCache(2)
	key := func(pool uint64) leaderEligibilityCacheKey {
		return leaderEligibilityCacheKey{
			poolStake: pool, totalStake: 3,
			numerator: 1, denominator: 2, mode: ConsensusModeCPraos,
		}
	}
	prepare := func(pool uint64) *LeaderEligibility {
		e, err := newLeaderEligibility(
			cache,
			pool,
			3,
			big.NewRat(1, 2),
			ConsensusModeCPraos,
		)
		require.NoError(t, err)
		return e
	}
	first, second := prepare(1), prepare(2)
	state, ok := cache.get(key(1)) // Reading the oldest entry makes it MRU.
	require.True(t, ok)
	require.Same(t, first.state, state)
	third := prepare(3)
	_, ok = cache.get(key(2))
	require.False(t, ok, "least recently used entry must be evicted")
	require.Len(t, cache.entries, 2)
	require.Equal(t, 2, cache.order.Len())
	state, ok = cache.get(key(3))
	require.True(t, ok)
	require.Same(t, third.state, state)

	// Reuse an existing key without consuming another slot or replacing state.
	again := prepare(1)
	require.NotSame(t, first, again)
	require.Same(t, first.state, again.state)
	require.Len(t, cache.entries, 2)
	require.Equal(t, 2, cache.order.Len())
	cache.put(key(1), first.state)
	require.Len(t, cache.entries, 2)
	require.Equal(t, 2, cache.order.Len())

	// An evicted entry is prepared again; old wrappers remain usable.
	rebuilt := prepare(2)
	require.NotSame(t, second.state, rebuilt.state)
	for _, e := range []*LeaderEligibility{first, second, third, rebuilt} {
		eligible, err := e.CheckCertifiedNatural(new(big.Int))
		require.NoError(t, err)
		require.True(t, eligible)
	}
	require.Len(t, cache.entries, 2)
	require.Equal(t, 2, cache.order.Len())
}

func TestLeaderEligibilityCacheNormalizedInputs(t *testing.T) {
	t.Parallel()
	cache := newLeaderEligibilityCache(4)
	first, err := newLeaderEligibility(
		cache,
		99,
		7,
		big.NewRat(2, 6),
		ConsensusModeCPraos,
	)
	require.NoError(t, err)
	second, err := newLeaderEligibility(
		cache,
		7,
		7,
		big.NewRat(1, 3),
		ConsensusModeCPraos,
	)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Same(t, first.state, second.state)
	require.Len(t, cache.entries, 1)
	key := leaderEligibilityCacheKey{
		poolStake: 7, totalStake: 7,
		numerator: 1, denominator: 3, mode: ConsensusModeCPraos,
	}
	state, ok := cache.get(key)
	require.True(t, ok)
	require.Same(t, first.state, state)
	for _, e := range []*LeaderEligibility{first, second} {
		eligible, err := e.CheckCertifiedNatural(new(big.Int))
		require.NoError(t, err)
		require.True(t, eligible)
		eligible, err = e.CheckCertifiedNatural(
			new(big.Int).Lsh(big.NewInt(1), 255),
		)
		require.NoError(t, err)
		require.False(t, eligible)
	}
}

func TestLeaderEligibilityCacheRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	cache := newLeaderEligibilityCache(2)
	valid, err := newLeaderEligibility(
		cache,
		1,
		1,
		big.NewRat(1, 2),
		ConsensusModeCPraos,
	)
	require.NoError(t, err)
	hugeDenominator := new(big.Int).Lsh(big.NewInt(1), 2000)
	hugeNumerator := new(
		big.Int,
	).Sub(new(big.Int).Set(hugeDenominator), big.NewInt(1))
	cases := []struct {
		name        string
		pool, total uint64
		f           *big.Rat
		mode        ConsensusMode
	}{
		{"unknown mode", 0, 0, nil, ConsensusMode(99)},
		{"nil coefficient", 1, 1, nil, ConsensusModeCPraos},
		{"zero coefficient", 0, 1, new(big.Rat), ConsensusModeCPraos},
		{"negative coefficient", 1, 1, big.NewRat(-1, 2), ConsensusModeCPraos},
		{"coefficient above one", 1, 1, big.NewRat(3, 2), ConsensusModeCPraos},
		{
			"below minimum",
			0,
			1,
			new(
				big.Rat,
			).SetFrac(
				big.NewInt(1),
				new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil),
			),
			ConsensusModeCPraos,
		},
		{
			"non Word64 coefficient",
			0,
			1,
			new(big.Rat).SetFrac(hugeNumerator, hugeDenominator),
			ConsensusModeTPraos,
		},
		{"zero total", 0, 0, big.NewRat(1, 1), ConsensusModeTPraos},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := newLeaderEligibility(
				cache,
				tc.pool,
				tc.total,
				tc.f,
				tc.mode,
			)
			require.Error(t, err)
			require.Nil(t, e)
			require.Len(t, cache.entries, 1)
			require.Equal(t, 1, cache.order.Len())
		})
	}
	again, err := newLeaderEligibility(
		cache,
		1,
		1,
		big.NewRat(1, 2),
		ConsensusModeCPraos,
	)
	require.NoError(t, err)
	require.Same(t, valid.state, again.state)
}

func TestLeaderEligibilityCacheCallerIsolation(t *testing.T) {
	t.Parallel()
	cache := newLeaderEligibilityCache(2)
	f := big.NewRat(1, 2)
	first, err := newLeaderEligibility(cache, 1, 1, f, ConsensusModeTPraos)
	require.NoError(t, err)
	second, err := newLeaderEligibility(
		cache,
		1,
		1,
		big.NewRat(2, 4),
		ConsensusModeTPraos,
	)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Same(t, first.state, second.state)

	// Caller-owned inputs and exported wrappers are not cached mutable state.
	f.SetInt64(1)
	v := new(big.Int).Lsh(big.NewInt(1), 511)
	eligible, err := first.CheckCertifiedNatural(v)
	require.NoError(t, err)
	require.True(t, eligible)
	require.Equal(t, 512, v.BitLen(), "checking must not mutate the natural")
	v.SetInt64(0)
	*first = LeaderEligibility{}
	_, err = first.CheckCertifiedNatural(v)
	require.Error(t, err)
	again, err := newLeaderEligibility(
		cache,
		1,
		1,
		big.NewRat(1, 2),
		ConsensusModeTPraos,
	)
	require.NoError(t, err)
	require.Same(t, second.state, again.state)
	for _, e := range []*LeaderEligibility{second, again} {
		eligible, err = e.CheckCertifiedNatural(
			new(big.Int).Lsh(big.NewInt(1), 511),
		)
		require.NoError(t, err)
		require.True(t, eligible)
		maximumMinusOne := new(
			big.Int,
		).Sub(new(big.Int).Lsh(big.NewInt(1), 512), big.NewInt(1))
		eligible, err = e.CheckCertifiedNatural(maximumMinusOne)
		require.NoError(t, err)
		require.False(
			t,
			eligible,
			"mutating f to one must not make cached decisions unconditional",
		)
	}
}

func TestLeaderEligibilityCacheConcurrentReaders(t *testing.T) {
	t.Parallel()
	cache := newLeaderEligibilityCache(3)
	shared, err := newLeaderEligibility(
		cache,
		1,
		1,
		big.NewRat(1, 2),
		ConsensusModeTPraos,
	)
	require.NoError(t, err)
	const workers = 16
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mode, bits := ConsensusModeCPraos, uint(256)
			if worker%2 != 0 {
				mode, bits = ConsensusModeTPraos, 512
			}
			denominator := int64(2)
			if worker%4 >= 2 {
				denominator = 4
			}
			// Every goroutine owns its rational; shared prepared state is read-only.
			f := big.NewRat(1, denominator)
			for range 12 {
				e, err := newLeaderEligibility(
					cache,
					uint64(worker+1),
					uint64(worker+1),
					f,
					mode,
				)
				if err != nil {
					errors <- err
					return
				}
				got, err := e.CheckCertifiedNatural(
					new(big.Int).Lsh(big.NewInt(1), bits-1),
				)
				if err != nil || got != (denominator == 2) {
					errors <- fmt.Errorf(
						"worker %d half-natural decision: got %v, error %v",
						worker, got, err,
					)
					return
				}
				got, err = shared.CheckVRFOutput(make([]byte, 64))
				if err != nil || !got {
					errors <- fmt.Errorf(
						"shared zero-output decision: got %v, error %v",
						got, err,
					)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Len(t, cache.entries, 3)
	require.Equal(t, 3, cache.order.Len())
}
