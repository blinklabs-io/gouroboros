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
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

func e34TestInt(t *testing.T, text string) *big.Int {
	t.Helper()
	require.NotEmpty(t, text)
	digits := text
	if digits[0] == '-' {
		digits = digits[1:]
	}
	require.NotEmpty(t, digits)
	for _, digit := range digits {
		require.True(t, digit >= '0' && digit <= '9', text)
	}
	n, ok := new(big.Int).SetString(text, 10)
	require.True(t, ok, text)
	return n
}

func e34TestDecimal(t *testing.T, text string) *big.Int {
	t.Helper()
	parts := strings.Split(text, ".")
	require.Len(t, parts, 2, text)
	require.Len(t, parts[1], 34, text)
	return e34TestInt(t, parts[0]+parts[1])
}

// Each requested fixture must occur exactly once in the published checksum
// manifest. Missing data, truncation and unknown schemas are hard failures.
func e34TestRows(t *testing.T, name string) [][]string {
	t.Helper()
	fs := fixtures.EmbeddedFixtures()
	manifest, err := fs.ReadFile("upstream/e34/SHA256SUMS")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(manifest), "\n"))
	checksums := make(map[string]string)
	for _, line := range strings.Split(
		strings.TrimSuffix(string(manifest), "\n"), "\n",
	) {
		require.GreaterOrEqual(t, len(line), 67, line)
		require.Equal(t, "  ", line[64:66])
		digest, err := hex.DecodeString(line[:64])
		require.NoError(t, err)
		require.Len(t, digest, sha256.Size)
		path := line[66:]
		require.NotEmpty(t, path)
		_, duplicate := checksums[path]
		require.False(t, duplicate, path)
		checksums[path] = line[:64]
		data, err := fs.ReadFile("upstream/e34/" + path)
		require.NoError(t, err, path)
		sum := sha256.Sum256(data)
		require.Equal(t, line[:64], hex.EncodeToString(sum[:]), path)
	}
	_, exists := checksums[name]
	require.True(t, exists, name)
	data, err := fs.ReadFile("upstream/e34/" + name)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.True(t, strings.HasSuffix(string(data), "\n"))
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	rows := make([][]string, len(lines))
	for i, line := range lines {
		require.NotEmpty(t, line)
		rows[i] = strings.Split(line, "\t")
	}
	return rows
}

func e34TestClass(t *testing.T, text string) e34Comparison {
	t.Helper()
	switch text {
	case "ABOVE":
		return e34Above
	case "BELOW":
		return e34Below
	case "MAX_REACHED":
		return e34MaxReached
	default:
		t.Fatalf("unknown E34 comparison %q", text)
		return e34MaxReached
	}
}

func TestE34Primitives(t *testing.T) {
	for _, row := range e34TestRows(t, "pallas/node-primitives.tsv") {
		require.Len(t, row, 4)
		a, b := e34TestInt(t, row[1]), e34TestInt(t, row[2])
		want := e34TestInt(t, row[3])
		for _, alias := range []string{"none", "left", "right"} {
			left, right := new(big.Int).Set(a), new(big.Int).Set(b)
			dst := new(big.Int)
			switch alias {
			case "left":
				dst = left
			case "right":
				dst = right
			}
			switch row[0] {
			case "multiply":
				e34Mul(dst, left, right)
			case "divide":
				require.NoError(t, e34Div(dst, left, right))
			case "ratio":
				require.NoError(t, e34FromRatio(dst, left, right))
			default:
				t.Fatalf("unknown E34 primitive %q", row[0])
			}
			require.Equal(t, want.String(), dst.String(), row)
			if alias != "left" {
				require.Equal(t, a.String(), left.String())
			}
			if alias != "right" {
				require.Equal(t, b.String(), right.String())
			}
		}
	}
}

func TestE34Comparisons(t *testing.T) {
	for _, source := range []string{"pallas", "jormanager"} {
		for _, row := range e34TestRows(t, source+"/node-comparisons.tsv") {
			require.Len(t, row, 4)
			q, x := e34TestInt(t, row[0]), e34TestInt(t, row[1])
			class, _, iterations, err := e34Compare(q, x)
			require.NoError(t, err)
			require.Equal(t, e34TestClass(t, row[2]), class, row)
			require.Equal(t, e34TestInt(t, row[3]).String(),
				big.NewInt(int64(iterations)).String(), row)
		}
	}
}

