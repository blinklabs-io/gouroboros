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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPraosChainSelector(t *testing.T) {
	selector := NewPraosChainSelector(2160)
	if selector.SecurityParam != 2160 {
		t.Errorf("expected security param 2160, got %d", selector.SecurityParam)
	}
}

func TestCompareLongerChainPreferred(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrfOutput := make([]byte, 64)
	chainA := NewSimpleChainTip(1000, 100, vrfOutput)
	chainB := NewSimpleChainTip(1000, 50, vrfOutput)

	result := selector.Compare(chainA, chainB)
	if result <= 0 {
		t.Error("longer chain (higher block number) should be preferred")
	}

	result = selector.Compare(chainB, chainA)
	if result >= 0 {
		t.Error("shorter chain should not be preferred")
	}
}

func TestCompareEqualLengthVRFTiebreaker(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	// Lower VRF output should be preferred
	lowVRF := make([]byte, 64)
	lowVRF[63] = 0x01 // Small value

	highVRF := make([]byte, 64)
	highVRF[0] = 0xFF // Large value (high byte set)

	chainA := NewSimpleChainTip(1000, 100, lowVRF)
	chainB := NewSimpleChainTip(1000, 100, highVRF)

	result := selector.Compare(chainA, chainB)
	if result <= 0 {
		t.Error("chain with lower VRF output should be preferred")
	}
}

func TestCompareEqualChains(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrfOutput := make([]byte, 64)
	vrfOutput[32] = 0x42

	chainA := NewSimpleChainTip(1000, 100, vrfOutput)
	chainB := NewSimpleChainTip(1000, 100, vrfOutput)

	result := selector.Compare(chainA, chainB)
	if result != 0 {
		t.Error("identical chains should be equal")
	}
}

func TestCompareNilChains(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrfOutput := make([]byte, 64)
	chain := NewSimpleChainTip(1000, 100, vrfOutput)

	// nil vs valid
	if selector.Compare(nil, chain) >= 0 {
		t.Error("valid chain should be preferred over nil")
	}

	// valid vs nil
	if selector.Compare(chain, nil) <= 0 {
		t.Error("valid chain should be preferred over nil")
	}

	// nil vs nil
	if selector.Compare(nil, nil) != 0 {
		t.Error("nil vs nil should be equal")
	}
}

func TestPreferred(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf1 := make([]byte, 64)
	vrf1[63] = 0x01

	vrf2 := make([]byte, 64)
	vrf2[63] = 0x02

	vrf3 := make([]byte, 64)
	vrf3[63] = 0x03

	candidates := []ChainTip{
		NewSimpleChainTip(500, 50, vrf2),
		NewSimpleChainTip(1000, 100, vrf1), // Longest chain
		NewSimpleChainTip(800, 80, vrf3),
	}

	preferred := selector.Preferred(candidates)
	if preferred == nil {
		t.Fatal("expected non-nil preferred chain")
	}
	if preferred.BlockNumber() != 100 {
		t.Errorf(
			"expected longest chain (block 100), got block %d",
			preferred.BlockNumber(),
		)
	}
}

func TestPreferredEqualLengthUsesVRF(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	lowVRF := make([]byte, 64)
	lowVRF[63] = 0x01

	highVRF := make([]byte, 64)
	highVRF[63] = 0xFF

	candidates := []ChainTip{
		NewSimpleChainTip(1000, 100, highVRF),
		NewSimpleChainTip(1000, 100, lowVRF), // Same length, lower VRF
	}

	preferred := selector.Preferred(candidates)
	if preferred == nil {
		t.Fatal("expected non-nil preferred chain")
	}

	// Check it's the one with low VRF
	if preferred.VRFOutput()[63] != 0x01 {
		t.Error("expected chain with lower VRF output to be preferred")
	}
}

func TestPreferredEmpty(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	preferred := selector.Preferred([]ChainTip{})
	if preferred != nil {
		t.Error("expected nil for empty candidates")
	}
}

func TestPreferredSingleCandidate(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf := make([]byte, 64)
	chain := NewSimpleChainTip(1000, 100, vrf)

	preferred := selector.Preferred([]ChainTip{chain})
	if preferred != chain {
		t.Error("single candidate should be returned")
	}
}

