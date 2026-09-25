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

package blockfetch

import (
	"math"
	"testing"

	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

func TestBlockFetchSlotNumberUsesConfiguredByronEpochLength(t *testing.T) {
	header := &ledgerbyron.ByronMainBlockHeader{}
	header.ConsensusData.SlotId.Epoch = 2
	header.ConsensusData.SlotId.Slot = 17

	slot, err := blockFetchSlotNumber(header, 600)
	require.NoError(t, err)
	require.Equal(t, uint64(1217), slot)

	config, err := NewConfig(WithByronSlotsPerEpoch(600))
	require.NoError(t, err)
	require.Equal(t, uint64(600), config.ByronSlotsPerEpoch)

	header.ConsensusData.SlotId.Epoch = math.MaxUint64
	_, err = blockFetchSlotNumber(header, 600)
	require.ErrorIs(t, err, ledgerbyron.ErrByronSlotNumberOverflow)
}
