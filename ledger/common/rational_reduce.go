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

package common

import (
	"errors"
	"math/big"
)

// maxEuclidSteps bounds the Euclidean algorithm run on the components of an
// untrusted rational. Euclid on (g*p, g*q) takes the same steps as on (p, q),
// and by Lame's theorem k steps imply min(p, q) >= F(k+1). F(94) exceeds
// 2^64-1, so every pair whose reduced form fits in Word64 finishes within 93
// steps, counting the swap when the numerator is the smaller operand. A
// larger limit would only spend more time on values that cannot be Word64.
const maxEuclidSteps = 96

var (
	errRationalZeroDenominator = errors.New("rational denominator is zero")
	errRationalNegative        = errors.New("rational is negative")
	errRationalNotWord64       = errors.New(
		"reduced rational does not fit in Word64",
	)
)

// reduceWord64Rational returns n/d in lowest terms with the sign moved to the
// numerator, as the reference decodeRationalWithTag followed by boundRational
// does below protocol version 12, and rejects a result that is negative or
// whose components exceed Word64. Unlike big.Rat.SetFrac, its cost is linear
// in the operand length: the Euclidean algorithm is step limited and every
// division it performs has a quotient narrower than 66 bits.
func reduceWord64Rational(n, d *big.Int) (uint64, uint64, error) {
	p, q, _, err := reduceWord64RationalSteps(n, d)
	return p, q, err
}

// reduceWord64RationalSteps is reduceWord64Rational that also reports the
// number of Euclidean steps taken.
func reduceWord64RationalSteps(
	n, d *big.Int,
) (uint64, uint64, int, error) {
	if d.Sign() == 0 {
		return 0, 0, 0, errRationalZeroDenominator
	}
	if n.Sign() == 0 {
		return 0, 1, 0, nil
	}
	if n.Sign() != d.Sign() {
		return 0, 0, 0, errRationalNegative
	}
	a := new(big.Int).Abs(n)
	b := new(big.Int).Abs(d)
	rem := new(big.Int)
	steps := 0
	for b.Sign() != 0 {
		if steps == maxEuclidSteps {
			return 0, 0, steps, errRationalNotWord64
		}
		// Every partial quotient of a reduced Word64 pair is below 2^64, so
		// a > 2^64*b is already out of domain. Rejecting it keeps each
		// division linear; a wide quotient would make it superlinear.
		if a.BitLen() > b.BitLen()+64 {
			return 0, 0, steps, errRationalNotWord64
		}
		rem.Rem(a, b)
		a, b, rem = b, rem, a
		steps++
	}
	// a is now gcd(|n|, |d|); the exact quotients are the reduced components.
	num := new(big.Int).Quo(new(big.Int).Abs(n), a)
	den := new(big.Int).Quo(new(big.Int).Abs(d), a)
	if !num.IsUint64() || !den.IsUint64() {
		return 0, 0, steps, errRationalNotWord64
	}
	return num.Uint64(), den.Uint64(), steps, nil
}