func TestCompareWithDensity(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	// Two chains with equal length but different density
	vrf := make([]byte, 64)

	// Chain A: 100 blocks in 200 slots (density = 0.5)
	chainA := NewSimpleChainTipWithDensity(1200, 100, vrf, 100, 200)

	// Chain B: 100 blocks in 500 slots (density = 0.2)
	chainB := NewSimpleChainTipWithDensity(1500, 100, vrf, 100, 500)

	// With standard Compare, they're equal (same block number, same VRF)
	result := selector.Compare(chainA, chainB)
	if result != 0 {
		t.Error("standard compare should show equal chains")
	}

	// With density comparison for a deep fork (fork at block 0, current
	// tip far beyond k=2160 blocks past the fork), chain A should be
	// preferred (higher density).
	result = selector.CompareWithDensity(
		chainA,
		chainB,
		ForkPoint{Slot: 1000, BlockNumber: 0},
		5000,
	)
	if result <= 0 {
		t.Error("chain with higher density should be preferred")
	}
}

// TestCompareWithDensityShallowForkIgnoresDensity verifies that forks no
// deeper than k use the ordinary longest-chain rule and do NOT consult
// density, per the Genesis rule: density-first selection only applies to
// forks deeper than k.
func TestCompareWithDensityShallowForkIgnoresDensity(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf := make([]byte, 64)

	// Chain A: fewer blocks, but much higher density.
	chainA := NewSimpleChainTipWithDensity(1100, 90, vrf, 90, 100)
	// Chain B: more blocks (longer chain), lower density.
	chainB := NewSimpleChainTipWithDensity(1200, 100, vrf, 100, 500)

	// Fork at block 1000, current tip block 1200: only 200 blocks deep,
	// which is well within k=2160, so this is NOT a deep fork.
	fork := ForkPoint{Slot: 1000, BlockNumber: 1000}
	if selector.IsDeepFork(fork, 1200) {
		t.Fatal("fork should not be considered deep for this test")
	}

	result := selector.CompareWithDensity(chainA, chainB, fork, 1200)
	if result >= 0 {
		t.Error(
			"shallow fork must use longest-chain rule; the longer " +
				"(but sparser) chain B should be preferred over A",
		)
	}
}

// TestCompareWithDensityDeepForkLongerButSparserLoses is the regression
// test for issue #1936: for forks deeper than k, a longer chain with
// strictly lower density must lose to a shorter chain with higher
// density.
func TestCompareWithDensityDeepForkLongerButSparserLoses(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf := make([]byte, 64)

	fork := ForkPoint{Slot: 1000, BlockNumber: 0}
	// tipBlockNumber is far enough past the fork point to make this a
	// deep fork (more than k=2160 blocks would be rolled back).
	tipBlockNumber := uint64(10000)

	// Chain A ("honest"): shorter chain, but denser (100 blocks in 200
	// slots => density 0.5).
	chainA := NewSimpleChainTipWithDensity(1200, 100, vrf, 100, 200)

	// Chain B ("adversarial"): longer chain (more blocks than A), but
	// much sparser (150 blocks in 3000 slots => density 0.05).
	chainB := NewSimpleChainTipWithDensity(4000, 150, vrf, 150, 3000)

	if !selector.IsDeepFork(fork, tipBlockNumber) {
		t.Fatal("fork should be considered deep for this test")
	}

	// Sanity check: under the ordinary (non-density) rule, the longer
	// chain B would win purely on block count.
	if selector.Compare(chainA, chainB) >= 0 {
		t.Fatal("expected chain B to have more blocks than chain A")
	}

	// Under the Genesis density-first rule for deep forks, the denser
	// (but shorter) chain A must win.
	result := selector.CompareWithDensity(
		chainA,
		chainB,
		fork,
		tipBlockNumber,
	)
	if result <= 0 {
		t.Error(
			"denser chain A must be preferred over longer but " +
				"sparser deep-fork chain B",
		)
	}

	// And PreferredWithDensity must select the denser chain too.
	preferred := selector.PreferredWithDensity(
		[]ChainTip{chainB, chainA},
		fork,
		tipBlockNumber,
	)
	if preferred != chainA {
		t.Error(
			"PreferredWithDensity must select the denser chain " +
				"for a deep fork, not the longer/sparser one",
		)
	}
}

