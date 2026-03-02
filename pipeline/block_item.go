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

package pipeline

import (
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// BlockItem represents a block as it moves through the pipeline.
// It is thread-safe and tracks the processing state at each stage.
type BlockItem struct {
	// Immutable fields (set at construction, never modified)
	// These are unexported to prevent modification; use getter methods.
	blockType      uint
	rawCbor        []byte
	tip            pcommon.Tip
	sequenceNumber uint64
	receivedAt     time.Time

	// Mutable fields protected by mutex
	mu sync.RWMutex

	// Decode stage results
	block          common.Block
	decodeError    error
	decodeDuration time.Duration

	// Validate stage results
	valid            bool
	validationError  error
	validateDuration time.Duration
	vrfOutput        string

	// Apply stage results
	applied       bool
	applyError    error
	applyDuration time.Duration
}

// NewBlockItem creates a new BlockItem with the given immutable fields.
// The rawCbor slice and tip.Point.Hash are copied to prevent data corruption
// from external modifications.
func NewBlockItem(blockType uint, rawCbor []byte, tip pcommon.Tip, seq uint64) *BlockItem {
	// Copy the rawCbor slice to prevent aliasing issues.
	// This ensures the BlockItem owns its data and is immune to
	// modifications by the caller, which is critical for concurrent
	// pipeline processing.
	cbor := make([]byte, len(rawCbor))
	copy(cbor, rawCbor)

	// Deep copy tip.Point.Hash to ensure full data ownership
	tipCopy := tip
	if len(tip.Point.Hash) > 0 {
		tipCopy.Point.Hash = make([]byte, len(tip.Point.Hash))
		copy(tipCopy.Point.Hash, tip.Point.Hash)
	}

	return &BlockItem{
		blockType:      blockType,
		rawCbor:        cbor,
		tip:            tipCopy,
		sequenceNumber: seq,
		receivedAt:     time.Now(),
	}
}

// BlockType returns the block type (e.g., ledger.BlockTypeConway).
func (b *BlockItem) BlockType() uint {
	return b.blockType
}

// RawCbor returns the raw CBOR bytes of the block.
// The returned slice should not be modified.
func (b *BlockItem) RawCbor() []byte {
	return b.rawCbor
}

// Tip returns the chain tip associated with this block.
// The returned Tip contains a slice (Point.Hash) that should not be modified.
func (b *BlockItem) Tip() pcommon.Tip {
	return b.tip
}

// SequenceNumber returns the sequence number assigned to this block.
func (b *BlockItem) SequenceNumber() uint64 {
	return b.sequenceNumber
}

// ReceivedAt returns the time when this block was received.
func (b *BlockItem) ReceivedAt() time.Time {
	return b.receivedAt
}

// Block returns the decoded block, or nil if not yet decoded or decode failed.
func (b *BlockItem) Block() common.Block {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.block
}

// SetBlock sets the decoded block and decode duration.
// Clears any previously set decode error for consistency.
func (b *BlockItem) SetBlock(block common.Block, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.block = block
	b.decodeError = nil
	b.decodeDuration = duration
}

// DecodeError returns the decode error, if any.
func (b *BlockItem) DecodeError() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.decodeError
}

// SetDecodeError sets the decode error and duration.
// Clears any previously set block for consistency.
func (b *BlockItem) SetDecodeError(err error, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.block = nil
	b.decodeError = err
	b.decodeDuration = duration
}

// DecodeDuration returns the time spent in the decode stage.
func (b *BlockItem) DecodeDuration() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.decodeDuration
}

// IsDecoded returns true if the block has been successfully decoded.
func (b *BlockItem) IsDecoded() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.block != nil
}

// SetValidation sets the validation result.
func (b *BlockItem) SetValidation(valid bool, vrfOutput string, err error, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.valid = valid
	b.vrfOutput = vrfOutput
	b.validationError = err
	b.validateDuration = duration
}

// IsValid returns true if the block passed validation.
func (b *BlockItem) IsValid() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.valid
}

// ValidationError returns the validation error, if any.
func (b *BlockItem) ValidationError() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.validationError
}

// ValidateDuration returns the time spent in the validate stage.
func (b *BlockItem) ValidateDuration() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.validateDuration
}

// VRFOutput returns the VRF output hex string from validation.
func (b *BlockItem) VRFOutput() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.vrfOutput
}

// SetApplied sets the apply result.
func (b *BlockItem) SetApplied(applied bool, err error, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applied = applied
	b.applyError = err
	b.applyDuration = duration
}

// IsApplied returns true if the block has been applied.
func (b *BlockItem) IsApplied() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.applied
}

// ApplyError returns the apply error, if any.
func (b *BlockItem) ApplyError() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.applyError
}

// ApplyDuration returns the time spent in the apply stage.
func (b *BlockItem) ApplyDuration() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.applyDuration
}

// Slot returns the slot number from the tip.
func (b *BlockItem) Slot() uint64 {
	return b.tip.Point.Slot
}

// BlockNumber returns the block number from the tip.
func (b *BlockItem) BlockNumber() uint64 {
	return b.tip.BlockNumber
}

// TotalDuration returns the total processing time from receipt to completion.
func (b *BlockItem) TotalDuration() time.Duration {
	return time.Since(b.receivedAt)
}
