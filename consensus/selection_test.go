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
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus/genesis"
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

	// Reverse direction: the higher-VRF chain must lose symmetrically.
	result = selector.Compare(chainB, chainA)
	if result >= 0 {
		t.Error("chain with higher VRF output should not be preferred")
	}
}

// TestCompareEmptyVRFOutputs locks the missing-VRF branches of Compare:
// a chain without a VRF output loses the tiebreak; two chains without VRF
// outputs are equal. These branches sit on the density-tie fallthrough
// path of CompareWithDensity, so they are load-bearing for Genesis
// selection as well.
func TestCompareEmptyVRFOutputs(t *testing.T) {
	selector := NewPraosChainSelector(2160)

	withVRF := NewSimpleChainTip(1000, 100, []byte{0x42})
	noVRFa := NewSimpleChainTip(1000, 100, nil)
	noVRFb := NewSimpleChainTip(1000, 100, []byte{})

	if selector.Compare(noVRFa, withVRF) >= 0 {
		t.Error("chain missing VRF output should lose the tiebreak")
	}
	if selector.Compare(withVRF, noVRFa) <= 0 {
		t.Error("chain with VRF output should win over one without")
	}
	if selector.Compare(noVRFa, noVRFb) != 0 {
		t.Error("two chains without VRF outputs should be equal")
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

// TestCompareWithDensityIssue1936Regression is the regression test from
// issue #1936: a LONGER deep fork with LOWER density within the genesis
// window must lose to a shorter, denser chain. Before the fix,
// CompareWithDensity ran the longest-chain comparison first, so the longer
// sparse chain won and density was never consulted.
func TestCompareWithDensityIssue1936Regression(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 129600)
	vrf := make([]byte, 64)
	forkSlot := uint64(1000)
	// Window covers slots 1001..130600.

	// Honest shape: shorter overall (block 1400), 4 blocks in the window.
	shorterDense := NewSimpleChainTipWithBlockSlots(
		9000, 1400, vrf, []uint64{1010, 1020, 1030, 1040},
	)
	// Adversarial shape: longer overall (block 1500), 1 block in the
	// window, the rest far beyond it.
	longerSparse := NewSimpleChainTipWithBlockSlots(
		201000, 1500, vrf, []uint64{1100, 150000, 180000, 200500},
	)

	if got := selector.CompareWithDensity(
		shorterDense, longerSparse, forkSlot,
	); got <= 0 {
		t.Errorf(
			"denser chain must win the deep-fork comparison, got %d",
			got,
		)
	}
	if got := selector.CompareWithDensity(
		longerSparse, shorterDense, forkSlot,
	); got >= 0 {
		t.Errorf(
			"sparser chain must lose the deep-fork comparison, got %d",
			got,
		)
	}
}

// TestCompareWithDensityWindowBoundary pins the exact window contract:
// blocks at slots forkSlot+1 .. forkSlot+windowSlots inclusive count,
// matching ouroboros-consensus densityDisconnect, which clips the candidate
// suffix at firstSlotAfterGenesisWindow = succ(intersection) + sgen (i.e.
// keeps slots <= intersection + sgen).
func TestCompareWithDensityWindowBoundary(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 100)
	vrf := make([]byte, 64)
	forkSlot := uint64(1000)
	// Window covers slots 1001..1100.

	inWindow := NewSimpleChainTipWithBlockSlots(
		1100, 10, vrf, []uint64{1100}, // last slot inside the window
	)
	pastWindow := NewSimpleChainTipWithBlockSlots(
		1101, 10, vrf, []uint64{1101}, // first slot past the window
	)

	if got := selector.CompareWithDensity(
		inWindow, pastWindow, forkSlot,
	); got <= 0 {
		t.Errorf(
			"block at forkSlot+window must count toward density, got %d",
			got,
		)
	}

	atForkSlot := NewSimpleChainTipWithBlockSlots(
		1000, 10, vrf, []uint64{1000}, // the intersection itself
	)
	if got := selector.CompareWithDensity(
		inWindow, atForkSlot, forkSlot,
	); got <= 0 {
		t.Errorf(
			"block at the fork slot itself must not count, got %d",
			got,
		)
	}
}

