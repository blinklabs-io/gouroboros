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
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// The tests in this file compare the selector with a transcription of the
// reference chain order, from ouroboros-consensus commit ec239d57:
//
//   - Protocol/Abstract.hs 392-398: SelectView's preferCandidate compares
//     block numbers first and consults the tiebreaker only at equal height.
//   - Protocol/Praos/Common.hs 138-180: comparePraos. The issue number is
//     compared only for tips with the same slot and issuer, the higher one
//     winning; otherwise the lower VRF wins when the VRF comparison is armed.
//   - Shelley/Ledger/Config.hs 86-93: the VRF comparison is unrestricted
//     before Conway and restricted to tips at most 5 slots apart from it.
//   - Storage/ChainDB/Impl/ChainSel.hs 1260-1266: candidates that the current
//     chain prefers are sorted by the unrestricted order, best first, with a
//     stable sort.

const refRestrictedMaxDist = 5

type refTip struct {
	blockNo uint64
	slot    uint64
	issuer  byte
	issueNo uint64
	vrf     []byte
}

func (r refTip) chainTip() *SimpleChainTip {
	tip := NewSimpleChainTip(r.slot, r.blockNo, r.vrf)
	tip.SetIssuer(common.IssuerVkey{r.issuer}, r.issueNo)
	return tip
}

func (r refTip) String() string {
	return fmt.Sprintf(
		"{block %d slot %d issuer %d issue %d vrf %x}",
		r.blockNo, r.slot, r.issuer, r.issueNo, r.vrf,
	)
}

func refSlotDist(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return b - a
}

// refPreferCandidate is SelectView's preferCandidate over comparePraos.
func refPreferCandidate(restricted bool, ours, cand refTip) bool {
	if ours.blockNo != cand.blockNo {
		return cand.blockNo > ours.blockNo
	}
	issueNoArmed := ours.slot == cand.slot && ours.issuer == cand.issuer
	vrfArmed := !restricted ||
		refSlotDist(ours.slot, cand.slot) <= refRestrictedMaxDist
	switch {
	case issueNoArmed && ours.issueNo < cand.issueNo:
		return true
	case issueNoArmed && ours.issueNo > cand.issueNo:
		return false
	case vrfArmed:
		// compare `on` Down . vrf: switching needs ours' VRF to be greater.
		return bytes.Compare(ours.vrf, cand.vrf) > 0
	}
	return false
}

// refCompare is the Ord instance (the unrestricted order) in Compare's sign
// convention: positive when a ranks above b.
func refCompare(a, b refTip) int {
	switch {
	case refPreferCandidate(false, b, a):
		return 1
	case refPreferCandidate(false, a, b):
		return -1
	}
	return 0
}

// refRestricted maps a protocol major version to the era's tiebreaker.
// Majors 7 and 8 are Babbage; 9 and later are Conway and its successors.
func refRestricted(major uint) bool {
	return major >= 9
}

// selectionModelTips is every tip over a grid that reaches each comparison
// boundary: equal and adjacent heights, the same slot, a distance of exactly
// five and of six, both issuers, both issue numbers, and both VRF orders.
func selectionModelTips() []refTip {
	var tips []refTip
	for _, blockNo := range []uint64{100, 101} {
		for _, slot := range []uint64{1000, 1005, 1006} {
			for _, issuer := range []byte{1, 2} {
				for _, issueNo := range []uint64{7, 8} {
					for _, vrf := range []byte{1, 2} {
						tips = append(tips, refTip{
							blockNo: blockNo,
							slot:    slot,
							issuer:  issuer,
							issueNo: issueNo,
							vrf:     []byte{vrf},
						})
					}
				}
			}
		}
	}
	return tips
}

// TestPreferCandidateMatchesReferenceTable checks PreferCandidate and Compare
// against the reference for every ordered pair of grid tips, at a Babbage, a
// Conway and a post-Conway protocol version.
func TestPreferCandidateMatchesReferenceTable(t *testing.T) {
	tips := selectionModelTips()
	for _, major := range []uint{8, 9, 10} {
		selector := NewPraosChainSelector(2160)
		selector.ProtocolMajor = major
		restricted := refRestricted(major)
		for _, ours := range tips {
			for _, cand := range tips {
				require.Equal(
					t,
					refPreferCandidate(restricted, ours, cand),
					selector.PreferCandidate(ours.chainTip(), cand.chainTip()),
					"PV%d PreferCandidate(%s, %s)", major, ours, cand,
				)
				require.Equal(
					t,
					refCompare(cand, ours),
					selector.Compare(cand.chainTip(), ours.chainTip()),
					"Compare(%s, %s)", cand, ours,
				)
			}
		}
	}
}

