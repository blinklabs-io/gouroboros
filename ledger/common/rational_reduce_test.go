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
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fibBig(k int) *big.Int {
	a, b := big.NewInt(0), big.NewInt(1)
	for i := 0; i < k; i++ {
		a, b = b, new(big.Int).Add(a, b)
	}
	return a
}

// scaledBig returns v multiplied by a deterministic factor of about
// bits bits.
func scaledBig(v *big.Int, bits int, seed int64) *big.Int {
	g := new(big.Int).Rand(
		rand.New(rand.NewSource(seed)),
		new(big.Int).Lsh(big.NewInt(1), uint(bits)),
	)
	g.SetBit(g, bits, 1)
	return g.Mul(g, v)
}

func randomBig(rng *rand.Rand, byteLen int) *big.Int {
	buf := make([]byte, byteLen)
	_, _ = rng.Read(buf)
	return new(big.Int).SetBytes(buf)
}

func TestReduceWord64RationalMatchesUpstreamDomain(t *testing.T) {
	t.Parallel()
	g := scaledBig(big.NewInt(1), 10*1024*8, 1)
	maxU64 := new(big.Int).SetUint64(math.MaxUint64)
	twoPow64 := new(big.Int).Lsh(big.NewInt(1), 64)
	for _, test := range []struct {
		name    string
		n, d    *big.Int
		wantP   uint64
		wantQ   uint64
		wantErr error
	}{
		{
			name:  "huge common factor",
			n:     new(big.Int).Mul(g, big.NewInt(3)),
			d:     new(big.Int).Mul(g, big.NewInt(4)),
			wantP: 3, wantQ: 4,
		},
		{
			name:  "negative over negative",
			n:     big.NewInt(-3),
			d:     big.NewInt(-4),
			wantP: 3, wantQ: 4,
		},
		{
			name: "zero numerator", n: big.NewInt(0), d: big.NewInt(5),
			wantP: 0, wantQ: 1,
		},
		{
			name: "zero over negative", n: big.NewInt(0), d: big.NewInt(-5),
			wantP: 0, wantQ: 1,
		},
		{
			name: "max Word64", n: maxU64, d: big.NewInt(1),
			wantP: math.MaxUint64, wantQ: 1,
		},
		{
			name: "above Word64", n: twoPow64, d: big.NewInt(1),
			wantErr: errRationalNotWord64,
		},
		{
			name: "negative numerator", n: big.NewInt(-3), d: big.NewInt(4),
			wantErr: errRationalNegative,
		},
		{
			name: "negative denominator", n: big.NewInt(3), d: big.NewInt(-4),
			wantErr: errRationalNegative,
		},
		{
			name: "zero denominator", n: big.NewInt(3), d: big.NewInt(0),
			wantErr: errRationalZeroDenominator,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p, q, err := reduceWord64Rational(test.n, test.d)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantP, p)
			assert.Equal(t, test.wantQ, q)
		})
	}
}

func TestReduceWord64RationalStepLimitEdge(t *testing.T) {
	t.Parallel()
	// F(94) is the first Fibonacci number above Word64, so F(93)/F(92) is
	// the slowest reduced pair a Word64 rational can be.
	hi, lo := fibBig(93), fibBig(92)
	require.True(t, hi.IsUint64())
	require.False(t, fibBig(94).IsUint64())
	for _, test := range []struct {
		name      string
		n, d      *big.Int
		scaleBits int
	}{
		{"descending", hi, lo, 0},
		{"ascending", lo, hi, 0},
		{"descending scaled", hi, lo, 80_000},
		{"ascending scaled", lo, hi, 80_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			n, d := test.n, test.d
			if test.scaleBits > 0 {
				g := scaledBig(big.NewInt(1), test.scaleBits, 2)
				n = new(big.Int).Mul(g, test.n)
				d = new(big.Int).Mul(g, test.d)
			}
			p, q, steps, err := reduceWord64RationalSteps(n, d)
			require.NoError(t, err)
			assert.Equal(t, test.n.Uint64(), p)
			assert.Equal(t, test.d.Uint64(), q)
			assert.LessOrEqual(t, steps, maxEuclidSteps)
		})
	}
}

