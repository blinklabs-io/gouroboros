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
//
// This is a legacy no-error API kept for backward compatibility. The only
// error CertifiedNatThresholdWithMode can currently return for the CPRAOS
// mode used here is an out-of-domain activeSlotCoeff (f > 1, an invalid
// probability). Rather than propagating that error or returning nil (which
// would make the result unsafe to use with .Cmp/.Sign/.Bytes/etc. without a
// nil check), this function conservatively treats invalid/out-of-domain
// input the same way it already treats other degenerate input: as "never
// lead", returning big.NewInt(0). Callers that need to detect invalid input
// explicitly should use CertifiedNatThresholdWithMode instead.
func CertifiedNatThreshold(
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
) *big.Int {
	threshold, err := CertifiedNatThresholdWithMode(
		poolStake,
		totalStake,
		activeSlotCoeff,
		ConsensusModeCPraos,
	)
	if err != nil {
		return big.NewInt(0)
	}
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

	// Compute 1-f EXACTLY as a rational *before* any conversion to
	// big.Float. big.Rat arithmetic is arbitrary precision, so this
	// subtraction never loses information, unlike converting f itself to
	// a fixed-precision big.Float first (see the NOTE above
	// oneMinusFPowerSigmaBounds for why that ordering is unsafe: a
	// rational f that is extremely close to 1, such as
	// (2^2000-1)/2^2000, rounds to *exactly* 1.0 at any working mantissa
	// precision on the order of a few hundred to a couple thousand bits,
	// making a subsequent 1-f computation silently produce 0 instead of
	// the true tiny positive value).
	oneMinusF := new(big.Rat).Sub(bigRatOne, activeSlotCoeff)

	// Exact-rational fast path: generalizes the sigma=1 case above. If
	// sigma's reduced denominator m is small enough to root-check
	// economically, and (1-f)'s numerator and denominator are each an
	// exact m-th power, then (1-f)^sigma is itself an exact rational,
	// computed here with no ln/exp approximation (and hence no
	// floating-point error) at all. This is what resolves both of the
	// reported near-integer cutoffs: f=3/4,sigma=1/2 (1-f=1/4, a perfect
	// square) and f=(2^2000-1)/2^2000,sigma=1/2 (1-f=2^-2000, also a
	// perfect square).
	if exact, ok := exactOneMinusFPowerSigmaThreshold(
		oneMinusF,
		poolStake,
		totalStake,
		upperBound,
	); ok {
		return exact, nil
	}

	// General case: (1-f)^sigma is generically irrational, so compute it
	// via the range-reduced ln/exp pipeline with a rigorously bounded
	// error, escalating precision if the resulting interval straddles an
	// integer boundary closely enough that floor(probability*upperBound)
	// cannot yet be determined unambiguously. This never runs the
	// pipeline more than once per precision level (see
	// oneMinusFPowerSigmaBounds), so the common, well-separated-from-any-
	// integer-boundary case costs the same as a single ln+exp evaluation,
	// same as before this fix.
	targetBits := uint(seriesTargetBits)
	for {
		threshold, resolved := thresholdFromBoundedProbability(
			oneMinusF,
			poolStake,
			totalStake,
			upperBound,
			targetBits,
		)
		if resolved || targetBits >= maxThresholdEscalationBits {
			// If the escalation cap is reached without resolving,
			// the true value is -- to within astronomically
			// unlikely odds -- exactly on an integer boundary that
			// the exact-rational fast path above could not detect
			// (e.g. sigma's reduced denominator exceeded
			// maxExactRootDegree). threshold is the tight lower
			// bound at the highest precision tried, the same
			// conservative "when genuinely unsure, don't over-
			// count eligibility" direction used elsewhere in this
			// function for degenerate input.
			return threshold, nil
		}
		targetBits *= 2
	}
}

// maxExactRootDegree bounds how large sigma's reduced denominator may be
// before exactOneMinusFPowerSigma gives up and defers to the escalating-
// precision interval computation instead. For a genuine stake ratio (whose
// reduced denominator is typically on the order of totalStake, often up to
// ~2^64), an exact m-th root essentially never exists, and root-checking it
// anyway would be needlessly expensive (an Exp of a value with on the order
// of totalStake's own bit length). This cap keeps the fast-path attempt
// itself cheap to try-and-fail for realistic inputs, while remaining large
// enough to cover any plausible small-denominator boundary case (such as
// the sigma=1/2 cases this generalizes from the sigma=1 fast path).
const maxExactRootDegree = 4096

