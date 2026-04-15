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

// thresholdPrecision is the number of mantissa bits used for big.Float
// arithmetic in the Taylor series computation. 1024 bits provides ~308
// decimal digits of precision, far exceeding the ~77 digits needed for
// a 256-bit threshold result.
const thresholdPrecision = 1024

// twoTo256 is 2^256, the upper bound for CPRAOS leader value comparison.
// WARNING: These package-level big.Int values must not be mutated. Always use
// them as read-only constants. Create new big.Int instances for calculations.
var twoTo256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsCPraos), nil)

// twoTo512 is 2^512, the upper bound for TPraos leader value comparison.
// WARNING: This package-level big.Int value must not be mutated. Always use
// it as a read-only constant. Create new big.Int instances for calculations.
var twoTo512 = new(big.Int).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsTPraos), nil)

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

	const prec = thresholdPrecision

	// Calculate σ = poolStake / totalStake as a big.Float
	sigma := new(big.Float).SetPrec(prec).Quo(
		new(big.Float).SetPrec(prec).SetUint64(poolStake),
		new(big.Float).SetPrec(prec).SetUint64(totalStake),
	)

	// Calculate (1-f)^σ using the approximation:
	// (1-f)^σ ≈ exp(σ * ln(1-f))
	//
	// We use big.Float internally to avoid the O(n²) GCD normalization
	// cost of big.Rat arithmetic over 100 Taylor series terms.
	f := new(big.Float).SetPrec(prec).SetRat(activeSlotCoeff)
	lnVal := lnOneMinusFloat(f)
	product := new(big.Float).SetPrec(prec).Mul(sigma, lnVal)
	oneMinusFPowerSigma := expFloat(product)

	// Calculate 1 - (1-f)^σ
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	probability := new(big.Float).SetPrec(prec).Sub(
		one,
		oneMinusFPowerSigma,
	)

	// Select the appropriate upper bound based on consensus mode
	var upperBound *big.Int
	if mode == ConsensusModeTPraos {
		upperBound = twoTo512
	} else {
		upperBound = twoTo256
	}

	// threshold = floor(probability * upperBound)
	upperBoundFloat := new(big.Float).SetPrec(prec).SetInt(upperBound)
	thresholdFloat := new(big.Float).SetPrec(prec).Mul(
		probability,
		upperBoundFloat,
	)
	threshold, _ := thresholdFloat.Int(nil)

	return threshold
}

// lnOneMinusFloat computes ln(1-x) for 0 < x < 1 using Taylor series:
// ln(1-x) = -x - x²/2 - x³/3 - x⁴/4 - ...
//
// Uses big.Float arithmetic with fixed precision to avoid the expensive
// GCD normalization that big.Rat incurs on every operation.
// 100 terms provides sufficient precision for active slot coefficients
// up to f=0.5 and beyond.
func lnOneMinusFloat(x *big.Float) *big.Float {
	const terms = 100

	prec := x.Prec()
	result := new(big.Float).SetPrec(prec)
	xPower := new(big.Float).SetPrec(prec).Set(x) // x^n, starts at x^1
	term := new(big.Float).SetPrec(prec)
	nFloat := new(big.Float).SetPrec(prec)

	for n := 1; n <= terms; n++ {
		// term = xPower / n
		nFloat.SetInt64(int64(n))
		term.Quo(xPower, nFloat)
		// result -= term
		result.Sub(result, term)
		// xPower *= x for next iteration
		xPower.Mul(xPower, x)
	}

	return result
}

// expFloat computes exp(x) for a big.Float x using Taylor series:
// exp(x) = 1 + x + x²/2! + x³/3! + x⁴/4! + ...
//
// Uses big.Float arithmetic with fixed precision for efficiency.
// 100 terms provides sufficient precision for active slot coefficients
// up to f=0.5 and beyond.
func expFloat(x *big.Float) *big.Float {
	const terms = 100

	prec := x.Prec()
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	result := new(big.Float).SetPrec(prec).Set(one) // Start with 1
	term := new(big.Float).SetPrec(prec).Set(one)   // x^n / n!
	nFloat := new(big.Float).SetPrec(prec)

	for n := 1; n <= terms; n++ {
		// term = term * x / n
		term.Mul(term, x)
		nFloat.SetInt64(int64(n))
		term.Quo(term, nFloat)
		// result += term
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
	// Use fixed-size buffer to avoid allocation (VRF output is 64 bytes)
	var buf [65]byte // 1 + 64 for VRF output
	buf[0] = 0x4C    // "L" prefix for domain separation
	if len(vrfOutput) <= 64 {
		copy(buf[1:], vrfOutput)
		hash := common.Blake2b256Hash(buf[:1+len(vrfOutput)])
		return hash.Bytes()
	}
	// Fallback for larger inputs (not expected in normal operation)
	data := make([]byte, 1+len(vrfOutput))
	data[0] = 0x4C
	copy(data[1:], vrfOutput)
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