func TestReduceWord64RationalAgreesWithSetFrac(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(20240607))
	for i := 0; i < 5000; i++ {
		var n, d *big.Int
		switch i % 3 {
		case 0:
			n = randomBig(rng, 1+rng.Intn(9))
			d = randomBig(rng, 1+rng.Intn(9))
		case 1:
			n = randomBig(rng, 1+rng.Intn(40))
			d = randomBig(rng, 1+rng.Intn(40))
		default:
			g := randomBig(rng, 1+rng.Intn(60))
			g.Add(g, big.NewInt(1))
			n = new(big.Int).Mul(g, randomBig(rng, 1+rng.Intn(8)))
			d = new(big.Int).Mul(g, randomBig(rng, 1+rng.Intn(8)))
		}
		if d.Sign() == 0 {
			d.SetInt64(1)
		}
		if rng.Intn(8) == 0 {
			n.Neg(n)
		}
		if rng.Intn(8) == 0 {
			d.Neg(d)
		}
		want := new(big.Rat).SetFrac(n, d)
		wantOK := want.Num().IsUint64() && want.Denom().IsUint64()
		p, q, err := reduceWord64Rational(n, d)
		require.Equal(t, wantOK, err == nil, "n=%s d=%s", n, d)
		if wantOK {
			require.Equal(t, want.Num().Uint64(), p, "n=%s d=%s", n, d)
			require.Equal(t, want.Denom().Uint64(), q, "n=%s d=%s", n, d)
		}
	}
}

func TestReduceWord64RationalCostIsBounded(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(7))
	n := randomBig(rng, 128*1024)
	d := randomBig(rng, 128*1024)
	_, _, steps, err := reduceWord64RationalSteps(n, d)
	require.ErrorIs(t, err, errRationalNotWord64)
	assert.Equal(t, maxEuclidSteps, steps)
}

// A partial quotient wider than Word64 cannot occur in the reduction of a
// Word64 pair, so a division whose operands differ by more than 64 bits is
// rejected before it runs; a long lopsided division is not linear in the
// operand length.
func TestReduceWord64RationalRejectsWideQuotientBeforeDividing(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(11))
	d := randomBig(rng, 64*1024)
	for _, test := range []struct {
		name     string
		n, d     *big.Int
		maxSteps int
	}{
		{"first quotient", new(big.Int).Lsh(d, 256*1024), d, 0},
		{
			"second quotient",
			new(big.Int).Add(d, randomBig(rng, 32*1024)),
			d,
			1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, steps, err := reduceWord64RationalSteps(test.n, test.d)
			require.ErrorIs(t, err, errRationalNotWord64)
			assert.LessOrEqual(t, steps, test.maxSteps)
		})
	}
	// The widest quotient a Word64 pair produces is still accepted.
	maxU64 := new(big.Int).SetUint64(math.MaxUint64)
	g := scaledBig(big.NewInt(1), 80_000, 3)
	p, q, err := reduceWord64Rational(
		new(big.Int).Mul(g, maxU64),
		g,
	)
	require.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64), p)
	assert.Equal(t, uint64(1), q)
}

// A component that is itself a tag-30 rational must be rejected from its
// header: decoding it into an interface would reduce it with big.Rat.SetFrac.
// The inner zero denominator is reported only if that decode runs.
func TestRawCBORIntegerRejectsNestedRationalUndecoded(t *testing.T) {
	t.Parallel()
	inner := []byte{0xd8, 0x1e, 0x82, 0x01, 0x00}
	for _, component := range [][]byte{
		inner,
		append([]byte{0xd9, 0xd9, 0xf7}, inner...),
	} {
		_, err := rawCBORInteger(component)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "denominator")
		assert.Contains(t, err.Error(), "expected integer")

		content := append(append([]byte{0x82}, component...), 0x01)
		raw, err := cbor.Encode(
			cbor.RawTag{Number: cbor.CborTagRational, Content: content},
		)
		require.NoError(t, err)
		err = ValidateNonNegativeBoundedRatCBOR(raw)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "denominator cannot be zero")
		err = validatePoolMarginCBOR(raw, false)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "denominator cannot be zero")
	}
	for _, component := range [][]byte{
		{0x01},
		{0x38, 0x00},
		{0xc2, 0x41, 0x01},
		{0xd9, 0xd9, 0xf7, 0x01},
	} {
		_, err := rawCBORInteger(component)
		require.NoError(t, err, "%x", component)
	}
}

