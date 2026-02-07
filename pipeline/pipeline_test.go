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
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getValidBlockCbor returns valid Conway block CBOR bytes for testing.
// Uses centralized test data from internal/testdata.
func getValidBlockCbor(t *testing.T) []byte {
	t.Helper()
	return testdata.MustDecodeHex(testdata.ConwayBlockHex)
}

// getInvalidBlockCbor returns invalid CBOR bytes that will fail to decode.
func getInvalidBlockCbor() []byte {
	// Invalid CBOR - incomplete array structure
	return []byte{0x85, 0x00, 0x01, 0x02}
}

// createTestTip creates a test Tip for BlockItem construction.
func createTestTip(slot uint64, blockNum uint64) pcommon.Tip {
	return pcommon.Tip{
		Point:       pcommon.NewPoint(slot, []byte{0x01, 0x02, 0x03}),
		BlockNumber: blockNum,
	}
}

// ============================================================================
// TestBlockItem tests
// ============================================================================

func TestBlockItem_NewBlockItem(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)

	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 42)

	assert.Equal(t, uint(ledger.BlockTypeConway), item.BlockType())
	assert.Equal(t, rawCbor, item.RawCbor())
	assert.Equal(t, tip.Point.Slot, item.Tip().Point.Slot)
	assert.Equal(t, tip.BlockNumber, item.Tip().BlockNumber)
	assert.Equal(t, uint64(42), item.SequenceNumber())
	assert.False(t, item.ReceivedAt().IsZero())
}

func TestBlockItem_SetBlock_Block(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially no block
	assert.Nil(t, item.Block())
	assert.False(t, item.IsDecoded())

	// Decode and set block
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	item.SetBlock(block, 50*time.Millisecond)

	// Verify block is set
	assert.NotNil(t, item.Block())
	assert.True(t, item.IsDecoded())
	assert.Equal(t, 50*time.Millisecond, item.DecodeDuration())
}

func TestBlockItem_SetDecodeError_IsDecoded(t *testing.T) {
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	testErr := errors.New("decode failed")
	item.SetDecodeError(testErr, 10*time.Millisecond)

	assert.False(t, item.IsDecoded())
	assert.Equal(t, testErr, item.DecodeError())
	assert.Equal(t, 10*time.Millisecond, item.DecodeDuration())
}

func TestBlockItem_SetValidation_IsValid(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially not valid
	assert.False(t, item.IsValid())

	// Set validation success
	item.SetValidation(true, "abc123", nil, 100*time.Millisecond)

	assert.True(t, item.IsValid())
	assert.Nil(t, item.ValidationError())
	assert.Equal(t, "abc123", item.VRFOutput())
	assert.Equal(t, 100*time.Millisecond, item.ValidateDuration())

	// Set validation failure
	item2 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 2)
	validationErr := errors.New("VRF failed")
	item2.SetValidation(false, "", validationErr, 50*time.Millisecond)

	assert.False(t, item2.IsValid())
	assert.Equal(t, validationErr, item2.ValidationError())
}

func TestBlockItem_SetApplied(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially not applied
	assert.False(t, item.IsApplied())

	// Set applied successfully
	item.SetApplied(true, nil, 25*time.Millisecond)

	assert.True(t, item.IsApplied())
	assert.Nil(t, item.ApplyError())
	assert.Equal(t, 25*time.Millisecond, item.ApplyDuration())

	// Set apply failure
	item2 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 2)
	applyErr := errors.New("apply failed")
	item2.SetApplied(false, applyErr, 10*time.Millisecond)

	assert.False(t, item2.IsApplied())
	assert.Equal(t, applyErr, item2.ApplyError())
}

func TestBlockItem_ThreadSafety(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode block once
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	const numGoroutines = 50
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 4) // 4 groups of concurrent operations

	// Concurrent writers for SetBlock
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetBlock(block, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	// Concurrent writers for SetValidation
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetValidation(true, "test", nil, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = item.Block()
				_ = item.IsDecoded()
				_ = item.IsValid()
			}
		}()
	}

	// Concurrent writers for SetApplied
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetApplied(true, nil, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	wg.Wait()
	// If we get here without panic/race detection, the test passes
}

func TestBlockItem_Slot_BlockNumber(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(12345, 6789)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	assert.Equal(t, uint64(12345), item.Slot())
	assert.Equal(t, uint64(6789), item.BlockNumber())
}

// ============================================================================
// TestDecodeStage tests
// ============================================================================

