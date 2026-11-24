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
	"crypto/ed25519"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

// tachysModelPool is one pool of a test distribution. IDs are compared
// numerically; tachysPool places the value in the first byte, so numeric
// order is byte order.
type tachysModelPool struct {
	id    byte
	stake uint64
}

// tachysModelDistributions enumerates every reference distribution over
// pools 1..3 with stake 0..maxStake, each pool present or absent. Zero-stake
// pools are kept because the reference PoolDistr keeps pools that have
// delegators but no stake.
func tachysModelDistributions(maxStake uint64) [][]tachysModelPool {
	var out [][]tachysModelPool
	var walk func(id byte, acc []tachysModelPool)
	walk = func(id byte, acc []tachysModelPool) {
		if id > 3 {
			out = append(out, append([]tachysModelPool(nil), acc...))
			return
		}
		walk(id+1, acc)
		for stake := uint64(0); stake <= maxStake; stake++ {
			walk(id+1, append(acc, tachysModelPool{id, stake}))
		}
	}
	walk(1, nil)
	return out
}

// tachysModelOrderings returns every ordering of pools.
func tachysModelOrderings(pools []TachysPool) [][]TachysPool {
	if len(pools) <= 1 {
		return [][]TachysPool{append([]TachysPool(nil), pools...)}
	}
	var out [][]TachysPool
	for i := range pools {
		rest := make([]TachysPool, 0, len(pools)-1)
		rest = append(rest, pools[:i]...)
		rest = append(rest, pools[i+1:]...)
		for _, tail := range tachysModelOrderings(rest) {
			out = append(out, append([]TachysPool{pools[i]}, tail...))
		}
	}
	return out
}

var tachysModelTwoTo256 = new(big.Int).Lsh(big.NewInt(1), 256)

// tachysSpecLeader is the CIP-0177 lookup, written independently of the
// selector: walk the pools in ID order and return the one whose interval
// [lo/T, hi/T) holds sample/2^256, compared as exact rationals.
func tachysSpecLeader(
	dist []tachysModelPool,
	total uint64,
	sample *big.Int,
) (byte, bool) {
	x := new(big.Rat).SetFrac(sample, tachysModelTwoTo256)
	var lo uint64
	for _, pool := range dist {
		hi := lo + pool.stake
		loRat := new(big.Rat).SetFrac64(int64(lo), int64(total)) //nolint:gosec
		hiRat := new(big.Rat).SetFrac64(int64(hi), int64(total)) //nolint:gosec
		if loRat.Cmp(x) <= 0 && x.Cmp(hiRat) < 0 {
			return pool.id, true
		}
		lo = hi
	}
	return 0, false
}

// tachysFirstSample is the smallest sample s with s/2^256 >= num/total,
// that is ceil(num*2^256/total).
func tachysFirstSample(num, total uint64) *big.Int {
	n := new(big.Int).Mul(new(big.Int).SetUint64(num), tachysModelTwoTo256)
	d := new(big.Int).SetUint64(total)
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}

func tachysSampleBytes(t *testing.T, sample *big.Int) []byte {
	t.Helper()
	require.True(t, sample.Sign() >= 0)
	require.Less(t, sample.Cmp(tachysModelTwoTo256), 0)
	return sample.FillBytes(make([]byte, 32))
}

// TestTachysModelSchedule checks the selector against the CIP-0177 leader
// schedule (each pool leads a stake-proportional interval of samples, in
// pool order) for every cell: every distribution of
// three pools with stake 0..3 passed through the Dingo caller (zero-stake
// pools dropped), every slice ordering, every declared total near the sum,
// both edges of every interval, and the hash path for a run of slots.
func TestTachysModelSchedule(t *testing.T) {
	t.Parallel()
	nonce := common.Blake2b256Hash([]byte("tachys model nonce"))
	for _, ref := range tachysModelDistributions(3) {
		name := fmt.Sprint(ref)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var input []TachysPool
			var spec []tachysModelPool
			var total uint64
			for _, pool := range ref {
				if pool.stake == 0 {
					continue
				}
				input = append(input, tachysPool(pool.id, pool.stake))
				spec = append(spec, pool)
				total += pool.stake
			}

			// A schedule exists exactly when stake does.
			if total == 0 {
				_, err := NewTachysLeaderSelector(input, total)
				require.Error(t, err)
				return
			}
			// Every wrong total is refused, so a leader is always defined.
			for _, wrong := range []uint64{0, total - 1, total + 1} {
				if wrong == total {
					continue
				}
				_, err := NewTachysLeaderSelector(input, wrong)
				require.Error(t, err, "declared total %d accepted", wrong)
			}

			var reference *TachysLeaderSelector
			for _, ordering := range tachysModelOrderings(input) {
				selector, err := NewTachysLeaderSelector(ordering, total)
				require.NoError(t, err)
				if reference == nil {
					reference = selector
				}
				checkTachysModelEdges(t, selector, spec, total)
				// Reusing the input slice changes no leader.
				for i := range ordering {
					ordering[i].Stake = 99
				}
				checkTachysModelEdges(t, selector, spec, total)
			}

			// On the hash path, exactly one pool leads, it is the pool the
			// CIP-0177 schedule names, and the same inputs give the same
			// leader, with every pool (zero-stake ones included) asked.
			for slot := uint64(0); slot < 16; slot++ {
				sample, err := TachysSample(nonce[:], slot)
				require.NoError(t, err)
				want, ok := tachysSpecLeader(
					spec, total, new(big.Int).SetBytes(sample[:]),
				)
				require.True(t, ok)
				leader, err := reference.Leader(nonce[:], slot)
				require.NoError(t, err)
				again, err := reference.Leader(nonce[:], slot)
				require.NoError(t, err)
				require.Equal(t, leader, again)
				require.Equal(t, tachysPool(want, 0).ID, leader)
				leaders := 0
				for _, pool := range ref {
					yes, err := reference.IsLeader(
						nonce[:], slot, tachysPool(pool.id, 0).ID,
					)
					require.NoError(t, err)
					if yes {
						leaders++
						require.NotZero(t, pool.stake)
					}
				}
				require.Equal(t, 1, leaders)
			}
		})
	}
}

