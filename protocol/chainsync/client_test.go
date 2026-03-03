// Copyright 2025 Blink Labs Software
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

package chainsync_test

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var conversationHandshakeFindIntersect = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  chainsync.ProtocolIdNtC,
		MessageType: chainsync.MessageTypeFindIntersect,
	},
}

type testInnerFunc func(*testing.T, *ouroboros.Connection)

func runTest(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
	options ...ouroboros.ConnectionOptionFunc,
) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
		close(asyncErrChan)
	}()
	// Build options list
	opts := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	}
	opts = append(opts, options...)

	oConn, err := ouroboros.New(opts...)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	// Async error handler
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		// We can't call t.Fatalf() from a different Goroutine, so we panic instead
		panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
	}()
	// Run test inner function
	innerFunc(t, oConn)
	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close Ouroboros connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Ouroboros object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestIntersectNotFound(t *testing.T) {
	conversation := append(
		conversationHandshakeFindIntersect,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(
					chainsync.Tip{
						// NOTE: these values don't matter
						BlockNumber: 12345,
						Point:       pcommon.NewPointOrigin(),
					},
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			// Start sync with "bad" intersect points
			err := oConn.ChainSync().Client.Sync([]pcommon.Point{})
			if err == nil {
				t.Fatalf("did not receive expected error")
			}
			if !errors.Is(err, chainsync.ErrIntersectNotFound) {
				t.Fatalf(
					"did not receive expected error\n  got:    %s\n  wanted: %s",
					err,
					chainsync.ErrIntersectNotFound,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

func TestGetCurrentTip(t *testing.T) {
	expectedTip := chainsync.Tip{
		BlockNumber: 12345,
		Point: pcommon.NewPoint(
			23456,
			test.DecodeHexString("0123456789abcdef"),
		),
	}
	conversation := append(
		conversationHandshakeFindIntersect,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			tip, err := oConn.ChainSync().Client.GetCurrentTip()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip, &expectedTip) {
				t.Fatalf(
					"did not receive expected tip\n  got:    %#v\n  wanted: %#v",
					tip,
					expectedTip,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

func TestGetAvailableBlockRange(t *testing.T) {
	expectedIntersect := pcommon.NewPoint(
		20001,
		test.DecodeHexString("123456789abcdef0"),
	)
	expectedTip := chainsync.Tip{
		BlockNumber: 12345,
		Point: pcommon.NewPoint(
			23456,
			test.DecodeHexString("0123456789abcdef"),
		),
	}
	// Create basic block and round-trip it through the CBOR encoder to get the hash populated
	// The slot value is one higher than our intersect point and the block height is less than
	// our expected tip
	testBlock := ledger.BabbageBlock{
		BlockHeader: &ledger.BabbageBlockHeader{},
	}
	testBlock.BlockHeader.Body.BlockNumber = 12001
	testBlock.BlockHeader.Body.Slot = 20002
	blockCbor, err := cbor.Encode(testBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	if _, err := cbor.Decode(blockCbor, &testBlock); err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	expectedStart := pcommon.NewPoint(
		testBlock.SlotNumber(),
		testBlock.Hash().Bytes(),
	)

	rollForwardMsg, err := chainsync.NewMsgRollForwardNtC(
		ledger.BlockTypeBabbage,
		blockCbor,
		expectedTip,
	)
	if err != nil {
		t.Fatalf("failed to create RollForward message: %s", err)
	}

	conversation := append(
		conversationHandshakeFindIntersect,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectFound(expectedIntersect, expectedTip),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgRollBackward(expectedIntersect, expectedTip),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				rollForwardMsg,
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange(
				[]pcommon.Point{expectedIntersect},
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(start, expectedStart) {
				t.Fatalf(
					"did not receive expected start point\n  got:    %#v\n  wanted: %#v",
					start,
					expectedStart,
				)
			}
			if !reflect.DeepEqual(end, expectedTip.Point) {
				t.Fatalf(
					"did not receive expected end point\n  got:    %#v\n  wanted: %#v",
					end,
					expectedTip.Point,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

func TestClientStartStopStart(t *testing.T) {
	conversation := append(
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
		// Stop() should send Done once started
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeDone,
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.ChainSync().Client
			// Start should be idempotent
			client.Start()
			client.Start()
			// Stop should work
			if err := client.Stop(); err != nil {
				t.Fatalf("unexpected error when stopping client: %s", err)
			}
			// Start again after stop should work (#597)
			client.Start()
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
		// Ensure we control protocol startup in the test.
		ouroboros.WithDelayProtocolStart(true),
	)
}

func TestUseCase_GetCurrentTip_Stop_Start_GetCurrentTip(t *testing.T) {
	expectedTip1 := chainsync.Tip{
		BlockNumber: 111,
		Point: pcommon.NewPoint(
			222,
			test.DecodeHexString("0123456789abcdef"),
		),
	}
	expectedTip2 := chainsync.Tip{
		BlockNumber: 333,
		Point: pcommon.NewPoint(
			444,
			test.DecodeHexString("fedcba9876543210"),
		),
	}
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		// First GetCurrentTip (FindIntersect -> IntersectNotFound(tip))
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip1),
			},
		},
		// Stop should send Done
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeDone,
		},
		// Second GetCurrentTip after restart
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip2),
			},
		},
	}

	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.ChainSync().Client
			// Ensure lifecycle state is running (connection may already auto-start).
			client.Start()

			tip1, err := client.GetCurrentTip()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip1, &expectedTip1) {
				t.Fatalf(
					"did not receive expected tip1\n  got:    %#v\n  wanted: %#v",
					tip1,
					&expectedTip1,
				)
			}

			if err := client.Stop(); err != nil {
				t.Fatalf("unexpected error when stopping client: %s", err)
			}
			client.Start()

			tip2, err := client.GetCurrentTip()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip2, &expectedTip2) {
				t.Fatalf(
					"did not receive expected tip2\n  got:    %#v\n  wanted: %#v",
					tip2,
					&expectedTip2,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

func TestSyncPipelining(t *testing.T) {
	const pipelineLimit = 5
	// We need at least 3 blocks: 1 to trigger pipelining, then 2 more to verify
	// the pipelined requests are being satisfied
	const totalBlocks = 4

	expectedIntersect := pcommon.NewPoint(
		0,
		test.DecodeHexString("0000000000000000"),
	)

	// Create test blocks
	testBlocks := make([]ledger.BabbageBlock, totalBlocks)
	blockCbors := make([][]byte, totalBlocks)
	for i := 0; i < totalBlocks; i++ {
		testBlocks[i] = ledger.BabbageBlock{
			BlockHeader: &ledger.BabbageBlockHeader{},
		}
		testBlocks[i].BlockHeader.Body.Slot = uint64(1000 + i)
		testBlocks[i].BlockHeader.Body.BlockNumber = uint64(100 + i)
		var err error
		blockCbors[i], err = cbor.Encode(testBlocks[i])
		if err != nil {
			t.Fatalf("failed to encode test block %d: %s", i, err)
		}
		if _, err := cbor.Decode(blockCbors[i], &testBlocks[i]); err != nil {
			t.Fatalf("failed to decode test block %d: %s", i, err)
		}
	}

	finalTip := chainsync.Tip{
		BlockNumber: uint64(100 + totalBlocks - 1),
		Point: pcommon.NewPoint(
			uint64(1000+totalBlocks-1),
			testBlocks[totalBlocks-1].Hash().Bytes(),
		),
	}

	makeRollForward := func(idx int) protocol.Message {
		msg, err := chainsync.NewMsgRollForwardNtC(
			ledger.BlockTypeBabbage,
			blockCbors[idx],
			finalTip,
		)
		if err != nil {
			t.Fatalf("failed to create RollForward message for block %d: %s", idx, err)
		}
		return msg
	}

	conversation := []ouroboros_mock.ConversationEntry{
		// Handshake
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		// FindIntersect -> IntersectFound
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectFound(expectedIntersect, finalTip),
			},
		},
		// Initial RequestNext from Sync()
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		// First block
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{makeRollForward(0)},
		},
		// Second RequestNext, first of pipelined batch
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		// Second Block
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{makeRollForward(1)},
		},
		// Third RequestNext, second of pipelined batch
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		// Third block
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{makeRollForward(2)},
		},
		// Fourth RequestNext, third of pipelined batch
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		// Fourth block
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{makeRollForward(3)},
		},
		// Fifth RequestNext, fourth of pipelined batch
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		// Signal that we're at tip
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgAwaitReply(),
			},
		},
	}

	receivedBlocks := make([]uint64, 0, totalBlocks)
	var receivedMu sync.Mutex

	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			doneChan := make(chan struct{})
			// Wait until we've received enough blocks and end the test
			go func() {
				for {
					receivedMu.Lock()
					count := len(receivedBlocks)
					receivedMu.Unlock()
					if count >= totalBlocks {
						// Signal to test that we've received all blocks
						close(doneChan)
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			}()

			err := oConn.ChainSync().Client.Sync([]pcommon.Point{expectedIntersect})
			if err != nil {
				t.Fatalf("unexpected error starting sync: %s", err)
			}

			select {
			case <-doneChan:
				// Success
			case <-time.After(5 * time.Second):
				receivedMu.Lock()
				defer receivedMu.Unlock()
				t.Fatalf("timeout waiting for blocks, received %d of %d", len(receivedBlocks), totalBlocks)
			}

			// Verify blocks received in order
			receivedMu.Lock()
			defer receivedMu.Unlock()
			if len(receivedBlocks) != totalBlocks {
				t.Fatalf("expected %d blocks, received %d", totalBlocks, len(receivedBlocks))
			}
			for i := 0; i < totalBlocks; i++ {
				expectedSlot := uint64(1000 + i)
				if receivedBlocks[i] != expectedSlot {
					t.Fatalf("block %d: expected slot %d, got %d", i, expectedSlot, receivedBlocks[i])
				}
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{
				PipelineLimit:       pipelineLimit,
				SkipBlockValidation: true,
				RollForwardFunc: func(ctx chainsync.CallbackContext, blockType uint, block any, tip chainsync.Tip) error {
					b := block.(ledger.Block)
					receivedMu.Lock()
					receivedBlocks = append(receivedBlocks, b.SlotNumber())
					count := len(receivedBlocks)
					receivedMu.Unlock()
					if count >= totalBlocks {
						return chainsync.ErrStopSyncProcess
					}
					return nil
				},
			},
		),
	)
}