func TestE34ExtraArithmetic(t *testing.T) {
	var nineTenths, logarithm big.Int
	require.NoError(t, e34FromRatio(
		&nineTenths, big.NewInt(9), big.NewInt(10),
	))
	require.NoError(t, e34Ln(&logarithm, &nineTenths))
	for _, row := range e34TestRows(t, "pallas/node-extra-arithmetic.tsv") {
		require.GreaterOrEqual(t, len(row), 5)
		x := e34TestInt(t, row[0])
		a := e34TestInt(t, row[1])
		b := e34TestInt(t, row[2])
		var denominator, q big.Int
		denominator.Sub(e34Scale, a)
		err := e34Div(&q, e34Scale, &denominator)
		if row[3] == "ERROR" {
			require.Len(t, row, 5)
			require.Equal(t, "divide-by-zero", row[4])
			require.Zero(t, denominator.Sign())
			require.Error(t, err)
			continue
		}
		require.Len(t, row, 8)
		require.NoError(t, err)
		var exponential, negativeLog, product, power, cutoff big.Int
		require.NoError(t, e34Exp(&exponential, x))
		require.NoError(t, e34Ln(&negativeLog, a))
		negativeLog.Neg(&negativeLog)
		e34Mul(&product, b, &logarithm)
		require.NoError(t, e34Exp(&power, &product))
		cutoff.Sub(e34Scale, &power)
		require.Equal(t, e34TestDecimal(t, row[3]).String(),
			exponential.String(), row)
		require.Equal(t, e34TestDecimal(t, row[4]).String(),
			negativeLog.String(), row)
		require.Equal(t, e34TestDecimal(t, row[5]).String(),
			cutoff.String(), row)
		product.Neg(&product)
		class, _, iterations, err := e34Compare(&q, &product)
		require.NoError(t, err)
		require.Equal(t, e34TestClass(t, row[6]), class, row)
		require.Equal(t, e34TestInt(t, row[7]).String(),
			big.NewInt(int64(iterations)).String(), row)
	}
}

func TestE34ArithmeticBoundaries(t *testing.T) {
	var result big.Int
	for _, numerator := range []int64{-7, -6, 0, 6, 7} {
		for _, denominator := range []int64{-3, 3} {
			require.NoError(t, e34FloorQuotient(
				&result, big.NewInt(numerator), big.NewInt(denominator),
			))
			want := numerator / denominator
			if numerator%denominator != 0 &&
				(numerator < 0) != (denominator < 0) {
				want--
			}
			require.Equal(t, want, result.Int64())
		}
	}
	require.Error(t, e34Div(&result, e34Scale, new(big.Int)))
	require.Error(t, e34FromRatio(&result, e34Scale, new(big.Int)))
	require.NoError(t, e34FromRatio(&result,
		e34TestInt(t, "9007199254740993"), big.NewInt(7)))
	require.Equal(t,
		"12867427506772847142857142857142857142857142857142",
		result.String())
	for _, value := range []int64{0, -1} {
		require.Error(t, e34Ln(&result, big.NewInt(value)))
	}
	require.Error(t, e34LnCF(&result, big.NewInt(-1), 1000))
	require.NoError(t, e34Exp(&result, new(big.Int)))
	require.Equal(t, e34Scale.String(), result.String())
	require.NoError(t, e34Exp(&result, new(big.Int).Neg(e34Scale)))
	require.Equal(t, "3678794411714423215955237792349248", result.String())
	require.NoError(t, e34Power(&result, new(big.Int), new(big.Int)))
	require.Equal(t, e34Scale.String(), result.String())
	require.Error(t, e34Power(&result, new(big.Int), big.NewInt(-1)))
	base := e34TestInt(t, "15000000000000000000000000000000001")
	var square, fourth, fifth, reciprocal big.Int
	e34Mul(&square, base, base)
	e34Mul(&fourth, &square, &square)
	e34Mul(&fifth, base, &fourth)
	require.NoError(t, e34Power(&result, base, big.NewInt(5)))
	require.Equal(t, fifth.String(), result.String())
	require.NoError(t, e34Div(&reciprocal, e34Scale, &fifth))
	require.NoError(t, e34Power(&result, base, big.NewInt(-5)))
	require.Equal(t, reciprocal.String(), result.String())
}

