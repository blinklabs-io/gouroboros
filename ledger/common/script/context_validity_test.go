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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
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
	infinityTag := uint(0)
	if !lower {
		infinityTag = 2
	}
	return data.NewConstr(
		0,
		data.NewConstr(infinityTag),
		data.NewConstr(1),
	)
}

func boolTag(value bool) uint {
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

func validityTransaction(
	t *testing.T,
	version string,
	start *uint64,
	end *uint64,
) lcommon.Transaction {
	t.Helper()
	body := make(map[uint]uint64, 2)
	if start != nil {
		body[8] = *start
	}
	if end != nil {
		body[3] = *end
	}
	bodyCbor, err := cbor.Encode(body)
	require.NoError(t, err)
	witnessCbor, err := cbor.Encode(map[uint]any{})
	require.NoError(t, err)
	txCbor, err := cbor.Encode([]any{
		cbor.RawMessage(bodyCbor),
		cbor.RawMessage(witnessCbor),
		true,
		nil,
	})
	require.NoError(t, err)
	switch version {
	case "V1":
		tx, err := alonzo.NewAlonzoTransactionFromCbor(txCbor)
		require.NoError(t, err)
		return tx
	case "V2":
		tx, err := babbage.NewBabbageTransactionFromCbor(txCbor)
		require.NoError(t, err)
		return tx
	default:
		t.Fatalf("unsupported Plutus version %q", version)
		return nil
	}
}

func TestValidityRangeMatchesCardanoLedger(t *testing.T) {
	zero := uint64(0)
	five := uint64(5)
	ten := uint64(10)
	testCases := []struct {
		name  string
		start *uint64
		end   *uint64
	}{
		{name: "unbounded"},
		{name: "upper only", end: &ten},
		{name: "lower only", start: &five},
		{name: "both bounds", start: &five, end: &ten},
		{name: "explicit zero lower", start: &zero, end: &ten},
		{name: "explicit zero upper", end: &zero},
		{name: "both explicit zero", start: &zero, end: &zero},
	}
	for _, version := range []string{"V1", "V2"} {
		t.Run(version, func(t *testing.T) {
			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					tx := validityTransaction(
						t,
						version,
						testCase.start,
						testCase.end,
					)
					var actual data.PlutusData
					switch version {
					case "V1":
						info, err := script.NewTxInfoV1FromTransaction(
							validitySlotState{},
							tx,
							nil,
						)
						require.NoError(t, err)
						actual = info.ValidRange.ToPlutusData()
					case "V2":
						info, err := script.NewTxInfoV2FromTransaction(
							validitySlotState{},
							tx,
							nil,
						)
						require.NoError(t, err)
						actual = info.ValidRange.ToPlutusData()
					}
					expected := expectedValidityRange(
						testCase.start,
						testCase.end,
					)
					require.True(
						t,
						expected.Equal(actual),
						"validity range mismatch:\n got: %s\nwant: %s",
						actual,
						expected,
					)
				})
			}
		})
	}
}