func BenchmarkReduceWord64Rational128KB(b *testing.B) {
	rng := rand.New(rand.NewSource(7))
	n := randomBig(rng, 128*1024)
	d := randomBig(rng, 128*1024)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = reduceWord64Rational(n, d)
	}
}

func ratCBOR(t testing.TB, n, d *big.Int) []byte {
	t.Helper()
	content, err := cbor.Encode([]any{n, d})
	require.NoError(t, err)
	raw, err := cbor.Encode(
		cbor.RawTag{Number: cbor.CborTagRational, Content: content},
	)
	require.NoError(t, err)
	return raw
}

func TestValidateNonNegativeBoundedRatCBORBelowVersion12(t *testing.T) {
	t.Parallel()
	g := scaledBig(big.NewInt(1), 10*1024*8, 3)
	for _, test := range []struct {
		name  string
		n, d  *big.Int
		valid bool
	}{
		{
			"huge common factor",
			new(big.Int).Mul(g, big.NewInt(3)),
			new(big.Int).Mul(g, big.NewInt(4)),
			true,
		},
		{"negative over negative", big.NewInt(-3), big.NewInt(-4), true},
		{"zero over positive", big.NewInt(0), big.NewInt(5), true},
		{
			"max Word64",
			new(big.Int).SetUint64(math.MaxUint64), big.NewInt(1), true,
		},
		{"above Word64", new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1), false},
		{"negative numerator", big.NewInt(-3), big.NewInt(4), false},
		{"zero denominator", big.NewInt(3), big.NewInt(0), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateNonNegativeBoundedRatCBORForVersion(
				ratCBOR(t, test.n, test.d), 11,
			)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateNonNegativeBoundedRatCBORHugeComponents(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(11))
	n := randomBig(rng, 128*1024)
	d := randomBig(rng, 128*1024)
	err := ValidateNonNegativeBoundedRatCBORForVersion(ratCBOR(t, n, d), 11)
	require.Error(t, err)
	require.NotNil(t, err)
	assert.Regexp(t, `numerator \d+ bits, denominator \d+ bits`, err.Error())
	assert.Less(t, len(err.Error()), 200)
}

func TestValidatePoolMarginCBORScaled(t *testing.T) {
	t.Parallel()
	g := scaledBig(big.NewInt(1), 10*1024*8, 4)
	scaled := func(n, d int64) []byte {
		return ratCBOR(
			t,
			new(big.Int).Mul(g, big.NewInt(n)),
			new(big.Int).Mul(g, big.NewInt(d)),
		)
	}
	require.NoError(t, validatePoolMarginCBOR(scaled(1, 2), false))
	require.NoError(t, validatePoolMarginCBOR(scaled(1, 1), false))
	err := validatePoolMarginCBOR(scaled(3, 2), false)
	require.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "3/2")
}

func TestValidatePoolMarginCBORHugeComponents(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(13))
	n := randomBig(rng, 128*1024)
	d := randomBig(rng, 128*1024)
	err := validatePoolMarginCBOR(ratCBOR(t, n, d), false)
	require.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
	require.NotNil(t, err)
	assert.Regexp(t, `numerator \d+ bits, denominator \d+ bits`, err.Error())
	assert.Less(t, len(err.Error()), 200)
}

func BenchmarkValidateNonNegativeBoundedRatCBOR128KB(b *testing.B) {
	rng := rand.New(rand.NewSource(11))
	raw := ratCBOR(b, randomBig(rng, 128*1024), randomBig(rng, 128*1024))
	b.ReportAllocs()
	for b.Loop() {
		_ = ValidateNonNegativeBoundedRatCBORForVersion(raw, 11)
	}
}

func BenchmarkValidatePoolMarginCBOR128KB(b *testing.B) {
	rng := rand.New(rand.NewSource(13))
	raw := ratCBOR(b, randomBig(rng, 128*1024), randomBig(rng, 128*1024))
	b.ReportAllocs()
	for b.Loop() {
		_ = validatePoolMarginCBOR(raw, false)
	}
}
