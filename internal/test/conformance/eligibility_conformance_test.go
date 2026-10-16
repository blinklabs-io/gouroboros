// Copyright 2026 Blink Labs Software
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

// Read only the embedded, independently generated corpus. Every manifest entry
// is checked, including the oracle provenance; missing fixtures never skip.
func eligibilityFixture(t *testing.T, name string) []byte {
	t.Helper()
	fs := fixtures.EmbeddedFixtures()
	manifest, err := fs.ReadFile("upstream/e34/SHA256SUMS")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(manifest), "\n"))
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		require.GreaterOrEqual(t, len(line), 67)
		require.Equal(t, "  ", line[64:66])
		digest, err := hex.DecodeString(line[:64])
		require.NoError(t, err)
		require.Len(t, digest, sha256.Size)
		path := line[66:]
		require.NotEmpty(t, path)
		require.False(t, seen[path])
		seen[path] = true
		data, err := fs.ReadFile("upstream/e34/" + path)
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		require.Equal(t, line[:64], hex.EncodeToString(sum[:]), path)
	}
	require.True(t, seen[name], name)
	data, err := fs.ReadFile("upstream/e34/" + name)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	return data
}

func eligibilityInteger(t *testing.T, text string) *big.Int {
	t.Helper()
	require.NotEmpty(t, text)
	for _, c := range strings.TrimPrefix(text, "-") {
		require.True(t, c >= '0' && c <= '9', text)
	}
	n, ok := new(big.Int).SetString(text, 10)
	require.True(t, ok, text)
	return n
}

func eligibilityUint(t *testing.T, text string) uint64 {
	t.Helper()
	n := eligibilityInteger(t, text)
	require.True(t, n.IsUint64())
	return n.Uint64()
}

// Replaces approximate probabilities, bit lengths and self-derived boundaries
// with every independently recorded public natural decision (including M).
func TestLeaderEligibilityNodeNaturals(t *testing.T) {
	for _, source := range []struct {
		name  string
		count int
	}{{"pallas", 648}, {"jormanager", 352}} {
		t.Run(source.name, func(t *testing.T) {
			data := string(
				eligibilityFixture(t, source.name+"/node-vectors.tsv"),
			)
			require.True(t, strings.HasSuffix(data, "\n"))
			rows := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
			require.Len(t, rows, source.count)
			for i, line := range rows {
				row := strings.Split(line, "\t")
				require.Len(t, row, 11)
				mode := consensus.ConsensusModeCPraos
				switch row[0] {
				case "32":
				case "64":
					mode = consensus.ConsensusModeTPraos
				default:
					t.Fatalf("unknown natural width %q", row[0])
				}
				f := new(
					big.Rat,
				).SetFrac(eligibilityInteger(t, row[4]), eligibilityInteger(t, row[5]))
				e, err := consensus.NewLeaderEligibility(
					eligibilityUint(t, row[2]),
					eligibilityUint(t, row[3]),
					f,
					mode,
				)
				require.NoError(t, err)
				require.Contains(t, []string{"true", "false"}, row[6])
				// Validate the entire row schema, even diagnostics not exposed by the API.
				eligibilityInteger(t, row[7])
				eligibilityInteger(t, row[9])
				if f.Cmp(big.NewRat(1, 1)) == 0 {
					require.Equal(t, "none", row[8])
					require.Equal(t, "none", row[10])
				} else {
					eligibilityInteger(t, row[8])
					eligibilityInteger(t, row[10])
				}
				got, err := e.CheckCertifiedNatural(
					eligibilityInteger(t, row[1]),
				)
				require.NoError(t, err)
				require.Equal(t, row[6] == "true", got, "row %d", i+1)
			}
		})
	}
}

type eligibilitySchedule struct {
	ActiveStake   string   `json:"activeStake"`
	AssignedSlots []uint64 `json:"assignedSlots"`
	VRFSkeyHex    string   `json:"vrfSkeyHex"`
	DDenominator  string   `json:"dDenominator"`
	DNumerator    string   `json:"dNumerator"`
	FDenominator  string   `json:"fDenominator"`
	FNumerator    string   `json:"fNumerator"`
	FirstSlot     uint64   `json:"firstSlot"`
	Mode          string   `json:"mode"`
	NonceHex      string   `json:"nonceHex"`
	PoolStake     string   `json:"poolStake"`
	SlotCount     uint64   `json:"slotCount"`
}

// Overlay selection is test-harness policy only; election APIs do not take d.
func eligibilityOverlay(offset uint64, numerator, denominator *big.Int) bool {
	ceil := func(n uint64) *big.Int {
		v := new(big.Int).Mul(new(big.Int).SetUint64(n), numerator)
		v.Add(v, new(big.Int).Sub(denominator, big.NewInt(1)))
		return v.Quo(v, denominator)
	}
	return ceil(offset).Cmp(ceil(offset+1)) < 0
}

