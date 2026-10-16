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
	"fmt"
	"math/big"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

var leaderEligibilityBenchmarkModes = []struct {
	name string
	mode ConsensusMode
}{
	{"CPraos", ConsensusModeCPraos},
	{"TPraos", ConsensusModeTPraos},
}

func BenchmarkLeaderEligibilityPrepare(b *testing.B) {
	for _, mode := range leaderEligibilityBenchmarkModes {
		b.Run(mode.name, func(b *testing.B) {
			f := big.NewRat(1, 17)
			require.NoError(b, validateConsensusMode(mode.mode))
			require.NoError(b, validateActiveSlotCoefficient(f))
			key := leaderEligibilityCacheKey{
				poolStake: 500000001, totalStake: 1000000003,
				numerator: f.Num().
					Uint64(),
				denominator: f.Denom().Uint64(), mode: mode.mode,
			}
			b.ReportAllocs()
			b.ResetTimer()
			var state *leaderEligibilityState
			for range b.N {
				var err error
				state, err = prepareLeaderEligibility(key)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(state)
		})
	}
}

func BenchmarkLeaderEligibilityPrepareCached(b *testing.B) {
	for _, mode := range leaderEligibilityBenchmarkModes {
		b.Run(mode.name, func(b *testing.B) {
			f := big.NewRat(1, 17)
			_, err := NewLeaderEligibility(500000001, 1000000003, f, mode.mode)
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			var e *LeaderEligibility
			for range b.N {
				e, err = NewLeaderEligibility(
					500000001, 1000000003, f, mode.mode,
				)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(e)
		})
	}
}

func BenchmarkLeaderEligibilityCheckVRFOutput(b *testing.B) {
	for _, mode := range leaderEligibilityBenchmarkModes {
		b.Run(mode.name, func(b *testing.B) {
			e, err := NewLeaderEligibility(
				500000001,
				1000000003,
				big.NewRat(1, 17),
				mode.mode,
			)
			require.NoError(b, err)
			outputs := []struct {
				name  string
				bytes []byte
			}{
				{"ZeroOutput", make([]byte, 64)},
				{"PatternOutput", []byte(strings.Repeat("\x5a", 64))},
			}
			for _, output := range outputs {
				b.Run(output.name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					var eligible bool
					for range b.N {
						var err error
						eligible, err = e.CheckVRFOutput(output.bytes)
						if err != nil {
							b.Fatal(err)
						}
					}
					runtime.KeepAlive(eligible)
				})
			}
		})
	}
	// Only TPraos maps a certified natural directly to raw VRF bytes. Do not
	// mislabel a CPraos natural as a raw output or search for a hash preimage.
	b.Run("TPraos/PinnedOneTwentieth", func(b *testing.B) {
		e, v, want := leaderEligibilityBenchmarkBoundary(
			b,
			ConsensusModeTPraos,
			"20",
		)
		output := v.FillBytes(make([]byte, 64))
		got, err := e.CheckVRFOutput(output)
		require.NoError(b, err)
		require.Equal(b, want, got)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			got, err = e.CheckVRFOutput(output)
			if err != nil {
				b.Fatal(err)
			}
		}
		runtime.KeepAlive(got)
	})
}

func BenchmarkLeaderEligibilityCheckCertifiedNatural(b *testing.B) {
	for _, mode := range leaderEligibilityBenchmarkModes {
		b.Run(mode.name+"/Ordinary", func(b *testing.B) {
			e, err := NewLeaderEligibility(
				500000001,
				1000000003,
				big.NewRat(1, 17),
				mode.mode,
			)
			require.NoError(b, err)
			v := new(big.Int)
			b.ReportAllocs()
			b.ResetTimer()
			var eligible bool
			for range b.N {
				eligible, err = e.CheckCertifiedNatural(v)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(eligible)
		})
	}
	for _, boundary := range []struct {
		name        string
		mode        ConsensusMode
		denominator string
	}{
		{"TPraos/PinnedOneTwentieth", ConsensusModeTPraos, "20"},
		{"CPraos/PinnedOneTwentieth", ConsensusModeCPraos, "20"},
		{"CPraos/PinnedOneHundredThousandth", ConsensusModeCPraos, "100000"},
	} {
		b.Run(boundary.name, func(b *testing.B) {
			e, v, want := leaderEligibilityBenchmarkBoundary(
				b,
				boundary.mode,
				boundary.denominator,
			)
			got, err := e.CheckCertifiedNatural(v)
			require.NoError(b, err)
			require.Equal(b, want, got)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				got, err = e.CheckCertifiedNatural(v)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(got)
		})
	}
}

// The oracle's MAX_REACHED input is a comparator workload, not a valid
// prepared leader input: its x exceeds the coefficient domain. Benchmark the
// real comparator directly rather than manufacture an invalid prepared state.
func BenchmarkLeaderEligibilityComparatorPinned(b *testing.B) {
	for index, row := range leaderEligibilityBenchmarkRows(b, "pallas/node-comparisons.tsv") {
		require.Len(b, row, 4)
		q := leaderEligibilityBenchmarkInt(b, row[0])
		x := leaderEligibilityBenchmarkInt(b, row[1])
		iterations, err := strconv.Atoi(row[3])
		require.NoError(b, err)
		var want e34Comparison
		switch row[2] {
		case "ABOVE":
			want = e34Above
		case "BELOW":
			want = e34Below
		case "MAX_REACHED":
			want = e34MaxReached
		default:
			b.Fatalf("unknown comparison class %q", row[2])
		}
		b.Run(
			fmt.Sprintf("%02d_%s_%dIterations", index, row[2], iterations),
			func(b *testing.B) {
				got, _, count, err := e34Compare(q, x)
				require.NoError(b, err)
				require.Equal(b, want, got)
				require.Equal(b, iterations, count)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					got, _, _, err = e34Compare(q, x)
					if err != nil {
						b.Fatal(err)
					}
				}
				runtime.KeepAlive(got)
			},
		)
	}
}

