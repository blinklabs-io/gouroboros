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

package bench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
)

var (
	benchVRFSeed          = []byte("consensus_bench_seed_for_vrf_32!")
	benchVRFSigner        *consensus.SimpleVRFSigner
	benchEpochNonce       = make([]byte, 32)
	benchVRFOutput        []byte
	benchActiveSlotCoeff  = big.NewRat(1, 20)
	consensusSink         any
	consensusParallelSink atomic.Bool
)

var benchConsensusModes = []struct {
	name string
	mode consensus.ConsensusMode
}{
	{"CPraos", consensus.ConsensusModeCPraos},
	{"TPraos", consensus.ConsensusModeTPraos},
}

func init() {
	var err error
	benchVRFSigner, err = consensus.NewSimpleVRFSigner(benchVRFSeed)
	if err != nil {
		panic(err)
	}
	for i := range benchEpochNonce {
		benchEpochNonce[i] = byte(i * 7)
	}
	input, err := vrf.MkInputVrf(50_000_000, benchEpochNonce)
	if err != nil {
		panic(err)
	}
	_, benchVRFOutput, err = benchVRFSigner.Prove(input)
	if err != nil {
		panic(err)
	}
}

// BenchmarkConsensusEligibilityPrepareCached measures warm immutable-state lookup
// plus the fresh public wrapper. Uncached preparation is measured in consensus.
func BenchmarkConsensusEligibilityPrepareCached(b *testing.B) {
	for _, m := range benchConsensusModes {
		b.Run(m.name, func(b *testing.B) {
			f := big.NewRat(1, 20)
			if _, err := consensus.NewLeaderEligibility(
				10_000_000_000, 100_000_000_000, f, m.mode,
			); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				e, err := consensus.NewLeaderEligibility(
					10_000_000_000,
					100_000_000_000,
					f,
					m.mode,
				)
				if err != nil {
					b.Fatal(err)
				}
				consensusSink = e
			}
		})
	}
}

// BenchmarkConsensusEligibilityCheckVRFOutput excludes preparation from timing.
// Boundary and MAX_REACHED comparator workloads live in the consensus suite.
func BenchmarkConsensusEligibilityCheckVRFOutput(b *testing.B) {
	for _, m := range benchConsensusModes {
		b.Run(m.name, func(b *testing.B) {
			e, err := consensus.NewLeaderEligibility(
				10_000_000_000,
				100_000_000_000,
				big.NewRat(1, 20),
				m.mode,
			)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				ok, err := e.CheckVRFOutput(benchVRFOutput)
				if err != nil {
					b.Fatal(err)
				}
				consensusSink = ok
			}
		})
	}
}

func BenchmarkConsensusLeaderValue(b *testing.B) {
	vrfOutput := make([]byte, 64)
	for i := range vrfOutput {
		vrfOutput[i] = byte(i)
	}
	b.ReportAllocs()
	for b.Loop() {
		consensusSink = consensus.VrfLeaderValue(vrfOutput)
	}
}

func BenchmarkConsensusVRFOutputToInt(b *testing.B) {
	leaderValue := consensus.VrfLeaderValue(benchVRFOutput)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		consensusSink = consensus.VRFOutputToInt(leaderValue)
	}
}

// BenchmarkConsensusFullLeaderElectionWorkflow includes input construction,
// real VRF signing, warm preparation lookup and finite eligibility evaluation.
// Immutable coefficients are safe to read concurrently; mutation is not supported.
func BenchmarkConsensusFullLeaderElectionWorkflow(b *testing.B) {
	for _, m := range benchConsensusModes {
		b.Run(m.name, func(b *testing.B) {
			if _, err := consensus.NewLeaderEligibility(10_000_000_000, 100_000_000_000, benchActiveSlotCoeff, m.mode); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			slot := uint64(50_000_000)
			for b.Loop() {
				result, err := consensus.IsSlotLeaderWithMode(
					slot,
					benchEpochNonce,
					10_000_000_000,
					100_000_000_000,
					benchActiveSlotCoeff,
					benchVRFSigner,
					m.mode,
				)
				slot++
				if err != nil {
					b.Fatal(err)
				}
				consensusSink = result
			}
		})
	}
}