// maxThresholdEscalationBits caps how far thresholdFromBoundedProbability's
// caller grows the working precision (targetBits) before giving up and
// returning a best-effort result. This is far beyond what any plausible
// protocol parameter (activeSlotCoeff, pool/total stake) would require to
// resolve -- reaching it means the true mathematical value is, to within
// astronomically unlikely odds, exactly on an integer boundary in a way the
// exact-rational fast path could not detect.
const maxThresholdEscalationBits = 1 << 20 // ~1,048,576 bits

// intervalGuardBits is extra working precision reserved, on top of the
// current targetBits accuracy level, to keep ordinary floating-point
// rounding error (and the bounded amplification that expFloatAtTarget's
// repeated squaring introduces for large-magnitude arguments) from eating
// into the declared error bound in oneMinusFPowerSigmaBounds. Half of this
// margin is spent on the series truncation target (see
// oneMinusFPowerSigmaBounds); the other half absorbs everything else.
const intervalGuardBits = 128

// exactIntegerNthRoot returns (r, true) if n == r^k exactly for some
// non-negative integer r, and (nil, false) otherwise. n must be
// non-negative; k must be >= 1. Uses Newton's method to find
// floor(n^(1/k)), then verifies the result exactly via integer
// exponentiation -- this is exact-or-nothing, never an approximation.
func exactIntegerNthRoot(n *big.Int, k int64) (*big.Int, bool) {
	if n.Sign() < 0 {
		return nil, false
	}
	if n.Sign() == 0 {
		return big.NewInt(0), true
	}
	if k == 1 {
		return new(big.Int).Set(n), true
	}

	kBig := big.NewInt(k)
	kMinus1 := big.NewInt(k - 1)

	// Initial over-estimate: x0 ~= 2^(ceil(bitlen(n)/k)+1).
	guessBits := n.BitLen()/int(k) + 1
	x := new(big.Int).Lsh(big.NewInt(1), uint(guessBits))

	// Newton's method for the integer k-th root converges monotonically
	// downward from an over-estimate to floor(n^(1/k)).
	for {
		xPow := new(big.Int).Exp(x, kMinus1, nil)
		if xPow.Sign() == 0 {
			x.Lsh(x, 1)
			continue
		}
		t := new(big.Int).Quo(n, xPow)
		next := new(big.Int).Add(new(big.Int).Mul(kMinus1, x), t)
		next.Quo(next, kBig)
		if next.Cmp(x) >= 0 {
			break
		}
		x = next
	}
	// Correct any remaining +/-1 slack to land exactly on
	// floor(n^(1/k)).
	for new(big.Int).Exp(x, kBig, nil).Cmp(n) > 0 {
		x.Sub(x, big.NewInt(1))
	}
	for {
		next := new(big.Int).Add(x, big.NewInt(1))
		if new(big.Int).Exp(next, kBig, nil).Cmp(n) > 0 {
			break
		}
		x = next
	}

	if new(big.Int).Exp(x, kBig, nil).Cmp(n) == 0 {
		return x, true
	}
	return nil, false
}

// exactOneMinusFPowerSigma attempts to compute (1-f)^sigma exactly as a
// rational. sigma = poolStake/totalStake is reduced to lowest terms n/m
// (m >= 2 is guaranteed by the caller, which handles sigma=1 -- i.e. m=1 --
// separately). If oneMinusF's numerator and denominator (already coprime,
// per big.Rat's invariant) are each an exact m-th power, then
// (1-f)^sigma = (numRoot/denRoot)^n exactly, computed via integer
// exponentiation with no floating-point approximation at all.
func exactOneMinusFPowerSigma(
	oneMinusF *big.Rat,
	poolStake, totalStake uint64,
) (*big.Rat, bool) {
	g := new(big.Int).GCD(
		nil,
		nil,
		new(big.Int).SetUint64(poolStake),
		new(big.Int).SetUint64(totalStake),
	)
	n := new(big.Int).Quo(new(big.Int).SetUint64(poolStake), g)
	m := new(big.Int).Quo(new(big.Int).SetUint64(totalStake), g)
	if !m.IsUint64() || m.Uint64() > maxExactRootDegree {
		return nil, false
	}
	mDeg := m.Int64()

	numRoot, ok := exactIntegerNthRoot(oneMinusF.Num(), mDeg)
	if !ok {
		return nil, false
	}
	denRoot, ok := exactIntegerNthRoot(oneMinusF.Denom(), mDeg)
	if !ok {
		return nil, false
	}

	return new(big.Rat).SetFrac(
		new(big.Int).Exp(numRoot, n, nil),
		new(big.Int).Exp(denRoot, n, nil),
	), true
}