// TestIsDeepFork pins the routing predicate's unit and boundary: depth is
// measured in BLOCKS, k blocks is still shallow, and k+1 blocks is deep.
func TestIsDeepFork(t *testing.T) {
	selector := NewPraosChainSelector(2160) // k = 2160 blocks

	fork := func(blockNumber uint64) ForkPoint {
		return ForkPoint{Slot: blockNumber * 20, BlockNumber: blockNumber}
	}

	// Rollback of 1000 blocks - not deep
	assert.False(
		t,
		selector.IsDeepFork(fork(1000), 2000),
		"rollback of 1000 blocks should not be deep (k=2160)",
	)

	// Rollback of 4000 blocks - deep
	assert.True(
		t,
		selector.IsDeepFork(fork(1000), 5000),
		"rollback of 4000 blocks should be deep (k=2160)",
	)

	// Rollback of exactly k+1 = 2161 blocks - deep
	assert.True(
		t,
		selector.IsDeepFork(fork(1000), 3161),
		"rollback of k+1=2161 blocks should be deep",
	)

	// Rollback of exactly k = 2160 blocks - still shallow
	assert.False(
		t,
		selector.IsDeepFork(fork(1000), 3160),
		"rollback of exactly k=2160 blocks should not be deep",
	)

	// Edge case: fork point ahead of the tip requires no rollback.
	assert.False(
		t,
		selector.IsDeepFork(fork(2000), 1000),
		"fork ahead of the tip should not be deep",
	)

	// Edge case: fork point equal to the tip requires no rollback.
	assert.False(
		t,
		selector.IsDeepFork(fork(1000), 1000),
		"fork at the tip should not be deep",
	)
}

// TestIsDeepForkUint64Boundary checks the depth subtraction near the uint64
// maximum: it must not underflow or wrap.
func TestIsDeepForkUint64Boundary(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	maxUint64 := ^uint64(0)

	// Tip at the uint64 maximum, fork k blocks back - still shallow.
	assert.False(
		t,
		selector.IsDeepFork(
			ForkPoint{Slot: 0, BlockNumber: maxUint64 - 2160},
			maxUint64,
		),
		"rollback of exactly k blocks at the uint64 max is not deep",
	)

	// Tip at the uint64 maximum, fork k+1 blocks back - deep.
	assert.True(
		t,
		selector.IsDeepFork(
			ForkPoint{Slot: 0, BlockNumber: maxUint64 - 2161},
			maxUint64,
		),
		"rollback of k+1 blocks at the uint64 max is deep",
	)

	// Fork at the uint64 maximum with a tip at zero must not wrap into a
	// huge positive depth.
	assert.False(
		t,
		selector.IsDeepFork(ForkPoint{Slot: 0, BlockNumber: maxUint64}, 0),
		"fork far ahead of the tip must not underflow into deep",
	)
}

func TestPreferredWithDensity(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf := make([]byte, 64)

	candidates := []ChainTip{
		NewSimpleChainTipWithDensity(1500, 100, vrf, 100, 500), // density 0.2
		NewSimpleChainTipWithDensity(
			1200,
			100,
			vrf,
			100,
			200,
		), // density 0.5 (preferred)
		NewSimpleChainTipWithDensity(1300, 100, vrf, 100, 300), // density 0.33
	}

	// A tip at block 5000 puts the fork (block 0) more than k=2160 blocks
	// in the past, making this a deep fork subject to density-first
	// comparison.
	preferred := selector.PreferredWithDensity(
		candidates,
		ForkPoint{Slot: 1000, BlockNumber: 0},
		5000,
	)
	if preferred == nil {
		t.Fatal("expected non-nil preferred chain")
	}

	// The one with density 0.5 should be preferred
	if preferred.Density(1000) != 0.5 {
		t.Errorf("expected density 0.5, got %f", preferred.Density(1000))
	}
}