func BenchmarkConsensusMkInputVrf(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		input, err := vrf.MkInputVrf(50_000_000, benchEpochNonce)
		if err != nil {
			b.Fatal(err)
		}
		consensusSink = input
	}
}

type benchmarkSchedule struct {
	ActiveStake   string
	AssignedSlots []uint64
	VrfSkeyHex    string
	DDenominator  string
	DNumerator    string
	FDenominator  string
	FNumerator    string
	FirstSlot     uint64
	Mode          string
	NonceHex      string
	PoolStake     string
	SlotCount     uint64
}

type benchmarkReference struct {
	fixture   benchmarkSchedule
	mode      consensus.ConsensusMode
	signer    *consensus.SimpleVRFSigner
	nonce     []byte
	evaluator *consensus.LeaderEligibility
}

// loadBenchmarkReference selects the complete shared 120-slot, 1/3, f=1/20,
// d=0 reference case, never a locally synthesized schedule or overlay policy.
func loadBenchmarkReference(
	b *testing.B,
	mode consensus.ConsensusMode,
) benchmarkReference {
	b.Helper()
	fs := fixtures.EmbeddedFixtures()
	const path = "upstream/e34/jormanager/node-schedules.json"
	data, err := fs.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	manifest, err := fs.ReadFile("upstream/e34/SHA256SUMS")
	if err != nil {
		b.Fatal(err)
	}
	if err := validateBenchmarkScheduleChecksum(data, manifest); err != nil {
		b.Fatal(err)
	}
	var corpus struct{ Cases []benchmarkSchedule }
	if err := json.Unmarshal(data, &corpus); err != nil {
		b.Fatal(err)
	}
	wantMode := "praos"
	if mode == consensus.ConsensusModeTPraos {
		wantMode = "tpraos"
	}
	for _, c := range corpus.Cases {
		if c.Mode != wantMode || c.SlotCount != 120 || c.PoolStake != "1" ||
			c.ActiveStake != "3" ||
			c.FNumerator != "1" ||
			c.FDenominator != "20" ||
			c.DNumerator != "0" ||
			c.DDenominator != "1" {
			continue
		}
		sk, err := hex.DecodeString(c.VrfSkeyHex)
		if err != nil || len(sk) != 64 {
			b.Fatal("malformed reference secret key")
		}
		signer, err := consensus.NewSimpleVRFSigner(sk[:32])
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(signer.PublicKey(), sk[32:]) {
			b.Fatal("reference public key mismatch")
		}
		nonce, err := hex.DecodeString(c.NonceHex)
		if err != nil || len(nonce) != 32 {
			b.Fatal("malformed reference nonce")
		}
		if c.FirstSlot > ^uint64(0)-c.SlotCount || len(c.AssignedSlots) == 0 {
			b.Fatal("malformed reference interval")
		}
		for i, slot := range c.AssignedSlots {
			if slot < c.FirstSlot || slot >= c.FirstSlot+c.SlotCount ||
				(i > 0 && slot <= c.AssignedSlots[i-1]) {
				b.Fatal("malformed reference assigned slots")
			}
		}
		e, err := consensus.NewLeaderEligibility(1, 3, big.NewRat(1, 20), mode)
		if err != nil {
			b.Fatal(err)
		}
		return benchmarkReference{c, mode, signer, nonce, e}
	}
	b.Fatal("required reference schedule missing")
	return benchmarkReference{}
}

func (r benchmarkReference) input(slot uint64) ([]byte, error) {
	if r.mode == consensus.ConsensusModeTPraos {
		return vrf.MkSeedTPraos(int64(slot), r.nonce, vrf.SeedL())
	}
	return vrf.MkInputVrf(int64(slot), r.nonce)
}