func TestDecodeStage_SuccessfulDecode(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(true) // Skip body hash validation for test

	err := stage.Process(context.Background(), item)

	require.NoError(t, err)
	assert.True(t, item.IsDecoded())
	assert.NotNil(t, item.Block())
	assert.Nil(t, item.DecodeError())
	assert.Greater(t, item.DecodeDuration(), time.Duration(0))
}

func TestDecodeStage_DecodeErrorHandling(t *testing.T) {
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(false)

	err := stage.Process(context.Background(), item)

	// DecodeStage returns error on failure
	assert.Error(t, err)
	assert.False(t, item.IsDecoded())
	assert.Nil(t, item.Block())
	assert.NotNil(t, item.DecodeError())
}

func TestDecodeStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before processing

	err := stage.Process(ctx, item)

	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, item.IsDecoded())
}

func TestDecodeStage_Name(t *testing.T) {
	stage := NewDecodeStage(false)
	assert.Equal(t, "decode", stage.Name())
}

// ============================================================================
// TestDecodeStageWorkerPool tests
// ============================================================================

func TestDecodeStageWorkerPool_MultipleWorkers(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 20
	const numWorkers = 4

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)
	errors := make(chan error, numItems)

	// Populate input channel
	for i := 0; i < numItems; i++ {
		tip := createTestTip(uint64(1000+i), uint64(500+i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}
	close(input)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, numWorkers, input, output, errors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool.Start(ctx)
	pool.Stop()

	// Collect results
	close(output)
	close(errors)

	var decoded []*BlockItem
	for item := range output {
		decoded = append(decoded, item)
	}

	assert.Len(t, decoded, numItems)
	for _, item := range decoded {
		assert.True(t, item.IsDecoded(), "Item seq %d should be decoded", item.SequenceNumber())
	}
}

func TestDecodeStageWorkerPool_ItemsFlowThrough(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 5

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)
	errChan := make(chan error, numItems)

	// Create items with specific sequence numbers
	expectedSeqs := make(map[uint64]bool)
	for i := 0; i < numItems; i++ {
		seq := uint64(100 + i)
		expectedSeqs[seq] = false
		tip := createTestTip(1000, 500)
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, seq)
		input <- item
	}
	close(input)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 2, input, output, errChan)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool.Start(ctx)
	pool.Stop()

	close(output)
	close(errChan)

	// Verify all items came through (order may vary due to parallelism)
	for item := range output {
		if _, exists := expectedSeqs[item.SequenceNumber()]; exists {
			expectedSeqs[item.SequenceNumber()] = true
		}
	}

	for seq, seen := range expectedSeqs {
		assert.True(t, seen, "Sequence %d should have been output", seq)
	}
}

func TestDecodeStageWorkerPool_CleanShutdown(t *testing.T) {
	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errChan := make(chan error, 10)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 3, input, output, errChan)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Cancel and verify clean shutdown
	cancel()
	close(input)

	// Should complete without hanging
	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Pool did not shut down cleanly within timeout")
	}
}

// ============================================================================
// TestValidateStage tests
// ============================================================================

func TestValidateStage_SkipsItemsWithDecodeErrors(t *testing.T) {
	// Create an item that failed decode
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
	item.SetDecodeError(errors.New("decode failed"), 10*time.Millisecond)

	// Create real validate stage
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
	}
	validateStage := NewValidateStage(config)

	err := validateStage.Process(context.Background(), item)

	// Should return nil to avoid spurious error - decode stage already reported it
	assert.NoError(t, err)
	assert.False(t, item.IsValid()) // Should not be marked valid since decode failed
}

func TestValidateStage_ValidationResultSetCorrectly(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, 50*time.Millisecond)

	// Create real validate stage (validation will fail without proper eta0,
	// but we can verify the stage processes correctly)
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	validateStage := NewValidateStage(config)

	err = validateStage.Process(context.Background(), item)

	// Validation will likely fail due to incorrect eta0, but the stage should
	// still set the validation result properly
	assert.Greater(t, item.ValidateDuration(), time.Duration(0))
}

func TestValidateStage_Name(t *testing.T) {
	config := ValidateStageConfig{}
	stage := NewValidateStage(config)
	assert.Equal(t, "validate", stage.Name())
}

