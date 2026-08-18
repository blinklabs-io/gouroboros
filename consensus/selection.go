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

// PraosChainSelector implements Ouroboros chain selection.
//
// Selection is routed explicitly on how deep the fork is, and the two
// regimes are kept separate:
//
//  1. Shallow forks (rollback of at most k blocks) use the ordinary
//     longest-chain rule: prefer more blocks, then the lower VRF output
//     as a tiebreaker. Density is never consulted here — a shorter but
//     denser shallow candidate must NOT beat a longer chain.
//  2. Deep forks (rollback of more than k blocks — the syncing regime the
//     Genesis rule governs) compare density first: the candidate with MORE
//     BLOCKS within the genesis window wins regardless of total length,
//     and only an exact tie falls back to the ordinary rule. This is what
//     stops a longer but sparser (adversarial) deep fork from winning.
//
// The routing predicate is IsDeepFork, and it is the only thing that
// decides which regime applies; every density entry point takes the depth
// inputs it needs, so a caller cannot reach the density rule for a shallow
// fork.
//
// Units: the security parameter k bounds a number of BLOCKS, not slots.
// Ouroboros.Consensus.Config.SecurityParam: "NOTE: This talks about the
// number of /blocks/ we can roll back, not the number of /slots/". Fork
// depth is therefore measured between block numbers, while the genesis
// window is a span of SLOTS; ForkPoint carries both so the two units
// cannot be transposed at a call site.
//
// NOTE: the density comparison here is an ORDERING derived from the
// Genesis density rule; it is not the GDD governor. In
// ouroboros-consensus (Ouroboros.Consensus.Genesis.Governor,
// densityDisconnect) the rule disconnects sparser PEERS and an exact tie
// disconnects both; an ordering cannot express "both lose", so an exact
// tie falls through to the ordinary comparison here — the same adaptation
// the Dingo node uses for fork resolution. Peer management remains the
// caller's concern.
type PraosChainSelector struct {
	// SecurityParam is the security parameter k: the maximum rollback
	// depth, in BLOCKS (not slots).
	SecurityParam uint64
	// GenesisWindowSlots is the Ouroboros Genesis density window sgen, in
	// SLOTS after the fork point. For Shelley-family eras this is 3k/f
	// (see genesis.ComputeGenesisWindow; mainnet 129600). When it is zero,
	// or when the tips being compared do not implement WindowBlockCounter,
	// deep-fork comparison falls back to the legacy ChainTip.Density
	// ratio; see compareDensity.
	GenesisWindowSlots uint64

	// warnFallbackDensity throttles the legacy-metric warning to once per
	// selector.
	warnFallbackDensity sync.Once
}

// ForkPoint identifies the intersection between the current selection and
// the candidate chains being compared: the last block they share.
//
// Selection needs that point in two different units, so both are carried
// explicitly rather than inferred from one another:
//
//   - Slot anchors the genesis window, which spans SLOTS.
//   - BlockNumber measures rollback depth, which is what k bounds (BLOCKS).
//
// Naming the fields keeps the two from being transposed at call sites,
// where both are plain uint64.
type ForkPoint struct {
	// Slot is the slot of the last block common to the chains compared.
	Slot uint64
	// BlockNumber is the block height of that same common block.
	BlockNumber uint64
}

// WindowBlockCounter is an optional extension of ChainTip that supplies the
// canonical Ouroboros Genesis density metric: an integer count of blocks
// within a fixed window of slots after the fork point.
//
// ChainTip itself is unchanged, so existing implementations keep working;
// a tip that also implements this interface gets the canonical metric
// instead of the ChainTip.Density ratio (see compareDensity).
type WindowBlockCounter interface {
	// BlocksInWindow returns the number of blocks on this chain whose slot
	// lies within the genesis window after forkSlot: a block at slot s
	// counts iff s > forkSlot && s-forkSlot <= windowSlots. State the
	// bound in that subtraction form in implementations too — computing
	// forkSlot+windowSlots can wrap near the uint64 maximum.
	//
	// This matches densityDisconnect, which clips each candidate suffix at
	// the first slot after the genesis window (succ(intersection) + sgen)
	// and counts the headers that remain. Selection may call this once per
	// pairwise comparison; implementations over large chains should
	// precompute or index rather than rescan.
	BlocksInWindow(forkSlot, windowSlots uint64) uint64
}

