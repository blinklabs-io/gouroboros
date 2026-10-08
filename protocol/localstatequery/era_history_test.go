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

package localstatequery

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestEraHistoryResultWireVariants(t *testing.T) {
	tests := []struct {
		name                string
		end                 any
		safeZone            any
		wantUnbounded       bool
		wantEndSlot         int
		wantSafeFromTip     *uint64
		wantSafeBeforeEpoch *uint64
		wantGenesisWindow   uint64
	}{
		{
			name:              "indefinite safe zone and unbounded end",
			end:               nil,
			safeZone:          []any{uint8(1)},
			wantUnbounded:     true,
			wantGenesisWindow: 129600,
		},
		{
			name:              "standard safe zone without epoch limit",
			end:               []any{0, 7, 1},
			safeZone:          []any{uint8(0), 42, []any{uint8(0)}},
			wantEndSlot:       7,
			wantSafeFromTip:   uint64Ptr(42),
			wantGenesisWindow: 129600,
		},
		{
			name:                "standard safe zone with epoch limit",
			end:                 []any{0, 11, 2},
			safeZone:            []any{uint8(0), 42, []any{uint8(1), 9}},
			wantEndSlot:         11,
			wantSafeFromTip:     uint64Ptr(42),
			wantSafeBeforeEpoch: uint64Ptr(9),
			wantGenesisWindow:   129600,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := cbor.Encode([]any{
				[]any{0, 0, 0},
				test.end,
				[]any{432000, 1000, test.safeZone, test.wantGenesisWindow},
			})
			require.NoError(t, err)

			var result EraHistoryResult
			_, err = cbor.Decode(wire, &result)
			if err != nil {
				t.Fatalf("decode era history result: %v", err)
			}
			if got := result.End.Unbounded; got != test.wantUnbounded {
				t.Fatalf("End.Unbounded = %t, want %t", got, test.wantUnbounded)
			}
			require.Equal(t, test.wantEndSlot, result.End.SlotNo)
			if test.wantSafeFromTip == nil {
				require.Nil(t, result.Params.SafeZone.SafeFromTip)
			} else {
				require.Equal(
					t,
					test.wantSafeFromTip,
					result.Params.SafeZone.SafeFromTip,
				)
			}
			if !equalUint64Ptr(
				test.wantSafeBeforeEpoch,
				result.Params.SafeZone.SafeBeforeEpoch,
			) {
				t.Fatal("SafeBeforeEpoch did not preserve the tagged epoch")
			}
			require.Equal(t, test.wantGenesisWindow, result.Params.GenesisWindow)

			roundTrip, err := cbor.Encode(result)
			require.NoError(t, err)
			require.Equal(t, wire, roundTrip)
		})
	}
}

func TestEraHistorySafeZoneRejectsMalformedVariants(t *testing.T) {
	tests := []struct {
		name string
		wire any
	}{
		{name: "empty", wire: []any{}},
		{name: "indefinite trailing field", wire: []any{uint8(1), 0}},
		{name: "standard missing field", wire: []any{uint8(0), 42}},
		{name: "unknown tag", wire: []any{uint8(2)}},
		{
			name: "wrong safe from tip type",
			wire: []any{uint8(0), "42", []any{uint8(0)}},
		},
		{
			name: "overflowing safe from tip",
			wire: []any{
				uint8(0),
				new(big.Int).Lsh(big.NewInt(1), 65),
				[]any{uint8(0)},
			},
		},
		{name: "negative safe from tip", wire: []any{uint8(0), -1, []any{uint8(0)}}},
		{
			name: "invalid safe before epoch",
			wire: []any{uint8(0), 42, []any{uint8(2)}},
		},
		{
			name: "safe before epoch trailing field",
			wire: []any{uint8(0), 42, []any{uint8(0), 9}},
		},
		{
			name: "wrong safe before epoch type",
			wire: []any{uint8(0), 42, []any{uint8(1), "9"}},
		},
		{
			name: "overflowing safe before epoch",
			wire: []any{
				uint8(0),
				42,
				[]any{uint8(1), new(big.Int).Lsh(big.NewInt(1), 65)},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := cbor.Encode(test.wire)
			require.NoError(t, err)
			var safeZone eraHistorySafeZone
			_, err = cbor.Decode(wire, &safeZone)
			require.Error(t, err)
		})
	}
}

