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

import "github.com/blinklabs-io/gouroboros/ledger/common"

const conwayVRFTiebreakerMaxDistance uint64 = 5

// PreferCandidate reports whether candidate should replace current. A tie
// keeps the current chain. A nil current tip permits initial selection.
// ProtocolMajor selects the era's VRF switching rule.
func (p *PraosChainSelector) PreferCandidate(current, candidate ChainTip) bool {
	if current != nil && candidate != nil &&
		p.ProtocolMajor >= common.ProtocolVersionConway &&
		current.BlockNumber() == candidate.BlockNumber() {
		earlier, later := current.Slot(), candidate.Slot()
		if earlier > later {
			earlier, later = later, earlier
		}
		if later-earlier > conwayVRFTiebreakerMaxDistance {
			return false
		}
	}
	return p.Compare(candidate, current) > 0
}

// PreferCandidateWithDensity applies the switching rule after the same
// shallow/deep fork routing as CompareWithDensity. Unequal deep-fork density
// takes precedence. Equal density falls back to PreferCandidate.
func (p *PraosChainSelector) PreferCandidateWithDensity(
	current, candidate ChainTip,
	fork ForkPoint,
	tipBlockNumber uint64,
) bool {
	if current == nil || candidate == nil {
		return p.Compare(candidate, current) > 0
	}
	if p.IsDeepFork(fork, tipBlockNumber) {
		if result := p.compareDensity(candidate, current, fork); result != 0 {
			return result > 0
		}
	}
	return p.PreferCandidate(current, candidate)
}

// PreferredCandidate returns the highest-ranked candidate that can replace
// current, or current if none can. Set ProtocolMajor to the active protocol
// version. A nil current tip permits initial selection.
func (p *PraosChainSelector) PreferredCandidate(
	current ChainTip,
	candidates []ChainTip,
) ChainTip {
	return p.selectPreferredCandidate(
		current, candidates, p.PreferCandidate, p.Compare,
	)
}

// PreferredCandidateWithDensity selects a replacement using the same density
// routing and fork context as CompareWithDensity. The switching restriction
// applies whenever the comparison falls back to ordinary Praos preference.
func (p *PraosChainSelector) PreferredCandidateWithDensity(
	current ChainTip,
	candidates []ChainTip,
	fork ForkPoint,
	tipBlockNumber uint64,
) ChainTip {
	return p.selectPreferredCandidate(
		current,
		candidates,
		func(ours, candidate ChainTip) bool {
			return p.PreferCandidateWithDensity(
				ours, candidate, fork, tipBlockNumber,
			)
		},
		func(a, b ChainTip) int {
			return p.CompareWithDensity(a, b, fork, tipBlockNumber)
		},
	)
}

func (p *PraosChainSelector) selectPreferredCandidate(
	current ChainTip,
	candidates []ChainTip,
	prefer func(current, candidate ChainTip) bool,
	compare func(a, b ChainTip) int,
) ChainTip {
	best := current
	for _, candidate := range candidates {
		// Restricted preference is not transitive. Test against the original
		// selection, not the best candidate found so far, before ranking.
		if prefer(current, candidate) && compare(candidate, best) > 0 {
			best = candidate
		}
	}
	return best
}
