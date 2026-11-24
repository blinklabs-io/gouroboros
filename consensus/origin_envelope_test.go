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

package consensus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnvelopeAfterOrigin checks the header envelope of the first block on a
// chain against the reference's envelope rules: with no previous block, the
// expected block number is 0 and any slot is allowed. A header that follows
// a real previous block keeps the strict rules.
func TestEnvelopeAfterOrigin(t *testing.T) {
	t.Parallel()
	v := &HeaderValidator{}
	for _, tc := range []struct {
		name       string
		input      ValidateHeaderInput
		slotOK     bool
		blockNumOK bool
	}{
		{"first block, number 0, slot 0", ValidateHeaderInput{PrevIsOrigin: true}, true, true},
		{"first block, number 0, later slot", ValidateHeaderInput{PrevIsOrigin: true, Slot: 42}, true, true},
		{"first block, number 1", ValidateHeaderInput{PrevIsOrigin: true, Slot: 42, BlockNumber: 1}, true, false},
		{"after block 0, number 1", ValidateHeaderInput{Slot: 5, BlockNumber: 1, PrevSlot: 4}, true, true},
		{"after block 0, same slot", ValidateHeaderInput{Slot: 4, BlockNumber: 1, PrevSlot: 4}, false, true},
		{"after block 3, number 3", ValidateHeaderInput{Slot: 9, BlockNumber: 3, PrevSlot: 8, PrevBlockNumber: 3}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if tc.slotOK {
				require.NoError(t, v.validateSlotOrdering(&input))
			} else {
				require.Error(t, v.validateSlotOrdering(&input))
			}
			if tc.blockNumOK {
				require.NoError(t, v.validateBlockNumber(&input))
			} else {
				require.Error(t, v.validateBlockNumber(&input))
			}
		})
	}
}

// TestPrevHashAfterOrigin checks that the first block needs no previous
// header hash, while any later block still does.
func TestPrevHashAfterOrigin(t *testing.T) {
	t.Parallel()
	v := &HeaderValidator{}
	require.NoError(t, v.validatePrevHash(&ValidateHeaderInput{PrevIsOrigin: true}))
	require.Error(t, v.validatePrevHash(&ValidateHeaderInput{BlockNumber: 1}))
}