// BenchmarkConsensusReferenceSchedule signs every absolute slot in the complete
// fixture interval with one prepared evaluator and matches the entire slot list.
func BenchmarkConsensusReferenceSchedule(b *testing.B) {
	for _, m := range benchConsensusModes {
		b.Run(m.name, func(b *testing.B) {
			r := loadBenchmarkReference(b, m.mode)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				assigned := make([]uint64, 0, len(r.fixture.AssignedSlots))
				for slot := r.fixture.FirstSlot; slot < r.fixture.FirstSlot+r.fixture.SlotCount; slot++ {
					input, err := r.input(slot)
					if err != nil {
						b.Fatal(err)
					}
					proof, output, err := r.signer.Prove(input)
					if err != nil {
						b.Fatal(err)
					}
					if len(proof) != 80 || len(output) != 64 {
						b.Fatal("malformed signed VRF result")
					}
					ok, err := r.evaluator.CheckVRFOutput(output)
					if err != nil {
						b.Fatal(err)
					}
					if ok {
						assigned = append(assigned, slot)
					}
				}
				if !slices.Equal(assigned, r.fixture.AssignedSlots) {
					b.Fatal("reference schedule mismatch")
				}
				consensusSink = assigned
			}
		})
	}
}

// BenchmarkConsensusParallelHeaderLeadership measures the inbound leadership
// portion, not KES/opcert validation: construct the mode-specific input, verify
// a real proof and its output, then check finite eligibility. Every fixture slot
// is covered, including accepted and rejected outputs. Preparation/signing occur
// before timing and immutable state is shared safely by all workers.
func BenchmarkConsensusParallelHeaderLeadership(b *testing.B) {
	for _, m := range benchConsensusModes {
		b.Run(m.name, func(b *testing.B) {
			r := loadBenchmarkReference(b, m.mode)
			type signedSlot struct {
				slot          uint64
				proof, output []byte
				eligible      bool
			}
			slots := make([]signedSlot, r.fixture.SlotCount)
			for i := range slots {
				slot := r.fixture.FirstSlot + uint64(i)
				input, err := r.input(slot)
				if err != nil {
					b.Fatal(err)
				}
				proof, output, err := r.signer.Prove(input)
				if err != nil {
					b.Fatal(err)
				}
				slots[i] = signedSlot{
					slot,
					proof,
					output,
					slices.Contains(r.fixture.AssignedSlots, slot),
				}
			}
			key := r.signer.PublicKey()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				var eligible bool
				for pb.Next() {
					s := slots[i%len(slots)]
					i++
					input, err := r.input(s.slot)
					if err != nil {
						b.Error(err)
						return
					}
					valid, err := vrf.Verify(key, s.proof, s.output, input)
					if err != nil || !valid {
						b.Errorf("reference proof verification failed: %v", err)
						return
					}
					eligible, err = r.evaluator.CheckVRFOutput(s.output)
					if err != nil || eligible != s.eligible {
						b.Errorf("reference leadership mismatch: %v", err)
						return
					}
				}
				consensusParallelSink.Store(eligible)
			})
		})
	}
}

func validateBenchmarkScheduleChecksum(data, manifest []byte) error {
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	matched := false
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "jormanager/node-schedules.json" {
			continue
		}
		if matched {
			return fmt.Errorf("duplicate reference schedule checksum")
		}
		if fields[0] != digest {
			return fmt.Errorf("reference schedule checksum mismatch")
		}
		matched = true
	}
	if !matched {
		return fmt.Errorf("reference schedule checksum missing")
	}
	return nil
}

func TestBenchmarkScheduleChecksum(t *testing.T) {
	data := []byte("reference schedule")
	row := fmt.Sprintf(
		"%x  jormanager/node-schedules.json\n",
		sha256.Sum256(data),
	)
	if err := validateBenchmarkScheduleChecksum(data, []byte(row)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, manifest, want string }{
		{"duplicate", row + row, "duplicate"},
		{"missing", "", "missing"},
		{"mismatch", strings.Repeat("0", 64) + "  jormanager/node-schedules.json\n", "mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBenchmarkScheduleChecksum(data, []byte(tc.manifest))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s checksum error", err, tc.want)
			}
		})
	}
}