func TestSimpleChainTipDensityCalculation(t *testing.T) {
	vrf := make([]byte, 64)

	// Pre-computed density
	tip := NewSimpleChainTipWithDensity(2000, 100, vrf, 50, 100)
	density := tip.Density(1900) // forkSlot doesn't matter when pre-computed
	if density != 0.5 {
		t.Errorf("expected density 0.5, got %f", density)
	}

	// Without pre-computed values, density returns 0
	tip2 := NewSimpleChainTip(1000, 100, vrf)
	density2 := tip2.Density(500)
	if density2 != 0 {
		t.Errorf(
			"expected density 0 without pre-computed values, got %f",
			density2,
		)
	}
}

func TestChainSelectionWithMainnetParams(t *testing.T) {
	// Use mainnet-like security parameter
	const mainnetSecurityParam = 2160
	selector := NewPraosChainSelector(mainnetSecurityParam)

	if selector.SecurityParam != 2160 {
		t.Errorf("expected mainnet k=2160, got %d", selector.SecurityParam)
	}

	// Verify basic selection works with mainnet params
	vrf := make([]byte, 64)
	chainA := NewSimpleChainTip(100000, 5000, vrf)
	chainB := NewSimpleChainTip(100000, 4999, vrf)

	if selector.Compare(chainA, chainB) <= 0 {
		t.Error("chain with more blocks should be preferred")
	}
}

func TestVRFTiebreakerDeterminism(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	vrf1 := make([]byte, 64)
	vrf1[0] = 0x10

	vrf2 := make([]byte, 64)
	vrf2[0] = 0x20

	chainA := NewSimpleChainTip(1000, 100, vrf1)
	chainB := NewSimpleChainTip(1000, 100, vrf2)

	// Run comparison multiple times to ensure determinism
	for range 10 {
		result := selector.Compare(chainA, chainB)
		if result <= 0 {
			t.Error("chain A (lower VRF) should always be preferred")
		}
	}
}

func TestPreferredManyChains(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	// Create many chains with different block numbers
	var candidates []ChainTip
	for i := uint64(1); i <= 100; i++ {
		vrf := make([]byte, 64)
		vrf[63] = byte(i)
		candidates = append(candidates, NewSimpleChainTip(i*10, i, vrf))
	}

	preferred := selector.Preferred(candidates)
	if preferred == nil {
		t.Fatal("expected non-nil preferred chain")
	}
	if preferred.BlockNumber() != 100 {
		t.Errorf("expected block 100, got %d", preferred.BlockNumber())
	}
}

// --- Genesis window metric (WindowBlockCounter) ---------------------------

// mainnetWindow is the mainnet genesis window sgen = 3k/f with k=2160 and
// f=1/20: 3*2160*20 = 129600 slots.
const mainnetWindow = uint64(129600)

// deepFork returns a fork point and a tip block number that are deep for
// k=2160: the tip is k+1 blocks ahead of the fork point.
func deepFork(forkSlot uint64) (ForkPoint, uint64) {
	return ForkPoint{Slot: forkSlot, BlockNumber: 0}, 2161
}

