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
	"log/slog"
	"math/big"
	"sync"
)

// PraosChainSelector implements Ouroboros Praos chain selection rules.
//
// Canonical Cardano decomposes chain selection as follows (matching
// IntersectMBO/ouroboros-consensus):
//
//  1. Ordinary adoption: prefer the chain with more blocks (higher block
//     number); equal length falls to tiebreakers (Compare). A node never
//     switches to a chain whose fork point is more than k BLOCKS behind its
//     current tip — such a fork is refused outright (ForkTooDeep), not
//     compared by any rule (Ouroboros.Consensus.Protocol.Abstract: "we never
//     switch to chains that fork off more than k blocks ago"). See
//     ExceedsMaxRollback.
//  2. Genesis density rule (syncing): between candidate chains, the one with
//     MORE BLOCKS within the genesis window — the first sgen slots after the
//     candidates' common intersection — wins, regardless of total length
//     (Ouroboros.Consensus.Genesis.Governor, densityDisconnect). The window
//     sgen is 3k/f slots for Shelley-family eras (cardano-ledger
//     computeStabilityWindow; mainnet: 3*2160/0.05 = 129600 slots). See
//     CompareWithDensity.
//
// NOTE: CompareWithDensity is an ORDERING adapted from the Genesis density
// rule; it is NOT the GDD governor. In ouroboros-consensus the density rule
// disconnects sparser PEERS (equal density disconnects too); an ordering
// cannot express "both lose", so equal density falls through to the ordinary
// comparison here (the same adaptation the Dingo node uses for fork
// resolution). Peer management remains the caller's concern.
type PraosChainSelector struct {
	// SecurityParam is the security parameter k: the maximum rollback depth
	// in BLOCKS (not slots).
	SecurityParam uint64
	// GenesisWindowSlots is the Ouroboros Genesis density window sgen, in
	// slots after the fork point. For Shelley-family eras this is 3k/f
	// (see genesis.ComputeGenesisWindow). Required (non-zero) for
	// CompareWithDensity/PreferredWithDensity to apply the density rule.
	GenesisWindowSlots uint64

	// warnZeroWindow throttles the missing-window misconfiguration warning
	// to once per selector.
	warnZeroWindow sync.Once
}

// NewPraosChainSelector creates a new Praos chain selector without a genesis
// window configured. Compare/Preferred work fully; density comparisons
// (CompareWithDensity/PreferredWithDensity) require a genesis window — use
// NewPraosChainSelectorWithWindow for those.
func NewPraosChainSelector(securityParam uint64) *PraosChainSelector {
	return &PraosChainSelector{
		SecurityParam: securityParam,
	}
}

