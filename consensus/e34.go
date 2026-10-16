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
	"errors"
	"math/big"
)

// These constants are read-only. Every arithmetic destination is local to a
// preparation or decision, so prepared states can be shared without locks.
var (
	e34Scale   = new(big.Int).Exp(big.NewInt(10), big.NewInt(34), nil)
	e34Epsilon = new(big.Int).Exp(big.NewInt(10), big.NewInt(10), nil)
)

// e34FloorQuotient implements Haskell div, not Go's truncating Quo.
// QuoRem permits dst to alias either input.
func e34FloorQuotient(dst, numerator, denominator *big.Int) error {
	if denominator.Sign() == 0 {
		return errors.New("E34 division by zero")
	}
	var n, d, r big.Int
	if denominator.Sign() < 0 {
		n.Neg(numerator)
		d.Neg(denominator)
		numerator, denominator = &n, &d
	}
	if numerator.Sign() >= 0 {
		dst.Quo(numerator, denominator)
		return nil
	}
	dst.QuoRem(numerator, denominator, &r)
	if r.Sign() < 0 {
		dst.Sub(dst, big.NewInt(1))
	}
	return nil
}

func e34FromRatio(dst, numerator, denominator *big.Int) error {
	// Multiplication supports aliased inputs, but scaling must not overwrite
	// an aliased denominator before the division reads it.
	var savedDenominator big.Int
	if dst == denominator {
		savedDenominator.Set(denominator)
		denominator = &savedDenominator
	}
	dst.Mul(numerator, e34Scale)
	return e34FloorQuotient(dst, dst, denominator)
}

func e34Mul(dst, a, b *big.Int) {
	dst.Mul(a, b)
	// The fixed positive scale cannot produce a division error.
	_ = e34FloorQuotient(dst, dst, e34Scale)
}

func e34Div(dst, a, b *big.Int) error {
	return e34FromRatio(dst, a, b)
}

func e34Power(dst, base, exponent *big.Int) error {
	if exponent.Sign() < 0 {
		var positive, power big.Int
		positive.Neg(exponent)
		if err := e34Power(&power, base, &positive); err != nil {
			return err
		}
		return e34Div(dst, e34Scale, &power)
	}
	if exponent.Sign() == 0 {
		dst.Set(e34Scale)
		return nil
	}
	var next, power big.Int
	if exponent.Bit(0) == 0 {
		next.Rsh(exponent, 1)
		if err := e34Power(&power, base, &next); err != nil {
			return err
		}
		e34Mul(dst, &power, &power)
	} else {
		next.Sub(exponent, big.NewInt(1))
		if err := e34Power(&power, base, &next); err != nil {
			return err
		}
		e34Mul(dst, base, &power)
	}
	return nil
}

func e34Exp(dst, v *big.Int) error {
	return e34ExpWithBudget(dst, v, 1000)
}

// The budget is private and only varied by termination tests.
func e34ExpWithBudget(dst, v *big.Int, budget int) error {
	if v.Sign() == 0 {
		dst.Set(e34Scale)
		return nil
	}
	if v.Sign() < 0 {
		var positive, result big.Int
		positive.Neg(v)
		if err := e34ExpWithBudget(&result, &positive, budget); err != nil {
			return err
		}
		return e34Div(dst, e34Scale, &result)
	}
	var n, remainder, scaledN, x big.Int
	n.QuoRem(v, e34Scale, &remainder)
	if remainder.Sign() != 0 {
		n.Add(&n, big.NewInt(1))
	}
	scaledN.Mul(&n, e34Scale)
	if err := e34Div(&x, v, &scaledN); err != nil {
		return err
	}
	var last, acc, divisor, next, absolute big.Int
	last.Set(e34Scale)
	acc.Set(e34Scale)
	divisor.Set(e34Scale)
	for i := 1; i < budget; i++ {
		e34Mul(&next, &last, &x)
		if err := e34Div(&next, &next, &divisor); err != nil {
			return err
		}
		absolute.Abs(&next)
		if absolute.Cmp(e34Epsilon) < 0 {
			break
		}
		acc.Add(&acc, &next)
		last.Set(&next)
		divisor.Add(&divisor, e34Scale)
	}
	return e34Power(dst, &acc, &n)
}

