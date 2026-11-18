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
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestPreferCandidateVRFDistance(t *testing.T) {
	for _, major := range []uint{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13} {
		for _, tc := range []struct {
			name                       string
			currentSlot, candidateSlot uint64
			withinLimit                bool
		}{
			{"same slot", 1000, 1000, true},
			{"five slots later", 1000, 1005, true},
			{"six slots later", 1000, 1006, false},
			{"five slots earlier", 1000, 995, true},
			{"six slots earlier", 1000, 994, false},
			{"upper boundary", math.MaxUint64 - 5, math.MaxUint64, true},
			{"full range later", 0, math.MaxUint64, false},
			{"full range earlier", math.MaxUint64, 0, false},
		} {
			t.Run(fmt.Sprintf("PV%d/%s", major, tc.name), func(t *testing.T) {
				selector := NewPraosChainSelector(2160)
				selector.ProtocolMajor = major
				current := NewSimpleChainTip(tc.currentSlot, 100, []byte{2})
				candidate := NewSimpleChainTip(tc.candidateSlot, 100, []byte{1})
				want := major < common.ProtocolVersionConway || tc.withinLimit
				require.Equal(
					t,
					want,
					selector.PreferCandidate(current, candidate),
					"protocol major %d",
					major,
				)
				require.False(t, selector.PreferCandidate(candidate, current))
				require.Negative(t, selector.Compare(current, candidate))
			})
		}
	}
}

func TestPreferCandidateHeightAndTies(t *testing.T) {
	selector := NewPraosChainSelector(2160)
	selector.ProtocolMajor = common.ProtocolVersionConway
	current := NewSimpleChainTip(1000, 100, []byte{2})
	taller := NewSimpleChainTip(2000, 101, []byte{3})
	shorter := NewSimpleChainTip(2000, 99, []byte{1})
	tie := NewSimpleChainTip(1005, 100, []byte{2})
	require.True(t, selector.PreferCandidate(current, taller))
	require.False(t, selector.PreferCandidate(current, shorter))
	require.False(t, selector.PreferCandidate(current, tie))
	require.False(t, selector.PreferCandidate(current, nil))
	require.False(t, selector.PreferCandidate(nil, nil))
	require.True(t, selector.PreferCandidate(nil, current))
	require.Same(t, current, selector.PreferredCandidate(current, nil))
	require.Same(t, current, selector.PreferredCandidate(current,
		[]ChainTip{nil, tie, shorter}))
	require.Nil(t, selector.PreferredCandidate(nil, nil))
}

func TestPreferredCandidateKeepsOriginalSelection(t *testing.T) {
	selector := NewPraosChainSelector(2160)
	selector.ProtocolMajor = common.ProtocolVersionConway
	current := NewSimpleChainTip(0, 100, []byte{3})
	nearby := NewSimpleChainTip(3, 100, []byte{2})
	distant := NewSimpleChainTip(6, 100, []byte{1})
	require.True(t, selector.PreferCandidate(current, nearby))
	require.True(t, selector.PreferCandidate(nearby, distant))
	require.False(t, selector.PreferCandidate(current, distant))
	for _, candidates := range [][]ChainTip{
		{nearby, distant}, {distant, nearby}, {nil, distant, current, nearby},
	} {
		require.Same(
			t,
			nearby,
			selector.PreferredCandidate(current, candidates),
		)
		require.Same(t, distant, selector.PreferredCandidate(nil, candidates))
	}
}

func TestPreferredCandidateRanksEligibleTipsWithoutDistanceLimit(t *testing.T) {
	selector := NewPraosChainSelector(2160)
	selector.ProtocolMajor = common.ProtocolVersionConway
	current := NewSimpleChainTip(1000, 100, []byte{3})
	earlier := NewSimpleChainTip(995, 100, []byte{2})
	later := NewSimpleChainTip(1005, 100, []byte{1})
	for _, candidates := range [][]ChainTip{{earlier, later}, {later, earlier}} {
		require.Same(t, later, selector.PreferredCandidate(current, candidates))
	}
}

func TestPreferredCandidateWithDensityVRFRestriction(t *testing.T) {
	selector := NewPraosChainSelectorWithWindow(2, 10)
	selector.ProtocolMajor = common.ProtocolVersionConway
	current := NewWindowedChainTip(1000, 100, []byte{3}, []uint64{2})
	nearby := NewWindowedChainTip(1003, 100, []byte{2}, []uint64{2})
	distant := NewWindowedChainTip(1006, 100, []byte{1}, []uint64{2})
	for _, fork := range []ForkPoint{
		{Slot: 990, BlockNumber: 99},
		{},
	} {
		require.False(t, selector.PreferCandidateWithDensity(
			current, distant, fork, 100,
		))
		for _, candidates := range [][]ChainTip{
			{nearby, distant}, {distant, nearby},
		} {
			require.Same(t, nearby, selector.PreferredCandidateWithDensity(
				current, candidates, fork, 100,
			))
		}
	}
	distant.blockSlots = append(distant.blockSlots, 3)
	require.True(t, selector.PreferCandidateWithDensity(
		current, distant, ForkPoint{}, 100,
	))
	require.Same(t, distant, selector.PreferredCandidateWithDensity(
		current, []ChainTip{nearby, distant}, ForkPoint{}, 100,
	))
	require.False(t, selector.PreferCandidateWithDensity(
		current, distant, ForkPoint{Slot: 990, BlockNumber: 99}, 100,
	))
	require.True(t, selector.PreferCandidateWithDensity(nil, current,
		ForkPoint{}, 100))
	require.False(t, selector.PreferCandidateWithDensity(current, nil,
		ForkPoint{}, 100))
}