// exactOneMinusFPowerSigmaThreshold attempts the exact-rational fast path
// (see exactOneMinusFPowerSigma) and, if successful, returns the exact
// integer threshold floor(upperBound * (1-(1-f)^sigma)) computed via
// integer arithmetic alone.
func exactOneMinusFPowerSigmaThreshold(
	oneMinusF *big.Rat,
	poolStake, totalStake uint64,
	upperBound *big.Int,
) (*big.Int, bool) {
	powerExact, ok := exactOneMinusFPowerSigma(oneMinusF, poolStake, totalStake)
	if !ok {
		return nil, false
	}
	probabilityExact := new(big.Rat).Sub(bigRatOne, powerExact)
	threshold := new(big.Int).Mul(upperBound, probabilityExact.Num())
	threshold.Quo(threshold, probabilityExact.Denom())
	return threshold, true
}

// thresholdFromBoundedProbability computes rigorous lower/upper bounds on
// the true integer threshold floor(upperBound*(1-(1-f)^sigma)) at the given
// targetBits of accuracy (see oneMinusFPowerSigmaBounds), and reports
// whether they agree (in which case the shared value is provably the
// correct threshold). If they disagree, the caller should retry at a
// higher targetBits; the returned threshold in that case is the (not yet
// proven correct) lower bound, useful only as a last-resort fallback if an
// escalation cap is reached.
func thresholdFromBoundedProbability(
	oneMinusF *big.Rat,
	poolStake, totalStake uint64,
	upperBound *big.Int,
	targetBits uint,
) (threshold *big.Int, resolved bool) {
	lo, hi := oneMinusFPowerSigmaBounds(
		oneMinusF,
		poolStake,
		totalStake,
		targetBits,
	)
	workPrec := targetBits + intervalGuardBits

	one := new(big.Float).SetPrec(workPrec).SetInt64(1)
	upperBoundFloat := new(big.Float).SetPrec(workPrec).SetInt(upperBound)

	// probability = 1 - (1-f)^sigma: since (1-f)^sigma's upper bound
	// (hi) corresponds to probability's *lower* bound and vice versa.
	probLo := new(big.Float).SetPrec(workPrec).Sub(one, hi)
	probHi := new(big.Float).SetPrec(workPrec).Sub(one, lo)

	thresholdLoFloat := new(big.Float).SetPrec(workPrec).Mul(
		probLo,
		upperBoundFloat,
	)
	thresholdHiFloat := new(big.Float).SetPrec(workPrec).Mul(
		probHi,
		upperBoundFloat,
	)

	thresholdLo, _ := thresholdLoFloat.Int(nil)
	thresholdHi, _ := thresholdHiFloat.Int(nil)

	return thresholdLo, thresholdLo.Cmp(thresholdHi) == 0
}

