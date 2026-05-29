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

package dijkstra

import (
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type LeiosCommitteeStakeParametersError struct {
	Reason string
}

func (e LeiosCommitteeStakeParametersError) Error() string {
	return "invalid Leios committee stake parameters: " + e.Reason
}

func validateLeiosCommitteeStakeParameters(
	committeeStakeCoverage *cbor.Rat,
	quorumStakeThreshold *cbor.Rat,
) error {
	one := big.NewRat(1, 1)
	if committeeStakeCoverage != nil {
		if committeeStakeCoverage.Rat == nil {
			return LeiosCommitteeStakeParametersError{
				Reason: "committee stake coverage is unset",
			}
		}
		if committeeStakeCoverage.Sign() <= 0 ||
			committeeStakeCoverage.Cmp(one) > 0 {
			return LeiosCommitteeStakeParametersError{
				Reason: "committee stake coverage must be in (0, 1]",
			}
		}
	}
	if quorumStakeThreshold != nil {
		if quorumStakeThreshold.Rat == nil {
			return LeiosCommitteeStakeParametersError{
				Reason: "quorum stake threshold is unset",
			}
		}
		if quorumStakeThreshold.Sign() < 0 ||
			quorumStakeThreshold.Cmp(one) > 0 {
			return LeiosCommitteeStakeParametersError{
				Reason: "quorum stake threshold must be in [0, 1]",
			}
		}
	}
	if committeeStakeCoverage != nil &&
		quorumStakeThreshold != nil &&
		quorumStakeThreshold.Cmp(committeeStakeCoverage.Rat) >= 0 {
		return LeiosCommitteeStakeParametersError{
			Reason: "quorum stake threshold must be less than committee stake coverage",
		}
	}
	return nil
}