// TestCompareWithDensityShallowForkShorterDenserLoses is the regression
// requested in review: with a genesis window configured and a denser but
// SHORTER candidate, a shallow fork must still be decided by the ordinary
// longest-chain rule, so the longer chain wins.
func TestCompareWithDensityShallowForkShorterDenserLoses(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)

	vrf := make([]byte, 64)

	// Chain A: shorter (90 blocks) but every block inside the window.
	denseSlots := make([]uint64, 0, 90)
	for i := uint64(1); i <= 90; i++ {
		denseSlots = append(denseSlots, 1000+i)
	}
	chainA := NewWindowedChainTip(1090, 90, vrf, denseSlots)

	// Chain B: longer (100 blocks) but only 5 of them fall inside the
	// window, so B is strictly SPARSER than A by the window metric. If
	// density were (incorrectly) consulted here, A would win; the shallow
	// rule must still pick the longer chain B.
	sparseSlots := make([]uint64, 0, 100)
	for i := uint64(1); i <= 5; i++ {
		sparseSlots = append(sparseSlots, 1000+i*100)
	}
	for i := uint64(1); i <= 95; i++ {
		sparseSlots = append(sparseSlots, 1000+mainnetWindow+i*100)
	}
	chainB := NewWindowedChainTip(200000, 100, vrf, sparseSlots)

	// Guard the premise: A really is denser than B inside the window.
	require.Greater(
		t,
		chainA.BlocksInWindow(1000, mainnetWindow),
		chainB.BlocksInWindow(1000, mainnetWindow),
		"premise: chain A must be denser in the window than chain B",
	)

	// Rollback of only 100 blocks - shallow for k=2160.
	fork := ForkPoint{Slot: 1000, BlockNumber: 900}

	require.False(
		t,
		selector.IsDeepFork(fork, 1000),
		"fork must be shallow for this test",
	)

	// A is strictly denser in the window, but the fork is shallow, so the
	// ordinary rule must apply and the LONGER chain B must win.
	assert.Negative(
		t,
		selector.CompareWithDensity(chainA, chainB, fork, 1000),
		"shallow fork must use longest-chain rule: longer chain B wins "+
			"even though A is denser",
	)

	// The same contract must hold through the public selection entry
	// point, in both candidate orders.
	assert.Same(
		t,
		chainB,
		selector.PreferredWithDensity(
			[]ChainTip{chainA, chainB},
			fork,
			1000,
		),
		"PreferredWithDensity must pick the longer chain on a shallow fork",
	)
	assert.Same(
		t,
		chainB,
		selector.PreferredWithDensity(
			[]ChainTip{chainB, chainA},
			fork,
			1000,
		),
		"shallow-fork selection must not depend on candidate order",
	)
}

// TestCompareWithDensityDeepForkUsesWindowCount is the issue #1936
// regression under the canonical integer window metric: a longer but
// sparser deep fork must lose to a shorter, denser one, in both argument
// orders.
func TestCompareWithDensityDeepForkUsesWindowCount(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)

	vrf := make([]byte, 64)
	fork, tip := deepFork(1000)

	// Honest chain: 50 blocks, all within the window.
	honestSlots := make([]uint64, 0, 50)
	for i := uint64(1); i <= 50; i++ {
		honestSlots = append(honestSlots, 1000+i*100)
	}
	honest := NewWindowedChainTip(6000, 50, vrf, honestSlots)

	// Adversarial chain: 80 blocks total (longer), but only 10 of them
	// fall inside the window; the rest are far beyond it.
	advSlots := make([]uint64, 0, 80)
	for i := uint64(1); i <= 10; i++ {
		advSlots = append(advSlots, 1000+i*100)
	}
	for i := uint64(1); i <= 70; i++ {
		advSlots = append(advSlots, 1000+mainnetWindow+i*100)
	}
	adversarial := NewWindowedChainTip(900000, 80, vrf, advSlots)

	// Sanity: the adversarial chain really is longer.
	assert.Negative(
		t,
		selector.Compare(honest, adversarial),
		"adversarial chain should be longer under the ordinary rule",
	)

	assert.Positive(
		t,
		selector.CompareWithDensity(honest, adversarial, fork, tip),
		"denser honest chain must win a deep fork",
	)
	assert.Negative(
		t,
		selector.CompareWithDensity(adversarial, honest, fork, tip),
		"deep-fork density comparison must be order-independent",
	)

	// End-to-end through the public selection entry point.
	assert.Same(
		t,
		honest,
		selector.PreferredWithDensity(
			[]ChainTip{adversarial, honest},
			fork,
			tip,
		),
		"PreferredWithDensity must select the denser chain on a deep fork",
	)
}

