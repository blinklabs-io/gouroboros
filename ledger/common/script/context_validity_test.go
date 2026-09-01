// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

type validitySlotState struct{}

func (validitySlotState) SlotToTime(slot uint64) (time.Time, error) {
	return time.UnixMilli(int64(slot)), nil
}

func (validitySlotState) TimeToSlot(time.Time) (uint64, error) {
	return 0, nil
}

func validityBound(
	present bool,
	value uint64,
	lower bool,
	closed bool,
) data.PlutusData {
	if present {
		return data.NewConstr(
			0,
			data.NewConstr(
				1,
				data.NewInteger(new(big.Int).SetUint64(value)),
			),
			data.NewConstr(boolTag(closed)),
		)
	}
	var infinityTag uint64
	if !lower {
		infinityTag = 2
	}
	return data.NewConstr(
		0,
		data.NewConstr(infinityTag),
		data.NewConstr(1),
	)
}

func boolTag(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func expectedValidityRange(
	start *uint64,
	end *uint64,
) data.PlutusData {
	startPresent := start != nil
	endPresent := end != nil
	var startValue uint64
	if startPresent {
		startValue = *start
	}
	var endValue uint64
	if endPresent {
		endValue = *end
	}
	return data.NewConstr(
		0,
		validityBound(startPresent, startValue, true, true),
		validityBound(endPresent, endValue, false, !startPresent),
	)
}

func requireValidityRange(
	t *testing.T,
	fixture mockledger.ValidityIntervalFixture,
	actual data.PlutusData,
) {
	t.Helper()
	expected := expectedValidityRange(fixture.StartSlot, fixture.EndSlot)
	require.True(
		t,
		expected.Equal(actual),
		"validity range mismatch:\n got: %s\nwant: %s",
		actual,
		expected,
	)
}

func TestValidityRangeMatchesCardanoLedger(t *testing.T) {
	t.Run("V1", func(t *testing.T) {
		for _, fixture := range mockledger.ValidityIntervalFixtures() {
			t.Run(fixture.Name, func(t *testing.T) {
				tx, err := fixture.AlonzoTransaction()
				require.NoError(t, err)
				info, err := script.NewTxInfoV1FromTransaction(
					validitySlotState{},
					tx,
					nil,
				)
				require.NoError(t, err)
				requireValidityRange(
					t,
					fixture,
					info.ValidRange.ToPlutusData(),
				)
			})
		}
	})
	t.Run("V2", func(t *testing.T) {
		for _, fixture := range mockledger.ValidityIntervalFixtures() {
			t.Run(fixture.Name, func(t *testing.T) {
				tx, err := fixture.BabbageTransaction()
				require.NoError(t, err)
				info, err := script.NewTxInfoV2FromTransaction(
					validitySlotState{},
					tx,
					nil,
				)
				require.NoError(t, err)
				requireValidityRange(
					t,
					fixture,
					info.ValidRange.ToPlutusData(),
				)
			})
		}
	})
}
