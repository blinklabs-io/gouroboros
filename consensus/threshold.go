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
	"slices"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ConsensusMode represents the consensus protocol variant.
type ConsensusMode int

const (
	// ConsensusModeCPraos is the current Praos consensus (Babbage+).
	// Uses BLAKE2b-256("L" || vrfOutput) with 2^256 threshold.
	ConsensusModeCPraos ConsensusMode = iota

	// ConsensusModeTPraos is the transitional Praos consensus (Shelley-Alonzo).
	// Uses raw 64-byte VRF output with 2^512 threshold.
	ConsensusModeTPraos
)

// Precision constants for VRF output comparison
const (
	// CPRAOS uses BLAKE2b-256 hash of VRF output, so we compare against 2^256
	vrfOutputBitsCPraos = 256
	// TPraos uses raw 64-byte VRF output, so we compare against 2^512
	vrfOutputBitsTPraos = 512
)

// twoTo256 is 2^256, the upper bound for CPRAOS leader value comparison.
// WARNING: These package-level big.Int values must not be mutated. Always use
// them as read-only constants. Create new big.Int instances for calculations.
var twoTo256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsCPraos), nil)

// twoTo512 is 2^512, the upper bound for TPraos leader value comparison.
// WARNING: This package-level big.Int value must not be mutated. Always use
// it as a read-only constant. Create new big.Int instances for calculations.
var twoTo512 = new(big.Int).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsTPraos), nil)

// oneRat is the constant 1/1 for reuse in calculations.
// WARNING: This package-level big.Rat value must not be mutated. Always use
// it as a read-only constant. Create new big.Rat instances for calculations.
var oneRat = big.NewRat(1, 1)

// CertifiedNatThreshold computes the leadership threshold for a pool using CPRAOS.
// For TPraos compatibility, use CertifiedNatThresholdWithMode.
//
// The threshold is computed as:
//
//	T = 2^256 * (1 - (1-f)^σ)
//
// Where:
//   - f is the active slot coefficient (e.g., 0.05 on mainnet)
//   - σ = poolStake / totalStake (the pool's relative stake)
//
// If the VRF leader value (BLAKE2b-256 hash of VRF output with "L" prefix,
// interpreted as an unsigned integer) is less than T, the pool is eligible
// to be a slot leader.
//
// This implementation uses arbitrary precision arithmetic to match
// Cardano's ledger specification.
func CertifiedNatThreshold(
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
) *big.Int {
	return CertifiedNatThresholdWithMode(poolStake, totalStake, activeSlotCoeff, ConsensusModeCPraos)
}

// CertifiedNatThresholdWithMode computes the leadership threshold for a pool
// using the specified consensus mode.
//
// For CPRAOS (Babbage+):
//
//	T = 2^256 * (1 - (1-f)^σ)
//
// For TPraos (Shelley-Alonzo):
//
//	T = 2^512 * (1 - (1-f)^σ)
func CertifiedNatThresholdWithMode(
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
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
	probability := new(big.Rat).Sub(oneRat, oneMinusFPowerSigma)

	// Select the appropriate upper bound based on consensus mode
	var upperBound *big.Int
	if mode == ConsensusModeTPraos {
		upperBound = twoTo512
	} else {
		upperBound = twoTo256
	}

	// threshold = floor(probability * upperBound)
	threshold := new(big.Int).Mul(probability.Num(), upperBound)
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
	term := new(big.Rat)
	denom := new(big.Rat)

	for n := 1; n <= terms; n++ {
		// Add -x^n / n to result
		denom.SetFrac64(int64(n), 1)
		term.Quo(xPower, denom)
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

	result := new(big.Rat).Set(oneRat) // Start with 1
	term := new(big.Rat).Set(oneRat)   // Current term (x^n / n!)
	denom := new(big.Rat)

	for n := 1; n <= terms; n++ {
		// term = term * x / n
		term.Mul(term, x)
		denom.SetFrac64(int64(n), 1)
		term.Quo(term, denom)

		// Add term to result
		result.Add(result, term)
	}

	return result
}

// VrfLeaderValue computes the CPRAOS leader value from a VRF output.
// This applies domain separation by hashing with "L" prefix:
//
//	leaderValue = BLAKE2b-256("L" || vrfOutput)
//
// The result is 32 bytes (256 bits) for comparison against the threshold.
func VrfLeaderValue(vrfOutput []byte) []byte {
	// Concatenate "L" prefix (0x4C) with VRF output for domain separation
	data := slices.Concat([]byte{0x4C}, vrfOutput)
	hash := common.Blake2b256Hash(data)
	return hash.Bytes()
}

// VRFOutputToInt converts a VRF leader value (32 bytes) to a big.Int
// for comparison against the leadership threshold.
// The value is interpreted as an unsigned big-endian integer.
func VRFOutputToInt(output []byte) *big.Int {
	return new(big.Int).SetBytes(output)
}

// IsVRFOutputBelowThreshold checks if a VRF output is below the leadership threshold
// using CPRAOS mode. For TPraos compatibility, use IsVRFOutputBelowThresholdWithMode.
//
// This is the core eligibility check for slot leadership.
// It first computes the CPRAOS leader value (BLAKE2b-256 hash with "L" prefix)
// then compares against the threshold.
func IsVRFOutputBelowThreshold(vrfOutput []byte, threshold *big.Int) bool {
	return IsVRFOutputBelowThresholdWithMode(vrfOutput, threshold, ConsensusModeCPraos)
}

// IsVRFOutputBelowThresholdWithMode checks if a VRF output is below the leadership
// threshold using the specified consensus mode.
//
// For CPRAOS (Babbage+):
//   - Computes BLAKE2b-256("L" || vrfOutput) to get 32-byte leader value
//   - Compares against threshold (based on 2^256)
//
// For TPraos (Shelley-Alonzo):
//   - Uses raw 64-byte VRF output directly
//   - Compares against threshold (based on 2^512)
//
// Unknown consensus modes are treated as CPRAOS (the current default).
func IsVRFOutputBelowThresholdWithMode(vrfOutput []byte, threshold *big.Int, mode ConsensusMode) bool {
	if threshold == nil {
		return false
	}
	if len(vrfOutput) == 0 {
		return false
	}

	var leaderValue []byte
	if mode == ConsensusModeTPraos {
		// TPraos: use raw VRF output directly
		leaderValue = vrfOutput
	} else {
		// CPRAOS (default): hash with "L" prefix
		leaderValue = VrfLeaderValue(vrfOutput)
	}

	vrfInt := VRFOutputToInt(leaderValue)
	return vrfInt.Cmp(threshold) < 0
}
