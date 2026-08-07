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
	"fmt"
	"math"
	"math/big"
	"sync"

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
var twoTo256 = new(
	big.Int,
).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsCPraos), nil)

// twoTo512 is 2^512, the upper bound for TPraos leader value comparison.
// WARNING: This package-level big.Int value must not be mutated. Always use
// it as a read-only constant. Create new big.Int instances for calculations.
var twoTo512 = new(
	big.Int,
).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsTPraos), nil)

// bigRatOne is the rational value 1, used to bound the valid domain of the
// active slot coefficient (a probability, so 0 <= f <= 1).
// WARNING: This package-level big.Rat value must not be mutated.
var bigRatOne = big.NewRat(1, 1)

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
	threshold, _ := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		activeSlotCoeff,
		ConsensusModeCPraos,
	)
	return threshold
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
//
// Returns an error if the consensus mode is unknown.
func CertifiedNatThresholdWithMode(
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) (*big.Int, error) {
	var upperBound *big.Int
	switch mode {
	case ConsensusModeCPraos:
		upperBound = twoTo256
	case ConsensusModeTPraos:
		upperBound = twoTo512
	default:
		return nil, fmt.Errorf("unknown consensus mode: %d", mode)
	}

	if activeSlotCoeff == nil {
		return big.NewInt(0), nil
	}
	// Domain guard: f must be a valid probability, i.e. 0 <= f <= 1.
	// f <= 0 is a degenerate "never lead" case: 1-(1-f)^sigma = 0
	// regardless of sigma (this holds for f == 0 exactly, and we treat
	// any non-positive f the same way rather than feeding it into the
	// ln/exp pipeline, which assumes 1-f > 0).
	if activeSlotCoeff.Sign() <= 0 {
		return big.NewInt(0), nil
	}
	fCmpOne := activeSlotCoeff.Cmp(bigRatOne)
	if fCmpOne > 0 {
		return nil, fmt.Errorf(
			"activeSlotCoeff must not exceed 1 (100%%), got %s",
			activeSlotCoeff.RatString(),
		)
	}
	if totalStake == 0 {
		return big.NewInt(0), nil
	}
	if poolStake == 0 {
		return big.NewInt(0), nil
	}
	if poolStake > totalStake {
		poolStake = totalStake
	}
	// f == 1 means certain leadership for any pool with positive stake
	// (sigma > 0 here, since poolStake/totalStake are both non-zero
	// above): (1-f)^sigma = 0^sigma = 0, so probability = 1 and the
	// threshold is exactly the upper bound. Handle this exactly rather
	// than falling into the ln/exp pipeline, which is only valid for
	// 1-f > 0.
	if fCmpOne == 0 {
		return new(big.Int).Set(upperBound), nil
	}
	// sigma == 1 (full stake) means (1-f)^sigma == 1-f exactly, so the
	// threshold can be computed exactly via rational arithmetic instead
	// of the ln/exp pipeline, avoiding any residual floating-point error
	// that could otherwise push an exact-rational cutoff across an
	// integer boundary (see the full-stake, f=1/2 differential test).
	if poolStake == totalStake {
		exact := new(big.Int).Mul(upperBound, activeSlotCoeff.Num())
		exact.Quo(exact, activeSlotCoeff.Denom())
		return exact, nil
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
	// cost of big.Rat arithmetic over the many Taylor series terms
	// involved (see lnOneMinusFloat/expFloat below).
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

	// threshold = floor(probability * upperBound)
	upperBoundFloat := new(big.Float).SetPrec(prec).SetInt(upperBound)
	thresholdFloat := new(big.Float).SetPrec(prec).Mul(
		probability,
		upperBoundFloat,
	)
	threshold, _ := thresholdFloat.Int(nil)

	return threshold, nil
}

// NOTE: lnOneMinusFloat and expFloat previously used a *fixed* 100-term
// Taylor series applied directly to the raw arguments. That is only
// accurate when the argument is small. ln(1-x) converges at rate O(x^n/n),
// so as the active slot coefficient f (i.e. x here) approaches 1, 100
// terms is nowhere near enough: differential testing against an
// independent continued-fraction implementation of ln (mirroring
// IntersectMBO/cardano-ledger's Cardano.Ledger.NonIntegral) showed the old
// code's threshold diverging from the true value by several *percent* of
// its magnitude for f around 0.9-0.99 (values used on some fast
// devnets/testnets), not merely a boundary rounding sliver. That is a real
// eligibility-decision bug, not just theoretical imprecision.
//
// The functions below fix this by range-reducing the argument first, so
// the number of Taylor terms required is bounded independently of the
// input's magnitude:
//   - lnOneMinusFloat/lnPositiveFloat normalize the input to a mantissa in
//     [0.5, 1) (via big.Float.MantExp) and use the fast-converging
//     artanh-based series ln(m) = 2*atanh((m-1)/(m+1)), whose ratio |z| is
//     always <= 1/3 regardless of the original input.
//   - expFloat halves the argument down (via MantExp) until it is tiny,
//     applies the Taylor series there (where it converges very fast), then
//     squares the result back up.
//
// numTermsForArtanhSeries/numTermsForExpSeries derive term counts that are
// provably sufficient (with a safety margin) for seriesTargetBits of
// accuracy, rather than relying on a hand-picked constant that happens to
// work for "typical" inputs.

// seriesTargetBits is the number of bits of accuracy the ln/exp series
// truncation error is sized against. This is deliberately *not* tied to
// thresholdPrecision (the big.Float mantissa width used for all
// arithmetic in this file): thresholdPrecision provides headroom against
// ordinary rounding in the surrounding multiply/floor pipeline, whereas
// the series only needs to be accurate enough that its own truncation
// error can never change which integer floor(probability*upperBound)
// rounds to. The largest upperBound in use is TPraos's 2^512, so an
// absolute error in probability below 2^-512 suffices; the extra
// guardBits below keeps a comfortable margin. Sizing terms off
// thresholdPrecision (1024) instead of this would roughly double the
// term count for no correctness benefit -- pure allocation/CPU cost.
const seriesTargetBits = vrfOutputBitsTPraos + guardBits

// guardBits is the safety margin (in bits) added on top of the minimum
// required accuracy when deriving term counts.
const guardBits = 64

// log2Of3 = log2(3), used to size the artanh series: since the reduced
// argument z satisfies |z| <= 1/3, the term magnitude shrinks by a factor
// of at least 3^2=9 every two terms.
const log2Of3 = 1.5849625007211562

// numTermsForArtanhSeries returns a term count sufficient to compute
// ln(m) for m in [0.5, 1) to seriesTargetBits bits of precision via
// ln(m) = 2*atanh((m-1)/(m+1)). Because m is always normalized to
// [0.5, 1), |z| <= 1/3 always -- the term count needed is fixed,
// independent of the original input's magnitude.
func numTermsForArtanhSeries() int {
	n := math.Ceil(float64(seriesTargetBits) / (2 * log2Of3))
	return int(n) + 16
}

// expReductionBits controls how small |x/2^k| must be before applying the
// Taylor series in expFloat. Reducing to a fixed small magnitude bounds the
// number of Taylor terms needed to a value independent of the original
// |x|, unlike applying the series directly to x (whose required term count
// grows with |x|, and would silently lose precision for the large negative
// exponents ln(1-f) produces as f approaches 1).
const expReductionBits = 16

// numTermsForExpSeries returns a term count sufficient to compute exp(y)
// for |y| <= 2^-reductionBits to seriesTargetBits bits of precision. This
// bound conservatively ignores the n! denominator (i.e. treats the series
// as if it only decayed geometrically by 2^-reductionBits per term), so it
// is an over-estimate, not a tight one.
func numTermsForExpSeries(reductionBits int) int {
	n := math.Ceil(float64(seriesTargetBits) / float64(reductionBits))
	return int(n) + 16
}

// atanhSeries computes atanh(z) = z + z^3/3 + z^5/5 + ... for the given
// number of terms.
func atanhSeries(z *big.Float, terms int) *big.Float {
	prec := z.Prec()
	z2 := new(big.Float).SetPrec(prec).Mul(z, z)
	term := new(big.Float).SetPrec(prec).Set(z)
	sum := new(big.Float).SetPrec(prec).Set(z)
	denom := new(big.Float).SetPrec(prec)
	scratch := new(big.Float).SetPrec(prec)

	for n := 1; n < terms; n++ {
		term.Mul(term, z2)
		denom.SetInt64(int64(2*n + 1))
		scratch.Quo(term, denom)
		sum.Add(sum, scratch)
	}

	return sum
}

// lnNormalized computes ln(m) for m in [0.5, 1) using the fast-converging
// series ln(m) = 2*atanh((m-1)/(m+1)). Since m is restricted to [0.5, 1),
// |z| = |(m-1)/(m+1)| <= 1/3 always, giving a fixed, input-independent
// convergence rate.
func lnNormalized(m *big.Float, terms int) *big.Float {
	prec := m.Prec()
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	z := new(big.Float).SetPrec(prec).Quo(
		new(big.Float).SetPrec(prec).Sub(m, one),
		new(big.Float).SetPrec(prec).Add(m, one),
	)
	two := new(big.Float).SetPrec(prec).SetInt64(2)
	return new(big.Float).SetPrec(prec).Mul(two, atanhSeries(z, terms))
}

var (
	ln2Once   sync.Once
	ln2Cached *big.Float
)

// ln2Float returns ln(2) at the requested precision, computed once (at
// thresholdPrecision) and cached; if a higher precision is requested it is
// recomputed fresh rather than serving a truncated cached value.
func ln2Float(prec uint) *big.Float {
	if prec <= thresholdPrecision {
		ln2Once.Do(func() {
			ln2Cached = computeLn2(thresholdPrecision)
		})
		return new(big.Float).SetPrec(prec).Set(ln2Cached)
	}
	return computeLn2(prec)
}

// computeLn2 computes ln(2) = -ln(0.5) using lnNormalized, since 0.5 is
// already in the required [0.5, 1) range.
func computeLn2(prec uint) *big.Float {
	half := new(big.Float).SetPrec(prec).SetFloat64(0.5)
	terms := numTermsForArtanhSeries()
	lnHalf := lnNormalized(half, terms)
	return new(big.Float).SetPrec(prec).Neg(lnHalf)
}

// lnPositiveFloat computes ln(y) for y > 0, by normalizing y to a mantissa
// m in [0.5, 1) with y = m * 2^exp (via big.Float.MantExp) and computing
// ln(y) = ln(m) + exp*ln(2). This bounds the series' required term count
// independently of how large or small y is.
func lnPositiveFloat(y *big.Float) *big.Float {
	prec := y.Prec()

	mant := new(big.Float).SetPrec(prec)
	exp := y.MantExp(mant) // y = mant * 2^exp, 0.5 <= mant < 1

	terms := numTermsForArtanhSeries()
	lnMant := lnNormalized(mant, terms)

	if exp == 0 {
		return lnMant
	}

	ln2 := ln2Float(prec)
	expTerm := new(big.Float).SetPrec(prec).Mul(
		new(big.Float).SetPrec(prec).SetInt64(int64(exp)),
		ln2,
	)
	return new(big.Float).SetPrec(prec).Add(lnMant, expTerm)
}

// lnOneMinusFloat computes ln(1-x) for 0 < x < 1 by delegating to
// lnPositiveFloat on y = 1-x. See the NOTE above this section for why a
// direct fixed-term power series in x is unsafe as x -> 1.
func lnOneMinusFloat(x *big.Float) *big.Float {
	prec := x.Prec()
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	y := new(big.Float).SetPrec(prec).Sub(one, x)
	return lnPositiveFloat(y)
}

// taylorExpSeries computes exp(y) = 1 + y + y^2/2! + y^3/3! + ... for |y|
// small (see expReductionBits) using the given term count.
func taylorExpSeries(y *big.Float, terms int) *big.Float {
	prec := y.Prec()
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	result := new(big.Float).SetPrec(prec).Set(one)
	term := new(big.Float).SetPrec(prec).Set(one)
	nFloat := new(big.Float).SetPrec(prec)

	for n := 1; n <= terms; n++ {
		term.Mul(term, y)
		nFloat.SetInt64(int64(n))
		term.Quo(term, nFloat)
		result.Add(result, term)
	}

	return result
}

// expFloat computes exp(x) for a big.Float x by halving x down (via
// big.Float.MantExp) until it is small enough for the Taylor series to
// converge within a bounded number of terms, then squaring the result
// back up. See the NOTE above this section for why applying the series
// directly to x is unsafe for the large-magnitude arguments that arise
// from ln(1-f) as f -> 1.
func expFloat(x *big.Float) *big.Float {
	prec := x.Prec()

	if x.Sign() == 0 {
		return new(big.Float).SetPrec(prec).SetInt64(1)
	}

	mant := new(big.Float).SetPrec(prec)
	e := x.MantExp(mant) // x = mant * 2^e, 0.5 <= |mant| < 1

	k := e + expReductionBits
	if k < 0 {
		k = 0
	}

	// y = x / 2^k, so |y| <= 2^-expReductionBits (or |x| itself, if that
	// was already smaller).
	y := new(big.Float).SetPrec(prec).SetMantExp(x, -k)

	terms := numTermsForExpSeries(expReductionBits)
	result := taylorExpSeries(y, terms)

	for i := 0; i < k; i++ {
		result.Mul(result, result)
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
	below, _ := IsVRFOutputBelowThresholdWithMode(
		vrfOutput,
		threshold,
		ConsensusModeCPraos,
	)
	return below
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
func IsVRFOutputBelowThresholdWithMode(
	vrfOutput []byte,
	threshold *big.Int,
	mode ConsensusMode,
) (bool, error) {
	var useRawOutput bool
	switch mode {
	case ConsensusModeCPraos:
	case ConsensusModeTPraos:
		useRawOutput = true
	default:
		return false, fmt.Errorf("unknown consensus mode: %d", mode)
	}

	if threshold == nil {
		return false, nil
	}
	if len(vrfOutput) == 0 {
		return false, nil
	}

	var leaderValue []byte
	if useRawOutput {
		leaderValue = vrfOutput
	} else {
		leaderValue = VrfLeaderValue(vrfOutput)
	}
	vrfInt := VRFOutputToInt(leaderValue)
	return vrfInt.Cmp(threshold) < 0, nil
}
