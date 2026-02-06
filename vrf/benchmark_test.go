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

package vrf

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// Benchmark seeds - using values from existing tests
var (
	// Standard test seed (exactly 32 bytes)
	benchSeed = []byte("test_seed_for_vrf_testing!!!_32!")

	// Cardano test vector seed
	cardanoSeedHex = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
)

// Pre-generated test data for benchmarks
var (
	benchPK    []byte
	benchSK    []byte
	benchProof []byte
	benchHash  []byte
	benchAlpha = []byte("benchmark test message for VRF")
)

func init() {
	var err error
	benchPK, benchSK, err = KeyGen(benchSeed)
	if err != nil {
		panic("failed to generate benchmark keys: " + err.Error())
	}

	benchProof, benchHash, err = Prove(benchSK, benchAlpha)
	if err != nil {
		panic("failed to generate benchmark proof: " + err.Error())
	}
}

// BenchmarkKeyGen benchmarks VRF key pair generation
func BenchmarkKeyGen(b *testing.B) {
	b.ReportAllocs()

	b.Run("StandardSeed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := KeyGen(benchSeed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("CardanoSeed", func(b *testing.B) {
		b.ReportAllocs()
		seed, _ := hex.DecodeString(cardanoSeedHex)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := KeyGen(seed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RandomSeed", func(b *testing.B) {
		b.ReportAllocs()
		seeds := make([][]byte, b.N)
		for i := 0; i < b.N; i++ {
			seeds[i] = make([]byte, SeedSize)
			if _, err := rand.Read(seeds[i]); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := KeyGen(seeds[i])
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkProve benchmarks VRF proof generation with various input sizes
func BenchmarkProve(b *testing.B) {
	b.ReportAllocs()

	// Different alpha (message) sizes to benchmark
	alphaSizes := []struct {
		name string
		size int
	}{
		{"Empty", 0},
		{"32B", 32},
		{"256B", 256},
		{"1KB", 1024},
	}

	for _, as := range alphaSizes {
		b.Run(as.name, func(b *testing.B) {
			b.ReportAllocs()
			alpha := make([]byte, as.size)
			if as.size > 0 {
				if _, err := rand.Read(alpha); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, err := Prove(benchSK, alpha)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerify benchmarks full VRF verification (Verify with output comparison)
func BenchmarkVerify(b *testing.B) {
	b.ReportAllocs()

	b.Run("Valid", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			valid, err := Verify(benchPK, benchProof, benchHash, benchAlpha)
			if err != nil {
				b.Fatal(err)
			}
			if !valid {
				b.Fatal("expected valid proof")
			}
		}
	})

	// Benchmark with various alpha sizes
	alphaSizes := []struct {
		name string
		size int
	}{
		{"Empty", 0},
		{"32B", 32},
		{"256B", 256},
		{"1KB", 1024},
	}

	for _, as := range alphaSizes {
		b.Run("Alpha_"+as.name, func(b *testing.B) {
			b.ReportAllocs()
			alpha := make([]byte, as.size)
			if as.size > 0 {
				if _, err := rand.Read(alpha); err != nil {
					b.Fatal(err)
				}
			}
			// Generate valid proof for this alpha
			proof, output, err := Prove(benchSK, alpha)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Verify(benchPK, proof, output, alpha)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerifyAndHash benchmarks VRF verification with hash extraction
func BenchmarkVerifyAndHash(b *testing.B) {
	b.ReportAllocs()

	b.Run("Standard", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := VerifyAndHash(benchPK, benchProof, benchAlpha)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	alphaSizes := []struct {
		name string
		size int
	}{
		{"Empty", 0},
		{"32B", 32},
		{"256B", 256},
		{"1KB", 1024},
	}

	for _, as := range alphaSizes {
		b.Run("Alpha_"+as.name, func(b *testing.B) {
			b.ReportAllocs()
			alpha := make([]byte, as.size)
			if as.size > 0 {
				if _, err := rand.Read(alpha); err != nil {
					b.Fatal(err)
				}
			}
			// Generate valid proof for this alpha
			proof, _, err := Prove(benchSK, alpha)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := VerifyAndHash(benchPK, proof, alpha)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkProofToHash benchmarks hash extraction from VRF proof
func BenchmarkProofToHash(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ProofToHash(benchProof)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMkInputVrf benchmarks VRF input creation for Cardano leader election
func BenchmarkMkInputVrf(b *testing.B) {
	b.ReportAllocs()

	// Realistic Cardano parameters
	// Slots typically range from 0 to ~500 million+ on mainnet
	// eta0 is the epoch nonce (32 bytes)
	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i)
	}

	b.Run("LowSlot", func(b *testing.B) {
		b.ReportAllocs()
		slot := int64(1000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = MkInputVrf(slot, eta0)
		}
	})

	b.Run("HighSlot", func(b *testing.B) {
		b.ReportAllocs()
		// Mainnet has processed ~140 million slots as of 2025
		slot := int64(140_000_000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = MkInputVrf(slot, eta0)
		}
	})

	b.Run("RandomEta0", func(b *testing.B) {
		b.ReportAllocs()
		etas := make([][]byte, b.N)
		for i := 0; i < b.N; i++ {
			etas[i] = make([]byte, 32)
			if _, err := rand.Read(etas[i]); err != nil {
				b.Fatal(err)
			}
		}
		slot := int64(50_000_000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = MkInputVrf(slot, etas[i])
		}
	})
}

// BenchmarkFullWorkflow benchmarks the complete VRF workflow:
// KeyGen -> Prove -> Verify
func BenchmarkFullWorkflow(b *testing.B) {
	b.ReportAllocs()

	b.Run("Complete", func(b *testing.B) {
		b.ReportAllocs()
		alpha := []byte("full workflow test message")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Generate new keypair
			pk, sk, err := KeyGen(benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			// Generate proof
			proof, output, err := Prove(sk, alpha)
			if err != nil {
				b.Fatal(err)
			}

			// Verify
			valid, err := Verify(pk, proof, output, alpha)
			if err != nil {
				b.Fatal(err)
			}
			if !valid {
				b.Fatal("verification failed")
			}
		}
	})

	b.Run("WithRandomKeys", func(b *testing.B) {
		b.ReportAllocs()
		seeds := make([][]byte, b.N)
		for i := 0; i < b.N; i++ {
			seeds[i] = make([]byte, SeedSize)
			if _, err := rand.Read(seeds[i]); err != nil {
				b.Fatal(err)
			}
		}
		alpha := []byte("random key workflow test")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pk, sk, err := KeyGen(seeds[i])
			if err != nil {
				b.Fatal(err)
			}

			proof, output, err := Prove(sk, alpha)
			if err != nil {
				b.Fatal(err)
			}

			valid, err := Verify(pk, proof, output, alpha)
			if err != nil {
				b.Fatal(err)
			}
			if !valid {
				b.Fatal("verification failed")
			}
		}
	})
}

// BenchmarkLeaderElectionWorkflow benchmarks a realistic Cardano leader election flow
func BenchmarkLeaderElectionWorkflow(b *testing.B) {
	b.ReportAllocs()

	// Simulate leader election: MkInputVrf -> Prove -> VerifyAndHash
	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i * 7) // Some nonce value
	}

	b.Run("SingleSlot", func(b *testing.B) {
		b.ReportAllocs()
		slot := int64(50_000_000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create VRF input for this slot
			vrfInput := MkInputVrf(slot, eta0)

			// Generate proof (stake pool operator would do this)
			proof, _, err := Prove(benchSK, vrfInput)
			if err != nil {
				b.Fatal(err)
			}

			// Verify (other nodes would do this)
			_, err = VerifyAndHash(benchPK, proof, vrfInput)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MultipleSlots", func(b *testing.B) {
		b.ReportAllocs()
		baseSlot := int64(50_000_000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			slot := baseSlot + int64(i)
			vrfInput := MkInputVrf(slot, eta0)

			proof, _, err := Prove(benchSK, vrfInput)
			if err != nil {
				b.Fatal(err)
			}

			_, err = VerifyAndHash(benchPK, proof, vrfInput)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkParallelProve benchmarks concurrent proof generation
func BenchmarkParallelProve(b *testing.B) {
	b.ReportAllocs()
	alpha := []byte("parallel benchmark test")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := Prove(benchSK, alpha)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkParallelVerify benchmarks concurrent verification
func BenchmarkParallelVerify(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := VerifyAndHash(benchPK, benchProof, benchAlpha)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
