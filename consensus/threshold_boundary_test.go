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
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// Independent reference implementation (test-only)
//
// The functions below are a second, deliberately *different*
// implementation of ln(y) and exp(x), used only by the tests in this file
// to differentially validate CertifiedNatThresholdWithMode. They mirror the
// algorithms in IntersectMBO/cardano-ledger's Cardano.Ledger.NonIntegral
// (continued-fraction ln via `ln'`/`lncf`/`splitLn`, and ceil-scaled Taylor
// exp via `exp'`/`scaleExp`/`taylorExp`) rather than threshold.go's
// range-reduction + artanh-series approach. Agreement between the two is
// therefore real differential evidence of correctness, not merely
// self-consistency of a single algorithm at different precisions.
//
// Reference: https://github.com/IntersectMBO/cardano-ledger/blob/master/libs/non-integral/src/Cardano/Ledger/NonIntegral.hs
// =============================================================================

// refPrec is higher than thresholdPrecision so that any residual
// approximation error on either side of the differential comparison is
// visible rather than accidentally hidden by matching precision.
const refPrec = 2048

// refSeriesTerms bounds the reference continued-fraction/Taylor loops.
// Both loops carry their own internal convergence check (matching the
// upstream Haskell `cf`/`taylorExp` epsilon-based early exit), so this is
// just a safety cap, not the primary source of precision.
const refSeriesTerms = 4000

// refEpsilon is the convergence tolerance used by the reference
// continued-fraction/Taylor series, expressed as 2^-(refPrec-64) so it is
// tighter than what's representable at refPrec but doesn't force every
// last loop iteration.
func refEpsilon() *big.Float {
	eps := new(big.Float).SetPrec(refPrec).SetInt64(1)
	two := new(big.Float).SetPrec(refPrec).SetInt64(2)
	for i := 0; i < int(refPrec)-64; i++ {
		eps.Quo(eps, two)
	}
	return eps
}

// refIpowInt computes base^n for any integer n (including negative) via
// repeated squaring (mirrors the Haskell `ipow`/`ipow'`, which computes
// 1/ipow'(base, -n) for negative n).
func refIpowInt(base *big.Float, n int64) *big.Float {
	prec := base.Prec()
	if n < 0 {
		one := new(big.Float).SetPrec(prec).SetInt64(1)
		return new(big.Float).SetPrec(prec).Quo(one, refIpowInt(base, -n))
	}
	result := new(big.Float).SetPrec(prec).SetInt64(1)
	b := new(big.Float).SetPrec(prec).Set(base)
	for n > 0 {
		if n&1 == 1 {
			result.Mul(result, b)
		}
		b.Mul(b, b)
		n >>= 1
	}
	return result
}

// refTaylorExp computes exp(y) via the direct Taylor series, iterating
// until the next term is smaller than eps (mirrors the Haskell
// `taylorExp`). This is only used on the small, ceil-scaled remainder
// produced by refExp, where direct Taylor application is not the
// slow-converging/cancellation-prone case that motivated fixing
// threshold.go's expFloat.
func refTaylorExp(y *big.Float) *big.Float {
	prec := y.Prec()
	eps := refEpsilon()
	result := new(big.Float).SetPrec(prec).SetInt64(1)
	term := new(big.Float).SetPrec(prec).SetInt64(1)
	divisor := new(big.Float).SetPrec(prec).SetInt64(1)
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	absTerm := new(big.Float).SetPrec(prec)

	for n := 0; n < refSeriesTerms; n++ {
		term.Mul(term, y)
		term.Quo(term, divisor)
		result.Add(result, term)
		divisor.Add(divisor, one)

		absTerm.Abs(term)
		if absTerm.Cmp(eps) < 0 {
			break
		}
	}
	return result
}

// ceilPositiveBigFloat returns ceil(x) as a big.Int for x > 0 (mirrors
// Haskell's `ceiling` as used by `scaleExp`), treating x < 1 as ceiling 1
// so the scaled remainder x/x' is always well-defined and < 1.
func ceilPositiveBigFloat(x *big.Float) *big.Int {
	i, acc := x.Int(nil) // truncates towards zero
	if acc != big.Exact {
		i.Add(i, big.NewInt(1))
	}
	if i.Sign() == 0 {
		i.SetInt64(1)
	}
	return i
}

