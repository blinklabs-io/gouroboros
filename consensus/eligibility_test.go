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
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func eligibilityTestUint64(t *testing.T, text string) uint64 {
	t.Helper()
	n := e34TestInt(t, text)
	require.True(t, n.IsUint64(), text)
	return n.Uint64()
}

func eligibilityTestBool(t *testing.T, text string) bool {
	t.Helper()
	switch text {
	case "true":
		return true
	case "false":
		return false
	default:
		t.Fatalf("invalid eligibility boolean %q", text)
		return false
	}
}

// This is only the integer quotient diagnostic from the specification, not a
// second eligibility engine. Expected decisions always come from pinned rows.
func eligibilityTestQ(maximum, natural *big.Int) *big.Int {
	q := new(big.Int).Mul(maximum, e34Scale)
	denominator := new(big.Int).Sub(maximum, natural)
	if denominator.Sign() != 0 {
		q.Quo(q, denominator)
	}
	return q
}

func TestLeaderEligibilityNaturalVectors(t *testing.T) {
	for _, source := range []struct {
		name  string
		count int
	}{
		{"pallas", 648},
		{"jormanager", 352},
	} {
		t.Run(source.name, func(t *testing.T) {
			rows := e34TestRows(t, source.name+"/node-vectors.tsv")
			require.Len(t, rows, source.count)
			counts := [2]int{}
			for group, name := range []string{"representable", "oracle_endpoint"} {
				t.Run(name, func(t *testing.T) {
					for i, row := range rows {
						require.Len(t, row, 11, "row %d", i+1)
						var mode ConsensusMode
						var bits uint
						switch row[0] {
						case "32":
							mode, bits = ConsensusModeCPraos, 256
						case "64":
							mode, bits = ConsensusModeTPraos, 512
						default:
							t.Fatalf(
								"row %d: unsupported natural width %q",
								i+1,
								row[0],
							)
						}
						maximum := new(big.Int).Lsh(big.NewInt(1), bits)
						v := e34TestInt(t, row[1])
						require.NotEqual(t, -1, v.Sign())
						require.LessOrEqual(t, v.Cmp(maximum), 0)
						endpoint := v.Cmp(maximum) == 0
						if endpoint != (group == 1) {
							continue
						}
						counts[group]++
						t.Run(fmt.Sprintf("row_%d", i+1), func(t *testing.T) {
							pool := eligibilityTestUint64(t, row[2])
							total := eligibilityTestUint64(t, row[3])
							numerator := eligibilityTestUint64(t, row[4])
							denominator := eligibilityTestUint64(t, row[5])
							require.NotZero(t, denominator)
							f := new(
								big.Rat,
							).SetFrac(new(big.Int).SetUint64(numerator), new(big.Int).SetUint64(denominator))
							want := eligibilityTestBool(t, row[6])
							e, err := NewLeaderEligibility(pool, total, f, mode)
							require.NoError(t, err)
							require.Equal(t, mode, e.state.mode)
							require.Equal(
								t,
								maximum.String(),
								e.state.maximum.String(),
							)
							require.Equal(
								t,
								e34TestInt(t, row[7]).String(),
								e.state.sigma.String(),
							)
							require.Equal(
								t,
								e34TestInt(t, row[9]).String(),
								eligibilityTestQ(maximum, v).String(),
							)
							if numerator == denominator {
								require.True(t, e.state.unconditional)
								require.Equal(t, "none", row[8])
								require.Equal(t, "none", row[10])
							} else {
								require.False(t, e.state.unconditional)
								require.Equal(t, e34TestInt(t, row[8]).String(), e.state.c.String())
								require.Equal(t, e34TestInt(t, row[10]).String(), e.state.x.String())
							}
							before := v.String()
							got, err := e.CheckCertifiedNatural(v)
							require.NoError(t, err)
							require.Equal(t, want, got)
							require.Equal(t, before, v.String())
							// Never encode the artificial endpoint into an N-bit buffer.
							if !endpoint && mode == ConsensusModeTPraos {
								output := v.FillBytes(make([]byte, 64))
								got, err = e.CheckVRFOutput(output)
								require.NoError(t, err)
								require.Equal(t, want, got)
							}
						})
					}
				})
			}
			require.Positive(t, counts[0])
			require.Positive(t, counts[1])
			require.Equal(t, source.count, counts[0]+counts[1])
			t.Logf(
				"validated %d natural rows (%d representable, %d oracle endpoints)",
				source.count,
				counts[0],
				counts[1],
			)
		})
	}
}

