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

package shelley_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/cbor"
	internaltest "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// maxRejectMapAllocBytes is far below the megabytes needed to hold every pair of
// an oversized map.
const maxRejectMapAllocBytes = 256 * 1024

func TestShelleyProtocolParameterUpdateRejectsUnknownKeysBeforeAllocating(t *testing.T) {
	// Not t.Parallel: heap allocation is measured with a process-wide counter.
	data := internaltest.MapOfUnknownKeys(internaltest.OversizedMapPairs)
	var err error
	allocated := internaltest.AllocatedBytes(func() {
		err = (&shelley.ShelleyProtocolParameterUpdate{}).UnmarshalCBOR(data)
	})
	require.Error(t, err)
	assert.Less(
		t,
		allocated,
		uint64(maxRejectMapAllocBytes),
		"shelley parameter update allocated before checking its keys",
	)
}

func TestShelleyTransactionPparamUpdateRejectsInvalidProposalsBeforeAllocating(t *testing.T) {
	// Not t.Parallel: heap allocation is measured with a process-wide counter.
	data := internaltest.ParamUpdateProposals(internaltest.OversizedProposalPairs)
	var update shelley.ShelleyTransactionPparamUpdate
	var err error
	allocated := internaltest.AllocatedBytes(func() {
		_, err = cbor.Decode(data, &update)
	})
	require.Error(t, err)
	assert.Less(
		t,
		allocated,
		uint64(maxRejectMapAllocBytes),
		"shelley update proposals allocated before checking their values",
	)
}

func TestShelleyTransactionPparamUpdateDecodesValidProposals(t *testing.T) {
	t.Parallel()
	// Each prefix is a different encoding of the same two-item array head.
	for name, head := range map[string][]byte{
		"definite":   {0x82},
		"indefinite": {0x9f},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// {hash(0x01 * 28): {0: 44}} then epoch 7.
			data := append([]byte{}, head...)
			data = append(data, 0xa1, 0x58, 0x1c)
			data = append(data, bytes.Repeat([]byte{0x01}, 28)...)
			data = append(data, 0xa1, 0x00, 0x18, 0x2c, 0x07)
			if name == "indefinite" {
				data = append(data, 0xff)
			}
			var update shelley.ShelleyTransactionPparamUpdate
			_, err := cbor.Decode(data, &update)
			require.NoError(t, err)
			assert.Equal(t, uint64(7), update.Epoch)
			require.Len(t, update.ProtocolParamUpdates, 1)
			for _, got := range update.ProtocolParamUpdates {
				require.NotNil(t, got.MinFeeA)
				assert.Equal(t, uint(44), *got.MinFeeA)
			}
		})
	}
}
