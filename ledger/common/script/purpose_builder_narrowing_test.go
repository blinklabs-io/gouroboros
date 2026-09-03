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

package script

import (
	"testing"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// TestScriptPurposeBuilderRejectsWireIndexAboveMaxInt32 covers the same
// narrowing class as TestBuildScriptPurposeRejectsWireIndexAboveMaxInt32, for
// the unexported builder used when assembling a Plutus script context. The
// index is representable on the wire (uint32) but negative once narrowed to a
// 32-bit platform int, which would bypass the bounds check and panic on the
// following slice index.
func TestScriptPurposeBuilderRejectsWireIndexAboveMaxInt32(t *testing.T) {
	var emptyMint lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	toPurpose := scriptPurposeBuilder(
		nil,
		nil,
		emptyMint,
		nil,
		nil,
		nil,
		nil,
		map[lcommon.Blake2b256]*lcommon.Datum{},
	)
	for _, testDef := range []struct {
		name string
		tag  lcommon.RedeemerTag
	}{
		{name: "spend", tag: lcommon.RedeemerTagSpend},
		{name: "mint", tag: lcommon.RedeemerTagMint},
		{name: "cert", tag: lcommon.RedeemerTagCert},
		{name: "reward", tag: lcommon.RedeemerTagReward},
		{name: "voting", tag: lcommon.RedeemerTagVoting},
		{name: "proposing", tag: lcommon.RedeemerTagProposing},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			redeemerKey := lcommon.RedeemerKey{
				Tag:   testDef.tag,
				Index: uint32(1) << 31,
			}
			var purpose ScriptPurpose
			var err error
			require.NotPanics(t, func() {
				purpose, err = toPurpose(redeemerKey)
			})
			require.Nil(t, purpose)
			require.ErrorAs(t, err, &UnmatchedRedeemerError{})
		})
	}
}
