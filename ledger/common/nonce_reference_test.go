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

package common_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// Epoch nonce evolution as a table test. gOuroboros keeps no chain state, so
// a caller keeps the nonces and calls CombineNonces, EpochNonceFromNonces and
// the VRF nonce derivations; the test runs that caller over every chain of up
// to a few blocks and compares its state with the reference's, recomputed
// from the chain, after every block and every rollback.
//
// The reference is ouroboros-consensus (PolyPraos.hs reupdate and tick) and
// cardano-ledger (Rules/Updn.hs, Rules/Tickn.hs). A nil slice is
// NeutralNonce.

const nonceRefSlotsPerEpoch = 3

type nonceRefConfig struct {
	name    string
	byron   bool // Shelley starts at epoch 1 after a Byron chain
	praos   bool // Praos block nonce and no extra entropy
	window  uint64
	maxSlot uint64
	entropy map[uint64]bool // epochs whose extraEntropy is set
}

type nonceRefBlock struct {
	id   int
	slot uint64
}

type nonceRefState struct {
	ev, cand, eta0, lab, leb []byte
	last                     uint64
}

func nonceRefHash(parts ...[]byte) []byte {
	h := common.Blake2b256Hash(bytes.Join(parts, nil))
	return h[:]
}

// nonceRefOp is the reference operator: two nonces hash their
// concatenation and NeutralNonce is the identity on both sides
// (cardano-ledger BaseTypes.hs).
func nonceRefOp(a, b []byte) []byte {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return nonceRefHash(a, b)
}

func nonceRefRawVrf(id int) []byte {
	tag := []byte(fmt.Sprintf("vrf output %d", id))
	return append(
		nonceRefHash(tag, []byte{0}),
		nonceRefHash(tag, []byte{1})...)
}

// nonceRefBlockNonce is the nonce a block contributes: TPraos hashes the
// nonce VRF output (mkNonceFromOutputVRF), Praos hashes "N" || output and
// hashes that again (vrfNonceValue).
func nonceRefBlockNonce(cfg nonceRefConfig, id int) []byte {
	raw := nonceRefRawVrf(id)
	if cfg.praos {
		return nonceRefHash(nonceRefHash([]byte("N"), raw))
	}
	return nonceRefHash(raw)
}

func nonceRefHeaderHash(id int) []byte {
	return nonceRefHash([]byte(fmt.Sprintf("header %d", id)))
}

var (
	nonceRefInitial  = nonceRefHash([]byte("shelley genesis"))
	nonceRefByronTip = nonceRefHash([]byte("last byron header"))
)

// nonceRefPrev is prevHashToNonce of block i's predecessor: NeutralNonce
// for the genesis, otherwise the predecessor's header hash.
func nonceRefPrev(
	cfg nonceRefConfig,
	chain []nonceRefBlock,
	i int,
) []byte {
	if i > 0 {
		return nonceRefHeaderHash(chain[i-1].id)
	}
	if cfg.byron {
		return nonceRefByronTip
	}
	return nil
}

func nonceRefEpoch(slot uint64) uint64 {
	return slot / nonceRefSlotsPerEpoch
}

// nonceRefBeforeFreeze is the candidate's update condition: the slot plus
// the window is strictly before the next epoch's first slot.
func nonceRefBeforeFreeze(cfg nonceRefConfig, slot uint64) bool {
	next := (nonceRefEpoch(slot) + 1) * nonceRefSlotsPerEpoch
	return slot+cfg.window < next
}

func (cfg nonceRefConfig) extraEntropy(epoch uint64) []byte {
	if cfg.praos || !cfg.entropy[epoch] {
		return nil
	}
	return nonceRefHash([]byte(fmt.Sprintf("extra entropy %d", epoch)))
}

// nonceRefInit is the state before the first Shelley block: the initial
// nonce, with NeutralNonce for the lab and last-epoch-block nonces.
func nonceRefInit() nonceRefState {
	return nonceRefState{
		ev:   nonceRefInitial,
		cand: nonceRefInitial,
		eta0: nonceRefInitial,
	}
}

func nonceRefRefFold(
	cfg nonceRefConfig,
	chain []nonceRefBlock,
) nonceRefState {
	st := nonceRefInit()
	for i, blk := range chain {
		e := nonceRefEpoch(blk.slot)
		if e > st.last {
			st.eta0 = nonceRefOp(
				nonceRefOp(st.cand, st.leb),
				cfg.extraEntropy(e),
			)
			st.leb = st.lab
		}
		st.ev = nonceRefOp(st.ev, nonceRefBlockNonce(cfg, blk.id))
		if nonceRefBeforeFreeze(cfg, blk.slot) {
			st.cand = st.ev
		}
		st.lab = nonceRefPrev(cfg, chain, i)
		st.last = e
	}
	return st
}

type nonceRefCounts struct {
	epochCalls, epochEntropy, postFreeze, skipped, rollbackCand int
}