// TestCompareWithDensityEqualDensityFallsThrough checks the tie rule: equal
// block counts in the window fall through to the ordinary comparison.
func TestCompareWithDensityEqualDensityFallsThrough(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)

	vrf := make([]byte, 64)
	fork, tip := deepFork(1000)

	// Both chains have exactly 3 blocks inside the window, but B is
	// longer overall, so the ordinary rule must pick B.
	inWindow := []uint64{1100, 1200, 1300}

	shortChain := NewWindowedChainTip(1300, 10, vrf, inWindow)
	longChain := NewWindowedChainTip(1300, 20, vrf, inWindow)

	assert.Negative(
		t,
		selector.CompareWithDensity(shortChain, longChain, fork, tip),
		"equal window density must fall through to the ordinary rule",
	)

	// Equal density AND equal length: the VRF tiebreaker decides.
	lowVRF := make([]byte, 64)
	lowVRF[0] = 0x01
	highVRF := make([]byte, 64)
	highVRF[0] = 0x02

	lowTip := NewWindowedChainTip(1300, 10, lowVRF, inWindow)
	highTip := NewWindowedChainTip(1300, 10, highVRF, inWindow)

	assert.Positive(
		t,
		selector.CompareWithDensity(lowTip, highTip, fork, tip),
		"equal density and length must fall through to the VRF tiebreak",
	)
}

// TestCompareWithDensityWindowBoundary pins the exact window bound: a block
// at forkSlot+windowSlots is inside, forkSlot+windowSlots+1 is outside, and
// a block at forkSlot itself is outside.
func TestCompareWithDensityWindowBoundary(t *testing.T) {
	const forkSlot = uint64(1000)
	const window = uint64(100)

	tip := NewWindowedChainTip(
		2000,
		4,
		make([]byte, 64),
		[]uint64{
			forkSlot,              // at the fork point: outside
			forkSlot + 1,          // first slot in the window: inside
			forkSlot + window,     // last slot in the window: inside
			forkSlot + window + 1, // first slot past the window: outside
		},
	)

	assert.Equal(
		t,
		uint64(2),
		tip.BlocksInWindow(forkSlot, window),
		"only forkSlot+1 and forkSlot+window fall inside the window",
	)

	// A zero window counts nothing.
	assert.Equal(
		t,
		uint64(0),
		tip.BlocksInWindow(forkSlot, 0),
		"a zero window must count no blocks",
	)
}

// TestBlocksInWindowUint64Boundary checks that the window bound is
// evaluated without wrapping near the uint64 maximum.
func TestBlocksInWindowUint64Boundary(t *testing.T) {
	maxUint64 := ^uint64(0)
	forkSlot := maxUint64 - 10

	tip := NewWindowedChainTip(
		maxUint64,
		2,
		make([]byte, 64),
		[]uint64{forkSlot + 1, maxUint64},
	)

	// forkSlot+windowSlots would overflow; the subtraction form must not.
	assert.Equal(
		t,
		uint64(2),
		tip.BlocksInWindow(forkSlot, maxUint64),
		"a window spanning past the uint64 max must not wrap",
	)
}

// TestCompareWithDensityZeroWindowFallsBack checks the misconfiguration
// path: with no genesis window, a deep fork still resolves (via the legacy
// density ratio) rather than failing or silently preferring nothing.
func TestCompareWithDensityZeroWindowFallsBack(t *testing.T) {
	// No window configured.
	selector := NewPraosChainSelector(2160)
	fork, tip := deepFork(1000)

	vrf := make([]byte, 64)
	dense := NewSimpleChainTipWithDensity(1200, 100, vrf, 100, 200)
	sparse := NewSimpleChainTipWithDensity(1500, 100, vrf, 100, 500)

	assert.Positive(
		t,
		selector.CompareWithDensity(dense, sparse, fork, tip),
		"with no window the legacy density ratio still decides deep forks",
	)
}

// TestCompareWithDensityFallsBackWithoutCounter checks the compatibility
// path: a tip that does not implement WindowBlockCounter keeps working via
// ChainTip.Density even when a window is configured.
func TestCompareWithDensityFallsBackWithoutCounter(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)
	fork, tip := deepFork(1000)

	dense := legacyTip{blockNumber: 100, density: 0.5}
	sparse := legacyTip{blockNumber: 100, density: 0.2}

	assert.Positive(
		t,
		selector.CompareWithDensity(dense, sparse, fork, tip),
		"tips without WindowBlockCounter must still compare by density",
	)
}