func TestLeaderEligibilityNodeSchedules(t *testing.T) {
	var corpus struct {
		Cases    []eligibilitySchedule `json:"cases"`
		Metadata json.RawMessage       `json:"metadata"`
	}
	decoder := json.NewDecoder(
		bytes.NewReader(
			eligibilityFixture(t, "jormanager/node-schedules.json"),
		),
	)
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&corpus))
	require.ErrorIs(t, decoder.Decode(new(any)), io.EOF)
	require.Len(t, corpus.Cases, 21)
	require.NotEmpty(t, corpus.Metadata)
	for index, c := range corpus.Cases {
		t.Run(fmt.Sprintf("%02d_%s", index, c.Mode), func(t *testing.T) {
			mode := consensus.ConsensusModeCPraos
			switch c.Mode {
			case "praos":
			case "tpraos":
				mode = consensus.ConsensusModeTPraos
			default:
				t.Fatalf("unknown mode %q", c.Mode)
			}
			secret, err := hex.DecodeString(c.VRFSkeyHex)
			require.NoError(t, err)
			require.Len(t, secret, 64)
			signer, err := consensus.NewSimpleVRFSigner(secret[:32])
			require.NoError(t, err)
			require.Equal(t, secret[32:], signer.PublicKey())
			nonce, err := hex.DecodeString(c.NonceHex)
			require.NoError(t, err)
			require.Len(t, nonce, 32)
			denominator := eligibilityInteger(t, c.FDenominator)
			require.Positive(t, denominator.Sign())
			f := new(
				big.Rat,
			).SetFrac(eligibilityInteger(t, c.FNumerator), denominator)
			e, err := consensus.NewLeaderEligibility(
				eligibilityUint(t, c.PoolStake),
				eligibilityUint(t, c.ActiveStake),
				f,
				mode,
			)
			require.NoError(t, err)
			dNum, dDen := eligibilityInteger(
				t,
				c.DNumerator,
			), eligibilityInteger(
				t,
				c.DDenominator,
			)
			require.Positive(t, dDen.Sign())
			require.GreaterOrEqual(t, dNum.Sign(), 0)
			require.LessOrEqual(t, dNum.Cmp(dDen), 0)
			require.True(
				t,
				eligibilityScheduleRangeValid(c.FirstSlot, c.SlotCount),
				"schedule range exceeds int64 VRF slots",
			)
			require.NotNil(t, c.AssignedSlots)
			for i, slot := range c.AssignedSlots {
				require.GreaterOrEqual(t, slot, c.FirstSlot)
				require.Less(t, slot, c.FirstSlot+c.SlotCount)
				if i > 0 {
					require.Greater(t, slot, c.AssignedSlots[i-1])
				}
			}
			elected := make([]bool, c.SlotCount)
			// The corpus includes full epochs. Independent slot partitions
			// keep race-enabled replay within the ordinary suite deadline.
			t.Run("slots", func(t *testing.T) {
				workers := min(uint64(64), c.SlotCount)
				for worker := uint64(0); worker < workers; worker++ {
					first := worker * c.SlotCount / workers
					end := (worker + 1) * c.SlotCount / workers
					t.Run(fmt.Sprint(worker), func(t *testing.T) {
						t.Parallel()
						for offset := first; offset < end; offset++ {
							if mode == consensus.ConsensusModeTPraos &&
								dNum.Sign() != 0 &&
								eligibilityOverlay(offset, dNum, dDen) {
								continue
							}
							slot := c.FirstSlot + offset
							var input []byte
							var inputErr error
							if mode == consensus.ConsensusModeTPraos {
								input, inputErr = vrf.MkSeedTPraos(
									int64(slot), nonce, vrf.SeedL(),
								)
							} else {
								input, inputErr = vrf.MkInputVrf(
									int64(slot), nonce,
								)
							}
							require.NoError(t, inputErr)
							_, output, err := signer.Prove(input)
							require.NoError(t, err)
							eligible, err := e.CheckVRFOutput(output)
							require.NoError(t, err)
							elected[offset] = eligible
						}
					})
				}
			})
			assigned := make([]uint64, 0)
			for offset, eligible := range elected {
				if eligible {
					assigned = append(assigned, c.FirstSlot+uint64(offset))
				}
			}
			require.Equal(t, c.AssignedSlots, assigned)
		})
	}
}

func eligibilityScheduleRangeValid(first, count uint64) bool {
	return first <= math.MaxInt64 && count > 0 &&
		count <= uint64(math.MaxInt64)-first
}

func TestLeaderEligibilityScheduleRange(t *testing.T) {
	for _, tc := range []struct {
		first, count uint64
		valid        bool
	}{
		{0, 1, true},
		{math.MaxInt64 - 1, 1, true},
		{math.MaxInt64, 0, false},
		{math.MaxInt64, 1, false},
		{math.MaxInt64 + 1, 1, false},
		{math.MaxUint64, 1, false},
		{0, math.MaxUint64, false},
	} {
		require.Equal(
			t,
			tc.valid,
			eligibilityScheduleRangeValid(tc.first, tc.count),
			"first=%d count=%d",
			tc.first,
			tc.count,
		)
	}
}