func TestEraHistoryTaggedUnionsRejectOversizedArrays(t *testing.T) {
	declaredMillion := []byte{0x9a, 0x00, 0x0f, 0x42, 0x40}
	completeThousand, err := cbor.Encode(make([]any, 1000))
	require.NoError(t, err)

	for _, wire := range [][]byte{declaredMillion, completeThousand} {
		var safeZone eraHistorySafeZone
		require.Error(t, safeZone.UnmarshalCBOR(wire))

		_, err := decodeSafeBeforeEpoch(wire)
		require.Error(t, err)
	}
}

func TestEraHistoryResultRejectsInvalidCounters(t *testing.T) {
	intOverflow := uint64(^uint(0)>>1) + 1
	uint64Overflow := new(big.Int).Lsh(big.NewInt(1), 65)
	tests := []struct {
		name   string
		begin  any
		end    any
		params any
	}{
		{
			name:  "overflowing begin timespan",
			begin: []any{intOverflow, 0, 0},
		},
		{
			name:  "wrong begin timespan type",
			begin: []any{"0", 0, 0},
		},
		{
			name:  "negative begin slot",
			begin: []any{0, -1, 0},
		},
		{
			name:  "overflowing begin slot",
			begin: []any{0, intOverflow, 0},
		},
		{
			name:  "negative begin epoch",
			begin: []any{0, 0, -1},
		},
		{
			name:  "overflowing begin epoch",
			begin: []any{0, 0, intOverflow},
		},
		{
			name: "negative end slot",
			end:  []any{0, -1, 0},
		},
		{
			name:   "negative epoch length",
			params: []any{-1, 1000, []any{uint8(1)}, 129600},
		},
		{
			name:   "overflowing epoch length",
			params: []any{intOverflow, 1000, []any{uint8(1)}, 129600},
		},
		{
			name:   "negative slot length",
			params: []any{432000, -1, []any{uint8(1)}, 129600},
		},
		{
			name:   "overflowing slot length",
			params: []any{432000, intOverflow, []any{uint8(1)}, 129600},
		},
		{
			name:   "negative genesis window",
			params: []any{432000, 1000, []any{uint8(1)}, -1},
		},
		{
			name:   "overflowing genesis window",
			params: []any{432000, 1000, []any{uint8(1)}, uint64Overflow},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			begin := test.begin
			if begin == nil {
				begin = []any{0, 0, 0}
			}
			end := test.end
			if end == nil {
				end = []any{0, 7, 1}
			}
			params := test.params
			if params == nil {
				params = []any{432000, 1000, []any{uint8(1)}, 129600}
			}
			wire, err := cbor.Encode([]any{begin, end, params})
			require.NoError(t, err)

			var result EraHistoryResult
			_, err = cbor.Decode(wire, &result)
			require.Error(t, err)
		})
	}
}

func TestEraHistoryEndRejectsNonBounds(t *testing.T) {
	for _, wire := range [][]byte{{0xf7}, {0x01}, {0x80}} {
		t.Run(fmt.Sprintf("%x", wire), func(t *testing.T) {
			var end eraHistoryResultEnd
			_, err := cbor.Decode(wire, &end)
			require.Error(t, err)
		})
	}
}

func TestEraHistoryMarshalRejectsInconsistentVariants(t *testing.T) {
	_, err := cbor.Encode(eraHistorySafeZone{
		SafeBeforeEpoch: uint64Ptr(9),
	})
	require.Error(t, err)

	_, err = cbor.Encode(eraHistoryResultEnd{
		eraHistoryResultBound: eraHistoryResultBound{SlotNo: 1},
		Unbounded:             true,
	})
	require.Error(t, err)

	for _, bound := range []eraHistoryResultBound{
		{SlotNo: -1},
		{EpochNo: -1},
	} {
		_, err = cbor.Encode(bound)
		require.Error(t, err)
	}

	for _, params := range []eraHistoryResultParams{
		{EpochLength: -1},
		{SlotLength: -1},
	} {
		_, err = cbor.Encode(params)
		require.Error(t, err)
	}
}

func TestEraHistoryNegativeRelativeTimeRoundTrip(t *testing.T) {
	wire, err := cbor.Encode([]any{
		[]any{-1, 0, 0},
		[]any{0, 7, 1},
		[]any{432000, 1000, []any{uint8(1)}, 129600},
	})
	require.NoError(t, err)

	var result EraHistoryResult
	_, err = cbor.Decode(wire, &result)
	require.NoError(t, err)
	require.Equal(t, -1, result.Begin.Timespan)

	roundTrip, err := cbor.Encode(result)
	require.NoError(t, err)
	require.Equal(t, wire, roundTrip)
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}

func equalUint64Ptr(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
