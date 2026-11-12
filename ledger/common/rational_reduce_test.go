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
