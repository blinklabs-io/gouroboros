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

// Package genesis provides the Ouroboros Genesis chain selection rule.
//
// Ouroboros Genesis is a refinement of Praos used during network synchronization.
// It addresses the vulnerability where adversaries could trick syncing nodes into
// committing to adversarial chains.
//
// Key differences from Praos:
//   - Instead of primarily using chain length, Genesis uses block density
//   - Density is compared at the intersection point of candidate chains
//   - Leverages the property that honest chains have more blocks than adversarial
//     chains within a specific window of slots
//
// The genesis window is defined as 3k/f slots, matching the forecast range.
// Once a node finishes syncing, it gracefully converges to standard Praos selection.
//
// Relationship to the consensus package: the tip-level ordering surface for
// candidate chains is consensus.PraosChainSelector.CompareWithDensity, which
// applies the same windowed block-count rule to ChainTip values and falls
// through to the ordinary Praos comparison on equal density. This package's
// GenesisSelector operates on fragment-level data (ChainFragment) and breaks
// density ties by total block count instead; its Density and ExpectedDensity
// float helpers predate the integer windowed-count metric and are retained
// for compatibility. Derive the window itself with ComputeGenesisWindow in
// both cases so the 3k/f ceiling semantics stay single-sourced.
package genesis

import (
	"errors"
	"fmt"
	"math/big"
)

// GenesisConfig contains configuration for Genesis chain selection.
// Parameters should be loaded from network genesis configuration.
type GenesisConfig struct {
	// SecurityParam is k - the maximum rollback depth
	SecurityParam uint64

	// ActiveSlotCoeff is f - the active slot coefficient
	ActiveSlotCoeff *big.Rat

	// GenesisWindow is 3k/f slots - the window for density comparison
	// If zero, it is computed from SecurityParam and ActiveSlotCoeff. A
	// nonzero value must match that checked computation exactly.
	GenesisWindow uint64
}

// ComputeGenesisWindow calculates the genesis window (ceiling(3k/f)) from
// parameters. It returns an error when the parameters are invalid or the
// result cannot be represented as a uint64.
func ComputeGenesisWindow(
	securityParam uint64,
	activeSlotCoeff *big.Rat,
) (uint64, error) {
	if securityParam == 0 {
		return 0, errors.New(
			"genesis security parameter must be greater than zero",
		)
	}
	if activeSlotCoeff == nil {
		return 0, errors.New("genesis active slot coefficient is required")
	}
	if activeSlotCoeff.Sign() <= 0 ||
		activeSlotCoeff.Cmp(big.NewRat(1, 1)) > 0 {
		return 0, fmt.Errorf(
			"genesis active slot coefficient must be greater than zero and at most one: got %s",
			activeSlotCoeff.String(),
		)
	}
	// Genesis window = 3k/f
	// With k=2160 and f=0.05, window = 3*2160/0.05 = 129600 slots
	// Use big.Int to avoid uint64 overflow when computing 3*securityParam
	three := big.NewInt(3)
	k := new(big.Int).SetUint64(securityParam)
	threeK := new(big.Rat).SetInt(new(big.Int).Mul(three, k))
	window := new(big.Rat).Quo(threeK, activeSlotCoeff)

	// Convert to uint64 with CEILING division, matching cardano-ledger's
	// computeStabilityWindow ("ceiling $ (3 * fromIntegral k) /. f",
	// eras/shelley/impl/src/Cardano/Ledger/Shelley/StabilityWindow.hs) —
	// ouroboros-consensus uses that value as the Shelley-era genesis window
	// (shelleyEraParams: eraGenesisWin = GenesisWindow stabilityWindow).
	// Floor vs ceiling differs whenever 3k/f is not integral; the window is
	// consensus-relevant, so the rounding must match exactly. On mainnet
	// (k=2160, f=1/20) 3k/f = 129600 exactly, so both roundings agree.
	num := window.Num()
	denom := window.Denom()
	result, remainder := new(big.Int).QuoRem(
		num, denom, new(big.Int),
	)
	if remainder.Sign() != 0 {
		result.Add(result, big.NewInt(1))
	}

	if !result.IsUint64() {
		return 0, fmt.Errorf(
			"computed genesis window overflows uint64: ceiling(3*%d/%s)",
			securityParam,
			activeSlotCoeff.String(),
		)
	}
	return result.Uint64(), nil
}

// ChainFragment represents a fragment of a chain for selection purposes
type ChainFragment interface {
	// IntersectionSlot returns the slot where this fragment intersects with our chain
	IntersectionSlot() uint64

	// TipSlot returns the slot of the tip of this fragment
	TipSlot() uint64

	// BlockCount returns the number of blocks in the fragment
	BlockCount() uint64

	// BlockCountInWindow returns the number of blocks within the genesis window
	// from the intersection point
	BlockCountInWindow(windowSlots uint64) uint64
}

