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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// Pre-computed test data for consensus benchmarks
var (
	// VRF keys for leader election benchmarks
	benchVRFSeed   = []byte("consensus_bench_seed_for_vrf_32!")
	benchVRFSigner *consensus.SimpleVRFSigner

	// Epoch nonce (eta0) - 32 bytes
	benchEpochNonce = make([]byte, 32)

	// Pre-computed VRF output for threshold comparison benchmarks
	benchVRFOutput []byte

	// Active slot coefficient (mainnet uses 1/20 = 0.05)
	benchActiveSlotCoeff = big.NewRat(1, 20)
)

func init() {
	var err error
	benchVRFSigner, err = consensus.NewSimpleVRFSigner(benchVRFSeed)
	if err != nil {
		panic("failed to create benchmark VRF signer: " + err.Error())
	}

	// Initialize epoch nonce with deterministic data
	for i := range benchEpochNonce {
		benchEpochNonce[i] = byte(i * 7)
	}

	// Pre-compute a VRF output for threshold comparison
	vrfInput := vrf.MkInputVrf(50_000_000, benchEpochNonce)
	_, benchVRFOutput, err = benchVRFSigner.Prove(vrfInput)
	if err != nil {
		panic("failed to generate benchmark VRF output: " + err.Error())
	}
}

// BenchmarkConsensusThreshold benchmarks CertifiedNatThreshold with various
// stake ratios.
// This is the core function for computing leadership probability thresholds.
func BenchmarkConsensusThreshold(b *testing.B) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	stakes := []struct {
		name       string
		poolStake  uint64
		totalStake uint64
	}{
		{"1pct", 1_000_000_000, 100_000_000_000},
		{"10pct", 10_000_000_000, 100_000_000_000},
		{"50pct", 50_000_000_000, 100_000_000_000},
	}

	for _, s := range stakes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = consensus.CertifiedNatThreshold(
					s.poolStake,
					s.totalStake,
					activeSlotCoeff,
				)
			}
		})
	}
}

// BenchmarkConsensusThresholdWithMode benchmarks threshold calculation for both
// CPRAOS (Babbage+) and TPraos (Shelley-Alonzo) consensus modes.
func BenchmarkConsensusThresholdWithMode(b *testing.B) {
	activeSlotCoeff := big.NewRat(1, 20)
	poolStake := uint64(10_000_000_000)
	totalStake := uint64(100_000_000_000)

	modes := []struct {
		name string
		mode consensus.ConsensusMode
	}{
		{"CPraos", consensus.ConsensusModeCPraos},
		{"TPraos", consensus.ConsensusModeTPraos},
	}

	for _, m := range modes {
		b.Run(m.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = consensus.CertifiedNatThresholdWithMode(
					poolStake,
					totalStake,
					activeSlotCoeff,
					m.mode,
				)
			}
		})
	}
}

// BenchmarkConsensusLeaderValue benchmarks VrfLeaderValue computation.
// This computes BLAKE2b-256("L" || vrfOutput) for CPRAOS leader election.
func BenchmarkConsensusLeaderValue(b *testing.B) {
	// Standard 64-byte VRF output
	vrfOutput := make([]byte, 64)
	for i := range vrfOutput {
		vrfOutput[i] = byte(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = consensus.VrfLeaderValue(vrfOutput)
	}
}

// BenchmarkConsensusVRFOutputToInt benchmarks conversion of VRF leader value to
// big.Int.
func BenchmarkConsensusVRFOutputToInt(b *testing.B) {
	leaderValue := consensus.VrfLeaderValue(benchVRFOutput)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = consensus.VRFOutputToInt(leaderValue)
	}
}