// refExp computes exp(x) for any real x via ceil-scaling (mirrors the
// Haskell `exp'`/`scaleExp`): x' = ceil(x), then exp(x) =
// exp(x/x')^x'. Negative x is handled via exp(x) = 1/exp(-x).
func refExp(x *big.Float) *big.Float {
	prec := x.Prec()
	if x.Sign() == 0 {
		return new(big.Float).SetPrec(prec).SetInt64(1)
	}
	if x.Sign() < 0 {
		neg := new(big.Float).SetPrec(prec).Neg(x)
		one := new(big.Float).SetPrec(prec).SetInt64(1)
		return new(big.Float).SetPrec(prec).Quo(one, refExp(neg))
	}

	xCeil := ceilPositiveBigFloat(x)
	xCeilFloat := new(big.Float).SetPrec(prec).SetInt(xCeil)
	scaled := new(big.Float).SetPrec(prec).Quo(x, xCeilFloat)
	expScaled := refTaylorExp(scaled)
	return refIpowInt(expScaled, xCeil.Int64())
}

// refExp1 is exp(1), computed once per call via refExp; used to bootstrap
// refLn's range-splitting step (mirrors Haskell's `exp1 = exp' 1`).
func refExp1(prec uint) *big.Float {
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	return refExp(one)
}

// refFindE finds the integer n with e^n <= x < e^(n+1) via doubling then
// bisection (mirrors Haskell's `findE`/`bound`/`contract`).
func refFindE(e, x *big.Float) int64 {
	prec := x.Prec()
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	invE := new(big.Float).SetPrec(prec).Quo(one, e)

	lower, upper := int64(-1), int64(1)
	xLow := new(big.Float).SetPrec(prec).Set(invE)
	xHigh := new(big.Float).SetPrec(prec).Set(e)
	for !(xLow.Cmp(x) <= 0 && x.Cmp(xHigh) <= 0) {
		xLow.Mul(xLow, xLow)
		xHigh.Mul(xHigh, xHigh)
		lower *= 2
		upper *= 2
	}

	for lower+1 != upper {
		mid := lower + (upper-lower)/2
		xMid := refIpowInt(e, mid)
		if x.Cmp(xMid) < 0 {
			upper = mid
		} else {
			lower = mid
		}
	}
	return lower
}

// refLncf computes ln(1+x) for x >= 0 via the continued-fraction expansion
// a_1=x, a_{2k}=a_{2k+1}=x*k^2 (k>=1), b_n=n (mirrors Haskell's `lncf`/`cf`).
func refLncf(x *big.Float) *big.Float {
	prec := x.Prec()
	eps := refEpsilon()

	aAt := func(idx int) *big.Float {
		if idx == 1 {
			return new(big.Float).SetPrec(prec).Set(x)
		}
		k := idx / 2
		kf := new(big.Float).SetPrec(prec).SetInt64(int64(k))
		k2 := new(big.Float).SetPrec(prec).Mul(kf, kf)
		return new(big.Float).SetPrec(prec).Mul(x, k2)
	}

	aNm2 := new(big.Float).SetPrec(prec).SetInt64(1) // A_{-1}=1
	bNm2 := new(big.Float).SetPrec(prec).SetInt64(0) // B_{-1}=0
	aNm1 := new(big.Float).SetPrec(prec).SetInt64(0) // A_0=b_0=0
	bNm1 := new(big.Float).SetPrec(prec).SetInt64(1) // B_0=1

	var lastVal, xn *big.Float
	for n := 1; n <= refSeriesTerms; n++ {
		an := aAt(n)
		bn := new(big.Float).SetPrec(prec).SetInt64(int64(n))

		aN := new(big.Float).SetPrec(prec).Add(
			new(big.Float).SetPrec(prec).Mul(bn, aNm1),
			new(big.Float).SetPrec(prec).Mul(an, aNm2),
		)
		bN := new(big.Float).SetPrec(prec).Add(
			new(big.Float).SetPrec(prec).Mul(bn, bNm1),
			new(big.Float).SetPrec(prec).Mul(an, bNm2),
		)
		xn = new(big.Float).SetPrec(prec).Quo(aN, bN)

		if lastVal != nil {
			d := new(big.Float).SetPrec(prec).Sub(xn, lastVal)
			d.Abs(d)
			if d.Cmp(eps) < 0 {
				break
			}
		}
		lastVal = new(big.Float).SetPrec(prec).Set(xn)
		aNm2, bNm2 = aNm1, bNm1
		aNm1, bNm1 = aN, bN
	}
	return xn
}

