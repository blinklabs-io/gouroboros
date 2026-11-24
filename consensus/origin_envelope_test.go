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
// header hash, while any later block still does. After Origin the
// reference's validateEnvelope accepts only GenesisHash as the predecessor:
// a Shelley header encodes it as null, which decodes to the zero hash, so a
// nonzero 32-byte predecessor names a block that is not on the chain.
func TestPrevHashAfterOrigin(t *testing.T) {
	t.Parallel()
	v := &HeaderValidator{}
	require.NoError(t, v.validatePrevHash(&ValidateHeaderInput{PrevIsOrigin: true}))
	require.NoError(t, v.validatePrevHash(&ValidateHeaderInput{
		PrevIsOrigin: true,
		PrevHash:     make([]byte, 32),
	}))
	nonzero := make([]byte, 32)
	nonzero[31] = 1
	require.Error(t, v.validatePrevHash(&ValidateHeaderInput{
		PrevIsOrigin: true,
		PrevHash:     nonzero,
	}))
	require.Error(t, v.validatePrevHash(&ValidateHeaderInput{
		PrevIsOrigin: true,
		PrevHash:     make([]byte, 31),
	}))
	require.Error(t, v.validatePrevHash(&ValidateHeaderInput{BlockNumber: 1}))
}

// refShelleyEnvelopeOK transcribes the reference's validateEnvelope
// sequence checks (ouroboros-consensus ec239d57, HeaderValidation.hs):
// after Origin the block number is 0, any slot is allowed and the
// predecessor is GenesisHash; after a block the number is one more, the
// slot is later and the predecessor is that block's hash. genesisPrev says
// the header names GenesisHash.
func refShelleyEnvelopeOK(
	origin bool,
	prevSlot, prevNo uint64,
	prevHash []byte,
	slot, no uint64,
	genesisPrev bool,
	headerPrev []byte,
) bool {
	if origin {
		return no == 0 && genesisPrev
	}
	return no == prevNo+1 && slot > prevSlot && !genesisPrev &&
		string(headerPrev) == string(prevHash)
}

// TestShelleyEnvelopeMatchesReferenceTable checks the three envelope
// predicates together against the reference on every combination of
// predecessor (Origin or a block), slot, block number and named
// predecessor hash (absent, the zero hash, the previous block's hash or
// another block's). This is model 13's ShelleyEnvelope table.
func TestShelleyEnvelopeMatchesReferenceTable(t *testing.T) {
	t.Parallel()
	v := &HeaderValidator{}
	prevHash := make([]byte, 32)
	prevHash[0] = 0xaa
	otherHash := make([]byte, 32)
	otherHash[0] = 0xbb
	type named struct {
		name    string
		bytes   []byte
		genesis bool
	}
	hashes := []named{
		{"absent", nil, true},
		{"zero", make([]byte, 32), true},
		{"previous", prevHash, false},
		{"other", otherHash, false},
	}
	const prevSlot, prevNo = 4, 3
	for _, origin := range []bool{true, false} {
		for _, slot := range []uint64{0, 4, 5} {
			for _, no := range []uint64{0, 1, 4, 5} {
				for _, h := range hashes {
					input := ValidateHeaderInput{
						Slot:         slot,
						BlockNumber:  no,
						PrevHash:     h.bytes,
						PrevIsOrigin: origin,
					}
					if !origin {
						input.PrevSlot = prevSlot
						input.PrevBlockNumber = prevNo
						input.PrevHeaderHash = prevHash
					}
					got := v.validateSlotOrdering(&input) == nil &&
						v.validateBlockNumber(&input) == nil &&
						v.validatePrevHash(&input) == nil
					want := refShelleyEnvelopeOK(
						origin, prevSlot, prevNo, prevHash,
						slot, no, h.genesis, h.bytes,
					)
					require.Equal(
						t, want, got,
						"origin=%v slot=%d no=%d prev=%s",
						origin, slot, no, h.name,
					)
				}
			}
		}
	}
}