func TestLeaderEligibilityE34Regressions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        ConsensusMode
		pool, total uint64
		f           *big.Rat
		natural     string
		neighbors   [3]bool
	}{
		{"exact_half", ConsensusModeTPraos, 1, 1, big.NewRat(1, 2), new(big.Int).Lsh(big.NewInt(1), 511).String(), [3]bool{true, true, true}},
		{"one_twentieth", ConsensusModeCPraos, 1, 3, big.NewRat(1, 20), "1962956998846591905535085628410160294139768371758753777856225283307809972526", [3]bool{true, true, false}},
		{"one_hundred_thousandth", ConsensusModeCPraos, 1, 3, big.NewRat(1, 100000), "385974917376970995734921210494643580273092695162968067819510714679869089", [3]bool{true, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewLeaderEligibility(tc.pool, tc.total, tc.f, tc.mode)
			require.NoError(t, err)
			center := e34TestInt(t, tc.natural)
			for i, want := range tc.neighbors {
				v := new(big.Int).Add(center, big.NewInt(int64(i-1)))
				got, err := e.CheckCertifiedNatural(v)
				require.NoError(t, err)
				require.Equal(t, want, got, "neighbor %d", i-1)
				q := eligibilityTestQ(&e.state.maximum, v)
				if tc.name == "exact_half" {
					wantQ := new(big.Int).Mul(e34Scale, big.NewInt(2))
					if i == 0 {
						wantQ.Sub(wantQ, big.NewInt(1))
					}
					require.Equal(t, wantQ.String(), q.String())
				}
				if tc.name == "one_hundred_thousandth" && i >= 1 {
					// The +1 quotient is an integer assertion, not an extra oracle row.
					require.Equal(
						t,
						"10000033333555557283965020314332458",
						q.String(),
					)
				}
			}
			if tc.name == "one_twentieth" {
				require.Equal(
					t,
					"3333333333333333333333333333333333",
					e.state.sigma.String(),
				)
				require.Equal(
					t,
					"-512932943875505334261962382072846",
					e.state.c.String(),
				)
				require.Equal(
					t,
					"170977647958501778087320794024282",
					e.state.x.String(),
				)
				require.Equal(
					t,
					"10172447681911010168490328908775615",
					eligibilityTestQ(&e.state.maximum, center).String(),
				)
			}
			if tc.name == "exact_half" {
				class, acc, iterations, err := e34Compare(
					eligibilityTestQ(&e.state.maximum, center),
					&e.state.x,
				)
				require.NoError(t, err)
				require.Equal(t, e34Below, class)
				require.Equal(t, 23, iterations)
				require.Equal(
					t,
					"20000000000000000000000000278149284",
					acc.String(),
				)
				output := make([]byte, 64)
				output[0] = 0x80
				got, err := e.CheckVRFOutput(output)
				require.NoError(t, err)
				require.True(t, got)
				got, err = IsSlotLeaderFromComponentsWithMode(
					output,
					1,
					1,
					big.NewRat(1, 2),
					ConsensusModeTPraos,
				)
				require.NoError(t, err)
				require.True(t, got)
			}
			for _, edge := range []struct {
				v    *big.Int
				want bool
			}{
				{big.NewInt(0), true},
				{new(big.Int).Sub(&e.state.maximum, big.NewInt(1)), false},
			} {
				got, err := e.CheckCertifiedNatural(edge.v)
				require.NoError(t, err)
				require.Equal(t, edge.want, got)
			}
		})
	}
}