func TestE34Termination(t *testing.T) {
	var result big.Int
	require.NoError(t, e34ExpWithBudget(&result, e34Scale, 1))
	require.Equal(t, e34Scale.String(), result.String())
	require.NoError(t, e34ExpWithBudget(&result, e34Scale, 2))
	require.Equal(t, new(big.Int).Lsh(e34Scale, 1).String(), result.String())
	for _, delta := range []int64{-1, 0} {
		input := new(big.Int).Add(e34Epsilon, big.NewInt(delta))
		require.NoError(t, e34Exp(&result, input))
		want := new(big.Int).Set(e34Scale)
		if delta == 0 {
			want.Add(want, input)
		}
		require.Equal(t, want.String(), result.String())
	}
	// The first convergent is computed even at depth zero. Depth one is
	// 1/(1+1/2)=2/3, with each fixed-point operation separately rounded.
	require.NoError(t, e34LnCF(&result, e34Scale, 0))
	require.Equal(t, e34Scale.String(), result.String())
	require.NoError(t, e34LnCF(&result, e34Scale, 1))
	require.Equal(t, "6666666666666666666666666666666666", result.String())
	require.NoError(t, e34LnCF(&result, new(big.Int), 1000))
	require.Zero(t, result.Sign())
}

func TestE34ComparatorBounds(t *testing.T) {
	// x=1 gives acc'=2, errorTerm=3/2 at the first iteration.
	lower := e34TestInt(t, "5000000000000000000000000000000000")
	upper := e34TestInt(t, "35000000000000000000000000000000000")
	for _, bound := range []*big.Int{lower, upper} {
		for _, delta := range []int64{-1, 0, 1} {
			q := new(big.Int).Add(bound, big.NewInt(delta))
			class, _, iterations, err := e34Compare(q, e34Scale)
			require.NoError(t, err)
			if bound == upper && delta >= 0 {
				require.Equal(t, e34Above, class)
				require.Equal(t, 1, iterations)
			} else if bound == lower && delta < 0 {
				require.Equal(t, e34Below, class)
				require.Equal(t, 1, iterations)
			} else {
				require.Greater(t, iterations, 1)
			}
		}
	}
}

func TestE34ExactHalfComparison(t *testing.T) {
	x := e34TestInt(t, "6931471805599453094172321354910343")
	q := new(big.Int).Lsh(e34Scale, 1)
	class, accumulator, iterations, err := e34Compare(q, x)
	require.NoError(t, err)
	require.Equal(t, e34Below, class)
	require.Equal(t, 23, iterations)
	require.Equal(t, "20000000000000000000000000278149284",
		accumulator.String())
}

func TestE34UnaryAliasing(t *testing.T) {
	half := e34TestInt(t, "5000000000000000000000000000000000")
	var logarithm big.Int
	require.NoError(t, e34Ln(&logarithm, half))
	require.Equal(t, "-6931471805599453094172321354910343",
		logarithm.String())
	require.Equal(t, "5000000000000000000000000000000000", half.String())
	require.NoError(t, e34Ln(half, half))
	require.Equal(t, logarithm.String(), half.String())
	input := new(big.Int).Neg(e34Scale)
	var exponential big.Int
	require.NoError(t, e34Exp(&exponential, input))
	require.Equal(t, new(big.Int).Neg(e34Scale).String(), input.String())
	require.NoError(t, e34Exp(input, input))
	require.Equal(t, exponential.String(), input.String())
	base := new(big.Int).Lsh(e34Scale, 1)
	exponent := big.NewInt(-3)
	require.NoError(t, e34Power(base, base, exponent))
	require.Equal(t, "1250000000000000000000000000000000", base.String())
	require.Equal(t, int64(-3), exponent.Int64())
	base.Lsh(e34Scale, 1)
	require.NoError(t, e34Power(exponent, base, exponent))
	require.Equal(t, "1250000000000000000000000000000000",
		exponent.String())
	require.Equal(t, new(big.Int).Lsh(e34Scale, 1).String(), base.String())
}