// refLn computes ln(x) for x > 0 by splitting off the integer part n such
// that e^n <= x < e^(n+1), then applying the continued fraction to the
// remaining fractional part (mirrors Haskell's `ln'`/`splitLn`).
func refLn(x *big.Float) *big.Float {
	prec := x.Prec()
	e := refExp1(prec)
	n := refFindE(e, x)
	yPrime := refIpowInt(e, n)
	xPrime := new(big.Float).SetPrec(prec).Quo(x, yPrime)
	xPrime.Sub(xPrime, new(big.Float).SetPrec(prec).SetInt64(1))
	nFloat := new(big.Float).SetPrec(prec).SetInt64(n)
	return new(big.Float).SetPrec(prec).Add(nFloat, refLncf(xPrime))
}

// refThreshold independently computes floor(upperBound * (1-(1-f)^sigma))
// using refLn/refExp instead of threshold.go's lnOneMinusFloat/expFloat.
func refThreshold(
	poolStake, totalStake uint64,
	f *big.Rat,
	upperBound *big.Int,
) *big.Int {
	sigma := new(big.Float).SetPrec(refPrec).Quo(
		new(big.Float).SetPrec(refPrec).SetUint64(poolStake),
		new(big.Float).SetPrec(refPrec).SetUint64(totalStake),
	)
	ff := new(big.Float).SetPrec(refPrec).SetRat(f)
	one := new(big.Float).SetPrec(refPrec).SetInt64(1)
	y := new(big.Float).SetPrec(refPrec).Sub(one, ff)

	lnVal := refLn(y)
	product := new(big.Float).SetPrec(refPrec).Mul(sigma, lnVal)
	oneMinusFPowerSigma := refExp(product)

	probability := new(big.Float).SetPrec(refPrec).Sub(
		one,
		oneMinusFPowerSigma,
	)
	upperBoundFloat := new(big.Float).SetPrec(refPrec).SetInt(upperBound)
	thresholdFloat := new(big.Float).SetPrec(refPrec).Mul(
		probability,
		upperBoundFloat,
	)
	threshold, _ := thresholdFloat.Int(nil)
	return threshold
}

// =============================================================================
// Differential tests
// =============================================================================

// TestCertifiedNatThresholdDifferentialAgainstIndependentReference compares
// CertifiedNatThresholdWithMode against the independent continued-fraction
// reference above, for both consensus modes, across active slot
// coefficients ranging from realistic (0.05) to pathological-but-valid
// (0.999, just short of the f==1 degenerate case) and stake ratios ranging
// from tiny to full. This directly exercises the conditions the issue
// flagged as risky: extreme active slot coefficients and extreme stake
// ratios, which stress ln(1-f)'s convergence as f -> 1.
func TestCertifiedNatThresholdDifferentialAgainstIndependentReference(
	t *testing.T,
) {
	fValues := []*big.Rat{
		big.NewRat(1, 20),     // 0.05, mainnet
		big.NewRat(1, 10),     // 0.1
		big.NewRat(1, 4),      // 0.25
		big.NewRat(1, 2),      // 0.5
		big.NewRat(9, 10),     // 0.9
		big.NewRat(95, 100),   // 0.95
		big.NewRat(99, 100),   // 0.99
		big.NewRat(999, 1000), // 0.999
	}

	stakeRatios := []struct {
		name string
		p, t uint64
	}{
		{"tiny", 1, 1_000_000_000_000},
		{"quarter", 250_000_000, 1_000_000_000},
		{"half", 500_000_000, 1_000_000_000},
		{"near-full", 999_999_999, 1_000_000_000},
		{"full", 1_000_000_000, 1_000_000_000},
	}

	modes := []struct {
		name string
		mode ConsensusMode
	}{
		{"CPraos", ConsensusModeCPraos},
		{"TPraos", ConsensusModeTPraos},
	}

	for _, m := range modes {
		for _, f := range fValues {
			for _, s := range stakeRatios {
				t.Run(
					m.name+"/f="+f.FloatString(3)+"/"+s.name,
					func(t *testing.T) {
						got, err := CertifiedNatThresholdWithMode(
							s.p,
							s.t,
							f,
							m.mode,
						)
						require.NoError(t, err)

						var upperBound *big.Int
						if m.mode == ConsensusModeCPraos {
							upperBound = twoTo256
						} else {
							upperBound = twoTo512
						}
						want := refThreshold(s.p, s.t, f, upperBound)

						diff := new(big.Int).Sub(got, want)
						diff.Abs(diff)

						// The two implementations use entirely different
						// series/range-reduction strategies, so an exact
						// floor() match is not guaranteed to the very last
						// ULP in pathological cases -- but any real
						// approximation bug shows up as a large relative
						// error, not an off-by-a-few-integers rounding
						// artifact. Require the discrepancy to be
						// vanishingly small relative to the threshold's
						// own magnitude.
						require.True(t, diff.BitLen() <= 4,
							"threshold diverges from independent reference: "+
								"got=%s want=%s diffBits=%d",
							got.String(), want.String(), diff.BitLen())
					},
				)
			}
		}
	}
}

