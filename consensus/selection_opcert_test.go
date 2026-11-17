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

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestCompareOperationalCertificates(t *testing.T) {
	tip := func(
		issuer byte,
		slot, height, issue uint64,
		vrf byte,
	) *SimpleChainTip {
		result := NewSimpleChainTip(slot, height, []byte{vrf})
		result.SetIssuer(common.IssuerVkey{issuer}, issue)
		return result
	}
	for _, tc := range []struct {
		name string
		a, b *SimpleChainTip
		want int
	}{
		{
			name: "new hot key wins with identical VRF",
			a:    tip(1, 1000, 100, 8, 2), b: tip(1, 1000, 100, 7, 2), want: 1,
		},
		{
			name: "issue number precedes VRF comparison",
			a:    tip(1, 1000, 100, 8, 2), b: tip(1, 1000, 100, 7, 1), want: 1,
		},
		{
			name: "different issuers ignore issue numbers",
			a:    tip(1, 1000, 100, 8, 2), b: tip(2, 1000, 100, 7, 1), want: -1,
		},
		{
			name: "different slots ignore issue numbers",
			a:    tip(1, 1000, 100, 8, 2), b: tip(1, 1001, 100, 7, 1), want: -1,
		},
		{
			name: "block height takes precedence",
			a:    tip(1, 1000, 99, 8, 1), b: tip(1, 1000, 100, 7, 2), want: -1,
		},
		{
			name: "equal issue numbers and VRFs tie",
			a:    tip(1, 1000, 100, 8, 2), b: tip(1, 1000, 100, 8, 2), want: 0,
		},
		{
			name: "equal issue numbers fall through to VRF",
			a:    tip(1, 1000, 100, 8, 2), b: tip(1, 1000, 100, 8, 1), want: -1,
		},
		{
			name: "maximum issue number does not overflow",
			a:    tip(1, 1000, 100, math.MaxUint64, 2),
			b:    tip(1, 1000, 100, 0, 2), want: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selector := NewPraosChainSelector(2160)
			require.Equal(t, tc.want, selector.Compare(tc.a, tc.b))
			require.Equal(t, -tc.want, selector.Compare(tc.b, tc.a))
		})
	}
}

func TestPreferredOperationalCertificate(t *testing.T) {
	older := NewWindowedChainTip(1000, 100, []byte{2}, []uint64{2, 3})
	newer := NewWindowedChainTip(1000, 100, []byte{2}, []uint64{2, 3})
	issuer := common.IssuerVkey{1}
	older.SetIssuer(issuer, 7)
	newer.SetIssuer(issuer, 8)
	selector := NewPraosChainSelectorWithWindow(2, 10)
	for _, tips := range [][]ChainTip{{older, newer}, {newer, older}} {
		require.Same(t, newer, selector.Preferred(tips))
		require.Same(t, newer, selector.PreferredWithDensity(
			tips, ForkPoint{Slot: 990, BlockNumber: 99}, 100,
		))
		require.Same(t, newer, selector.PreferredWithDensity(
			tips, ForkPoint{Slot: 0, BlockNumber: 0}, 100,
		))
	}
	older.blockSlots = append(older.blockSlots, 4)
	require.Same(t, older, selector.PreferredWithDensity(
		[]ChainTip{older, newer}, ForkPoint{}, 100,
	))
}
