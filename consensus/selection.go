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
	"math/big"
)

// PraosChainSelector implements Ouroboros Praos chain selection rules.
//
// Chain selection in Praos follows these rules:
//  1. Prefer the chain with more blocks (higher block number)
//  2. For equal length chains, prefer the one with lower VRF output (tiebreaker)
//  3. For deep forks (diverging more than k slots ago), compare chain density
type PraosChainSelector struct {
	// SecurityParam is the security parameter k (max rollback depth)
	SecurityParam uint64
}

// NewPraosChainSelector creates a new Praos chain selector.
func NewPraosChainSelector(securityParam uint64) *PraosChainSelector {
	return &PraosChainSelector{
		SecurityParam: securityParam,
	}
}

// Compare returns:
//   - positive if chain a is preferred over chain b
//   - negative if chain b is preferred over chain a
//   - zero if the chains are equivalent
//
// This implements the basic Praos chain selection without density comparison.
func (p *PraosChainSelector) Compare(a, b ChainTip) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Rule 1: Prefer longer chain (higher block number)
	if a.BlockNumber() != b.BlockNumber() {
		if a.BlockNumber() > b.BlockNumber() {
			return 1
		}
		return -1
	}

	// Rule 2: Equal length - prefer lower VRF output (tiebreaker)
	aVRFBytes := a.VRFOutput()
	bVRFBytes := b.VRFOutput()

	// Handle empty VRF outputs - missing VRF is less preferred
	if len(aVRFBytes) == 0 && len(bVRFBytes) == 0 {
		return 0 // Both missing VRF, consider equal
	}
	if len(aVRFBytes) == 0 {
		return -1 // a missing VRF is less preferred
	}
	if len(bVRFBytes) == 0 {
		return 1 // b missing VRF is less preferred
	}

	aVRF := new(big.Int).SetBytes(aVRFBytes)
	bVRF := new(big.Int).SetBytes(bVRFBytes)

	// Lower VRF output is preferred, so we negate the comparison
	cmp := aVRF.Cmp(bVRF)
	if cmp < 0 {
		return 1 // a has lower VRF, preferred
	}
	if cmp > 0 {
		return -1 // b has lower VRF, preferred
	}

	return 0
}

// CompareWithDensity compares chains considering density for deep forks.
// This should be used when the fork point is older than k slots.
//
// Parameters:
//   - a, b: the chain tips to compare
//   - forkSlot: the slot where the chains diverged
//
// Returns the same values as Compare.
func (p *PraosChainSelector) CompareWithDensity(
	a, b ChainTip,
	forkSlot uint64,
) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// First try standard comparison
	result := p.Compare(a, b)
	if result != 0 {
		return result
	}

	// For deep forks, compare density
	aDensity := a.Density(forkSlot)
	bDensity := b.Density(forkSlot)

	if aDensity > bDensity {
		return 1
	}
	if bDensity > aDensity {
		return -1
	}

	return 0
}

// selectPreferred returns the preferred chain using the given comparison function.
// The compare function should return positive if the first argument is preferred.
func (p *PraosChainSelector) selectPreferred(
	candidates []ChainTip,
	compare func(a, b ChainTip) int,
) ChainTip {
	if len(candidates) == 0 {
		return nil
	}

	preferred := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if compare(candidates[i], preferred) > 0 {
			preferred = candidates[i]
		}
	}

	return preferred
}

// Preferred returns the preferred chain from a set of candidates.
// Returns nil if candidates is empty.
func (p *PraosChainSelector) Preferred(candidates []ChainTip) ChainTip {
	return p.selectPreferred(candidates, p.Compare)
}

// PreferredWithDensity returns the preferred chain considering density.
// Use this when some candidates may represent deep forks.
func (p *PraosChainSelector) PreferredWithDensity(
	candidates []ChainTip,
	forkSlot uint64,
) ChainTip {
	return p.selectPreferred(candidates, func(a, b ChainTip) int {
		return p.CompareWithDensity(a, b, forkSlot)
	})
}

// IsDeepFork checks if a fork point is considered "deep" (older than k slots).
// Deep forks use density-based comparison instead of simple length comparison.
func (p *PraosChainSelector) IsDeepFork(
	forkSlot uint64,
	currentSlot uint64,
) bool {
	if currentSlot < forkSlot {
		return false
	}
	return currentSlot-forkSlot > p.SecurityParam
}

// SimpleChainTip is a simple implementation of ChainTip for testing.
type SimpleChainTip struct {
	slot        uint64
	blockNumber uint64
	vrfOutput   []byte
	// For density calculation
	blocksAfterFork uint64
	slotsAfterFork  uint64
}

// NewSimpleChainTip creates a new SimpleChainTip.
func NewSimpleChainTip(
	slot, blockNumber uint64,
	vrfOutput []byte,
) *SimpleChainTip {
	return &SimpleChainTip{
		slot:        slot,
		blockNumber: blockNumber,
		vrfOutput:   vrfOutput,
	}
}

// NewSimpleChainTipWithDensity creates a SimpleChainTip with density information.
func NewSimpleChainTipWithDensity(
	slot, blockNumber uint64,
	vrfOutput []byte,
	blocksAfterFork, slotsAfterFork uint64,
) *SimpleChainTip {
	return &SimpleChainTip{
		slot:            slot,
		blockNumber:     blockNumber,
		vrfOutput:       vrfOutput,
		blocksAfterFork: blocksAfterFork,
		slotsAfterFork:  slotsAfterFork,
	}
}

// Slot returns the tip slot.
func (s *SimpleChainTip) Slot() uint64 {
	return s.slot
}

// BlockNumber returns the tip block height.
func (s *SimpleChainTip) BlockNumber() uint64 {
	return s.blockNumber
}

// VRFOutput returns the VRF output of the tip block.
func (s *SimpleChainTip) VRFOutput() []byte {
	return s.vrfOutput
}

// Density returns the block density from the given slot.
// Density = blocks / slots for the portion of the chain after forkSlot.
// Note: forkSlot is part of the ChainTip interface but unused here as
// blocksAfterFork/slotsAfterFork are pre-computed during construction.
func (s *SimpleChainTip) Density(_ uint64) float64 {
	if s.slotsAfterFork == 0 {
		return 0
	}
	return float64(s.blocksAfterFork) / float64(s.slotsAfterFork)
}