// NewPraosChainSelectorWithWindow creates a Praos chain selector with an
// explicit Ouroboros Genesis density window (in slots). For Shelley-family
// networks the window is 3k/f slots — derive it with
// genesis.ComputeGenesisWindow(securityParam, activeSlotCoeff) rather than
// hard-coding, since it is network- and era-dependent.
func NewPraosChainSelectorWithWindow(
	securityParam uint64,
	genesisWindowSlots uint64,
) *PraosChainSelector {
	return &PraosChainSelector{
		SecurityParam:      securityParam,
		GenesisWindowSlots: genesisWindowSlots,
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

// CompareWithDensity compares two candidate chains under the Ouroboros
// Genesis density rule and is intended for the syncing context, where
// candidates may diverge deep in the past. forkSlot is the slot of the
// candidates' common intersection (for multiple candidates, the youngest
// point common to all — the same anchor ouroboros-consensus uses via
// sharedCandidatePrefix).
//
// Decision order (canonical Genesis, densityDisconnect):
//  1. The chain with MORE BLOCKS within the genesis window — slots
//     forkSlot+1 .. forkSlot+GenesisWindowSlots inclusive — is preferred,
//     regardless of total chain length.
//  2. Equal density falls through to the ordinary Praos comparison
//     (Compare). This is the ordering-context adaptation of GDD's tie rule:
//     the governor disconnects BOTH peers on an exact tie, which an ordering
//     cannot express; the Dingo node applies the same fallthrough for fork
//     resolution.
//
// A selector without a genesis window configured (GenesisWindowSlots == 0)
// cannot apply the density rule; this is a misconfiguration for deep-fork
// comparison, logged loudly, and the ordinary comparison is used instead.
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

	if p.GenesisWindowSlots == 0 {
		p.warnZeroWindow.Do(func() {
			slog.Warn(
				"density comparison without genesis window; using ordinary comparison",
				"securityParam", p.SecurityParam,
			)
		})
		return p.Compare(a, b)
	}

	// Genesis rule: density within the window decides first.
	aBlocks := a.BlocksInWindow(forkSlot, p.GenesisWindowSlots)
	bBlocks := b.BlocksInWindow(forkSlot, p.GenesisWindowSlots)
	if aBlocks != bBlocks {
		if aBlocks > bBlocks {
			return 1
		}
		return -1
	}

	// Equal density: fall through to the ordinary Praos comparison.
	return p.Compare(a, b)
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

// PreferredWithDensity returns the preferred chain from a set of syncing
// candidates under the Genesis density rule (see CompareWithDensity).
// forkSlot is the slot of the candidates' common intersection — the youngest
// point shared by ALL candidates, matching the anchor ouroboros-consensus
// derives via sharedCandidatePrefix.
func (p *PraosChainSelector) PreferredWithDensity(
	candidates []ChainTip,
	forkSlot uint64,
) ChainTip {
	return p.selectPreferred(candidates, func(a, b ChainTip) int {
		return p.CompareWithDensity(a, b, forkSlot)
	})
}

// ExceedsMaxRollback reports whether adopting a candidate whose fork point
// is at forkPointBlockNumber would require rolling back more than k BLOCKS
// from a chain currently at tipBlockNumber. k (SecurityParam) bounds
// rollback depth in blocks, not slots (ouroboros-consensus SecurityParam:
// "This talks about the number of /blocks/ we can roll back, not the number
// of /slots/").
//
// Such a candidate is NOT adoptable in canonical Cardano — the node refuses
// to switch (ChainSync ForkTooDeep) because the block k deep is immutable.
// Callers should treat a true result as "refuse the candidate", not as a
// signal to route to a different comparison rule; the Genesis density rule
// (CompareWithDensity) is about choosing among syncing candidates, not
// about adopting deep forks.
//
// This method replaces IsDeepFork, which compared a SLOT distance against k
// and therefore misclassified (~20x too eager at mainnet f=0.05) while also
// suggesting the wrong role ("deep ⇒ use density" instead of "exceeds max
// rollback ⇒ refuse"). The rename makes the semantic change compile-visible.
func (p *PraosChainSelector) ExceedsMaxRollback(
	forkPointBlockNumber uint64,
	tipBlockNumber uint64,
) bool {
	if tipBlockNumber < forkPointBlockNumber {
		return false
	}
	return tipBlockNumber-forkPointBlockNumber > p.SecurityParam
}

// SimpleChainTip is a simple implementation of ChainTip for testing.
type SimpleChainTip struct {
	slot        uint64
	blockNumber uint64
	vrfOutput   []byte
	// blockSlots holds the slots of this chain's blocks after the fork
	// point, for exact density (blocks-in-window) counting.
	blockSlots []uint64
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

// NewSimpleChainTipWithBlockSlots creates a SimpleChainTip carrying the
// explicit slots of the chain's blocks (typically those after the fork
// point), enabling exact BlocksInWindow counting in tests.
func NewSimpleChainTipWithBlockSlots(
	slot, blockNumber uint64,
	vrfOutput []byte,
	blockSlots []uint64,
) *SimpleChainTip {
	return &SimpleChainTip{
		slot:        slot,
		blockNumber: blockNumber,
		vrfOutput:   vrfOutput,
		blockSlots:  blockSlots,
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

// BlocksInWindow returns the number of blocks whose slots lie within
// forkSlot+1 .. forkSlot+windowSlots inclusive, counted exactly from the
// explicit block slots this tip was constructed with.
func (s *SimpleChainTip) BlocksInWindow(
	forkSlot, windowSlots uint64,
) uint64 {
	var count uint64
	for _, blockSlot := range s.blockSlots {
		if blockSlot > forkSlot && blockSlot-forkSlot <= windowSlots {
			count++
		}
	}
	return count
}