// TestLeaderEligibilityBoundaryAgainstIndependentReference checks
// certified-nat values immediately below, at, and immediately above the
// true (independently computed) eligibility cutoff, for both TPraos and
// CPraos modes and several active slot coefficients/stake ratios. The
// eligibility rule is a strict "<" comparison against the threshold, so:
//   - cutoff-1 must be eligible (below threshold)
//   - cutoff itself must NOT be eligible (not below threshold)
//   - cutoff+1 must NOT be eligible
//
// This is exactly the boundary the issue asked to be covered: values
// adjacent to the cutoff, not just broad statistical behavior.
func TestLeaderEligibilityBoundaryAgainstIndependentReference(t *testing.T) {
	type boundaryCase struct {
		name string
		f    *big.Rat
		p, t uint64
	}

	cases := []boundaryCase{
		{"f=0.05,sigma=0.5", big.NewRat(1, 20), 1, 2},
		{"f=0.1,sigma=0.1", big.NewRat(1, 10), 1, 10},
		{"f=0.5,sigma=0.5", big.NewRat(1, 2), 1, 2},
		{"f=0.9,sigma=0.9", big.NewRat(9, 10), 9, 10},
		{"f=0.99,sigma=0.99", big.NewRat(99, 100), 99, 100},
		{"f=0.999,tiny-sigma", big.NewRat(999, 1000), 1, 1_000_000},
	}

	modes := []struct {
		name          string
		mode          ConsensusMode
		upperBoundVar *big.Int
		outputLen     int
	}{
		{"CPraos", ConsensusModeCPraos, twoTo256, 32},
		{"TPraos", ConsensusModeTPraos, twoTo512, 64},
	}

	for _, m := range modes {
		for _, c := range cases {
			t.Run(m.name+"/"+c.name, func(t *testing.T) {
				cutoff := refThreshold(c.p, c.t, c.f, m.upperBoundVar)
				require.True(t, cutoff.Sign() > 0,
					"expected a positive cutoff for this case")

				threshold, err := CertifiedNatThresholdWithMode(
					c.p,
					c.t,
					c.f,
					m.mode,
				)
				require.NoError(t, err)

				// Sanity: production threshold must itself be close to
				// the independently-computed cutoff (see the differential
				// test above for the tight bound); here we only need it
				// exact enough that the boundary values below don't
				// straddle a difference between the two.
				diff := new(big.Int).Sub(threshold, cutoff)
				diff.Abs(diff)
				require.True(t, diff.BitLen() <= 4,
					"production threshold too far from reference cutoff "+
						"to test boundary meaningfully")

				below := new(big.Int).Sub(cutoff, big.NewInt(1))
				at := new(big.Int).Set(cutoff)
				above := new(big.Int).Add(cutoff, big.NewInt(1))

				toBytes := func(v *big.Int) []byte {
					buf := make([]byte, m.outputLen)
					vb := v.Bytes()
					copy(buf[m.outputLen-len(vb):], vb)
					return buf
				}

				// For TPraos, IsVRFOutputBelowThresholdWithMode uses the
				// raw bytes directly, so we can drive the exact boundary
				// values through the public API. For CPraos the public
				// API hashes the input first (BLAKE2b-256 with "L"
				// prefix), so we instead replicate the same comparison
				// the implementation performs internally
				// (VRFOutputToInt(leaderValue).Cmp(threshold) < 0) using
				// the exact boundary values as if they were already the
				// post-hash leader value -- this is exactly what
				// IsVRFOutputBelowThresholdWithMode does after hashing,
				// so it validates the same comparison logic.
				checkBelow := func(v *big.Int) bool {
					if m.mode == ConsensusModeTPraos {
						ok, err := IsVRFOutputBelowThresholdWithMode(
							toBytes(v),
							threshold,
							m.mode,
						)
						require.NoError(t, err)
						return ok
					}
					return VRFOutputToInt(toBytes(v)).Cmp(threshold) < 0
				}

				require.True(t, checkBelow(below),
					"value immediately below cutoff must be eligible")
				require.False(t, checkBelow(at),
					"value exactly at cutoff must not be eligible")
				require.False(t, checkBelow(above),
					"value immediately above cutoff must not be eligible")
			})
		}
	}
}