// oneMinusFPowerSigmaBounds computes conservative lower/upper bounds
// [lo, hi] on the true mathematical value of (1-f)^sigma = oneMinusF^sigma,
// such that lo <= true value <= hi is guaranteed, accurate to
// approximately targetBits bits (relative). It evaluates the range-reduced
// ln/exp pipeline (lnPositiveFloatAtTarget/expFloatAtTarget) exactly once
// at a working precision with generous guard bits, then widens the single
// result by a matching conservative error bound -- it does not re-run the
// pipeline twice, so this costs about the same as a single ln+exp
// evaluation regardless of whether the bound ultimately needs escalating.
//
// Error budget: half of intervalGuardBits is spent sizing the ln/exp
// series truncation error to targetBits+intervalGuardBits/2 (rather than
// just targetBits), so that even after expFloatAtTarget's bounded squaring
// amplification (at most a factor of 2^(k) for k on the order of the
// input's exponent plus a small constant -- utterly negligible for any
// plausible activeSlotCoeff/stake ratio, whose ln argument magnitude is at
// most on the order of thousands, not exponential in targetBits), the
// accumulated error stays below 2^-targetBits relative, which is the bound
// this function declares to its caller.
func oneMinusFPowerSigmaBounds(
	oneMinusF *big.Rat,
	poolStake, totalStake uint64,
	targetBits uint,
) (lo, hi *big.Float) {
	workPrec := targetBits + intervalGuardBits
	seriesTarget := targetBits + intervalGuardBits/2

	y := new(big.Float).SetPrec(workPrec).SetRat(oneMinusF)
	sigma := new(big.Float).SetPrec(workPrec).Quo(
		new(big.Float).SetPrec(workPrec).SetUint64(poolStake),
		new(big.Float).SetPrec(workPrec).SetUint64(totalStake),
	)

	lnVal := lnPositiveFloatAtTarget(y, seriesTarget)
	product := new(big.Float).SetPrec(workPrec).Mul(sigma, lnVal)
	result := expFloatAtTarget(product, seriesTarget)

	// Conservative relative error bound: result * 2^-targetBits.
	eps := new(big.Float).SetPrec(workPrec).SetMantExp(
		new(big.Float).SetPrec(workPrec).SetInt64(1),
		-int(targetBits),
	)
	epsAbs := new(big.Float).SetPrec(workPrec).Mul(result, eps)
	epsAbs.Abs(epsAbs)

	lo = new(big.Float).SetPrec(workPrec).Sub(result, epsAbs)
	hi = new(big.Float).SetPrec(workPrec).Add(result, epsAbs)
	return lo, hi
}

// NOTE: the ln/exp series below previously (before range-reduction was
// introduced) used a *fixed* 100-term Taylor series applied directly to
// the raw arguments. That is only accurate when the argument is small.
// ln(1-x) converges at rate O(x^n/n), so as the active slot coefficient f
// (i.e. x here) approaches 1, 100 terms is nowhere near enough:
// differential testing against an independent continued-fraction
// implementation of ln (mirroring IntersectMBO/cardano-ledger's
// Cardano.Ledger.NonIntegral) showed the old code's threshold diverging
// from the true value by several *percent* of its magnitude for f around
// 0.9-0.99 (values used on some fast devnets/testnets), not merely a
// boundary rounding sliver. That is a real eligibility-decision bug, not
// just theoretical imprecision.
//
// The functions below fix this by range-reducing the argument first, so
// the number of Taylor terms required is bounded independently of the
// input's magnitude:
//   - lnPositiveFloatAtTarget normalizes the input to a mantissa in
//     [0.5, 1) (via big.Float.MantExp) and uses the fast-converging
//     artanh-based series ln(m) = 2*atanh((m-1)/(m+1)), whose ratio |z| is
//     always <= 1/3 regardless of the original input.
//   - expFloatAtTarget halves the argument down (via MantExp) until it is
//     tiny, applies the Taylor series there (where it converges very
//     fast), then squares the result back up.
//
// numTermsForArtanhSeriesAtTarget/numTermsForExpSeriesAtTarget derive term
// counts that are provably sufficient (with a safety margin) for a given
// targetBits of accuracy, rather than relying on a hand-picked constant
// that happens to work for "typical" inputs. targetBits itself is not
// fixed: CertifiedNatThresholdWithMode starts at seriesTargetBits (enough
// for any input that isn't suspiciously close to an integer threshold
// cutoff) and escalates it via oneMinusFPowerSigmaBounds for the rare
// cases that need more.

// seriesTargetBits is the initial number of bits of accuracy the ln/exp
// series truncation error is sized against, before any escalation. The
// largest upperBound in use is TPraos's 2^512, so an absolute error in
// probability below 2^-512 suffices for the overwhelming majority of
// inputs; the extra guardBits below keeps a comfortable margin.
const seriesTargetBits = vrfOutputBitsTPraos + guardBits

// guardBits is the safety margin (in bits) added on top of the minimum
// required accuracy when deriving term counts.
const guardBits = 64

// log2Of3 = log2(3), used to size the artanh series: since the reduced
// argument z satisfies |z| <= 1/3, the term magnitude shrinks by a factor
// of at least 3^2=9 every two terms.
const log2Of3 = 1.5849625007211562