func e34Ln(dst, v *big.Int) error {
	if v.Sign() <= 0 {
		return errors.New("E34 logarithm requires a positive value")
	}
	var e, lowerValue, upperValue big.Int
	if err := e34Exp(&e, e34Scale); err != nil {
		return err
	}
	if err := e34Div(&lowerValue, e34Scale, &e); err != nil {
		return err
	}
	upperValue.Set(&e)
	lower, upper := big.NewInt(-1), big.NewInt(1)
	for lowerValue.Cmp(v) > 0 || upperValue.Cmp(v) < 0 {
		e34Mul(&lowerValue, &lowerValue, &lowerValue)
		e34Mul(&upperValue, &upperValue, &upperValue)
		lower.Lsh(lower, 1)
		upper.Lsh(upper, 1)
	}
	var distance, mid, power big.Int
	for {
		distance.Sub(upper, lower)
		if distance.Cmp(big.NewInt(1)) == 0 {
			break
		}
		mid.Rsh(&distance, 1)
		mid.Add(lower, &mid)
		if err := e34Power(&power, &e, &mid); err != nil {
			return err
		}
		if v.Cmp(&power) < 0 {
			upper.Set(&mid)
		} else {
			lower.Set(&mid)
		}
	}
	if err := e34Power(&power, &e, lower); err != nil {
		return err
	}
	var r, fraction big.Int
	if err := e34Div(&r, v, &power); err != nil {
		return err
	}
	r.Sub(&r, e34Scale)
	if err := e34LnCF(&fraction, &r, 1000); err != nil {
		return err
	}
	power.Mul(lower, e34Scale)
	dst.Add(&power, &fraction)
	return nil
}

func e34LnCF(dst, r *big.Int, budget int) error {
	if r.Sign() < 0 {
		return errors.New("E34 continued fraction requires nonnegative input")
	}
	var aOlder, bOlder, aPrev, bPrev big.Int
	aOlder.Set(e34Scale)
	bPrev.Set(e34Scale)
	var a, b, coefficient, denominator, left, right big.Int
	var convergent, previous, difference, k big.Int
	for depth := 0; ; depth++ {
		coefficient.Set(r)
		if depth > 0 {
			k.SetInt64(int64((depth + 1) / 2))
			k.Mul(&k, &k)
			// k is integral: converting k² before multiplying is exact.
			coefficient.Mul(r, &k)
		}
		denominator.SetInt64(int64(depth + 1))
		denominator.Mul(&denominator, e34Scale)
		e34Mul(&left, &denominator, &aPrev)
		e34Mul(&right, &coefficient, &aOlder)
		a.Add(&left, &right)
		e34Mul(&left, &denominator, &bPrev)
		e34Mul(&right, &coefficient, &bOlder)
		b.Add(&left, &right)
		if err := e34Div(&convergent, &a, &b); err != nil {
			return err
		}
		difference.Sub(&previous, &convergent)
		difference.Abs(&difference)
		if depth == budget ||
			(depth > 0 && difference.Cmp(e34Epsilon) < 0) {
			dst.Set(&convergent)
			return nil
		}
		previous.Set(&convergent)
		aOlder.Set(&aPrev)
		bOlder.Set(&bPrev)
		aPrev.Set(&a)
		bPrev.Set(&b)
	}
}

type e34Comparison uint8

const (
	e34Above e34Comparison = iota
	e34Below
	e34MaxReached
)

func e34Compare(q, x *big.Int) (e34Comparison, big.Int, int, error) {
	var acc, errTerm, divisor, nextErr big.Int
	var errorBound, bound, product big.Int
	acc.Set(e34Scale)
	errTerm.Set(x)
	divisor.Set(e34Scale)
	for iteration := 1; iteration <= 1000; iteration++ {
		divisor.Add(&divisor, e34Scale)
		product.Mul(&errTerm, x)
		if err := e34FloorQuotient(&nextErr, &product, e34Scale); err != nil {
			return e34MaxReached, acc, iteration, err
		}
		product.Mul(&nextErr, e34Scale)
		if err := e34FloorQuotient(
			&nextErr, &product, &divisor,
		); err != nil {
			return e34MaxReached, acc, iteration, err
		}
		acc.Add(&acc, &errTerm)
		errorBound.Mul(&nextErr, big.NewInt(3))
		errorBound.Abs(&errorBound)
		bound.Add(&acc, &errorBound)
		if q.Cmp(&bound) >= 0 {
			return e34Above, acc, iteration, nil
		}
		bound.Sub(&acc, &errorBound)
		if q.Cmp(&bound) < 0 {
			return e34Below, acc, iteration, nil
		}
		errTerm, nextErr = nextErr, errTerm
	}
	return e34MaxReached, acc, 1000, nil
}