// legacyTip implements only ChainTip - no WindowBlockCounter - modelling a
// downstream implementation written against the pre-existing interface.
type legacyTip struct {
	blockNumber uint64
	density     float64
}

func (l legacyTip) Slot() uint64             { return l.blockNumber * 20 }
func (l legacyTip) BlockNumber() uint64      { return l.blockNumber }
func (l legacyTip) VRFOutput() []byte        { return make([]byte, 64) }
func (l legacyTip) Density(_ uint64) float64 { return l.density }

// TestOnlyWindowedTipClaimsWindowBlockCounter pins which types may claim the
// optional extension. Only a tip that actually carries block slots may: a
// tip that cannot count must not advertise the capability, because
// selection cannot distinguish "answered zero" from "had no data".
func TestOnlyWindowedTipClaimsWindowBlockCounter(t *testing.T) {
	var windowed ChainTip = NewWindowedChainTip(
		1000,
		10,
		make([]byte, 64),
		[]uint64{1001},
	)
	_, ok := windowed.(WindowBlockCounter)
	assert.True(t, ok, "WindowedChainTip must implement WindowBlockCounter")

	var legacy ChainTip = legacyTip{blockNumber: 10}
	_, ok = legacy.(WindowBlockCounter)
	assert.False(
		t,
		ok,
		"a ChainTip-only implementation must remain valid",
	)

	// The tip built without block slots must NOT claim the capability.
	var plain ChainTip = NewSimpleChainTipWithDensity(
		1000, 10, make([]byte, 64), 5, 100,
	)
	_, ok = plain.(WindowBlockCounter)
	assert.False(
		t,
		ok,
		"a tip with no block slots must not claim WindowBlockCounter",
	)
}

// TestDeepForkLegacyTipsStillDiscriminate is the regression for the silent
// degradation this type split closes. With a window configured but both
// tips built from the legacy constructor, the selector must fall back to
// the density ratio and still prefer the denser chain on a deep fork.
// Before the split both tips claimed WindowBlockCounter and answered zero,
// so density was bypassed entirely and the longer-but-sparser chain won.
func TestDeepForkLegacyTipsStillDiscriminate(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)
	fork, tip := deepFork(1000)

	// dense is SHORTER (2000 < 2600) but four times denser.
	dense := NewSimpleChainTipWithDensity(
		3000, 2000, make([]byte, 64), 1000, 2000,
	) // 0.5
	sparse := NewSimpleChainTipWithDensity(
		9000, 2600, make([]byte, 64), 1600, 8000,
	) // 0.2

	assert.Positive(
		t,
		selector.CompareWithDensity(dense, sparse, fork, tip),
		"deep fork: the denser chain must win even via the fallback ratio",
	)
	assert.Same(
		t,
		dense,
		selector.PreferredWithDensity(
			[]ChainTip{dense, sparse}, fork, tip,
		),
		"PreferredWithDensity must pick the denser chain on a deep fork",
	)
}

// TestWindowedTipDensityFallback covers the mixed case: a windowed tip
// compared against a ChainTip-only tip falls back to the ratio, so the
// windowed tip must derive a meaningful ratio from its block slots rather
// than reporting zero and losing by default.
func TestWindowedTipDensityFallback(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, mainnetWindow)
	fork, tip := deepFork(1000)

	// 10 blocks in the 100 slots after the fork: ratio 0.1.
	slots := make([]uint64, 0, 10)
	for i := uint64(1); i <= 10; i++ {
		slots = append(slots, 1000+i*10)
	}
	windowed := NewWindowedChainTip(1100, 10, make([]byte, 64), slots)
	assert.InDelta(
		t,
		0.1,
		windowed.Density(1000),
		1e-9,
		"a windowed tip must derive its legacy ratio from its block slots",
	)

	sparse := legacyTip{blockNumber: 10, density: 0.05}
	assert.Positive(
		t,
		selector.CompareWithDensity(windowed, sparse, fork, tip),
		"mixed comparison must fall back to a meaningful ratio",
	)
}
