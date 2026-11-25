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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

// An update whose proposal map is empty is still an update: the reference
// PPUP rule (ppupTransitionNonEmpty in Shelley/Rules/Ppup.hs) checks its
// target epoch, and only an absent update skips the rule.
func TestClassicPPUPEmptyUpdateChecksTargetEpoch(t *testing.T) {
	state := classicPPUPTestState{epoch: 10, cutoff: 50}
	pp := &shelley.ShelleyProtocolParameters{ProtocolMajor: 2}
	emptyUpdate := func(epoch uint64) classicPPUPTestTransaction {
		return classicPPUPTestTransaction{
			Epoch:   epoch,
			Updates: map[common.Blake2b224]common.ProtocolParameterUpdate{},
		}
	}
	var epochErr common.ProtocolParameterUpdateEpochError
	require.ErrorAs(
		t,
		shelley.UtxoValidateProtocolParameterUpdates(emptyUpdate(11), 49, state, pp),
		&epochErr,
	)
	require.ErrorAs(
		t,
		shelley.UtxoValidateProtocolParameterUpdates(emptyUpdate(10), 50, state, pp),
		&epochErr,
	)
	require.NoError(
		t,
		shelley.UtxoValidateProtocolParameterUpdates(emptyUpdate(10), 49, state, pp),
	)
	require.NoError(
		t,
		shelley.UtxoValidateProtocolParameterUpdates(emptyUpdate(11), 50, state, pp),
	)
	require.NoError(
		t,
		shelley.UtxoValidateProtocolParameterUpdates(
			classicPPUPTestTransaction{Epoch: 11},
			49,
			state,
			pp,
		),
	)
}
