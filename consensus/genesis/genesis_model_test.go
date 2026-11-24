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
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// The tests in this file compare GenesisSelector with a transcription of
// the reference Genesis density rule, from ouroboros-consensus commit
// ec239d57:
//
//   - Genesis/Governor.hs 294-319: sharedCandidatePrefix. The LoE fragment
//     is the prefix all candidates share after the immutable tip.
//   - Genesis/Governor.hs 387-447: densityDisconnect's bounds. A
//     candidate's suffix after the LoE tip is clipped to the slots before
//     succ(LoE tip) + sgen; the lower bound is its length, the upper bound
//     adds the window slots after its head unless the peer has shown a
//     header at or past the window's end.
//   - Genesis/Governor.hs 448-501: the losing peers. A peer loses to
//     another whose clipped fragment starts at a different point
//     (AF.lastPoint: the oldest block, or the anchor when empty) and whose
//     lower bound reaches its own upper bound (its lower bound when idle),
//     provided the winner offers more than k blocks or the loser has shown
//     its whole window. An exact tie is enough.
//   - Storage/ChainDB/Impl/ChainSel.hs 907-950: trimToLoE. Chain
//     selection sees each candidate cut to at most k blocks past the LoE
//     tip; the bounds above use the whole candidates.
//   - MiniProtocol/ChainSync/Client/State.hs 72-76: a peer that has sent no
//     header is not considered.
//
// A chain is the list of its block slots after the immutable tip, which
// sits at slot 0; chains with equal slot prefixes share those blocks.

type genesisModelConfig struct {
	peers   int
	maxSlot uint64
	maxLen  int
	k       uint64
}

// sgen is ceiling(3k/f) at f = 1, the smallest window a valid
// configuration allows.
func (c genesisModelConfig) sgen() uint64 { return 3 * c.k }

// exactFragment counts the blocks of a candidate suffix inside the window
// the reference uses: slot s counts iff s > anchor and s-anchor <= window.
type exactFragment struct {
	anchor uint64
	slots  []uint64
}

func (f *exactFragment) IntersectionSlot() uint64 { return f.anchor }

func (f *exactFragment) TipSlot() uint64 {
	if len(f.slots) == 0 {
		return f.anchor
	}
	return f.slots[len(f.slots)-1]
}

func (f *exactFragment) BlockCount() uint64 { return uint64(len(f.slots)) }

func (f *exactFragment) BlockCountInWindow(windowSlots uint64) uint64 {
	var n uint64
	for _, s := range f.slots {
		if s > f.anchor && s-f.anchor <= windowSlots {
			n++
		}
	}
	return n
}

// refGenesisState is one test state: what every peer has sent, and which
// peers have said they have nothing more.
type refGenesisState struct {
	cands [][]uint64
	idle  []bool
}

func (s refGenesisState) String() string {
	return fmt.Sprintf("cands %v idle %v", s.cands, s.idle)
}

// refGenesisBounds is the reference's view of one state.
type refGenesisBounds struct {
	loeLen   int
	loeSlot  uint64
	clipLen  []int
	lb, ub   []uint64
	moreThan []bool
	losing   []bool
}

// firstPoint is AF.lastPoint of p's clipped fragment: the slots leading to
// its first block after the LoE tip, or to the LoE tip when it is empty.
func (b refGenesisBounds) firstPoint(st refGenesisState, p int) []uint64 {
	return st.cands[p][:b.loeLen+min(b.clipLen[p], 1)]
}

func sharedLen(a, b []uint64) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func refGDD(cfg genesisModelConfig, st refGenesisState) refGenesisBounds {
	n := len(st.cands)
	b := refGenesisBounds{
		clipLen:  make([]int, n),
		lb:       make([]uint64, n),
		ub:       make([]uint64, n),
		moreThan: make([]bool, n),
		losing:   make([]bool, n),
	}
	b.loeLen = len(st.cands[0])
	for _, c := range st.cands[1:] {
		b.loeLen = min(b.loeLen, sharedLen(st.cands[0], c))
	}
	if b.loeLen > 0 {
		b.loeSlot = st.cands[0][b.loeLen-1]
	}
	firstAfter := b.loeSlot + 1 + cfg.sgen()
	hasAfter := make([]bool, n)
	for p, c := range st.cands {
		suffix := c[b.loeLen:]
		for _, s := range suffix {
			if s < firstAfter {
				b.clipLen[p]++
			}
		}
		head := b.loeSlot
		if b.clipLen[p] > 0 {
			head = suffix[b.clipLen[p]-1]
		}
		hasAfter[p] = len(c) > 0 && c[len(c)-1] >= firstAfter
		b.lb[p] = uint64(b.clipLen[p])
		b.ub[p] = b.lb[p]
		if !hasAfter[p] {
			b.ub[p] += firstAfter - (head + 1)
		}
		b.moreThan[p] = uint64(len(suffix)) > cfg.k
	}
	for p0 := range st.cands {
		if len(st.cands[p0]) == 0 {
			continue
		}
		if !st.idle[p0] && b.clipLen[p0] == 0 && !hasAfter[p0] {
			continue
		}
		bar := b.ub[p0]
		if st.idle[p0] {
			bar = b.lb[p0]
		}
		for p1 := range st.cands {
			if len(st.cands[p1]) == 0 ||
				equalSlots(b.firstPoint(st, p0), b.firstPoint(st, p1)) {
				continue
			}
			if !b.moreThan[p1] && b.lb[p0] != b.ub[p0] {
				continue
			}
			if b.lb[p1] >= bar {
				b.losing[p0] = true
				break
			}
		}
	}
	return b
}