// numTermsForArtanhSeriesAtTarget returns a term count sufficient to
// compute ln(m) for m in [0.5, 1) to targetBits bits of precision via
// ln(m) = 2*atanh((m-1)/(m+1)). Because m is always normalized to
// [0.5, 1), |z| <= 1/3 always -- the term count needed depends only on
// targetBits, not on the original input's magnitude.
func numTermsForArtanhSeriesAtTarget(targetBits uint) int {
	n := math.Ceil(float64(targetBits) / (2 * log2Of3))
	return int(n) + 16
}

// expReductionBits controls how small |x/2^k| must be before applying the
// Taylor series in expFloatAtTarget. Reducing to a fixed small magnitude
// bounds the number of Taylor terms needed to a value independent of the
// original |x|, unlike applying the series directly to x (whose required
// term count grows with |x|, and would silently lose precision for the
// large negative exponents ln(1-f) produces as f approaches 1).
const expReductionBits = 16

// numTermsForExpSeriesAtTarget returns a term count sufficient to compute
// exp(y) for |y| <= 2^-reductionBits to targetBits bits of precision. This
// bound conservatively ignores the n! denominator (i.e. treats the series
// as if it only decayed geometrically by 2^-reductionBits per term), so it
// is an over-estimate, not a tight one.
func numTermsForExpSeriesAtTarget(targetBits uint, reductionBits int) int {
	n := math.Ceil(float64(targetBits) / float64(reductionBits))
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

// computeLn2AtTarget computes ln(2) = -ln(0.5) using lnNormalized (0.5 is
// already in the required [0.5, 1) range), to targetBits bits of series
// accuracy, for lnPositiveFloatAtTarget's callers. It is deliberately not
// cached: it is only invoked a handful of times per
// CertifiedNatThresholdWithMode call (at most once per precision-
// escalation retry), so the cost of recomputing it at whatever targetBits
// the caller currently needs is negligible next to caching it at a single
// fixed precision that later callers might need to exceed anyway.
func computeLn2AtTarget(prec, targetBits uint) *big.Float {
	half := new(big.Float).SetPrec(prec).SetFloat64(0.5)
	terms := numTermsForArtanhSeriesAtTarget(targetBits)
	lnHalf := lnNormalized(half, terms)
	return new(big.Float).SetPrec(prec).Neg(lnHalf)
}

// lnPositiveFloatAtTarget computes ln(y) for y > 0, by normalizing y to a
// mantissa m in [0.5, 1) with y = m * 2^exp (via big.Float.MantExp) and
// computing ln(y) = ln(m) + exp*ln(2), to targetBits bits of series
// accuracy. This bounds the series' required term count independently of
// how large or small y is, and independently of targetBits (used by the
// escalating-precision interval computation in oneMinusFPowerSigmaBounds,
// which may need far more accuracy than the baseline seriesTargetBits to
// resolve values that land extremely close to an integer cutoff).
func lnPositiveFloatAtTarget(y *big.Float, targetBits uint) *big.Float {
	prec := y.Prec()

	mant := new(big.Float).SetPrec(prec)
	exp := y.MantExp(mant) // y = mant * 2^exp, 0.5 <= mant < 1

	terms := numTermsForArtanhSeriesAtTarget(targetBits)
	lnMant := lnNormalized(mant, terms)

	if exp == 0 {
		return lnMant
	}

	ln2 := computeLn2AtTarget(prec, targetBits)
	expTerm := new(big.Float).SetPrec(prec).Mul(
		new(big.Float).SetPrec(prec).SetInt64(int64(exp)),
		ln2,
	)
	return new(big.Float).SetPrec(prec).Add(lnMant, expTerm)
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

// expFloatAtTarget computes exp(x) for a big.Float x by halving x down
// (via big.Float.MantExp) until it is small enough for the Taylor series
// to converge within a bounded number of terms (sufficient for targetBits
// bits of accuracy), then squaring the result back up. See the NOTE above
// this section for why applying the series directly to x is unsafe for
// the large-magnitude arguments that arise from ln(1-f) as f -> 1.
func expFloatAtTarget(x *big.Float, targetBits uint) *big.Float {
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

	terms := numTermsForExpSeriesAtTarget(targetBits, expReductionBits)
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
