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

package byron_test

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type legacySlotNumberHeader uint64

func (h legacySlotNumberHeader) SlotNumber() uint64 { return uint64(h) }

func TestSlotNumberFromEpochAndSlot(t *testing.T) {
	tests := []struct {
		name          string
		epoch         uint64
		slot          uint64
		slotsPerEpoch uint64
		want          uint64
		wantErr       error
	}{
		{
			name:          "security parameter k=2160",
			epoch:         2,
			slot:          17,
			slotsPerEpoch: 21600,
			want:          43217,
		},
		{
			name:          "security parameter k=60",
			epoch:         2,
			slot:          17,
			slotsPerEpoch: 600,
			want:          1217,
		},
		{
			name:          "slot count is not bounded by epoch length",
			epoch:         2,
			slot:          1000,
			slotsPerEpoch: 600,
			want:          2200,
		},
		{
			name:          "maximum representable slot",
			epoch:         math.MaxUint64,
			slotsPerEpoch: 1,
			want:          math.MaxUint64,
		},
		{
			name:          "multiplication overflow",
			epoch:         math.MaxUint64,
			slotsPerEpoch: 2,
			wantErr:       byron.ErrByronSlotNumberOverflow,
		},
		{
			name:          "addition overflow",
			epoch:         1,
			slot:          math.MaxUint64,
			slotsPerEpoch: 1,
			wantErr:       byron.ErrByronSlotNumberOverflow,
		},
		{
			name:    "zero epoch length",
			epoch:   1,
			wantErr: byron.ErrByronSlotsPerEpochZero,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := byron.SlotNumberFromEpochAndSlot(
				test.epoch,
				test.slot,
				test.slotsPerEpoch,
			)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestSlotNumberFromHeaderRejectsUnsupportedEpochLength(t *testing.T) {
	got, err := byron.SlotNumberFromHeader(legacySlotNumberHeader(7), 0)
	require.NoError(t, err)
	require.Equal(t, uint64(7), got)

	_, err = byron.SlotNumberFromHeader(legacySlotNumberHeader(7), 600)
	require.ErrorContains(t, err, "does not support configured Byron epoch length")
}

func TestByronHeaderSlotNumberWithEpochLengthPreservesRawCounts(t *testing.T) {
	header := &byron.ByronMainBlockHeader{}
	header.ConsensusData.SlotId.Epoch = 2
	header.ConsensusData.SlotId.Slot = 1000

	slot, err := header.SlotNumberWithEpochLength(600)
	require.NoError(t, err)
	require.Equal(t, uint64(2200), slot)
	require.Equal(t, uint64(2), header.ConsensusData.SlotId.Epoch)
	require.Equal(t, uint64(1000), header.ConsensusData.SlotId.Slot)

	ebb := &byron.ByronEpochBoundaryBlockHeader{}
	ebb.ConsensusData.Epoch = 2
	ebbSlot, err := ebb.SlotNumberWithEpochLength(600)
	require.NoError(t, err)
	require.Equal(t, uint64(1200), ebbSlot)
	require.Equal(t, uint64(2), ebb.ConsensusData.Epoch)
}

func TestSlotNumberFromBlockHeaderUsesConfiguredEpochLength(t *testing.T) {
	header := &byron.ByronMainBlockHeader{}
	header.ConsensusData.SlotId.Epoch = 2
	header.ConsensusData.SlotId.Slot = 17

	slot, err := byron.SlotNumberFromBlockHeader(header, 600)
	require.NoError(t, err)
	require.Equal(t, uint64(1217), slot)
}

func TestSlotNumberFromBlockHeaderPreservesOtherEraSlot(t *testing.T) {
	header := &shelley.ShelleyBlockHeader{
		Body: shelley.ShelleyBlockHeaderBody{Slot: 17},
	}

	slot, err := byron.SlotNumberFromBlockHeader(header, 600)
	require.NoError(t, err)
	require.Equal(t, uint64(17), slot)
}