// TestPreferCandidateTableReachesEveryCase guards the table above against
// becoming vacuous: the grid must contain a pair decided by each rule.
func TestPreferCandidateTableReachesEveryCase(t *testing.T) {
	tips := selectionModelTips()
	seen := map[string]bool{}
	for _, ours := range tips {
		for _, cand := range tips {
			switch {
			case ours.blockNo != cand.blockNo:
				seen["height"] = true
			case ours.slot == cand.slot && ours.issuer == cand.issuer &&
				ours.issueNo != cand.issueNo:
				if !bytes.Equal(ours.vrf, cand.vrf) &&
					refPreferCandidate(false, ours, cand) &&
					bytes.Compare(cand.vrf, ours.vrf) > 0 {
					seen["issue number beats a worse VRF"] = true
				}
			case ours.issuer == cand.issuer && ours.slot != cand.slot &&
				ours.issueNo < cand.issueNo &&
				!refPreferCandidate(false, ours, cand):
				seen["issue number ignored across slots"] = true
			case ours.slot == cand.slot && ours.issuer != cand.issuer &&
				ours.issueNo < cand.issueNo &&
				!refPreferCandidate(false, ours, cand):
				seen["issue number ignored across issuers"] = true
			case refSlotDist(ours.slot, cand.slot) == refRestrictedMaxDist &&
				refPreferCandidate(true, ours, cand):
				seen["VRF armed at distance five"] = true
			case refSlotDist(ours.slot, cand.slot) == refRestrictedMaxDist+1 &&
				refPreferCandidate(false, ours, cand) &&
				!refPreferCandidate(true, ours, cand):
				seen["VRF disarmed at distance six"] = true
			}
		}
	}
	for _, name := range []string{
		"height",
		"issue number beats a worse VRF",
		"issue number ignored across slots",
		"issue number ignored across issuers",
		"VRF armed at distance five",
		"VRF disarmed at distance six",
	} {
		require.True(t, seen[name], "grid never reaches %q", name)
	}
}

// refPreferredCandidate is the reference's choice among candidates: keep
// those the current chain prefers, sort the rest best first with a stable
// sort on the unrestricted order, and take the first.
func refPreferredCandidate(
	restricted bool,
	current *refTip,
	candidates []refTip,
) *refTip {
	var eligible []refTip
	for _, cand := range candidates {
		if current == nil || refPreferCandidate(restricted, *current, cand) {
			eligible = append(eligible, cand)
		}
	}
	if len(eligible) == 0 {
		return current
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return refCompare(eligible[i], eligible[j]) > 0
	})
	return &eligible[0]
}

// TestPreferredCandidateMatchesReferenceTable checks PreferredCandidate
// against the reference's sort-and-pick for every ordered list of up to three
// distinct candidates. Here the VRF is a function of issuer and slot, as it is
// on a real chain; with free VRFs the unrestricted order has cycles and no
// sort would be well defined.
func TestPreferredCandidateMatchesReferenceTable(t *testing.T) {
	tips := make([]refTip, 0, 24)
	for _, blockNo := range []uint64{100, 101} {
		for _, slot := range []uint64{1000, 1003, 1006} {
			for _, issuer := range []byte{1, 2} {
				for _, issueNo := range []uint64{7, 8} {
					tips = append(tips, refTip{
						blockNo: blockNo,
						slot:    slot,
						issuer:  issuer,
						issueNo: issueNo,
						// Injective in (issuer, slot), and not monotone in slot.
						vrf: []byte{byte((slot*7 + uint64(issuer)*5) % 31)},
					})
				}
			}
		}
	}
	currents := []*refTip{nil, &tips[0], &tips[5], &tips[13]}
	for _, major := range []uint{8, 9} {
		selector := NewPraosChainSelector(2160)
		selector.ProtocolMajor = major
		restricted := refRestricted(major)
		for _, current := range currents {
			var currentTip ChainTip
			if current != nil {
				currentTip = current.chainTip()
			}
			for i := range tips {
				for j := range tips {
					for k := range tips {
						if i == j || j == k || i == k {
							continue
						}
						list := []refTip{tips[i], tips[j], tips[k]}
						chainTips := make([]ChainTip, len(list))
						for n, tip := range list {
							chainTips[n] = tip.chainTip()
						}
						want := refPreferredCandidate(restricted, current, list)
						got := selector.PreferredCandidate(
							currentTip,
							chainTips,
						)
						if want == nil {
							require.Nil(t, got)
							continue
						}
						require.NotNil(t, got)
						require.Equal(
							t,
							*want,
							refTip{
								blockNo: got.BlockNumber(),
								slot:    got.Slot(),
								issuer:  got.IssuerVkey()[0],
								issueNo: got.OpCertSequenceNumber(),
								vrf:     got.VRFOutput(),
							},
							"PV%d current %v candidates %v",
							major,
							current,
							list,
						)
					}
				}
			}
		}
	}
}