// genesisModelChains lists every strictly increasing slot list over
// 1..maxSlot of at most maxLen blocks.
func genesisModelChains(cfg genesisModelConfig) [][]uint64 {
	var out [][]uint64
	var grow func(prefix []uint64, next uint64)
	grow = func(prefix []uint64, next uint64) {
		out = append(out, append([]uint64(nil), prefix...))
		if len(prefix) == cfg.maxLen {
			return
		}
		for s := next; s <= cfg.maxSlot; s++ {
			grow(append(prefix, s), s+1)
		}
	}
	grow(nil, 1)
	return out
}

// forEachGenesisState visits every test state: each peer's chain
// and idle flag chosen independently.
func forEachGenesisState(cfg genesisModelConfig, visit func(refGenesisState)) {
	chains := genesisModelChains(cfg)
	st := refGenesisState{
		cands: make([][]uint64, cfg.peers),
		idle:  make([]bool, cfg.peers),
	}
	var fill func(p int)
	fill = func(p int) {
		if p == cfg.peers {
			visit(st)
			return
		}
		for _, c := range chains {
			for _, idle := range []bool{false, true} {
				st.cands[p], st.idle[p] = c, idle
				fill(p + 1)
			}
		}
	}
	fill(0)
}

func genesisModelSelector(t *testing.T, cfg genesisModelConfig) *GenesisSelector {
	t.Helper()
	sel, err := NewGenesisSelector(GenesisConfig{
		SecurityParam:   cfg.k,
		ActiveSlotCoeff: big.NewRat(1, 1),
	})
	require.NoError(t, err)
	require.Equal(t, cfg.sgen(), sel.config.GenesisWindow)
	return sel
}

// TestGenesisModelPreferredAgainstGDD checks Preferred, on fragments
// anchored at the LoE tip, against the reference bounds in every test
// state: it picks a chain with the most blocks in the window, breaks a
// window tie by total length and then by candidate order, and the governor
// would drop the chain it picked only on an exact tie with another chain.
// A second pass gives Preferred the candidates trimmed to k blocks past the
// LoE tip, as the reference's chain selection sees them, and checks the
// same order on the trimmed fragments.
func TestGenesisModelPreferredAgainstGDD(t *testing.T) {
	configs := []genesisModelConfig{
		{peers: 2, maxSlot: 5, maxLen: 3, k: 1},
		{peers: 3, maxSlot: 5, maxLen: 3, k: 1},
		{peers: 2, maxSlot: 8, maxLen: 4, k: 2},
	}
	for _, cfg := range configs {
		t.Run(fmt.Sprintf("%+v", cfg), func(t *testing.T) {
			sel := genesisModelSelector(t, cfg)
			var states, denserShorter, tieBothLose, choiceLosesOnTie, dropWhileSending, trimChanges int
			forEachGenesisState(cfg, func(st refGenesisState) {
				states++
				ref := refGDD(cfg, st)
				frags := make([]ChainFragment, len(st.cands))
				for p, c := range st.cands {
					frags[p] = &exactFragment{
						anchor: ref.loeSlot,
						slots:  c[ref.loeLen:],
					}
				}
				best := sel.Preferred(frags)
				choice := -1
				for p := range frags {
					if frags[p] == best {
						choice = p
						break
					}
				}
				require.GreaterOrEqual(t, choice, 0, "%v", st)

				want := 0
				for p := 1; p < len(frags); p++ {
					wb := frags[want].BlockCount()
					switch {
					case ref.lb[p] > ref.lb[want],
						ref.lb[p] == ref.lb[want] && frags[p].BlockCount() > wb:
						want = p
					}
				}
				require.Equal(t, want, choice, "%v: lb %v", st, ref.lb)
				for p := range frags {
					require.LessOrEqual(t, ref.lb[p], ref.lb[choice], "%v", st)
					if len(st.cands[p]) > len(st.cands[choice]) {
						denserShorter++
					}
				}

				trimmed := make([]ChainFragment, len(st.cands))
				trimLb := make([]uint64, len(st.cands))
				for p, c := range st.cands {
					f := &exactFragment{
						anchor: ref.loeSlot,
						slots:  c[ref.loeLen:min(len(c), ref.loeLen+int(cfg.k))],
					}
					trimmed[p] = f
					trimLb[p] = f.BlockCountInWindow(cfg.sgen())
					require.LessOrEqual(t, trimLb[p], ref.lb[p], "%v", st)
				}
				bestTrim := sel.Preferred(trimmed)
				trimChoice := -1
				for p := range trimmed {
					if trimmed[p] == bestTrim {
						trimChoice = p
						break
					}
				}
				wantTrim := 0
				for p := 1; p < len(trimmed); p++ {
					switch {
					case trimLb[p] > trimLb[wantTrim],
						trimLb[p] == trimLb[wantTrim] &&
							trimmed[p].BlockCount() > trimmed[wantTrim].BlockCount():
						wantTrim = p
					}
				}
				require.Equal(t, wantTrim, trimChoice, "%v: trimmed lb %v", st, trimLb)
				if trimChoice != choice {
					trimChanges++
				}

				if ref.losing[choice] {
					tie := false
					for q := range frags {
						if q != choice && ref.lb[q] == ref.lb[choice] &&
							!equalSlots(ref.firstPoint(st, q), ref.firstPoint(st, choice)) {
							tie = true
						}
					}
					require.True(t, tie, "%v: GDD drops the chosen chain without a tie (lb %v ub %v)", st, ref.lb, ref.ub)
					choiceLosesOnTie++
				}
				for p := range frags {
					for q := p + 1; q < len(frags); q++ {
						if ref.losing[p] && ref.losing[q] && ref.lb[p] == ref.lb[q] {
							tieBothLose++
						}
					}
					if ref.losing[p] && !st.idle[p] && ref.lb[p] < ref.ub[p] {
						anyMore := false
						for q := range frags {
							anyMore = anyMore || (len(st.cands[q]) > 0 && ref.moreThan[q])
						}
						require.True(t, anyMore, "%v: dropped while sending, no peer offers more than k", st)
						dropWhileSending++
					}
				}
			})
			// Every cell group the reference bounds distinguish must be
			// reached, or the table has stopped covering them.
			require.Positive(t, denserShorter, "density never beat length")
			require.Positive(t, tieBothLose, "no tie dropped both peers")
			require.Positive(t, choiceLosesOnTie, "the chosen chain never lost on a tie")
			require.Positive(t, trimChanges, "trimming to the LoE never changed the pick")
			if cfg.peers == 2 && cfg.k == 1 {
				require.Positive(t, dropWhileSending, "no peer dropped while sending")
			}
			t.Logf("%d states", states)
		})
	}
}