// NewPraosChainSelector creates a new Praos chain selector.
//
// The selector has no genesis window configured, so deep-fork comparison
// uses the legacy ChainTip.Density ratio. Use
// NewPraosChainSelectorWithWindow for the canonical Genesis metric.
func NewPraosChainSelector(securityParam uint64) *PraosChainSelector {
	return &PraosChainSelector{
		SecurityParam: securityParam,
	}
}

// NewPraosChainSelectorWithWindow creates a selector that applies the
// canonical Genesis density metric, counting blocks within
// genesisWindowSlots slots after the fork point.
//
// securityParam is k in BLOCKS; genesisWindowSlots is sgen in SLOTS,
// derived with genesis.ComputeGenesisWindow(k, f).
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

// IsDeepFork reports whether adopting a candidate that branches at fork
// would require rolling back more than k BLOCKS from the current
// selection, whose tip is at tipBlockNumber.
//
// This is the routing predicate for chain selection: false selects the
// ordinary longest-chain rule, true selects the Genesis density rule (see
// CompareWithDensity).
//
// The unit is deliberate. k bounds blocks, not slots
// (Ouroboros.Consensus.Config.SecurityParam: "NOTE: This talks about the
// number of /blocks/ we can roll back, not the number of /slots/"), so
// depth is measured between block numbers. Measuring it in slots
// misclassifies forks by roughly 1/f — about 20x at mainnet f=0.05 —
// treating ordinary shallow forks as deep.
//
// A fork point at or ahead of the tip requires no rollback and is never
// deep; the subtraction below therefore cannot underflow.
func (p *PraosChainSelector) IsDeepFork(
	fork ForkPoint,
	tipBlockNumber uint64,
) bool {
	if tipBlockNumber <= fork.BlockNumber {
		return false
	}
	return tipBlockNumber-fork.BlockNumber > p.SecurityParam
}

// compareDensity compares the density of two tips over the genesis window
// after fork. It returns a positive value if a is denser, a negative value
// if b is denser, and zero if they are equal.
//
// The canonical Genesis metric is an integer count of blocks within the
// window, which requires a configured window and both tips to implement
// WindowBlockCounter. When either is missing this falls back to the legacy
// ChainTip.Density ratio and warns once, because that ratio is measured
// over the whole fork suffix rather than a fixed window and so is not the
// Genesis metric.
func (p *PraosChainSelector) compareDensity(
	a, b ChainTip,
	fork ForkPoint,
) int {
	aCounter, aOK := a.(WindowBlockCounter)
	bCounter, bOK := b.(WindowBlockCounter)

	if p.GenesisWindowSlots > 0 && aOK && bOK {
		aBlocks := aCounter.BlocksInWindow(fork.Slot, p.GenesisWindowSlots)
		bBlocks := bCounter.BlocksInWindow(fork.Slot, p.GenesisWindowSlots)
		if aBlocks > bBlocks {
			return 1
		}
		if bBlocks > aBlocks {
			return -1
		}
		return 0
	}

	p.warnFallbackDensity.Do(func() {
		slog.Warn(
			"deep-fork comparison using legacy density ratio; configure a "+
				"genesis window and implement WindowBlockCounter for the "+
				"canonical Genesis metric",
			"securityParam", p.SecurityParam,
			"genesisWindowSlots", p.GenesisWindowSlots,
		)
	})

	aDensity := a.Density(fork.Slot)
	bDensity := b.Density(fork.Slot)
	if aDensity > bDensity {
		return 1
	}
	if bDensity > aDensity {
		return -1
	}
	return 0
}