func TestLeaderEligibilityDomain(t *testing.T) {
	const domain = "activeSlotCoeff is outside the node PositiveUnitInterval domain"
	giantDenominator := new(big.Int).Lsh(big.NewInt(1), 2000)
	giantNearOne := new(
		big.Rat,
	).SetFrac(new(big.Int).Sub(giantDenominator, big.NewInt(1)), giantDenominator)
	maxWord := new(big.Int).SetUint64(^uint64(0))
	nearOne := new(
		big.Rat,
	).SetFrac(new(big.Int).Sub(maxWord, big.NewInt(1)), maxWord)
	minimum := new(
		big.Rat,
	).SetFrac(big.NewInt(1), new(big.Int).SetUint64(10_000_000_000_000_000_000))
	belowMinimum := new(big.Rat).SetFrac(big.NewInt(1), maxWord)
	for _, tc := range []struct {
		name        string
		pool, total uint64
		f           *big.Rat
		mode        ConsensusMode
		err         string
	}{
		{"mode_before_nil_and_zero", 0, 0, nil, ConsensusMode(99), "unknown consensus mode: 99"},
		{"mode_before_domain", 0, 0, giantNearOne, ConsensusMode(-1), "unknown consensus mode: -1"},
		{"nil_before_zero_total", 0, 0, nil, ConsensusModeCPraos, "activeSlotCoeff is nil"},
		{"zero_before_zero_total", 0, 0, big.NewRat(0, 1), ConsensusModeCPraos, domain},
		{"negative", 1, 1, big.NewRat(-1, 20), ConsensusModeCPraos, domain},
		{"above_one", 1, 1, big.NewRat(21, 20), ConsensusModeCPraos, domain},
		{"giant_near_one", 1, 1, giantNearOne, ConsensusModeCPraos, domain},
		{"giant_denominator", 1, 1, new(big.Rat).SetFrac(big.NewInt(1), giantDenominator), ConsensusModeCPraos, domain},
		{"below_minimum", 0, 1, belowMinimum, ConsensusModeCPraos, domain},
		{"domain_before_zero_total", 0, 0, belowMinimum, ConsensusModeTPraos, domain},
		{"zero_total", 0, 0, big.NewRat(1, 20), ConsensusModeCPraos, "total stake cannot be zero"},
		{"one_zero_total", 1, 0, big.NewRat(1, 1), ConsensusModeTPraos, "total stake cannot be zero"},
		{"minimum", 1, 1, minimum, ConsensusModeCPraos, ""},
		{"third", 1, 1, big.NewRat(1, 3), ConsensusModeCPraos, ""},
		{"near_one_word64", 1, 1, nearOne, ConsensusModeTPraos, ""},
		{"one", 1, 1, big.NewRat(1, 1), ConsensusModeTPraos, ""},
		{"zero_pool", 0, 1, big.NewRat(1, 20), ConsensusModeCPraos, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewLeaderEligibility(tc.pool, tc.total, tc.f, tc.mode)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				require.Nil(t, e)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, e)
			got, err := e.CheckCertifiedNatural(big.NewInt(0))
			require.NoError(t, err)
			require.Equal(t, tc.pool > 0, got)
		})
	}
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			clamped, err := NewLeaderEligibility(
				^uint64(0),
				3,
				big.NewRat(1, 20),
				mode,
			)
			require.NoError(t, err)
			require.Equal(t, e34Scale.String(), clamped.state.sigma.String())
			for _, edge := range []struct {
				v    *big.Int
				want bool
			}{
				{big.NewInt(0), true},
				{new(big.Int).Sub(&clamped.state.maximum, big.NewInt(1)), false},
			} {
				got, err := clamped.CheckCertifiedNatural(edge.v)
				require.NoError(t, err)
				require.Equal(t, edge.want, got)
			}
			zero, err := NewLeaderEligibility(0, 3, big.NewRat(1, 1), mode)
			require.NoError(t, err)
			for _, v := range []*big.Int{big.NewInt(0), new(big.Int).Set(&zero.state.maximum)} {
				got, err := zero.CheckCertifiedNatural(v)
				require.NoError(t, err)
				require.True(t, got)
			}
		})
	}
}