// TestGenesisModelGDDSameFirstBlock pins the reference's AF.lastPoint
// guard: peers whose clipped fragments start with the same block are not
// compared, however they continue.
func TestGenesisModelGDDSameFirstBlock(t *testing.T) {
	cfg := genesisModelConfig{peers: 3, k: 1}
	st := refGenesisState{
		cands: [][]uint64{{1, 2}, {1, 3}, {}},
		idle:  []bool{true, true, false},
	}
	ref := refGDD(cfg, st)
	require.Equal(t, []uint64{2, 2, 0}, ref.lb, "%v", st)
	require.Equal(t, []bool{false, false, false}, ref.losing, "%v", st)

	// The same pair with different first blocks: equal density, so both lose.
	st.cands = [][]uint64{{1, 2}, {2, 3}, {}}
	ref = refGDD(cfg, st)
	require.Equal(t, []bool{true, true, false}, ref.losing, "%v", st)
}

func equalSlots(a, b []uint64) bool {
	return len(a) == len(b) && sharedLen(a, b) == len(a)
}

// TestGenesisModelCompareCells is Compare's decision table: the window
// count decides, and only an equal count falls back to total length.
func TestGenesisModelCompareCells(t *testing.T) {
	sel := genesisModelSelector(t, genesisModelConfig{k: 1})
	// Window slots 1..3 after anchor 0. The first entry of each pair is in
	// the window, the second is the total.
	frag := func(inWindow, total uint64) *exactFragment {
		f := &exactFragment{}
		for i := uint64(1); i <= total; i++ {
			s := i
			if i > inWindow {
				s = 3 + i
			}
			f.slots = append(f.slots, s)
		}
		require.Equal(t, inWindow, f.BlockCountInWindow(3))
		return f
	}
	sign := func(a, b uint64) int {
		switch {
		case a > b:
			return 1
		case a < b:
			return -1
		}
		return 0
	}
	for _, wa := range []uint64{1, 2} {
		for _, wb := range []uint64{1, 2} {
			for _, extraA := range []uint64{0, 1, 2} {
				for _, extraB := range []uint64{0, 1, 2} {
					a, b := frag(wa, wa+extraA), frag(wb, wb+extraB)
					want := sign(wa, wb)
					if want == 0 {
						want = sign(wa+extraA, wb+extraB)
					}
					require.Equal(t, want, sel.Compare(a, b),
						"window %d/%d total %d/%d", wa, wb, wa+extraA, wb+extraB)
				}
			}
		}
	}
}