// checkTachysModelEdges checks the leader against the CIP-0177 schedule at
// both edges of every interval and at the smallest and largest samples, and
// from the edges that each interval's width follows the stake: pool p leads
// exactly the samples
// ceil(lo*2^256/T) through ceil(hi*2^256/T)-1, a count within one of
// stake*2^256/T.
func checkTachysModelEdges(
	t *testing.T,
	selector *TachysLeaderSelector,
	spec []tachysModelPool,
	total uint64,
) {
	t.Helper()
	check := func(sample *big.Int) byte {
		want, ok := tachysSpecLeader(spec, total, sample)
		require.True(t, ok)
		got, err := selector.LeaderForSample(tachysSampleBytes(t, sample))
		require.NoError(t, err)
		require.Equal(
			t, tachysPool(want, 0).ID, got, "sample %x", sample,
		)
		return want
	}
	check(big.NewInt(0))
	check(new(big.Int).Sub(tachysModelTwoTo256, big.NewInt(1)))
	var lo uint64
	for _, pool := range spec {
		hi := lo + pool.stake
		first := tachysFirstSample(lo, total)
		next := tachysFirstSample(hi, total)
		require.Equal(t, pool.id, check(first))
		last := new(big.Int).Sub(next, big.NewInt(1))
		require.Equal(t, pool.id, check(last))
		if first.Sign() > 0 {
			require.NotEqual(
				t, pool.id, check(new(big.Int).Sub(first, big.NewInt(1))),
			)
		}
		if next.Cmp(tachysModelTwoTo256) < 0 {
			require.NotEqual(t, pool.id, check(next))
		}
		// count*T is within T of stake*2^256.
		count := new(big.Int).Sub(next, first)
		scaled := new(big.Int).Mul(count, new(big.Int).SetUint64(total))
		share := new(big.Int).Mul(
			new(big.Int).SetUint64(pool.stake), tachysModelTwoTo256,
		)
		diff := new(big.Int).Sub(scaled, share)
		require.Less(
			t, diff.CmpAbs(new(big.Int).SetUint64(total)), 0,
			"pool %d leads %s samples", pool.id, count,
		)
		lo = hi
	}
}

