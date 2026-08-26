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

package genesis

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGenesisConfig returns a GenesisConfig for testing with mainnet-like parameters.
func testGenesisConfig() GenesisConfig {
	f := big.NewRat(1, 20) // 0.05
	return GenesisConfig{
		SecurityParam:   2160,
		ActiveSlotCoeff: f,
		GenesisWindow:   129600,
	}
}

func mustNewGenesisSelector(
	t *testing.T,
	config GenesisConfig,
) *GenesisSelector {
	t.Helper()
	selector, err := NewGenesisSelector(config)
	require.NoError(t, err)
	return selector
}

func TestComputeGenesisWindow(t *testing.T) {
	// With k=2160 and f=0.05, window = 3*2160/0.05 = 129600 slots
	f := big.NewRat(1, 20)
	window, err := ComputeGenesisWindow(2160, f)
	require.NoError(t, err)

	expected := uint64(129600)
	if window != expected {
		t.Errorf("expected genesis window %d, got %d", expected, window)
	}
}

// TestComputeGenesisWindowCeiling pins the rounding to CEILING division,
// matching cardano-ledger's computeStabilityWindow
// ("ceiling $ (3 * fromIntegral k) /. f"): whenever 3k/f is not integral,
// the window rounds up, never down.
func TestComputeGenesisWindowCeiling(t *testing.T) {
	// k=2160, f=7/100: 3*2160*100/7 = 648000/7 = 92571.43... -> 92572.
	window, err := ComputeGenesisWindow(2160, big.NewRat(7, 100))
	require.NoError(t, err)
	assert.Equal(
		t, uint64(92572), window, "expected ceiling(648000/7) = 92572",
	)

	// Exact division is unaffected: k=10, f=1/2 -> 3*10*2 = 60.
	window, err = ComputeGenesisWindow(10, big.NewRat(1, 2))
	require.NoError(t, err)
	assert.Equal(t, uint64(60), window, "expected exact division 60")
}

func TestComputeGenesisWindowAcceptsMaximumWindow(t *testing.T) {
	window, err := ComputeGenesisWindow(^uint64(0)/3, big.NewRat(1, 1))
	require.NoError(t, err)
	require.Equal(t, ^uint64(0), window)
}

// TestComputeGenesisWindowRejectsInvalidParameters pins the checked contract:
// invalid parameters and unrepresentable results are rejected explicitly.
func TestComputeGenesisWindowRejectsInvalidParameters(t *testing.T) {
	tests := []struct {
		name            string
		securityParam   uint64
		activeSlotCoeff *big.Rat
		expectError     string
	}{
		{
			name:            "zero security parameter",
			activeSlotCoeff: big.NewRat(1, 20),
			expectError:     "security parameter",
		},
		{
			name:          "nil active slot coefficient",
			securityParam: 2160,
			expectError:   "active slot coefficient",
		},
		{
			name:            "zero active slot coefficient",
			securityParam:   2160,
			activeSlotCoeff: big.NewRat(0, 1),
			expectError:     "active slot coefficient",
		},
		{
			name:            "negative active slot coefficient",
			securityParam:   2160,
			activeSlotCoeff: big.NewRat(-1, 20),
			expectError:     "active slot coefficient",
		},
		{
			name:            "active slot coefficient above one",
			securityParam:   2160,
			activeSlotCoeff: big.NewRat(2, 1),
			expectError:     "active slot coefficient",
		},
		{
			name:            "window overflows uint64",
			securityParam:   ^uint64(0),
			activeSlotCoeff: big.NewRat(1, 1),
			expectError:     "overflows uint64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window, err := ComputeGenesisWindow(
				test.securityParam,
				test.activeSlotCoeff,
			)
			require.Zero(t, window)
			require.ErrorContains(t, err, test.expectError)
		})
	}
}

// callNewGenesisSelector lets this regression exercise both the historical
// one-result constructor and the validated two-result constructor. That keeps
// the invalid-input assertions runnable against the exact pre-fix revision.
func callNewGenesisSelector(
	t *testing.T,
	config GenesisConfig,
) (*GenesisSelector, error) {
	t.Helper()
	results := reflect.ValueOf(NewGenesisSelector).Call(
		[]reflect.Value{reflect.ValueOf(config)},
	)
	require.Contains(t, []int{1, 2}, len(results))
	selector, _ := results[0].Interface().(*GenesisSelector)
	if len(results) == 1 || results[1].IsNil() {
		return selector, nil
	}
	err, ok := results[1].Interface().(error)
	require.True(t, ok, "second constructor result must be an error")
	return selector, err
}

func TestNewGenesisSelectorRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      GenesisConfig
		expectError string
	}{
		{
			name: "zero security parameter",
			config: GenesisConfig{
				ActiveSlotCoeff: big.NewRat(1, 20),
				GenesisWindow:   1,
			},
			expectError: "security parameter",
		},
		{
			name: "nil active slot coefficient",
			config: GenesisConfig{
				SecurityParam: 1,
				GenesisWindow: 3,
			},
			expectError: "active slot coefficient",
		},
		{
			name: "zero active slot coefficient",
			config: GenesisConfig{
				SecurityParam:   1,
				ActiveSlotCoeff: big.NewRat(0, 1),
				GenesisWindow:   3,
			},
			expectError: "active slot coefficient",
		},
		{
			name: "negative active slot coefficient",
			config: GenesisConfig{
				SecurityParam:   1,
				ActiveSlotCoeff: big.NewRat(-1, 20),
				GenesisWindow:   3,
			},
			expectError: "active slot coefficient",
		},
		{
			name: "active slot coefficient above one",
			config: GenesisConfig{
				SecurityParam:   1,
				ActiveSlotCoeff: big.NewRat(2, 1),
				GenesisWindow:   3,
			},
			expectError: "active slot coefficient",
		},
		{
			name: "genesis window below derived value",
			config: GenesisConfig{
				SecurityParam:   2160,
				ActiveSlotCoeff: big.NewRat(1, 20),
				GenesisWindow:   129599,
			},
			expectError: "does not match computed genesis window",
		},
		{
			name: "genesis window above derived value",
			config: GenesisConfig{
				SecurityParam:   2160,
				ActiveSlotCoeff: big.NewRat(1, 20),
				GenesisWindow:   129601,
			},
			expectError: "does not match computed genesis window",
		},
		{
			name: "derived genesis window overflows uint64",
			config: GenesisConfig{
				SecurityParam:   ^uint64(0),
				ActiveSlotCoeff: big.NewRat(1, 1),
			},
			expectError: "overflows uint64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector, err := callNewGenesisSelector(t, test.config)
			require.Nil(t, selector)
			require.ErrorContains(t, err, test.expectError)
		})
	}
}

func TestNewGenesisSelectorAcceptsBoundaryConfig(t *testing.T) {
	selector, err := NewGenesisSelector(GenesisConfig{
		SecurityParam:   1,
		ActiveSlotCoeff: big.NewRat(1, 1),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), selector.config.GenesisWindow)
}

func TestNewGenesisSelectorAcceptsExactGenesisWindow(t *testing.T) {
	selector, err := NewGenesisSelector(GenesisConfig{
		SecurityParam:   2160,
		ActiveSlotCoeff: big.NewRat(1, 20),
		GenesisWindow:   129600,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(129600), selector.config.GenesisWindow)
}

// TestNewGenesisSelectorDerivedWindowCeiling pins the derived-window path
// through NewGenesisSelector with a non-integral 3k/f, so the constructor
// inherits the ceiling semantics too.
func TestNewGenesisSelectorDerivedWindowCeiling(t *testing.T) {
	selector := mustNewGenesisSelector(t, GenesisConfig{
		SecurityParam:   2160,
		ActiveSlotCoeff: big.NewRat(7, 100),
	})
	// 3*2160*100/7 = 648000/7 = 92571.43... -> 92572.
	assert.Equal(
		t, uint64(92572), selector.config.GenesisWindow,
		"expected derived window 92572",
	)
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
	selector := mustNewGenesisSelector(t, config)

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

	selector := mustNewGenesisSelector(t, config)

	expectedWindow := uint64(129600)
	if selector.config.GenesisWindow != expectedWindow {
		t.Errorf("expected computed window %d, got %d",
			expectedWindow, selector.config.GenesisWindow)
	}
}

func TestCompareByDensity(t *testing.T) {
	config := testGenesisConfig()
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

	var candidates []ChainFragment
	best := selector.Preferred(candidates)
	if best != nil {
		t.Error("expected nil for empty candidates")
	}
}

func TestPreferredSingle(t *testing.T) {
	config := testGenesisConfig()
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
	selector := mustNewGenesisSelector(t, config)

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
