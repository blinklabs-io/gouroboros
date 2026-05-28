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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestBuildScriptPurposeGuardingRedeemerDoesNotPanic(t *testing.T) {
	var purpose script.ScriptPurpose
	require.NotPanics(t, func() {
		purpose = script.BuildScriptPurpose(
			common.RedeemerKey{Tag: common.RedeemerTagGuarding},
			nil,
			nil,
			common.MultiAsset[common.MultiAssetTypeMint]{},
			nil,
			nil,
			nil,
			nil,
			nil,
		)
	})
	require.Nil(t, purpose)
}

func TestBuildScriptPurposeSpendingNilOutputDoesNotPanic(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000001",
		0,
	)
	resolvedInputs := map[string]common.Utxo{
		input.String(): {
			Id:     input,
			Output: nil,
		},
	}
	var purpose script.ScriptPurpose
	require.NotPanics(t, func() {
		purpose = script.BuildScriptPurpose(
			common.RedeemerKey{Tag: common.RedeemerTagSpend},
			resolvedInputs,
			[]common.TransactionInput{input},
			common.MultiAsset[common.MultiAssetTypeMint]{},
			nil,
			nil,
			nil,
			nil,
			nil,
		)
	})
	require.Nil(t, purpose)
}