// CompareWithDensity compares two chains using the Ouroboros Genesis rule,
// branching explicitly on how deep the fork is relative to the security
// parameter k:
//
//   - Forks requiring a rollback of at most k blocks (see IsDeepFork) are
//     not "deep" and are resolved using the ordinary longest-chain rule
//     (Compare), exactly as ordinary Praos selection would. Density is not
//     consulted, so a shorter but denser shallow candidate cannot win.
//   - Forks requiring a rollback of more than k blocks are resolved by
//     density over the genesis window *first*; the longest-chain rule is
//     only used as a tiebreaker when densities are equal. This prevents a
//     longer but sparser (and therefore potentially adversarial) deep fork
//     from beating a shorter, denser one.
//
// Parameters:
//   - a, b: the chain tips to compare
//   - fork: the intersection the chains diverge at, carrying both the slot
//     (which anchors the genesis window) and the block number (which
//     measures rollback depth)
//   - tipBlockNumber: block height of the current selection, against which
//     rollback depth is measured
//
// Returns the same values as Compare.
func (p *PraosChainSelector) CompareWithDensity(
	a, b ChainTip,
	fork ForkPoint,
	tipBlockNumber uint64,
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

	// Shallow forks (at most k blocks deep) use ordinary longest-chain
	// selection; density is not considered per the Genesis rule.
	if !p.IsDeepFork(fork, tipBlockNumber) {
		return p.Compare(a, b)
	}

	// Deep forks: density within the genesis window decides first.
	if result := p.compareDensity(a, b, fork); result != 0 {
		return result
	}

	// Equal density - fall back to the ordinary rule as a tiebreaker.
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

// PreferredWithDensity returns the preferred chain from a set of
// candidates, applying exactly the contract CompareWithDensity documents:
// shallow forks are resolved by the ordinary rule, and only forks deeper
// than k blocks are resolved by density.
//
// fork is the candidates' common intersection — the youngest point shared
// by ALL candidates, matching the anchor ouroboros-consensus derives via
// sharedCandidatePrefix — and tipBlockNumber is the current selection's
// height, so depth is measured on the same basis as CompareWithDensity.
func (p *PraosChainSelector) PreferredWithDensity(
	candidates []ChainTip,
	fork ForkPoint,
	tipBlockNumber uint64,
) ChainTip {
	return p.selectPreferred(candidates, func(a, b ChainTip) int {
		return p.CompareWithDensity(a, b, fork, tipBlockNumber)
	})
}

// SimpleChainTip is a simple implementation of ChainTip for testing.
//
// It deliberately does NOT implement WindowBlockCounter: it holds no
// per-block slots, so it cannot answer a window count. See
// WindowedChainTip for a tip that can.
type SimpleChainTip struct {
	slot        uint64
	blockNumber uint64
	vrfOutput   []byte
	// For the legacy density ratio
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

// WindowedChainTip is a chain tip that carries the slot of each of its
// blocks and can therefore answer the canonical Genesis window count.
//
// It is a distinct type on purpose. WindowBlockCounter is satisfied by a
// type, not by an instance, so a tip that holds no per-block slots must not
// carry the method at all — otherwise it would claim the capability and
// answer zero, which selection cannot distinguish from a chain that
// genuinely has no blocks in the window. Keeping the capability and the
// data in the same type makes that state unrepresentable.
type WindowedChainTip struct {
	*SimpleChainTip
	// blockSlots holds the slot of each block on this chain.
	blockSlots []uint64
}

// NewWindowedChainTip creates a tip that supports the canonical Genesis
// window metric, from the slots of the blocks on the chain.
func NewWindowedChainTip(
	slot, blockNumber uint64,
	vrfOutput []byte,
	blockSlots []uint64,
) *WindowedChainTip {
	return &WindowedChainTip{
		SimpleChainTip: NewSimpleChainTip(slot, blockNumber, vrfOutput),
		blockSlots:     blockSlots,
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

// BlocksInWindow implements WindowBlockCounter. A block at slot s counts
// iff s > forkSlot && s-forkSlot <= windowSlots; the bound is evaluated in
// that subtraction form so it cannot wrap near the uint64 maximum.
func (w *WindowedChainTip) BlocksInWindow(
	forkSlot, windowSlots uint64,
) uint64 {
	if windowSlots == 0 {
		return 0
	}
	var count uint64
	for _, blockSlot := range w.blockSlots {
		if blockSlot > forkSlot && blockSlot-forkSlot <= windowSlots {
			count++
		}
	}
	return count
}

// Density overrides the embedded ratio, which would otherwise report zero
// because a windowed tip carries block slots rather than precomputed
// blocks/slots totals. It derives the legacy ratio from those slots so a
// windowed tip stays comparable when the other side of a comparison
// implements only ChainTip and the selector must fall back.
func (w *WindowedChainTip) Density(forkSlot uint64) float64 {
	var blocks, maxSlot uint64
	for _, blockSlot := range w.blockSlots {
		if blockSlot <= forkSlot {
			continue
		}
		blocks++
		if blockSlot > maxSlot {
			maxSlot = blockSlot
		}
	}
	if blocks == 0 {
		return 0
	}
	return float64(blocks) / float64(maxSlot-forkSlot)
}