func TestUseCase_MultiCycle_GetCurrentTip_Stop_Start(t *testing.T) {
	expectedTip1 := chainsync.Tip{
		BlockNumber: 1,
		Point: pcommon.NewPoint(
			10,
			test.DecodeHexString("0102030405060708"),
		),
	}
	expectedTip2 := chainsync.Tip{
		BlockNumber: 2,
		Point: pcommon.NewPoint(
			20,
			test.DecodeHexString("1112131415161718"),
		),
	}
	expectedTip3 := chainsync.Tip{
		BlockNumber: 3,
		Point: pcommon.NewPoint(
			30,
			test.DecodeHexString("2122232425262728"),
		),
	}

	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		// cycle 1
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip1),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeDone,
		},
		// cycle 2
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip2),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeDone,
		},
		// cycle 3
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeFindIntersect,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectNotFound(expectedTip3),
			},
		},
	}

	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.ChainSync().Client
			client.Start()

			tip1, err := client.GetCurrentTip()
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip1, &expectedTip1) {
				t.Fatalf(
					"unexpected tip1\n  got:    %#v\n  wanted: %#v",
					tip1,
					&expectedTip1,
				)
			}
			if err := client.Stop(); err != nil {
				t.Fatalf("unexpected stop error: %s", err)
			}
			client.Start()

			tip2, err := client.GetCurrentTip()
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip2, &expectedTip2) {
				t.Fatalf(
					"unexpected tip2\n  got:    %#v\n  wanted: %#v",
					tip2,
					&expectedTip2,
				)
			}
			if err := client.Stop(); err != nil {
				t.Fatalf("unexpected stop error: %s", err)
			}
			client.Start()

			tip3, err := client.GetCurrentTip()
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tip3, &expectedTip3) {
				t.Fatalf(
					"unexpected tip3\n  got:    %#v\n  wanted: %#v",
					tip3,
					&expectedTip3,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

// TestStopAfterConnectionClose verifies that Stop() does not return an error
// when called after the connection has already been closed.
// This tests the fix for not returning error on connection close.
func TestStopAfterConnectionClose(t *testing.T) {
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		// No Done message expected because connection will be closed first
	}
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
		close(asyncErrChan)
	}()

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithDelayProtocolStart(true),
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
	require.NoError(t, err, "unexpected error when creating Ouroboros object")

	client := oConn.ChainSync().Client
	client.Start()

	// Close the connection first (simulating remote close)
	require.NoError(t, oConn.Close(), "unexpected error when closing connection")

	// Wait for connection to fully close
	select {
	case <-oConn.ErrorChan():
	case <-time.After(5 * time.Second):
		require.Fail(t, "connection did not close within timeout")
	}

	// Now Stop() should not return an error even though connection is closed
	require.NoError(t, client.Stop(), "Stop() should not return error after connection close")

	// Verify IsDone returns true
	require.True(t, client.IsDone(), "IsDone() should return true after connection close")

	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			require.Fail(t, err.Error())
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "mock connection did not shut down within timeout")
	}
}