func TestLeaderEligibilityInvalidMethods(t *testing.T) {
	for _, e := range []*LeaderEligibility{nil, {}, {state: nil}} {
		got, err := e.CheckCertifiedNatural(nil)
		require.False(t, got)
		require.EqualError(
			t,
			err,
			"leader eligibility evaluator is uninitialized",
		)
		got, err = e.CheckVRFOutput(nil)
		require.False(t, got)
		require.EqualError(
			t,
			err,
			"leader eligibility evaluator is uninitialized",
		)
	}
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		for _, f := range []*big.Rat{big.NewRat(1, 20), big.NewRat(1, 1)} {
			e, err := NewLeaderEligibility(1, 1, f, mode)
			require.NoError(t, err)
			got, err := e.CheckCertifiedNatural(nil)
			require.False(t, got)
			require.EqualError(t, err, "certified natural is nil")
			for _, v := range []*big.Int{big.NewInt(-1), new(big.Int).Add(&e.state.maximum, big.NewInt(1))} {
				got, err = e.CheckCertifiedNatural(v)
				require.False(t, got)
				require.EqualError(
					t,
					err,
					"certified natural is outside [0, M]",
				)
			}
			for _, size := range []int{0, 1, 32, 63, 65, 128} {
				got, err = e.CheckVRFOutput(make([]byte, size))
				require.False(t, got)
				require.EqualError(
					t,
					err,
					fmt.Sprintf("VRF output: expected 64 bytes, got %d", size),
				)
			}
		}
	}
}

func TestLeaderEligibilityInputAndWrapperIsolation(t *testing.T) {
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		f := big.NewRat(1, 2)
		before := f.RatString()
		e, err := NewLeaderEligibility(1, 1, f, mode)
		require.NoError(t, err)
		require.Equal(t, before, f.RatString())
		other, err := NewLeaderEligibility(1, 1, big.NewRat(1, 2), mode)
		require.NoError(t, err)
		require.NotSame(t, e, other)
		f.SetInt64(0)
		v := new(big.Int)
		got, err := e.CheckCertifiedNatural(v)
		require.NoError(t, err)
		require.True(t, got)
		require.Zero(t, v.Sign())
		v.Set(&e.state.maximum)
		got, err = e.CheckCertifiedNatural(v)
		require.NoError(t, err)
		require.False(t, got)
		*e = LeaderEligibility{}
		cached, err := NewLeaderEligibility(1, 1, big.NewRat(1, 2), mode)
		require.NoError(t, err)
		for _, survivor := range []*LeaderEligibility{other, cached} {
			got, err := survivor.CheckCertifiedNatural(big.NewInt(0))
			require.NoError(t, err)
			require.True(t, got)
			got, err = survivor.CheckCertifiedNatural(&survivor.state.maximum)
			require.NoError(t, err)
			require.False(t, got)
		}
	}
}

func TestLeaderEligibilityRawOutputUtilities(t *testing.T) {
	output := make([]byte, 64)
	for i := range output {
		output[i] = byte(i)
	}
	before := append([]byte(nil), output...)
	preimage := append([]byte{'L'}, output...)
	digest := blake2b.Sum256(preimage)
	require.Equal(t, digest[:], VrfLeaderValue(output))
	want := new(big.Int)
	for _, b := range output {
		want.Lsh(want, 8)
		want.Add(want, big.NewInt(int64(b)))
	}
	require.Equal(t, want.String(), VRFOutputToInt(output).String())
	// Every canonical 64-byte output is valid input even if it is not a proof.
	// No raw-output assertion below claims cryptographic proof validity.
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		e, err := NewLeaderEligibility(1, 1, big.NewRat(1, 1), mode)
		require.NoError(t, err)
		got, err := e.CheckVRFOutput(output)
		require.NoError(t, err)
		require.True(t, got)
	}
	require.Equal(t, before, output)
}