// GenesisSelector implements Ouroboros Genesis chain selection
type GenesisSelector struct {
	config GenesisConfig
}

// NewGenesisSelector creates a validated Genesis chain selector.
func NewGenesisSelector(config GenesisConfig) (*GenesisSelector, error) {
	computedWindow, err := ComputeGenesisWindow(
		config.SecurityParam,
		config.ActiveSlotCoeff,
	)
	if err != nil {
		return nil, err
	}
	if config.GenesisWindow == 0 {
		config.GenesisWindow = computedWindow
	} else if config.GenesisWindow != computedWindow {
		return nil, fmt.Errorf(
			"configured genesis window %d does not match computed genesis window %d",
			config.GenesisWindow,
			computedWindow,
		)
	}
	return &GenesisSelector{config: config}, nil
}

// Compare compares two chain fragments using Genesis rules
//
// Returns:
//   - positive if a is preferred
//   - negative if b is preferred
//   - zero if equal
//
// Genesis selection rules:
//  1. Compare density within the genesis window from intersection
//  2. Higher density (more blocks per slot) is preferred
//  3. If density is equal, fall back to chain length
func (g *GenesisSelector) Compare(a, b ChainFragment) int {
	// Count blocks within the genesis window
	blocksA := a.BlockCountInWindow(g.config.GenesisWindow)
	blocksB := b.BlockCountInWindow(g.config.GenesisWindow)

	// Compare by density (blocks within window)
	if blocksA != blocksB {
		if blocksA > blocksB {
			return 1
		}
		return -1
	}

	// Equal density - fall back to total length
	totalA := a.BlockCount()
	totalB := b.BlockCount()

	if totalA != totalB {
		if totalA > totalB {
			return 1
		}
		return -1
	}

	// Completely equal
	return 0
}

// Preferred returns the preferred chain from a set of candidates
func (g *GenesisSelector) Preferred(candidates []ChainFragment) ChainFragment {
	if len(candidates) == 0 {
		return nil
	}

	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if g.Compare(candidates[i], best) > 0 {
			best = candidates[i]
		}
	}
	return best
}

// ShouldUseGenesis determines if Genesis selection should be used
//
// Genesis selection is used during initial sync when:
//   - The local chain tip is far behind the network tip
//   - The node hasn't yet validated enough blocks to trust length-based selection
//
// Parameters:
//   - localTipSlot: the slot of the local chain tip
//   - networkTipSlot: the estimated slot of the network tip
//   - syncThreshold: how many slots behind to consider "syncing"
//
// Returns true if Genesis selection should be used
func (g *GenesisSelector) ShouldUseGenesis(
	localTipSlot, networkTipSlot, syncThreshold uint64,
) bool {
	if networkTipSlot <= localTipSlot {
		return false
	}

	slotsBehind := networkTipSlot - localTipSlot

	// Use Genesis if we're more than the threshold behind
	return slotsBehind > syncThreshold
}

// DefaultSyncThreshold returns a reasonable sync threshold
// Using the genesis window as the threshold ensures we have enough
// density information to make secure selections
func (g *GenesisSelector) DefaultSyncThreshold() uint64 {
	return g.config.GenesisWindow
}

// SimpleChainFragment is a simple implementation of ChainFragment for testing
type SimpleChainFragment struct {
	Intersection uint64
	Tip          uint64
	Blocks       uint64
}

// IntersectionSlot returns the intersection slot
func (f *SimpleChainFragment) IntersectionSlot() uint64 {
	return f.Intersection
}

// TipSlot returns the tip slot
func (f *SimpleChainFragment) TipSlot() uint64 {
	return f.Tip
}

// BlockCount returns total block count
func (f *SimpleChainFragment) BlockCount() uint64 {
	return f.Blocks
}

// BlockCountInWindow returns blocks within the specified window
func (f *SimpleChainFragment) BlockCountInWindow(windowSlots uint64) uint64 {
	// Guard against underflow: if Tip <= Intersection, no blocks in window
	if f.Tip <= f.Intersection {
		return 0
	}
	fragmentSlots := f.Tip - f.Intersection
	// If window is larger than or equal to fragment, return all blocks
	if windowSlots >= fragmentSlots {
		return f.Blocks
	}
	// Estimate proportionally
	proportion := float64(windowSlots) / float64(fragmentSlots)
	return uint64(float64(f.Blocks) * proportion)
}

// Density computes the block density (blocks per slot) for a fragment
func Density(blocks, slots uint64) float64 {
	if slots == 0 {
		return 0
	}
	return float64(blocks) / float64(slots)
}

// ExpectedDensity returns the expected density given the active slot coefficient
func ExpectedDensity(activeSlotCoeff *big.Rat) float64 {
	if activeSlotCoeff == nil {
		return 0.0
	}
	f, _ := activeSlotCoeff.Float64()
	return f
}
