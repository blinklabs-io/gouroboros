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

package genesis

import (
	"math/big"
	"testing"
)

// testGenesisConfig returns a GenesisConfig for testing with mainnet-like parameters.
func testGenesisConfig() GenesisConfig {
	f := big.NewRat(1, 20) // 0.05
	return GenesisConfig{
		SecurityParam:   2160,
		ActiveSlotCoeff: f,
		GenesisWindow:   ComputeGenesisWindow(2160, f),
	}
}

func TestComputeGenesisWindow(t *testing.T) {
	// With k=2160 and f=0.05, window = 3*2160/0.05 = 129600 slots
	f := big.NewRat(1, 20)
	window := ComputeGenesisWindow(2160, f)

	expected := uint64(129600)
	if window != expected {
		t.Errorf("expected genesis window %d, got %d", expected, window)
	}
}

func TestGenesisConfig(t *testing.T) {
	config := testGenesisConfig()

	if config.SecurityParam != 2160 {
		t.Errorf("expected security param 2160, got %d", config.SecurityParam)
	}

	expectedWindow := uint64(129600)
	if config.GenesisWindow != expectedWindow {
		t.Errorf(
			"expected genesis window %d, got %d",
			expectedWindow,
			config.GenesisWindow,
		)
	}
}

func TestNewGenesisSelector(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	if selector == nil {
		t.Fatal("expected non-nil selector")
	}

	if selector.config.GenesisWindow != config.GenesisWindow {
		t.Errorf("expected window %d, got %d",
			config.GenesisWindow, selector.config.GenesisWindow)
	}
}

func TestNewGenesisSelectorComputesWindow(t *testing.T) {
	// Config with no window specified
	config := GenesisConfig{
		SecurityParam:   2160,
		ActiveSlotCoeff: big.NewRat(1, 20),
		GenesisWindow:   0, // Will be computed
	}

	selector := NewGenesisSelector(config)

	expectedWindow := uint64(129600)
	if selector.config.GenesisWindow != expectedWindow {
		t.Errorf("expected computed window %d, got %d",
			expectedWindow, selector.config.GenesisWindow)
	}
}

func TestCompareByDensity(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	// Chain A: More blocks (higher density for same tip)
	chainA := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       5000,
	}

	// Chain B: Fewer blocks (lower density for same tip)
	chainB := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       3000,
	}

	result := selector.Compare(chainA, chainB)
	if result <= 0 {
		t.Error("expected chain A to be preferred (higher density)")
	}

	// Reverse comparison
	result = selector.Compare(chainB, chainA)
	if result >= 0 {
		t.Error("expected chain B to be less preferred")
	}
}

func TestCompareByLength(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	// Same tip slot, different block counts - more blocks = higher density
	chainA := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       5000,
	}

	chainB := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       4500, // Fewer total blocks
	}

	result := selector.Compare(chainA, chainB)
	if result <= 0 {
		t.Error("expected chain A to be preferred (more blocks)")
	}
}

func TestCompareEqual(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	// Equal chains
	chain := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       5000,
	}

	result := selector.Compare(chain, chain)
	if result != 0 {
		t.Errorf("expected equal comparison (0), got %d", result)
	}
}

func TestPreferred(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	candidates := []ChainFragment{
		&SimpleChainFragment{
			Intersection: 0,
			Tip:          100000,
			Blocks:       3000,
		},
		&SimpleChainFragment{
			Intersection: 0,
			Tip:          100000,
			Blocks:       5000, // Best - most blocks
		},
		&SimpleChainFragment{
			Intersection: 0,
			Tip:          100000,
			Blocks:       4000,
		},
	}

	best := selector.Preferred(candidates)
	if best == nil {
		t.Fatal("expected non-nil preferred chain")
	}

	if best.BlockCount() != 5000 {
		t.Error("expected the chain with highest density to be preferred")
	}
}

func TestPreferredEmpty(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	var candidates []ChainFragment
	best := selector.Preferred(candidates)
	if best != nil {
		t.Error("expected nil for empty candidates")
	}
}

func TestPreferredSingle(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	chain := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       5000,
	}

	candidates := []ChainFragment{chain}
	best := selector.Preferred(candidates)
	if best != chain {
		t.Error("expected single candidate to be returned")
	}
}