// TestTachysModelValidator checks the Tachys validator and block builder
// against CIP-0177: a header is valid exactly when its issuer is scheduled
// and the VRF, KES, opcert and registered-VRF-key checks pass, whatever the
// Praos lottery would have said; a producer forges exactly when scheduled.
func TestTachysModelValidator(t *testing.T) {
	t.Parallel()
	coldA := ed25519.NewKeyFromSeed(bytes32(0xa1))
	coldB := ed25519.NewKeyFromSeed(bytes32(0xb2))
	keyA := coldA.Public().(ed25519.PublicKey)
	keyB := coldB.Public().(ed25519.PublicKey)
	poolA := common.PoolKeyHash(common.Blake2b224Hash(keyA))
	poolB := common.PoolKeyHash(common.Blake2b224Hash(keyB))
	selector, err := NewTachysLeaderSelector(
		[]TachysPool{{ID: poolA, Stake: 1}, {ID: poolB, Stake: 1}},
		2,
	)
	require.NoError(t, err)
	nonce := common.Blake2b256Hash([]byte("tachys model validator nonce"))
	var slot uint64
	for slot = 1; ; slot++ {
		require.Less(t, slot, uint64(1000))
		leader, err := selector.Leader(nonce[:], slot)
		require.NoError(t, err)
		if leader == poolA {
			break
		}
	}

	vrfSeed := common.Blake2b256Hash([]byte("tachys model VRF seed"))
	signer, err := NewSimpleVRFSigner(vrfSeed[:])
	require.NoError(t, err)
	kesSeed := common.Blake2b256Hash([]byte("tachys model KES seed"))
	kesSecret, kesPublic, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed[:])
	require.NoError(t, err)
	config := testNetworkConfig()
	// A tiny active slot coefficient makes the Praos lottery fail for any
	// output, so an accepted header cannot have passed it.
	config.ActiveSlotCoeff.Rat = new(big.Rat).SetFrac(
		big.NewInt(1),
		new(big.Int).SetUint64(10_000_000_000_000_000_000),
	)
	validator, err := NewTachysHeaderValidator(config, selector)
	require.NoError(t, err)
	vrfInput, err := vrf.MkInputVrf(int64(slot), nonce[:]) //nolint:gosec
	require.NoError(t, err)
	vrfProof, vrfOutput, err := signer.Prove(vrfInput)
	require.NoError(t, err)
	message := []byte("tachys model header body")
	kesSignature, err := kes.Sign(kesSecret, 0, message)
	require.NoError(t, err)
	previousHash := make([]byte, 32)
	opCertBody := common.OpCertSignableBytes(kesPublic, 0, 0)
	flip := func(b []byte) []byte {
		out := append([]byte(nil), b...)
		out[0] ^= 0xff
		return out
	}

	for _, issuer := range []struct {
		name      string
		cold      ed25519.PrivateKey
		key       ed25519.PublicKey
		scheduled bool
	}{
		{"scheduled", coldA, keyA, true},
		{"unscheduled", coldB, keyB, false},
	} {
		for cell := range 1 << 5 {
			vrfOK := cell&1 == 0
			kesOK := cell&2 == 0
			ocertOK := cell&4 == 0
			vrfKeyOK := cell&8 == 0
			zeroStake := cell&16 != 0
			name := fmt.Sprintf(
				"%s/vrf=%t/kes=%t/ocert=%t/vrfkey=%t/zerostake=%t",
				issuer.name, vrfOK, kesOK, ocertOK, vrfKeyOK, zeroStake,
			)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				input := &ValidateHeaderInput{
					Slot:               slot,
					BlockNumber:        1,
					PrevHash:           previousHash,
					IssuerVkey:         issuer.key,
					VrfKey:             signer.PublicKey(),
					VrfProof:           vrfProof,
					VrfOutput:          vrfOutput,
					KesSignature:       kesSignature,
					HeaderBodyCbor:     message,
					OpCertHotVkey:      kesPublic,
					OpCertSignature:    ed25519.Sign(issuer.cold, opCertBody),
					OpCertCounterState: opCertSequence(0),
					PrevHeaderHash:     previousHash,
					EpochNonce:         nonce[:],
					PoolStake:          1,
					TotalStake:         2,
					RegisteredVrfKeyHash: common.Blake2b256Hash(signer.PublicKey()).
						Bytes(),
				}
				if !vrfOK {
					input.VrfProof = flip(vrfProof)
				}
				if !kesOK {
					input.KesSignature = flip(kesSignature)
				}
				if !ocertOK {
					input.OpCertSignature = flip(input.OpCertSignature)
				}
				if !vrfKeyOK {
					input.RegisteredVrfKeyHash = flip(
						input.RegisteredVrfKeyHash,
					)
				}
				if zeroStake {
					// Praos would refuse these stake inputs outright.
					input.PoolStake = 0
					input.TotalStake = 0
				}
				result := validator.ValidateHeader(input)
				want := issuer.scheduled && vrfOK && kesOK && ocertOK &&
					vrfKeyOK
				require.Equal(t, want, result.Valid, "%v", result.Errors)
				unscheduled := false
				for _, e := range result.Errors {
					if e != nil && strings.Contains(
						e.Error(),
						"issuer is not the scheduled Tachys leader",
					) {
						unscheduled = true
					}
				}
				// The schedule is checked only after the VRF proof.
				require.Equal(
					t, vrfOK && !issuer.scheduled, unscheduled,
					"%v", result.Errors,
				)
			})
		}
		t.Run(issuer.name+"/forge", func(t *testing.T) {
			t.Parallel()
			builder, err := NewTachysBlockBuilder(
				signer, nil, nil, issuer.key, selector,
			)
			require.NoError(t, err)
			for _, stakes := range [][2]uint64{{0, 0}, {1, 2}, {2, 2}} {
				result, err := builder.CheckSlotLeadership(
					slot, nonce[:], stakes[0], stakes[1],
				)
				require.NoError(t, err)
				require.Equal(t, issuer.scheduled, result.Eligible)
			}
		})
	}
}