// TestCompareWithDensityEqualFallsThroughToCompare pins the tie rule:
// equal within-window density falls through to the ordinary Praos
// comparison (the ordering-context adaptation of GDD's tie handling, as the
// Dingo node also does for fork resolution).
func TestCompareWithDensityEqualFallsThroughToCompare(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 100)
	vrf := make([]byte, 64)
	forkSlot := uint64(1000)

	longer := NewSimpleChainTipWithBlockSlots(
		5000, 200, vrf, []uint64{1010, 1020},
	)
	shorter := NewSimpleChainTipWithBlockSlots(
		4000, 100, vrf, []uint64{1030, 1040},
	)

	// Equal density (2 blocks each in window) -> ordinary rule -> longer
	// chain preferred, in both argument orders.
	if got := selector.CompareWithDensity(
		longer, shorter, forkSlot,
	); got <= 0 {
		t.Errorf(
			"equal density must fall through to Compare (longer wins), got %d",
			got,
		)
	}
	if got := selector.CompareWithDensity(
		shorter, longer, forkSlot,
	); got >= 0 {
		t.Errorf(
			"equal density must fall through to Compare (shorter loses), got %d",
			got,
		)
	}
}

// TestCompareWithDensityEqualDensityEqualLengthVRFDecides locks the FULL
// fallthrough chain end-to-end: equal within-window density, equal block
// number, so the decision reaches Compare's VRF tiebreak.
func TestCompareWithDensityEqualDensityEqualLengthVRFDecides(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 100)
	forkSlot := uint64(1000)

	lowVRF := make([]byte, 64)
	lowVRF[63] = 0x01
	highVRF := make([]byte, 64)
	highVRF[0] = 0xFF

	a := NewSimpleChainTipWithBlockSlots(
		2000, 100, lowVRF, []uint64{1010, 1020},
	)
	b := NewSimpleChainTipWithBlockSlots(
		2000, 100, highVRF, []uint64{1030, 1040},
	)

	if got := selector.CompareWithDensity(a, b, forkSlot); got <= 0 {
		t.Errorf(
			"density and length tied: lower VRF must win via Compare, got %d",
			got,
		)
	}
	if got := selector.CompareWithDensity(b, a, forkSlot); got >= 0 {
		t.Errorf(
			"density and length tied: higher VRF must lose via Compare, got %d",
			got,
		)
	}
}

// TestCompareWithDensityZeroWindowFallsBack documents the misconfiguration
// path: a selector without a genesis window cannot apply the density rule
// and falls back to the ordinary comparison (loudly, via slog).
func TestCompareWithDensityZeroWindowFallsBack(t *testing.T) {
	selector := NewPraosChainSelector(2160) // no window configured
	vrf := make([]byte, 64)

	shorterDense := NewSimpleChainTipWithBlockSlots(
		9000, 100, vrf, []uint64{1010, 1020, 1030},
	)
	longerSparse := NewSimpleChainTipWithBlockSlots(
		201000, 200, vrf, []uint64{150000},
	)

	// Without a window the ordinary rule governs: longer chain wins, in
	// both argument orders.
	if got := selector.CompareWithDensity(
		longerSparse, shorterDense, 1000,
	); got <= 0 {
		t.Errorf(
			"zero window must fall back to ordinary comparison, got %d",
			got,
		)
	}
	if got := selector.CompareWithDensity(
		shorterDense, longerSparse, 1000,
	); got >= 0 {
		t.Errorf(
			"zero window fallback must be symmetric, got %d",
			got,
		)
	}
}

// TestCompareWithDensityNilChains mirrors the nil handling of Compare.
func TestCompareWithDensityNilChains(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 100)
	vrf := make([]byte, 64)
	chain := NewSimpleChainTipWithBlockSlots(1100, 10, vrf, []uint64{1050})

	if selector.CompareWithDensity(nil, chain, 1000) >= 0 {
		t.Error("valid chain should be preferred over nil")
	}
	if selector.CompareWithDensity(chain, nil, 1000) <= 0 {
		t.Error("valid chain should be preferred over nil")
	}
	if selector.CompareWithDensity(nil, nil, 1000) != 0 {
		t.Error("nil vs nil should be equal")
	}
}

// TestGenesisWindowMainnetCrossCheck ties the selector's window to the
// shared derivation: 3k/f (cardano-ledger computeStabilityWindow, used by
// ouroboros-consensus as the Shelley-era genesis window) = 129600 slots on
// mainnet (k=2160, f=1/20).
func TestGenesisWindowMainnetCrossCheck(t *testing.T) {
	window := genesis.ComputeGenesisWindow(2160, big.NewRat(1, 20))
	if window != 129600 {
		t.Fatalf("expected mainnet genesis window 129600, got %d", window)
	}
	selector := NewPraosChainSelectorWithWindow(2160, window)
	if selector.GenesisWindowSlots != 129600 {
		t.Errorf(
			"expected selector window 129600, got %d",
			selector.GenesisWindowSlots,
		)
	}
}

