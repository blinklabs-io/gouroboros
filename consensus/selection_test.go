// Copyright 2025 Blink Labs Software
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

	// With density comparison, chain A should be preferred (higher density)
	result = selector.CompareWithDensity(chainA, chainB, 1000)
	if result <= 0 {
		t.Error("chain with higher density should be preferred")
	}
}

func TestIsDeepFork(t *testing.T) {
	selector := NewPraosChainSelector(2160) // k = 2160

	// Fork at slot 1000, current slot 2000 - not deep (1000 slots)
	if selector.IsDeepFork(1000, 2000) {
		t.Error("fork 1000 slots ago should not be deep (k=2160)")
	}

	// Fork at slot 1000, current slot 5000 - deep (4000 slots)
	if !selector.IsDeepFork(1000, 5000) {
		t.Error("fork 4000 slots ago should be deep (k=2160)")
	}

	// Fork at slot 1000, current slot 3161 - exactly at boundary (2161 slots)
	if !selector.IsDeepFork(1000, 3161) {
		t.Error("fork 2161 slots ago should be deep (k=2160)")
	}

	// Fork at slot 1000, current slot 3160 - just under boundary (2160 slots)
	if selector.IsDeepFork(1000, 3160) {
		t.Error("fork exactly 2160 slots ago should not be deep")
	}

	// Edge case: current slot before fork slot
	if selector.IsDeepFork(2000, 1000) {
		t.Error("fork in future should not be deep")
	}
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

	preferred := selector.PreferredWithDensity(candidates, 1000)
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
