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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// refOpCertCounterOK transcribes the reference counter rule. Praos
// (ouroboros-consensus ec239d57, Protocol/PolyPraos.hs 1062-1069): m <= n and
// n <= m + 1. TPraos (cardano-ledger 1210d37f,
// libs/cardano-protocol-tpraos/.../Rules/OCert.hs 101-107): m <= n only.
//
// The reference computes m + 1 in Word64, which wraps when m is the maximum;
// the table leaves that counter out rather than reproduce the wrap.
func refOpCertCounterOK(mode ConsensusMode, m, n uint64) bool {
	if n < m {
		return false
	}
	return mode != ConsensusModeCPraos || n <= m+1
}

// TestOpCertCounterMatchesReferenceTable checks validateOpCertSequence
// against the reference for every recorded counter and issue number on a
// grid around zero, small values and the top of the range, in both modes.
func TestOpCertCounterMatchesReferenceTable(t *testing.T) {
	counters := []uint64{0, 1, 2, 7, math.MaxUint64 - 2, math.MaxUint64 - 1}
	for _, mode := range []ConsensusMode{
		ConsensusModeTPraos,
		ConsensusModeCPraos,
	} {
		validator := NewHeaderValidatorWithMode(testNetworkConfig(), mode)
		for _, m := range counters {
			var issueNumbers []uint64
			for _, below := range []uint64{2, 1} {
				if m >= below {
					issueNumbers = append(issueNumbers, m-below)
				}
			}
			for _, above := range []uint64{0, 1, 2, 3} {
				if m <= math.MaxUint64-above {
					issueNumbers = append(issueNumbers, m+above)
				}
			}
			issueNumbers = append(issueNumbers, 0, math.MaxUint64)
			for _, n := range issueNumbers {
				previous := m
				got, err := validator.validateOpCertSequence(
					&ValidateHeaderInput{
						OpCertSequenceNumber: n,
						OpCertCounterState:   &previous,
					},
				)
				want := refOpCertCounterOK(mode, m, n)
				require.Equal(
					t, want, err == nil,
					"mode %d counter %d issue %d: %v", mode, m, n, err,
				)
				if want {
					// The reference records n as the issuer's new counter.
					require.Equal(t, n, got)
				}
			}
		}
	}
}

// refKESPeriodOK transcribes the reference KES window (PolyPraos.hs
// 1050-1051; OCert.hs 86-90): with kp = slot / slotsPerKESPeriod and c0 the
// certificate's start period, c0 <= kp and kp < c0 + maxKESEvo.
func refKESPeriodOK(slot, slotsPerKESPeriod, c0, maxKESEvo uint64) bool {
	kp := slot / slotsPerKESPeriod
	return c0 <= kp && kp-c0 < maxKESEvo
}

// TestKESPeriodMatchesReferenceTable checks validateKESPeriod against the
// reference window on both sides of each bound.
func TestKESPeriodMatchesReferenceTable(t *testing.T) {
	for _, slotsPerKESPeriod := range []uint64{1, 3, 129600} {
		for _, maxKESEvo := range []uint64{0, 1, 2, 62} {
			config := testNetworkConfig()
			config.SlotsPerKESPeriod = slotsPerKESPeriod
			config.MaxKESEvolutions = maxKESEvo
			validator := NewHeaderValidator(config)
			for _, c0 := range []uint64{0, 1, 2, 5} {
				for period := uint64(0); period <= c0+maxKESEvo+1; period++ {
					for _, offset := range []uint64{0, slotsPerKESPeriod - 1} {
						slot := period*slotsPerKESPeriod + offset
						err := validator.validateKESPeriod(&ValidateHeaderInput{
							Slot:            slot,
							OpCertKesPeriod: c0,
						})
						require.Equal(
							t,
							refKESPeriodOK(
								slot,
								slotsPerKESPeriod,
								c0,
								maxKESEvo,
							),
							err == nil,
							"slot %d period length %d start %d max %d: %v",
							slot,
							slotsPerKESPeriod,
							c0,
							maxKESEvo,
							err,
						)
					}
				}
			}
		}
	}
}
