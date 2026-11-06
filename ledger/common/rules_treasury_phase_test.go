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
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// Conway LEDGER validates the treasury value only inside its Phase2Valid
// branch, so the shared validator must keep skipping phase-2-invalid
// transactions while the any-phase variant must not.
func TestCurrentTreasuryValuePhaseHandling(t *testing.T) {
	t.Parallel()
	supplied := uint64(999)
	ls := mockledger.NewLedgerStateBuilder().WithTreasuryAmount(1000).Build()
	pp := &conway.ConwayProtocolParameters{}
	newTx := func(isValid bool) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxCurrentTreasuryValue: supplied,
			},
			TxIsValid: isValid,
		}
	}

	require.ErrorAs(
		t,
		common.UtxoValidateCurrentTreasuryValue(newTx(true), 0, ls, pp),
		&common.CurrentTreasuryValueMismatchError{},
	)
	require.NoError(
		t,
		common.UtxoValidateCurrentTreasuryValue(newTx(false), 0, ls, pp),
	)
	for _, isValid := range []bool{true, false} {
		require.ErrorAs(
			t,
			common.UtxoValidateCurrentTreasuryValueAnyPhase(
				newTx(isValid),
				0,
				ls,
				pp,
			),
			&common.CurrentTreasuryValueMismatchError{},
		)
	}
}
