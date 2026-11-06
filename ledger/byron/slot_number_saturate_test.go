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
	"github.com/stretchr/testify/require"
)

func TestByronHeaderSlotNumberSaturatesOnOverflow(t *testing.T) {
	t.Parallel()

	const perEpoch = byron.ByronSlotsPerEpoch
	const maxEpoch = uint64(math.MaxUint64 / perEpoch)
	const remainder = uint64(math.MaxUint64 % perEpoch)

	tests := []struct {
		name  string
		epoch uint64
		slot  uint64
		want  uint64
		// ebbWant is the EBB result; an EBB has no within-epoch slot.
		ebbWant uint64
		// mainOnly marks cases that only the main header can express.
		mainOnly bool
	}{
		{
			name:    "in range",
			epoch:   2,
			slot:    17,
			want:    43217,
			ebbWant: 43200,
		},
		{
			name:    "largest epoch without overflow",
			epoch:   maxEpoch,
			want:    maxEpoch * perEpoch,
			ebbWant: maxEpoch * perEpoch,
		},
		{
			name:     "exact maximum representable slot",
			epoch:    maxEpoch,
			slot:     remainder,
			want:     math.MaxUint64,
			mainOnly: true,
		},
		{
			name:    "multiplication overflow saturates",
			epoch:   maxEpoch + 1,
			slot:    7,
			want:    math.MaxUint64,
			ebbWant: math.MaxUint64,
		},
		{
			name:    "issue repro epoch saturates",
			epoch:   (math.MaxUint64 / perEpoch) + 5,
			slot:    7,
			want:    math.MaxUint64,
			ebbWant: math.MaxUint64,
		},
		{
			name:     "addition overflow saturates",
			epoch:    maxEpoch,
			slot:     remainder + 1,
			want:     math.MaxUint64,
			mainOnly: true,
		},
	}

	for _, test := range tests {
		t.Run("main/"+test.name, func(t *testing.T) {
			t.Parallel()
			header := &byron.ByronMainBlockHeader{}
			header.ConsensusData.SlotId.Epoch = test.epoch
			header.ConsensusData.SlotId.Slot = test.slot
			require.Equal(t, test.want, header.SlotNumber())
		})
		if test.mainOnly {
			continue
		}
		t.Run("ebb/"+test.name, func(t *testing.T) {
			t.Parallel()
			header := &byron.ByronEpochBoundaryBlockHeader{}
			header.ConsensusData.Epoch = test.epoch
			require.Equal(t, test.ebbWant, header.SlotNumber())
		})
	}
}
