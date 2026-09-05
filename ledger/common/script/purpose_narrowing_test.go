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

package script_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/stretchr/testify/require"
)

// wireRedeemerIndexAboveMaxInt32 is a redeemer index that a transaction may
// legally carry on the wire (redeemer_index is a uint32) but that becomes
// negative when narrowed to a 32-bit platform int. Every bounds check that
// guards a slice index must reject it against an empty collection on both
// 32-bit and 64-bit builds.
const wireRedeemerIndexAboveMaxInt32 = uint32(1) << 31

// TestBuildScriptPurposeRejectsWireIndexAboveMaxInt32 covers every tag branch
// in BuildScriptPurpose that indexes a collection with the redeemer index. On a
// 32-bit build a narrowing bounds check lets the index through and the
// subsequent slice index panics.
func TestBuildScriptPurposeRejectsWireIndexAboveMaxInt32(t *testing.T) {
	var emptyMint common.MultiAsset[common.MultiAssetTypeMint]
	for _, testDef := range []struct {
		name string
		tag  common.RedeemerTag
	}{
		{name: "spend", tag: common.RedeemerTagSpend},
		{name: "mint", tag: common.RedeemerTagMint},
		{name: "cert", tag: common.RedeemerTagCert},
		{name: "reward", tag: common.RedeemerTagReward},
		{name: "voting", tag: common.RedeemerTagVoting},
		{name: "proposing", tag: common.RedeemerTagProposing},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			redeemerKey := common.RedeemerKey{
				Tag:   testDef.tag,
				Index: wireRedeemerIndexAboveMaxInt32,
			}
			var purpose script.ScriptPurpose
			var err error
			require.NotPanics(t, func() {
				purpose, err = script.BuildScriptPurpose(
					redeemerKey,
					map[string]common.Utxo{},
					nil,
					emptyMint,
					nil,
					map[*common.Address]*big.Int{},
					common.VotingProcedures{},
					nil,
					map[common.Blake2b256]*common.Datum{},
				)
			})
			require.Nil(t, purpose)
			require.ErrorAs(t, err, &script.UnmatchedRedeemerError{})
		})
	}
}
