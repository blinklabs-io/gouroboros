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

package kes

import (
	"fmt"
	"testing"
)

// Test seed (exactly 32 bytes) - matches canonical test vectors from input-output-hk/kes
// Note: "lenght" typo is intentional
var benchSeed = []byte("test string of 32 byte of lenght")
var benchMessage = []byte("test message for benchmark")

// BenchmarkKeyGen benchmarks KES key generation at various depths
func BenchmarkKeyGen(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, err := KeyGen(depth, benchSeed)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSign benchmarks KES signing at various depths
func BenchmarkSign(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			sk, _, err := KeyGen(depth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Sign(sk, 0, benchMessage)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSignAtDifferentPeriods benchmarks signing at different periods for Cardano depth
func BenchmarkSignAtDifferentPeriods(b *testing.B) {
	// Test at periods 0, 31 (middle of left subtree), 32 (start of right subtree), 63 (last period)
	periods := []uint64{0, 31, 32, 63}

	for _, period := range periods {
		b.Run(fmt.Sprintf("period_%d", period), func(b *testing.B) {
			sk, _, err := KeyGen(CardanoKesDepth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			// Update key to desired period
			for p := uint64(0); p < period; p++ {
				sk, err = Update(sk)
				if err != nil {
					b.Fatal(err)
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Sign(sk, period, benchMessage)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkUpdate benchmarks single key update at various depths
func BenchmarkUpdate(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			// Pre-generate keys for update
			keys := make([]*SecretKey, b.N)
			for i := 0; i < b.N; i++ {
				sk, _, err := KeyGen(depth, benchSeed)
				if err != nil {
					b.Fatal(err)
				}
				keys[i] = sk
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Update(keys[i])
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkUpdateMultiplePeriods benchmarks updating through multiple periods
func BenchmarkUpdateMultiplePeriods(b *testing.B) {
	// Benchmark updating through 1, 8, 32, 63 periods
	periodCounts := []int{1, 8, 32, 63}

	for _, numUpdates := range periodCounts {
		b.Run(fmt.Sprintf("updates_%d", numUpdates), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sk, _, err := KeyGen(CardanoKesDepth, benchSeed)
				if err != nil {
					b.Fatal(err)
				}
				for j := 0; j < numUpdates; j++ {
					sk, err = Update(sk)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkUpdateAtBoundary benchmarks update at the subtree boundary (period 31->32)
func BenchmarkUpdateAtBoundary(b *testing.B) {
	// Pre-generate keys at period 31 (just before boundary)
	keys := make([]*SecretKey, b.N)
	for i := 0; i < b.N; i++ {
		sk, _, err := KeyGen(CardanoKesDepth, benchSeed)
		if err != nil {
			b.Fatal(err)
		}
		// Evolve to period 31
		for p := 0; p < 31; p++ {
			sk, err = Update(sk)
			if err != nil {
				b.Fatal(err)
			}
		}
		keys[i] = sk
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Update(keys[i])
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNewSumKesFromBytes benchmarks signature deserialization
func BenchmarkNewSumKesFromBytes(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			sk, _, err := KeyGen(depth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			sig, err := Sign(sk, 0, benchMessage)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := NewSumKesFromBytes(depth, sig)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerify benchmarks signature verification at various depths
func BenchmarkVerify(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			sk, pk, err := KeyGen(depth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			sig, err := Sign(sk, 0, benchMessage)
			if err != nil {
				b.Fatal(err)
			}

			kesSig, err := NewSumKesFromBytes(depth, sig)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !kesSig.Verify(0, pk, benchMessage) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// BenchmarkVerifyAtDifferentPeriods benchmarks verification at different periods
func BenchmarkVerifyAtDifferentPeriods(b *testing.B) {
	periods := []uint64{0, 31, 32, 63}

	for _, period := range periods {
		b.Run(fmt.Sprintf("period_%d", period), func(b *testing.B) {
			sk, pk, err := KeyGen(CardanoKesDepth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			// Update key to desired period
			for p := uint64(0); p < period; p++ {
				sk, err = Update(sk)
				if err != nil {
					b.Fatal(err)
				}
			}

			sig, err := Sign(sk, period, benchMessage)
			if err != nil {
				b.Fatal(err)
			}

			kesSig, err := NewSumKesFromBytes(CardanoKesDepth, sig)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !kesSig.Verify(period, pk, benchMessage) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// BenchmarkVerifySignedKES benchmarks the full verification function for Cardano depth
func BenchmarkVerifySignedKES(b *testing.B) {
	sk, pk, err := KeyGen(CardanoKesDepth, benchSeed)
	if err != nil {
		b.Fatal(err)
	}

	sig, err := Sign(sk, 0, benchMessage)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !VerifySignedKES(pk, 0, benchMessage, sig) {
			b.Fatal("verification failed")
		}
	}
}

// BenchmarkHashPair benchmarks the Blake2b hash of two public keys
func BenchmarkHashPair(b *testing.B) {
	left := make([]byte, PublicKeySize)
	right := make([]byte, PublicKeySize)
	for i := range left {
		left[i] = byte(i)
		right[i] = byte(i + 32)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashPair(left, right)
	}
}

// BenchmarkPublicKey benchmarks public key extraction from secret key
func BenchmarkPublicKey(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			sk, _, err := KeyGen(depth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = PublicKey(sk)
			}
		})
	}
}

// BenchmarkFullWorkflow benchmarks the complete workflow: KeyGen -> Sign -> Verify
func BenchmarkFullWorkflow(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Key generation
				sk, pk, err := KeyGen(depth, benchSeed)
				if err != nil {
					b.Fatal(err)
				}

				// Signing
				sig, err := Sign(sk, 0, benchMessage)
				if err != nil {
					b.Fatal(err)
				}

				// Deserialization
				kesSig, err := NewSumKesFromBytes(depth, sig)
				if err != nil {
					b.Fatal(err)
				}

				// Verification
				if !kesSig.Verify(0, pk, benchMessage) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// BenchmarkKeyGenSignUpdateCycle benchmarks KeyGen followed by multiple sign-update cycles
func BenchmarkKeyGenSignUpdateCycle(b *testing.B) {
	// Test with 8 periods (sign and update 8 times)
	numCycles := 8

	b.Run(fmt.Sprintf("cycles_%d", numCycles), func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sk, _, err := KeyGen(CardanoKesDepth, benchSeed)
			if err != nil {
				b.Fatal(err)
			}

			for period := uint64(0); period < uint64(numCycles); period++ {
				// Sign at current period
				_, err := Sign(sk, period, benchMessage)
				if err != nil {
					b.Fatal(err)
				}

				// Update to next period (except after last sign)
				if period < uint64(numCycles-1) {
					sk, err = Update(sk)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		}
	})
}

// BenchmarkSignatureSize benchmarks the SignatureSize helper function
func BenchmarkSignatureSize(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth, 10, 20}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = SignatureSize(depth)
			}
		})
	}
}

// BenchmarkMaxPeriod benchmarks the MaxPeriod helper function
func BenchmarkMaxPeriod(b *testing.B) {
	depths := []uint64{1, 2, 3, CardanoKesDepth, 10, 20}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = MaxPeriod(depth)
			}
		})
	}
}