// nonceRefNonce turns a stored nonce into a common.Nonce: nil is
// NeutralNonce.
func nonceRefNonce(b []byte) common.Nonce {
	if b == nil {
		return common.Nonce{Type: common.NonceTypeNeutral}
	}
	n := common.Nonce{Type: common.NonceTypeNonce}
	copy(n.Value[:], b)
	return n
}

// nonceRefBytes stores a common.Nonce: NeutralNonce is nil.
func nonceRefBytes(n common.Nonce) []byte {
	if n.Type == common.NonceTypeNeutral {
		return nil
	}
	return append([]byte(nil), n.Value[:]...)
}

// nonceRefCallerApply is the caller's step through gOuroboros: the block
// nonce from the raw VRF output, CombineNonces for the evolving nonce and
// EpochNonceFromNonces at an epoch boundary.
func nonceRefCallerApply(
	t *testing.T,
	cfg nonceRefConfig,
	st nonceRefState,
	blk nonceRefBlock,
	prev []byte,
	counts *nonceRefCounts,
) nonceRefState {
	t.Helper()
	e := nonceRefEpoch(blk.slot)
	if e > st.last {
		if e > st.last+1 {
			counts.skipped++
		}
		ee := cfg.extraEntropy(e)
		eta0, err := common.EpochNonceFromNonces(
			nonceRefNonce(st.cand),
			nonceRefNonce(st.leb),
			nonceRefNonce(ee),
		)
		if err != nil {
			t.Fatalf("EpochNonceFromNonces: %v", err)
		}
		st.eta0 = nonceRefBytes(eta0)
		counts.epochCalls++
		if ee != nil {
			counts.epochEntropy++
		}
		st.leb = st.lab
	}
	derive := common.NonceFromTPraosVRF
	if cfg.praos {
		derive = common.NonceFromPraosVRF
	}
	blockNonce, err := derive(nonceRefRawVrf(blk.id))
	if err != nil {
		t.Fatalf("block nonce: %v", err)
	}
	ev, err := common.CombineNonces(nonceRefNonce(st.ev), blockNonce)
	if err != nil {
		t.Fatalf("CombineNonces: %v", err)
	}
	st.ev = nonceRefBytes(ev)
	if nonceRefBeforeFreeze(cfg, blk.slot) {
		st.cand = st.ev
	} else {
		counts.postFreeze++
	}
	st.lab = prev
	st.last = e
	return st
}

func nonceRefCheck(
	t *testing.T,
	cfg nonceRefConfig,
	chain []nonceRefBlock,
	got nonceRefState,
) {
	t.Helper()
	want := nonceRefRefFold(cfg, chain)
	fields := []struct {
		name      string
		got, want []byte
	}{
		{"evolving", got.ev, want.ev},
		{"candidate", got.cand, want.cand},
		{"epoch", got.eta0, want.eta0},
		{"lab", got.lab, want.lab},
		{"last epoch block", got.leb, want.leb},
	}
	for _, f := range fields {
		if !bytes.Equal(f.got, f.want) {
			t.Fatalf(
				"%s: chain %v: %s nonce %x, reference %x",
				cfg.name, chain, f.name, f.got, f.want,
			)
		}
	}
	if got.last != want.last {
		t.Fatalf("%s: chain %v: last epoch %d, reference %d",
			cfg.name, chain, got.last, want.last)
	}
	// The VRF input for the tip's slot under this epoch nonce (mkInputVRF
	// and TPraos mkSeed with seedEta).
	tip := chain[len(chain)-1].slot
	var slotBytes [8]byte
	binary.BigEndian.PutUint64(slotBytes[:], tip)
	wantInput := nonceRefHash(slotBytes[:], want.eta0)
	gotInput, err := consensus.ComputeVRFInputFromNonce(
		tip,
		nonceRefNonce(got.eta0),
	)
	if err != nil {
		t.Fatalf("ComputeVRFInputFromNonce: %v", err)
	}
	if !bytes.Equal(gotInput, wantInput) {
		t.Fatalf("%s: chain %v: VRF input %x, reference %x",
			cfg.name, chain, gotInput, wantInput)
	}
	seed, err := vrf.MkSeedTPraosFromNonce(
		tip,
		nonceRefNonce(got.eta0),
		vrf.SeedEta(),
	)
	if err != nil {
		t.Fatalf("MkSeedTPraosFromNonce: %v", err)
	}
	seedEta := nonceRefHash(make([]byte, 8))
	for i := range wantInput {
		wantInput[i] ^= seedEta[i]
	}
	if !bytes.Equal(seed, wantInput) {
		t.Fatalf("%s: chain %v: nonce VRF seed %x, reference %x",
			cfg.name, chain, seed, wantInput)
	}
}