func leaderEligibilityBenchmarkBoundary(
	b *testing.B,
	mode ConsensusMode,
	denominator string,
) (*LeaderEligibility, *big.Int, bool) {
	b.Helper()
	width, pool, total := "32", "1", "3"
	natural := "1962956998846591905535085628410160294139768371758753777856225283307809972526"
	if denominator == "100000" {
		natural = "385974917376970995734921210494643580273092695162968067819510714679869089"
	}
	if mode == ConsensusModeTPraos {
		width = "64"
		natural = "227294891979458954018742436978024956463215135819825818702980970856110236242914983087163605023587875563114626563440376829246817166990888313684076947226558"
	}
	for _, row := range leaderEligibilityBenchmarkRows(b, "pallas/node-vectors.tsv") {
		require.Len(b, row, 11)
		if row[0] != width || row[1] != natural || row[2] != pool ||
			row[3] != total ||
			row[4] != "1" ||
			row[5] != denominator {
			continue
		}
		poolStake := leaderEligibilityBenchmarkInt(b, row[2])
		totalStake := leaderEligibilityBenchmarkInt(b, row[3])
		require.True(b, poolStake.IsUint64())
		require.True(b, totalStake.IsUint64())
		f := new(
			big.Rat,
		).SetFrac(leaderEligibilityBenchmarkInt(b, row[4]), leaderEligibilityBenchmarkInt(b, row[5]))
		e, err := NewLeaderEligibility(
			poolStake.Uint64(),
			totalStake.Uint64(),
			f,
			mode,
		)
		require.NoError(b, err)
		require.Contains(b, []string{"true", "false"}, row[6])
		return e, leaderEligibilityBenchmarkInt(b, row[1]), row[6] == "true"
	}
	b.Fatalf(
		"missing pinned boundary: width=%s v=%s f=1/%s",
		width,
		natural,
		denominator,
	)
	return nil, nil, false
}

func leaderEligibilityBenchmarkInt(b *testing.B, text string) *big.Int {
	b.Helper()
	digits := strings.TrimPrefix(text, "-")
	require.NotEmpty(b, digits)
	for _, digit := range digits {
		require.True(b, digit >= '0' && digit <= '9', text)
	}
	n, ok := new(big.Int).SetString(text, 10)
	require.True(b, ok, text)
	return n
}

// Benchmarks enforce the same embedded-corpus integrity contract as tests.
// All setup and checksum work happens before the measured subbenchmark loop.
func leaderEligibilityBenchmarkRows(b *testing.B, name string) [][]string {
	b.Helper()
	fs := fixtures.EmbeddedFixtures()
	manifest, err := fs.ReadFile("upstream/e34/SHA256SUMS")
	require.NoError(b, err)
	require.True(b, strings.HasSuffix(string(manifest), "\n"))
	checksums := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		require.GreaterOrEqual(b, len(line), 67)
		require.Equal(b, "  ", line[64:66])
		digest, err := hex.DecodeString(line[:64])
		require.NoError(b, err)
		require.Len(b, digest, sha256.Size)
		path := line[66:]
		require.NotEmpty(b, path)
		_, duplicate := checksums[path]
		require.False(b, duplicate, path)
		checksums[path] = line[:64]
		data, err := fs.ReadFile("upstream/e34/" + path)
		require.NoError(b, err)
		sum := sha256.Sum256(data)
		require.Equal(b, line[:64], hex.EncodeToString(sum[:]), path)
	}
	_, exists := checksums[name]
	require.True(b, exists, name)
	data, err := fs.ReadFile("upstream/e34/" + name)
	require.NoError(b, err)
	require.NotEmpty(b, data)
	require.True(b, strings.HasSuffix(string(data), "\n"))
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	rows := make([][]string, len(lines))
	for i, line := range lines {
		require.NotEmpty(b, line)
		rows[i] = strings.Split(line, "\t")
	}
	return rows
}