func TestShouldUseGenesis(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	tests := []struct {
		name           string
		localTipSlot   uint64
		networkTipSlot uint64
		syncThreshold  uint64
		expected       bool
	}{
		{
			name:           "far behind - use genesis",
			localTipSlot:   1000,
			networkTipSlot: 200000,
			syncThreshold:  50000,
			expected:       true,
		},
		{
			name:           "caught up - use praos",
			localTipSlot:   200000,
			networkTipSlot: 200100,
			syncThreshold:  50000,
			expected:       false,
		},
		{
			name:           "at threshold boundary - use praos",
			localTipSlot:   150000,
			networkTipSlot: 200000,
			syncThreshold:  50000,
			expected:       false,
		},
		{
			name:           "just over threshold - use genesis",
			localTipSlot:   149999,
			networkTipSlot: 200000,
			syncThreshold:  50000,
			expected:       true,
		},
		{
			name:           "ahead of network - use praos",
			localTipSlot:   200000,
			networkTipSlot: 190000,
			syncThreshold:  50000,
			expected:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := selector.ShouldUseGenesis(
				tc.localTipSlot,
				tc.networkTipSlot,
				tc.syncThreshold,
			)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestDefaultSyncThreshold(t *testing.T) {
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	threshold := selector.DefaultSyncThreshold()
	if threshold != config.GenesisWindow {
		t.Errorf(
			"expected threshold %d, got %d",
			config.GenesisWindow,
			threshold,
		)
	}
}

func TestSimpleChainFragment(t *testing.T) {
	fragment := &SimpleChainFragment{
		Intersection: 1000,
		Tip:          5000,
		Blocks:       200,
	}

	if fragment.IntersectionSlot() != 1000 {
		t.Error("intersection slot mismatch")
	}

	if fragment.TipSlot() != 5000 {
		t.Error("tip slot mismatch")
	}

	if fragment.BlockCount() != 200 {
		t.Error("block count mismatch")
	}

	// Window larger than fragment returns all blocks
	if fragment.BlockCountInWindow(5000) != 200 {
		t.Error("expected all blocks for large window")
	}

	// Window smaller than fragment estimates proportionally
	// Fragment spans 4000 slots (5000-1000), requesting 2000 slot window
	// Expected: 200 * 2000/4000 = 100
	if fragment.BlockCountInWindow(2000) != 100 {
		t.Errorf(
			"expected proportional blocks (100), got %d",
			fragment.BlockCountInWindow(2000),
		)
	}
}

func TestSimpleChainFragmentEstimatesBlocks(t *testing.T) {
	fragment := &SimpleChainFragment{
		Intersection: 0,
		Tip:          10000,
		Blocks:       500,
	}

	// If window is larger than fragment, return all blocks
	blocks := fragment.BlockCountInWindow(20000)
	if blocks != 500 {
		t.Errorf("expected all blocks (500) for large window, got %d", blocks)
	}

	// For smaller window, estimate proportionally
	blocks = fragment.BlockCountInWindow(5000)
	// 5000/10000 * 500 = 250
	if blocks != 250 {
		t.Errorf("expected proportional blocks (250), got %d", blocks)
	}
}

func TestDensity(t *testing.T) {
	// Test density calculation
	d := Density(100, 1000)
	expected := 0.1
	if d != expected {
		t.Errorf("expected density %f, got %f", expected, d)
	}

	// Zero slots
	d = Density(100, 0)
	if d != 0 {
		t.Errorf("expected 0 for zero slots, got %f", d)
	}
}

func TestExpectedDensity(t *testing.T) {
	f := big.NewRat(1, 20)
	expected := 0.05
	d := ExpectedDensity(f)
	if d != expected {
		t.Errorf("expected density %f, got %f", expected, d)
	}
}

func TestGenesisVsPraosScenario(t *testing.T) {
	// Scenario: Compare chains with different densities
	config := testGenesisConfig()
	selector := NewGenesisSelector(config)

	// Honest chain: Higher density (more blocks per slot)
	honestChain := &SimpleChainFragment{
		Intersection: 0,
		Tip:          100000,
		Blocks:       5000, // 5% density
	}

	// Adversary chain: Longer tip but lower density
	adversaryChain := &SimpleChainFragment{
		Intersection: 0,
		Tip:          200000, // Longer span
		Blocks:       6000,   // More total blocks but lower density (3%)
	}

	// Genesis selection should prefer honest chain (higher density)
	result := selector.Compare(honestChain, adversaryChain)
	if result <= 0 {
		t.Error("Genesis should prefer honest chain with higher density")
	}

	// Verify adversary has more total blocks (Praos would prefer it)
	if adversaryChain.BlockCount() <= honestChain.BlockCount() {
		t.Error("test setup error: adversary chain should have more blocks")
	}
}
