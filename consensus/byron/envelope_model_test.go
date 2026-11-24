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

package byron

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

type envelopeKind int

const (
	envelopeOrigin envelopeKind = iota
	envelopeMain
	envelopeEBB
)

func (k envelopeKind) String() string {
	return [...]string{"origin", "main", "EBB"}[k]
}

type envelopeHeader interface {
	SlotNumber() uint64
	BlockNumber() uint64
	Hash() common.Blake2b256
}

const envelopeSlotsPerEpoch = 10

// envelopeTestHeader builds a header of kind at an absolute slot; an EBB's
// slot must be an epoch boundary.
func envelopeTestHeader(
	kind envelopeKind,
	slot, blockNumber uint64,
	prev envelopeHeader,
) interface {
	envelopeHeader
	PrevHash() common.Blake2b256
} {
	switch kind {
	case envelopeEBB:
		header := &byron.ByronEpochBoundaryBlockHeader{}
		header.ConsensusData.Epoch = slot / envelopeSlotsPerEpoch
		header.ConsensusData.Difficulty.Value = blockNumber
		if prev != nil {
			header.PrevBlock = prev.Hash()
		}
		return header
	default:
		header := &byron.ByronMainBlockHeader{}
		header.ConsensusData.SlotId.Epoch = slot / envelopeSlotsPerEpoch
		header.ConsensusData.SlotId.Slot = slot % envelopeSlotsPerEpoch
		header.ConsensusData.Difficulty.Value = blockNumber
		if prev != nil {
			header.PrevBlock = prev.Hash()
		}
		return header
	}
}

// envelopeAsBlock wraps a header built by envelopeTestHeader in its block.
// The predecessor's kind comes from the block, not from how it is passed:
// the reference takes it from byronHeaderIsEBB (Byron/Ledger/
// HeaderValidation.hs 30-32, getTipInfo).
func envelopeAsBlock(header envelopeHeader) envelopeHeader {
	switch h := header.(type) {
	case *byron.ByronEpochBoundaryBlockHeader:
		return &byron.ByronEpochBoundaryBlock{BlockHeader: h}
	case *byron.ByronMainBlockHeader:
		return &byron.ByronMainBlock{BlockHeader: h}
	}
	panic(fmt.Sprintf("unexpected envelope header %T", header))
}

// refEnvelopeOK transcribes the reference envelope rule for Byron
// (ouroboros-consensus ec239d57, HeaderValidation.hs 378-423 and
// Byron/Ledger/HeaderValidation.hs 39-52). After Origin the block number is
// 0 and any slot is allowed. Otherwise an EBB after a main block shares the
// predecessor's block number and every other pair increments it; a main
// block after an EBB may share the EBB's slot and every other pair needs a
// later slot.
func refEnvelopeOK(
	prev envelopeKind,
	prevSlot, prevBlock uint64,
	cur envelopeKind,
	slot, block uint64,
) bool {
	if prev == envelopeOrigin {
		return block == 0
	}
	expectedBlock := prevBlock + 1
	if prev == envelopeMain && cur == envelopeEBB {
		expectedBlock = prevBlock
	}
	minimumSlot := prevSlot + 1
	if prev == envelopeEBB && cur == envelopeMain {
		minimumSlot = prevSlot
	}
	return block == expectedBlock && slot >= minimumSlot
}

// TestByronEnvelopeMatchesReferenceTable checks the envelope's slot and
// block-number rules for every pair of predecessor and header kinds, with
// the predecessor passed as a header or as a block, and the header at an
// earlier, the same or a later slot and at the same, the next or a skipped
// block number.
func TestByronEnvelopeMatchesReferenceTable(t *testing.T) {
	for _, prevAsBlock := range []bool{false, true} {
		t.Run(fmt.Sprintf("predecessor as block %t", prevAsBlock), func(t *testing.T) {
			checkByronEnvelopeTable(t, prevAsBlock)
		})
	}
}

func checkByronEnvelopeTable(t *testing.T, prevAsBlock bool) {
	config := ByronConfig{
		ProtocolMagic: 7,
		SlotsPerEpoch: envelopeSlotsPerEpoch,
	}
	const prevSlot, prevBlock = 20, 5
	for _, prev := range []envelopeKind{
		envelopeOrigin, envelopeMain, envelopeEBB,
	} {
		var prevHeader envelopeHeader
		if prev != envelopeOrigin {
			prevHeader = envelopeTestHeader(prev, prevSlot, prevBlock, nil)
			if prevAsBlock {
				prevHeader = envelopeAsBlock(prevHeader)
			}
		}
		for _, cur := range []envelopeKind{envelopeMain, envelopeEBB} {
			slots := []uint64{prevSlot - 1, prevSlot, prevSlot + 1}
			if cur == envelopeEBB {
				// An EBB only exists on an epoch boundary.
				slots = []uint64{
					prevSlot - envelopeSlotsPerEpoch,
					prevSlot,
					prevSlot + envelopeSlotsPerEpoch,
				}
			}
			blocks := []uint64{prevBlock - 1, prevBlock, prevBlock + 1}
			if prev == envelopeOrigin {
				blocks = []uint64{0, 1}
			}
			for _, slot := range slots {
				for _, block := range blocks {
					name := fmt.Sprintf(
						"%s then %s slot %d block %d",
						prev, cur, slot, block,
					)
					header := envelopeTestHeader(cur, slot, block, prevHeader)
					var err error
					if prevHeader == nil {
						err = ValidateByronBlockHeader(
							header, nil, config, cur == envelopeEBB,
						)
					} else {
						err = ValidateByronBlockHeader(
							header, prevHeader, config, cur == envelopeEBB,
						)
					}
					assert.Equal(
						t,
						refEnvelopeOK(
							prev, prevSlot, prevBlock, cur, slot, block,
						),
						err == nil,
						"%s: %v", name, err,
					)
				}
			}
		}
	}
}