// BenchmarkConsensusIsSlotLeader benchmarks the full leader election check.
// This includes VRF input creation, proof generation, threshold calculation,
// and eligibility comparison.
func BenchmarkConsensusIsSlotLeader(b *testing.B) {
	if benchVRFSigner == nil {
		b.Fatal("benchVRFSigner not initialized")
	}

	stakes := []struct {
		name       string
		poolStake  uint64
		totalStake uint64
	}{
		{"1pct", 1_000_000_000, 100_000_000_000},
		{"10pct", 10_000_000_000, 100_000_000_000},
		{"50pct", 50_000_000_000, 100_000_000_000},
	}

	for _, s := range stakes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				slot := uint64(50_000_000 + i)
				_, err := consensus.IsSlotLeader(
					slot,
					benchEpochNonce,
					s.poolStake,
					s.totalStake,
					benchActiveSlotCoeff,
					benchVRFSigner,
				)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkConsensusIsSlotLeaderFromComponents benchmarks leader election from
// pre-computed VRF output and threshold components.
func BenchmarkConsensusIsSlotLeaderFromComponents(b *testing.B) {
	stakes := []struct {
		name       string
		poolStake  uint64
		totalStake uint64
	}{
		{"1pct", 1_000_000_000, 100_000_000_000},
		{"10pct", 10_000_000_000, 100_000_000_000},
		{"50pct", 50_000_000_000, 100_000_000_000},
	}

	for _, s := range stakes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = consensus.IsSlotLeaderFromComponents(
					benchVRFOutput,
					s.poolStake,
					s.totalStake,
					benchActiveSlotCoeff,
				)
			}
		})
	}
}

// BenchmarkConsensusMkInputVrf benchmarks VRF input creation (nonce
// computation).
// This is the first step in leader election: creating the VRF input from
// slot number and epoch nonce.
func BenchmarkConsensusMkInputVrf(b *testing.B) {
	slots := []struct {
		name string
		slot int64
	}{
		{"LowSlot", 1000},
		{"MidSlot", 50_000_000},
		{"HighSlot", 140_000_000},
	}

	for _, s := range slots {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = vrf.MkInputVrf(s.slot, benchEpochNonce)
			}
		})
	}
}

// BenchmarkConsensusIsVRFOutputBelowThreshold benchmarks the threshold
// comparison.
func BenchmarkConsensusIsVRFOutputBelowThreshold(b *testing.B) {
	threshold := consensus.CertifiedNatThreshold(
		10_000_000_000,
		100_000_000_000,
		benchActiveSlotCoeff,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = consensus.IsVRFOutputBelowThreshold(benchVRFOutput, threshold)
	}
}

// BenchmarkConsensusFullLeaderElectionWorkflow benchmarks the complete leader
// election workflow from slot to eligibility determination.
func BenchmarkConsensusFullLeaderElectionWorkflow(b *testing.B) {
	if benchVRFSigner == nil {
		b.Fatal("benchVRFSigner not initialized")
	}

	poolStake := uint64(10_000_000_000)
	totalStake := uint64(100_000_000_000)
	baseSlot := int64(50_000_000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		slot := baseSlot + int64(i)

		// Step 1: Create VRF input
		vrfInput := vrf.MkInputVrf(slot, benchEpochNonce)

		// Step 2: Generate VRF proof
		_, output, err := benchVRFSigner.Prove(vrfInput)
		if err != nil {
			b.Fatal(err)
		}

		// Step 3: Compute threshold
		threshold := consensus.CertifiedNatThreshold(
			poolStake,
			totalStake,
			benchActiveSlotCoeff,
		)

		// Step 4: Check eligibility
		_ = consensus.IsVRFOutputBelowThreshold(output, threshold)
	}
}

// BenchmarkConsensusParallelThreshold benchmarks concurrent threshold
// calculations.
func BenchmarkConsensusParallelThreshold(b *testing.B) {
	activeSlotCoeff := big.NewRat(1, 20)
	poolStake := uint64(10_000_000_000)
	totalStake := uint64(100_000_000_000)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = consensus.CertifiedNatThreshold(
				poolStake,
				totalStake,
				activeSlotCoeff,
			)
		}
	})
}