// nonceRefChains lists every chain of at most maxBlocks blocks in strictly
// increasing slots from first to maxSlot, numbering blocks from 1.
func nonceRefChains(
	first, maxSlot uint64,
	maxBlocks int,
) [][]nonceRefBlock {
	var out [][]nonceRefBlock
	var walk func(chain []nonceRefBlock, from uint64)
	walk = func(chain []nonceRefBlock, from uint64) {
		out = append(out, append([]nonceRefBlock(nil), chain...))
		if len(chain) == maxBlocks {
			return
		}
		for s := from; s <= maxSlot; s++ {
			walk(
				append(chain, nonceRefBlock{id: len(chain) + 1, slot: s}),
				s+1,
			)
		}
	}
	walk(nil, first)
	return out
}

func TestEpochNonceAsReference(t *testing.T) {
	const maxBlocks, maxRollback = 5, 2
	configs := []nonceRefConfig{
		{
			name:    "shelley start, TPraos",
			window:  1,
			maxSlot: 8,
			entropy: map[uint64]bool{2: true},
		},
		{
			name:    "byron start, TPraos",
			byron:   true,
			window:  1,
			maxSlot: 11,
			entropy: map[uint64]bool{3: true},
		},
		{
			name:    "shelley start, Praos, wider window",
			praos:   true,
			window:  2,
			maxSlot: 8,
		},
	}
	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			first := uint64(0)
			if cfg.byron {
				first = nonceRefSlotsPerEpoch
			}
			var counts nonceRefCounts
			for _, chain := range nonceRefChains(first, cfg.maxSlot, maxBlocks) {
				if len(chain) == 0 {
					continue
				}
				// Apply block by block, keeping a snapshot per block.
				snaps := []nonceRefState{nonceRefInit()}
				st := snaps[0]
				for i, blk := range chain {
					st = nonceRefCallerApply(
						t, cfg, st, blk,
						nonceRefPrev(cfg, chain, i), &counts,
					)
					snaps = append(snaps, st)
					nonceRefCheck(t, cfg, chain[:i+1], st)
				}
				// Roll back and grow a different block in every free slot.
				for d := 1; d <= maxRollback && d <= len(chain); d++ {
					kept := chain[:len(chain)-d]
					restored := snaps[len(kept)]
					before := nonceRefRefFold(cfg, chain).cand
					after := nonceRefRefFold(cfg, kept).cand
					if !bytes.Equal(before, after) {
						counts.rollbackCand++
					}
					from := first
					if len(kept) > 0 {
						from = kept[len(kept)-1].slot + 1
					}
					for s := from; s <= cfg.maxSlot; s++ {
						blk := nonceRefBlock{id: len(chain) + 1, slot: s}
						grown := append(
							append([]nonceRefBlock(nil), kept...),
							blk,
						)
						next := nonceRefCallerApply(
							t, cfg, restored, blk,
							nonceRefPrev(cfg, grown, len(kept)), &counts,
						)
						nonceRefCheck(t, cfg, grown, next)
					}
				}
			}
			// Reachability: every cell group was exercised.
			if counts.epochCalls == 0 || counts.postFreeze == 0 ||
				counts.skipped == 0 || counts.rollbackCand == 0 {
				t.Fatalf("a cell group was not reached: %+v", counts)
			}
			if !cfg.praos && counts.epochEntropy == 0 {
				t.Fatalf("extra entropy was never mixed in: %+v", counts)
			}
		})
	}
}

// TestNonceBlockNonceCells checks each consensus family's block nonce
// and the operator against the reference: TPraos mkNonceFromOutputVRF hashes
// the 64-byte output, Praos vrfNonceValue hashes "N" || output and hashes that
// again, and NeutralNonce is the identity on both sides of the operator.
func TestNonceBlockNonceCells(t *testing.T) {
	raw := nonceRefRawVrf(1)
	tpraos, err := common.NonceFromTPraosVRF(raw)
	if err != nil {
		t.Fatal(err)
	}
	praos, err := common.NonceFromPraosVRF(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := nonceRefBytes(tpraos); !bytes.Equal(got, nonceRefHash(raw)) {
		t.Fatalf("TPraos block nonce %x", got)
	}
	wantPraos := nonceRefHash(nonceRefHash([]byte("N"), raw))
	if got := nonceRefBytes(praos); !bytes.Equal(got, wantPraos) {
		t.Fatalf("Praos block nonce %x", got)
	}
	prev := nonceRefInitial
	cells := []struct {
		name       string
		a, b, want []byte
	}{
		{"neutral, nonce", nil, nonceRefHash(raw), nonceRefHash(raw)},
		{"nonce, neutral", prev, nil, prev},
		{"neutral, neutral", nil, nil, nil},
		{"nonce, nonce", prev, nonceRefHash(raw), nonceRefHash(prev, nonceRefHash(raw))},
	}
	for _, c := range cells {
		got, err := common.CombineNonces(nonceRefNonce(c.a), nonceRefNonce(c.b))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(nonceRefBytes(got), c.want) {
			t.Fatalf("%s: got %x, want %x", c.name, nonceRefBytes(got), c.want)
		}
	}
}