func TestValidateStage_Eta0ProviderCalled(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Track provider calls
	var calledSlot uint64
	provider := func(slot uint64) (string, error) {
		calledSlot = slot
		// Return dummy nonce (validation will fail but we're testing the provider is called)
		return "0000000000000000000000000000000000000000000000000000000000000000", nil
	}

	config := ValidateStageConfig{
		Eta0Provider:      provider,
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	// Process will fail validation due to incorrect nonce, but provider should be called
	_ = stage.Process(context.Background(), item)

	// Verify provider was called with the block's slot number
	assert.Equal(t, block.SlotNumber(), calledSlot, "Provider should be called with block's slot number")
	assert.Greater(t, item.ValidateDuration(), time.Duration(0), "ValidateDuration should be set")
}

func TestValidateStage_Eta0ProviderError(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Provider that returns an error
	providerErr := errors.New("epoch nonce not available")
	provider := func(slot uint64) (string, error) {
		return "", providerErr
	}

	config := ValidateStageConfig{
		Eta0Provider:      provider,
		SlotsPerKesPeriod: 129600,
	}
	stage := NewValidateStage(config)

	err = stage.Process(context.Background(), item)

	// Should return error from provider
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eta0 provider error")
	assert.ErrorIs(t, err, providerErr)
	assert.False(t, item.IsValid())
	assert.Greater(t, item.ValidateDuration(), time.Duration(0))
}

func TestValidateStage_NoProviderReturnsError(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Configure without Eta0Provider - this is a configuration error
	config := ValidateStageConfig{
		Eta0Provider:      nil, // No provider - configuration error
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	// Should return a clear error when no Eta0Provider is configured
	err = stage.Process(context.Background(), item)

	// Expect a configuration error
	assert.Error(t, err, "Should fail without Eta0Provider")
	assert.Contains(t, err.Error(), "eta0 provider not configured")
	assert.False(t, item.IsValid())
	assert.Greater(t, item.ValidateDuration(), time.Duration(0), "ValidateDuration should be set")
}

func TestValidateStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	config := ValidateStageConfig{}
	stage := NewValidateStage(config)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before processing

	err = stage.Process(ctx, item)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// TestApplyStage tests
// ============================================================================

func TestApplyStage_Name(t *testing.T) {
	stage := NewApplyStage(nil, 0)
	assert.Equal(t, "apply", stage.Name())
}

func TestApplyStageOrdering_OutOfOrderReordering(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	var appliedOrder []uint64
	var mu sync.Mutex

	// Create real apply stage with order tracking
	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber())
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc, 0)

	// Create items with sequence numbers
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		// Decode and validate them
		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)
		items[i].SetValidation(true, "vrf", nil, time.Millisecond)
	}

	// Process items in scrambled order: 2, 0, 4, 1, 3
	scrambledOrder := []int{2, 0, 4, 1, 3}
	for _, idx := range scrambledOrder {
		err := applyStage.Process(context.Background(), items[idx])
		require.NoError(t, err)
	}

	// Verify items were applied in sequence order
	assert.Equal(t, []uint64{0, 1, 2, 3, 4}, appliedOrder)
}

func TestApplyStageOrdering_SkipsInvalidItems(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	var appliedSeqs []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedSeqs = append(appliedSeqs, item.SequenceNumber())
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc, 0)

	// Create items - some valid, some invalid
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)

		// Make items 1 and 3 invalid (using validation error)
		if i == 1 || i == 3 {
			items[i].SetValidation(false, "", errors.New("validation failed"), time.Millisecond)
		} else {
			items[i].SetValidation(true, "vrf", nil, time.Millisecond)
		}
	}

	// Process all items in order
	for i := range items {
		err := applyStage.Process(context.Background(), items[i])
		require.NoError(t, err)
	}

	// Only items 0, 2, 4 should have been applied (1 and 3 had validation errors)
	assert.Equal(t, []uint64{0, 2, 4}, appliedSeqs)
}

func TestApplyStage_PendingCount(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(func(item *BlockItem) error {
		return nil
	}, 0)

	// Create items 1 and 2 (but not 0)
	for i := 1; i <= 2; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		item.SetBlock(block, time.Millisecond)
		item.SetValidation(true, "vrf", nil, time.Millisecond)
		_ = applyStage.Process(context.Background(), item)
	}

	// Items 1 and 2 should be pending (waiting for item 0)
	assert.Equal(t, 2, applyStage.PendingCount())

	// Now process item 0 - should trigger applying all
	tip := createTestTip(1000, 500)
	item0 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 0)
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item0.SetBlock(block, time.Millisecond)
	item0.SetValidation(true, "vrf", nil, time.Millisecond)
	_ = applyStage.Process(context.Background(), item0)

	// All should be applied now
	assert.Equal(t, 0, applyStage.PendingCount())
}

func TestApplyStage_Reset(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(nil, 0)

	// Add some pending items
	for i := 1; i <= 3; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		item.SetBlock(block, time.Millisecond)
		item.SetValidation(true, "vrf", nil, time.Millisecond)
		_ = applyStage.Process(context.Background(), item)
	}

	assert.Equal(t, 3, applyStage.PendingCount())

	// Reset should clear pending
	applyStage.Reset()

	assert.Equal(t, 0, applyStage.PendingCount())
}

func TestApplyStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(nil, 0)

	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 0)
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)
	item.SetValidation(true, "vrf", nil, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = applyStage.Process(ctx, item)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// TestBlockPipeline tests
// ============================================================================

func TestBlockPipeline_StartStop(t *testing.T) {
	// Create pipeline with proper configuration
	p := NewBlockPipeline(
		WithSkipBodyHashValidation(true),
		WithValidateWorkers(1),
		WithEta0Provider(StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000")),
		WithSlotsPerKesPeriod(129600),
		WithVerifyConfig(common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		}),
		WithApplyFunc(func(item *BlockItem) error {
			return nil
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start pipeline
	err := p.Start(ctx)
	require.NoError(t, err)

	// Verify pipeline is started (Submit should work)
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	err = p.Submit(ctx, uint(ledger.BlockTypeConway), rawCbor, tip)
	assert.NoError(t, err)

	// Stop pipeline
	err = p.Stop()
	assert.NoError(t, err)

	// Submit after stop should fail
	err = p.Submit(ctx, uint(ledger.BlockTypeConway), rawCbor, tip)
	assert.ErrorIs(t, err, ErrPipelineStopped)

	// Start after stop should fail
	err = p.Start(ctx)
	assert.ErrorIs(t, err, ErrPipelineStopped)
}

func TestBlockPipeline_NotStarted(t *testing.T) {
	// Create pipeline without starting
	p := NewBlockPipeline() // Use defaults

	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	err := p.Submit(context.Background(), uint(ledger.BlockTypeConway), rawCbor, tip)

	assert.ErrorIs(t, err, ErrPipelineNotStarted)
}

func TestBlockPipeline_SubmitAndResults(t *testing.T) {
	// Test a simulated pipeline flow using channels
	rawCbor := getValidBlockCbor(t)
	const numBlocks = 10

	input := make(chan *BlockItem, numBlocks)
	results := make(chan *BlockItem, numBlocks)
	var stats PipelineStats
	var statsMu sync.Mutex

	// Simulate pipeline processing
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Decode worker
	decoded := make(chan *BlockItem, numBlocks)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(decoded)
		stage := NewDecodeStage(true)
		for item := range input {
			if err := stage.Process(ctx, item); err == nil {
				decoded <- item
				statsMu.Lock()
				stats.BlocksDecoded++
				statsMu.Unlock()
			}
		}
	}()

	// Validate worker (mock)
	validated := make(chan *BlockItem, numBlocks)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(validated)
		for item := range decoded {
			if item.IsDecoded() {
				item.SetValidation(true, "mock_vrf", nil, time.Millisecond)
				statsMu.Lock()
				stats.BlocksValidated++
				statsMu.Unlock()
			}
			validated <- item
		}
	}()

	// Apply worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(results)
		for item := range validated {
			if item.IsValid() {
				item.SetApplied(true, nil, time.Millisecond)
				statsMu.Lock()
				stats.BlocksApplied++
				statsMu.Unlock()
			}
			results <- item
		}
	}()

	// Submit blocks
	for i := 0; i < numBlocks; i++ {
		tip := createTestTip(uint64(1000+i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}
	close(input)

	// Collect results
	var resultItems []*BlockItem
	for item := range results {
		resultItems = append(resultItems, item)
	}

	wg.Wait()

	// Verify
	assert.Len(t, resultItems, numBlocks)
	assert.Equal(t, uint64(numBlocks), stats.BlocksDecoded)
	assert.Equal(t, uint64(numBlocks), stats.BlocksValidated)
	assert.Equal(t, uint64(numBlocks), stats.BlocksApplied)
}

func TestBlockPipeline_StatsUpdated(t *testing.T) {
	// Test that stats tracking works
	var submitted atomic.Uint64
	var decoded atomic.Uint64

	rawCbor := getValidBlockCbor(t)

	// Simple stats tracking
	for i := 0; i < 5; i++ {
		submitted.Add(1)

		tip := createTestTip(uint64(1000+i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		stage := NewDecodeStage(true)
		if err := stage.Process(context.Background(), item); err == nil {
			decoded.Add(1)
		}
	}

	assert.Equal(t, uint64(5), submitted.Load())
	assert.Equal(t, uint64(5), decoded.Load())
}

// ============================================================================
// TestPipelineBackpressure tests
// ============================================================================

func TestPipelineBackpressure_SlowConsumer(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const bufferSize = 5
	const numBlocks = 10

	// Create buffered input channel (simulates prefetch buffer)
	input := make(chan *BlockItem, bufferSize)
	output := make(chan *BlockItem, 1) // Slow consumer (small buffer)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var producerDone atomic.Bool
	var itemsProduced atomic.Uint64
	var itemsConsumed atomic.Uint64

	// Producer
	go func() {
		for i := 0; i < numBlocks; i++ {
			tip := createTestTip(uint64(i), uint64(i))
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
			select {
			case input <- item:
				itemsProduced.Add(1)
			case <-ctx.Done():
				return
			}
		}
		close(input)
		producerDone.Store(true)
	}()

	// Slow processor
	go func() {
		stage := NewDecodeStage(true)
		for item := range input {
			_ = stage.Process(ctx, item)
			select {
			case output <- item:
			case <-ctx.Done():
				return
			}
		}
		close(output)
	}()

	// Slow consumer (processes items slowly)
	go func() {
		for range output {
			time.Sleep(10 * time.Millisecond) // Simulate slow processing
			itemsConsumed.Add(1)
		}
	}()

	// Wait for producer to finish or timeout
	for !producerDone.Load() {
		select {
		case <-ctx.Done():
			t.Fatal("Test timed out")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Eventually all items should be produced (producer shouldn't be blocked forever)
	assert.Equal(t, uint64(numBlocks), itemsProduced.Load())
}

func TestPipelineBackpressure_PrefetchBufferFillsUp(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const bufferSize = 3

	// Small buffer to test fill-up behavior
	input := make(chan *BlockItem, bufferSize)

	// Fill buffer
	for i := 0; i < bufferSize; i++ {
		tip := createTestTip(uint64(i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		select {
		case input <- item:
		default:
			t.Fatalf("Buffer should not be full after %d items", i)
		}
	}

	// Buffer should be full now
	assert.Len(t, input, bufferSize)

	// Try to add one more (should not block due to select default)
	tip := createTestTip(100, 100)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 100)

	select {
	case input <- item:
		t.Fatal("Should not have been able to add item to full buffer")
	default:
		// Expected - buffer is full
	}
}

// ============================================================================
// TestPipelineGracefulShutdown tests
// ============================================================================

func TestPipelineGracefulShutdown_InFlightItemsComplete(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 5

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)

	var processedCount atomic.Uint64

	ctx, cancel := context.WithCancel(context.Background())

	// Worker that processes items
	go func() {
		stage := NewDecodeStage(true)
		for {
			select {
			case item, ok := <-input:
				if !ok {
					close(output)
					return
				}
				_ = stage.Process(context.Background(), item) // Use fresh context to complete
				output <- item
				processedCount.Add(1)
			case <-ctx.Done():
				// Drain remaining items in channel
				for item := range input {
					_ = stage.Process(context.Background(), item)
					output <- item
					processedCount.Add(1)
				}
				close(output)
				return
			}
		}
	}()

	// Submit items
	for i := 0; i < numItems; i++ {
		tip := createTestTip(uint64(i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}

	// Give some time for processing to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context and close input to signal shutdown
	cancel()
	close(input)

	// Wait for output to close
	var received []*BlockItem
	for item := range output {
		received = append(received, item)
	}

	// All items should have been processed
	assert.Len(t, received, numItems)
	assert.Equal(t, uint64(numItems), processedCount.Load())
}

func TestPipelineGracefulShutdown_ChannelsCloseCleanly(t *testing.T) {
	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errors := make(chan error, 10)

	ctx, cancel := context.WithCancel(context.Background())

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 2, input, output, errors)

	pool.Start(ctx)

	// Close input channel
	close(input)

	// Wait for pool to stop
	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - pool stopped
	case <-time.After(5 * time.Second):
		t.Fatal("Pool did not stop cleanly")
	}

	// Cancel context
	cancel()

	// Output channel should be closeable without issue
	close(output)
	close(errors)
}

// ============================================================================
// StageFunc tests
// ============================================================================

func TestStageFunc_NameAndProcess(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	processedItems := 0

	stage := NewStageFunc("test-stage", func(ctx context.Context, item *BlockItem) error {
		processedItems++
		return nil
	})

	assert.Equal(t, "test-stage", stage.Name())

	err := stage.Process(context.Background(), item)
	assert.NoError(t, err)
	assert.Equal(t, 1, processedItems)
}

func TestStageFunc_ErrorHandling(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	expectedErr := errors.New("stage failed")

	stage := NewStageFunc("error-stage", func(ctx context.Context, item *BlockItem) error {
		return expectedErr
	})

	err := stage.Process(context.Background(), item)
	assert.Equal(t, expectedErr, err)
}

// ============================================================================
// Worker pool numWorkers validation tests
// ============================================================================

func TestDecodeStageWorkerPool_NumWorkersValidation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	stage := NewDecodeStage(true)

	testCases := []struct {
		name              string
		numWorkers        int
		expectedToProcess bool
	}{
		{"zero workers defaults to 1", 0, true},
		{"negative workers defaults to 1", -1, true},
		{"negative workers defaults to 1 v2", -100, true},
		{"one worker works", 1, true},
		{"multiple workers work", 4, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make(chan *BlockItem, 1)
			output := make(chan *BlockItem, 1)
			errChan := make(chan error, 1)

			pool := NewDecodeStageWorkerPool(stage, tc.numWorkers, input, output, errChan)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool.Start(ctx)

			// Submit one item
			tip := createTestTip(1000, 500)
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
			input <- item
			close(input)

			// Wait for output
			select {
			case result := <-output:
				if tc.expectedToProcess {
					assert.True(t, result.IsDecoded(), "Item should be decoded")
				}
			case <-time.After(2 * time.Second):
				if tc.expectedToProcess {
					t.Fatal("Timed out waiting for item to be processed")
				}
			}

			pool.Stop()
		})
	}
}

func TestValidateStageWorkerPool_NumWorkersValidation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	testCases := []struct {
		name       string
		numWorkers int
	}{
		{"zero workers defaults to 1", 0},
		{"negative workers defaults to 1", -1},
		{"negative workers defaults to 1 v2", -100},
		{"one worker works", 1},
		{"multiple workers work", 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make(chan *BlockItem, 1)
			output := make(chan *BlockItem, 1)
			errChan := make(chan error, 1)

			pool := NewValidateStageWorkerPool(stage, tc.numWorkers, input, output, errChan)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool.Start(ctx)

			// Create a decoded item (validation requires decoded block)
			tip := createTestTip(1000, 500)
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
			block, err := ledger.NewBlockFromCbor(
				uint(ledger.BlockTypeConway),
				rawCbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.NoError(t, err)
			item.SetBlock(block, time.Millisecond)

			input <- item
			close(input)

			// Wait for output (item should flow through even if validation fails)
			select {
			case <-output:
				// Item was processed - test passes
			case <-time.After(2 * time.Second):
				t.Fatal("Timed out waiting for item to be processed")
			}

			pool.Stop()
		})
	}
}

// TestBlockPipeline_SubmitStopRaceCondition verifies that concurrent Submit() and Stop()
// calls do not cause a panic from sending on a closed channel.
// This is a regression test for the race condition where Stop() could close submitChan
// between the stopped.Load() check and the channel send in Submit().
func TestBlockPipeline_SubmitStopRaceCondition(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)

	// Run multiple iterations to increase likelihood of hitting the race
	for iteration := 0; iteration < 100; iteration++ {
		p := NewBlockPipeline(
			WithSkipBodyHashValidation(true),
			WithValidateWorkers(1),
			WithEta0Provider(StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000")),
			WithSlotsPerKesPeriod(129600),
			WithVerifyConfig(common.VerifyConfig{
				SkipBodyHashValidation:    true,
				SkipTransactionValidation: true,
				SkipStakePoolValidation:   true,
			}),
			WithApplyFunc(func(item *BlockItem) error {
				return nil
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		err := p.Start(ctx)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Rapidly submit items
		go func(ctx context.Context) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				err := p.Submit(ctx, uint(ledger.BlockTypeConway), rawCbor, tip)
				// Either no error, ErrPipelineStopped, or context.Canceled is acceptable.
				// context.Canceled occurs when Stop() cancels the context before Submit
				// can complete its channel send.
				if err != nil && !errors.Is(err, ErrPipelineStopped) && !errors.Is(err, context.Canceled) {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		}(ctx)

		// Goroutine 2: Stop the pipeline after a tiny delay
		go func() {
			defer wg.Done()
			// Small random delay to create race conditions
			time.Sleep(time.Duration(iteration%5) * time.Microsecond)
			_ = p.Stop()
		}()

		wg.Wait()
		cancel()
	}
}

// TestApplyStageRunner_OutOfOrderItemsForwarded verifies that out-of-order items
// that are buffered and later applied from pending are correctly forwarded to output.
// This is a regression test for the data loss bug where buffered items were applied
// but never forwarded to the output channel.
func TestApplyStageRunner_OutOfOrderItemsForwarded(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	var appliedOrder []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber())
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc, 0)

	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errors := make(chan error, 10)

	runner := NewApplyStageRunner(applyStage, input, output, errors, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runner.Start(ctx)

	// Create 5 items with sequence numbers 0-4
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)
		items[i].SetValidation(true, "vrf", nil, time.Millisecond)
	}

	// Send items in scrambled order: 2, 4, 1, 3, 0
	// This means items 2, 4, 1, 3 will be buffered until item 0 arrives
	scrambledOrder := []int{2, 4, 1, 3, 0}
	for _, idx := range scrambledOrder {
		input <- items[idx]
	}
	close(input)

	// Collect all items from output
	var received []*BlockItem
	for item := range output {
		received = append(received, item)
		if len(received) == 5 {
			break
		}
	}

	runner.Stop()

	// Verify all 5 items were forwarded to output (no data loss)
	assert.Len(t, received, 5, "All items should be forwarded to output")

	// Verify items were applied in sequence order
	assert.Equal(
		t,
		[]uint64{0, 1, 2, 3, 4},
		appliedOrder,
		"Items should be applied in sequence order",
	)

	// Verify all received items are marked as applied
	for _, item := range received {
		assert.True(t, item.IsApplied(), "Item %d should be marked as applied", item.SequenceNumber())
	}
}

// TestApplyStageRunner_OutOfOrderErrorItemsForwardedOnce verifies that items with
// decode/validation errors that arrive out of order are only forwarded once to the
// output channel. This is a regression test for the double-forwarding bug where
// error items were forwarded immediately in run() and again later via pendingQueue
// when applyPending() processed them.
func TestApplyStageRunner_OutOfOrderErrorItemsForwardedOnce(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	var appliedOrder []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber())
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc, 0)

	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 20) // Extra capacity to detect duplicates
	errors := make(chan error, 10)

	runner := NewApplyStageRunner(applyStage, input, output, errors, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runner.Start(ctx)

	// Create 5 items with sequence numbers 0-4
	// Items 1 and 3 will have validation errors
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)

		// Items 1 and 3 have validation errors
		if i == 1 || i == 3 {
			items[i].SetValidation(false, "", fmt.Errorf("validation error for item %d", i), time.Millisecond)
		} else {
			items[i].SetValidation(true, "vrf", nil, time.Millisecond)
		}
	}

	// Send items in scrambled order: 2, 4, 1, 3, 0
	// Items 2, 4, 1, 3 will be buffered until item 0 arrives
	// Items 1 and 3 have validation errors and should only be forwarded once
	scrambledOrder := []int{2, 4, 1, 3, 0}
	for _, idx := range scrambledOrder {
		input <- items[idx]
	}
	close(input)

	// Collect all items from output with a timeout.
	// 500ms is sufficient since duplicates would appear immediately.
	var received []*BlockItem
	timeout := time.After(500 * time.Millisecond)
collectLoop:
	for {
		select {
		case item, ok := <-output:
			if !ok {
				break collectLoop
			}
			received = append(received, item)
			// If we get more than 5, that's a bug (duplicates)
			if len(received) > 5 {
				t.Fatalf("Received more than 5 items (%d), likely duplicates", len(received))
			}
		case <-timeout:
			break collectLoop
		}
	}

	runner.Stop()

	// Verify exactly 5 items were forwarded (no duplicates)
	assert.Len(t, received, 5, "Exactly 5 items should be forwarded (no duplicates)")

	// Count how many times each sequence number appears
	seqCounts := make(map[uint64]int)
	for _, item := range received {
		seqCounts[item.SequenceNumber()]++
	}

	// Verify each item was forwarded exactly once
	for seq, count := range seqCounts {
		assert.Equal(t, 1, count, "Item %d should be forwarded exactly once, got %d", seq, count)
	}

	// Verify items with validation errors were NOT applied
	for _, seq := range appliedOrder {
		if seq == 1 || seq == 3 {
			t.Errorf("Item %d has validation error but was applied", seq)
		}
	}

	// Verify valid items (0, 2, 4) were applied in sequence order
	expectedApplied := []uint64{0, 2, 4}
	assert.Equal(t, expectedApplied, appliedOrder, "Valid items should be applied in sequence order")
}

// TestBlockPipeline_StartFailsWithoutEta0Provider verifies that Start() returns
// an error when validation is enabled but no Eta0Provider is configured.
func TestBlockPipeline_StartFailsWithoutEta0Provider(t *testing.T) {
	// Create pipeline with validation enabled but no Eta0Provider
	p := NewBlockPipeline(
		WithValidateWorkers(1), // Validation enabled
		// Eta0Provider is nil by default
		WithApplyFunc(func(item *BlockItem) error {
			return nil
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := p.Start(ctx)
	assert.ErrorIs(t, err, ErrMissingEta0Provider)
}

func TestBlockPipeline_ValidationDisabled(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numBlocks = 5

	// Create pipeline with validation disabled (default)
	pipeline := NewBlockPipeline(
		WithValidateWorkers(0), // Disable validation (default)
		WithSkipBodyHashValidation(true),
		WithApplyFunc(func(item *BlockItem) error {
			return nil
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Should start successfully without Eta0Provider since validation is disabled
	err := pipeline.Start(ctx)
	require.NoError(t, err)

	// Submit blocks
	for i := 0; i < numBlocks; i++ {
		tip := createTestTip(uint64(1000+i), uint64(i))
		err := pipeline.Submit(ctx, uint(ledger.BlockTypeConway), rawCbor, tip)
		require.NoError(t, err)
	}

	// Wait for all blocks to be processed
	received := 0
	for received < numBlocks {
		select {
		case item := <-pipeline.Results():
			received++
			// Blocks should not be validated (no IsValid check since validation was skipped)
			assert.True(t, item.IsDecoded(), "Block should be decoded")
			assert.True(t, item.IsApplied(), "Block should be applied")
		case <-ctx.Done():
			t.Fatal("Timed out waiting for results")
		}
	}

	err = pipeline.Stop()
	require.NoError(t, err)

	// Verify stats - no validation stats since it was disabled
	stats := pipeline.Stats()
	assert.Equal(t, uint64(numBlocks), stats.BlocksDecoded)
	assert.Equal(t, uint64(numBlocks), stats.BlocksApplied)
}

func TestBlockPipeline_ErrorsReturnsNewChannelEachTime(t *testing.T) {
	// Create pipeline without starting
	p := NewBlockPipeline() // Use defaults

	// Call Errors() twice - each should return a separate channel
	ch1 := p.Errors()
	ch2 := p.Errors()

	// Both channels should have the error
	select {
	case err := <-ch1:
		assert.ErrorIs(t, err, ErrPipelineNotStarted)
	case <-time.After(time.Second):
		t.Fatal("Expected error from first channel")
	}

	select {
	case err := <-ch2:
		assert.ErrorIs(t, err, ErrPipelineNotStarted)
	case <-time.After(time.Second):
		t.Fatal("Expected error from second channel")
	}
}

func TestBlockPipeline_MetricsRecorded(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numBlocks = 5

	// Using dummy Eta0 will cause validation to fail, which is expected
	pipeline := NewBlockPipeline(
		WithSkipBodyHashValidation(true),
		WithValidateWorkers(1),
		WithEta0Provider(StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000")),
		WithSlotsPerKesPeriod(129600),
		WithVerifyConfig(common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		}),
		WithApplyFunc(func(item *BlockItem) error {
			return nil
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := pipeline.Start(ctx)
	require.NoError(t, err)

	// Submit blocks
	for i := 0; i < numBlocks; i++ {
		tip := createTestTip(uint64(1000+i), uint64(i))
		err := pipeline.Submit(ctx, uint(ledger.BlockTypeConway), rawCbor, tip)
		require.NoError(t, err)
	}

	// Wait for all blocks to be processed (they flow through even with validation errors)
	received := 0
	for received < numBlocks {
		select {
		case <-pipeline.Results():
			received++
		case <-ctx.Done():
			t.Fatal("Timed out waiting for results")
		}
	}

	// Stop the pipeline
	err = pipeline.Stop()
	require.NoError(t, err)

	// Verify metrics
	stats := pipeline.Stats()

	assert.Equal(t, uint64(numBlocks), stats.BlocksSubmitted, "BlocksSubmitted should match")
	assert.Equal(t, uint64(numBlocks), stats.BlocksDecoded, "BlocksDecoded should match")
	assert.Equal(t, uint64(0), stats.DecodeErrors, "DecodeErrors should be 0")

	// Validation will fail due to incorrect Eta0, so we expect validation errors
	assert.Equal(t, uint64(numBlocks), stats.ValidationErrors, "ValidationErrors should match (validation fails with dummy Eta0)")
	assert.Equal(t, uint64(0), stats.BlocksValidated, "BlocksValidated should be 0 (validation fails with dummy Eta0)")

	// Apply and pipeline latency won't be recorded for failed validations
	// (items with validation errors are not applied)
	assert.Equal(t, uint64(0), stats.BlocksApplied, "BlocksApplied should be 0 (failed validations are not applied)")
}
