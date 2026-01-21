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

package consensus

import (
	"math/big"
)

// Precision constant for VRF output comparison
const (
	// VRF output is 64 bytes (512 bits), so we compare against 2^512
	vrfOutputBits = 512
)

// twoTo512 is 2^512, the upper bound for VRF output comparison
var twoTo512 = new(big.Int).Exp(big.NewInt(2), big.NewInt(vrfOutputBits), nil)

// CertifiedNatThreshold computes the leadership threshold for a pool.
//
// The threshold is computed as:
//
//	T = 2^512 * (1 - (1-f)^σ)
//
// Where:
//   - f is the active slot coefficient (e.g., 0.05 on mainnet)
//   - σ = poolStake / totalStake (the pool's relative stake)
//
// If the VRF output (interpreted as an unsigned integer) is less than T,
// the pool is eligible to be a slot leader.
//
// This implementation uses arbitrary precision arithmetic to match
// Cardano's ledger specification.
func CertifiedNatThreshold(
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
) *big.Int {
	if activeSlotCoeff == nil {
		return big.NewInt(0)
	}
	if totalStake == 0 {
		return big.NewInt(0)
	}
	if poolStake == 0 {
		return big.NewInt(0)
	}
	if poolStake > totalStake {
		poolStake = totalStake
	}

	// Calculate σ = poolStake / totalStake as a rational
	sigma := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(poolStake),
		new(big.Int).SetUint64(totalStake),
	)

	// Calculate (1-f)^σ using the approximation:
	// (1-f)^σ ≈ exp(σ * ln(1-f))
	//
	// For the Cardano implementation, we use a Taylor series expansion
	// of ln(1-f) and exp to achieve sufficient precision.
	oneMinusFPowerSigma := expRational(
		new(big.Rat).Mul(sigma, lnOneMinus(activeSlotCoeff)),
	)

	// Calculate 1 - (1-f)^σ
	probability := new(big.Rat).Sub(big.NewRat(1, 1), oneMinusFPowerSigma)

	// Multiply by 2^512 and convert to integer
	// threshold = floor(probability * 2^512)
	threshold := new(big.Int).Mul(probability.Num(), twoTo512)
	threshold.Div(threshold, probability.Denom())

	return threshold
}

// lnOneMinus computes ln(1-x) for 0 < x < 1 using Taylor series:
// ln(1-x) = -x - x²/2 - x³/3 - x⁴/4 - ...
//
// We use enough terms to achieve the required precision for Cardano's
// active slot coefficient range (typically 0.05).
func lnOneMinus(x *big.Rat) *big.Rat {
	// Number of terms to compute (more terms = more precision)
	// 20 terms provides sufficient precision for Cardano's f=0.05
	const terms = 20

	result := new(big.Rat)
	xPower := new(big.Rat).Set(x) // x^n starting with x^1

	for n := 1; n <= terms; n++ {
		// Add -x^n / n to result
		term := new(big.Rat).Quo(xPower, big.NewRat(int64(n), 1))
		result.Sub(result, term)

		// xPower = xPower * x for next iteration
		xPower.Mul(xPower, x)
	}

	return result
}

// expRational computes exp(x) for a rational x using Taylor series:
// exp(x) = 1 + x + x²/2! + x³/3! + x⁴/4! + ...
func expRational(x *big.Rat) *big.Rat {
	// Number of terms to compute
	// 20 terms provides sufficient precision for Cardano's typical values
	const terms = 20

	result := big.NewRat(1, 1) // Start with 1
	term := big.NewRat(1, 1)   // Current term (x^n / n!)

	for n := 1; n <= terms; n++ {
		// term = term * x / n
		term.Mul(term, x)
		term.Quo(term, big.NewRat(int64(n), 1))

		// Add term to result
		result.Add(result, term)
	}

	return result
}

// VRFOutputToInt converts a VRF output (64 bytes) to a big.Int
// for comparison against the leadership threshold.
// The VRF output is interpreted as an unsigned big-endian integer.
func VRFOutputToInt(output []byte) *big.Int {
	return new(big.Int).SetBytes(output)
}

// IsVRFOutputBelowThreshold checks if a VRF output is below the leadership threshold.
// This is the core eligibility check for slot leadership.
func IsVRFOutputBelowThreshold(output []byte, threshold *big.Int) bool {
	if threshold == nil {
		return false
	}
	if len(output) == 0 {
		return false
	}
	vrfInt := VRFOutputToInt(output)
	return vrfInt.Cmp(threshold) < 0
}