// TestExceedsMaxRollback pins the corrected unit and boundary: k bounds
// rollback depth in BLOCKS (ouroboros-consensus SecurityParam: "the number
// of /blocks/ we can roll back, not the number of /slots/"). A rollback of
// exactly k blocks is permitted; k+1 is not ("we never switch to chains
// that fork off more than k blocks ago").
func TestExceedsMaxRollback(t *testing.T) {
	selector := NewPraosChainSelector(2160) // k = 2160 blocks

	// Fork point 1000 blocks behind the tip - well within k.
	if selector.ExceedsMaxRollback(4000, 5000) {
		t.Error("rollback of 1000 blocks should be within k=2160")
	}

	// Rollback of exactly k blocks - still permitted.
	if selector.ExceedsMaxRollback(1000, 3160) {
		t.Error("rollback of exactly k=2160 blocks should be permitted")
	}

	// Rollback of k+1 blocks - exceeds, not adoptable.
	if !selector.ExceedsMaxRollback(1000, 3161) {
		t.Error("rollback of 2161 blocks should exceed k=2160")
	}

	// Far beyond k.
	if !selector.ExceedsMaxRollback(1000, 100000) {
		t.Error("rollback of 99000 blocks should exceed k=2160")
	}

	// Edge case: fork point ahead of the tip (no rollback at all).
	if selector.ExceedsMaxRollback(2000, 1000) {
		t.Error("fork point ahead of tip requires no rollback")
	}
}

func TestPreferredWithDensity(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2160, 100)
	vrf := make([]byte, 64)
	forkSlot := uint64(1000)
	// Window covers slots 1001..1100.

	// The densest candidate (3 blocks in window) is also the SHORTEST -
	// density must still win.
	densest := NewSimpleChainTipWithBlockSlots(
		1090, 50, vrf, []uint64{1010, 1050, 1090},
	)
	candidates := []ChainTip{
		NewSimpleChainTipWithBlockSlots(
			5000, 500, vrf, []uint64{1020},
		), // 1 in window, longest
		densest,
		NewSimpleChainTipWithBlockSlots(
			3000, 300, vrf, []uint64{1030, 1060},
		), // 2 in window
	}

	preferred := selector.PreferredWithDensity(candidates, forkSlot)
	if preferred == nil {
		t.Fatal("expected non-nil preferred chain")
	}
	if preferred != ChainTip(densest) {
		t.Errorf(
			"expected densest candidate (block %d) preferred, got block %d",
			densest.BlockNumber(), preferred.BlockNumber(),
		)
	}

	// Empty candidate set returns nil.
	if got := selector.PreferredWithDensity(nil, forkSlot); got != nil {
		t.Error("expected nil for empty candidates")
	}

	// Full tie (same density, length, VRF): the FIRST candidate is kept,
	// mirroring Preferred's stick-with-incumbent behavior.
	first := NewSimpleChainTipWithBlockSlots(
		2000, 100, vrf, []uint64{1010},
	)
	second := NewSimpleChainTipWithBlockSlots(
		2000, 100, vrf, []uint64{1020},
	)
	tied := selector.PreferredWithDensity(
		[]ChainTip{first, second}, forkSlot,
	)
	if tied != ChainTip(first) {
		t.Error("full tie must keep the first candidate")
	}
}

// TestSimpleChainTipBlocksInWindow pins the exact counting contract of the
// test helper: slots forkSlot+1 .. forkSlot+windowSlots inclusive.
func TestSimpleChainTipBlocksInWindow(t *testing.T) {
	vrf := make([]byte, 64)
	tip := NewSimpleChainTipWithBlockSlots(
		2000, 100, vrf,
		[]uint64{1000, 1001, 1050, 1100, 1101, 2000},
	)

	// forkSlot=1000, window=100 -> slots 1001..1100 -> 1001, 1050, 1100.
	if got := tip.BlocksInWindow(1000, 100); got != 3 {
		t.Errorf("expected 3 blocks in window, got %d", got)
	}

	// Slot equal to forkSlot is excluded; slot forkSlot+window included.
	if got := tip.BlocksInWindow(1000, 101); got != 4 {
		t.Errorf("expected 4 blocks with window 101, got %d", got)
	}

	// A tip without block slots counts zero.
	empty := NewSimpleChainTip(1000, 100, vrf)
	if got := empty.BlocksInWindow(500, 1000); got != 0 {
		t.Errorf("expected 0 blocks without block slots, got %d", got)
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
