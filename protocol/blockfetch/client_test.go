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

package blockfetch_test

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var conversationHandshakeRequestRange = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtNResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  blockfetch.ProtocolId,
		MessageType: blockfetch.MessageTypeRequestRange,
	},
}

type testInnerFunc func(*testing.T, *ouroboros.Connection)

func runTest(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
	options ...ouroboros.ConnectionOptionFunc,
) {
	runTestWithExpectedProtocolError(t, conversation, innerFunc, nil, options...)
}

func runTestWithExpectedProtocolError(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
	expectedErr error,
	options ...ouroboros.ConnectionOptionFunc,
) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	mockDone := make(chan struct{})
	go func() {
		defer close(asyncErrChan)
		defer close(mockDone)
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
	}()
	// Build options list
	opts := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
	}
	opts = append(opts, options...)

	oConn, err := ouroboros.New(opts...)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	connDone := make(chan struct{})
	var connErrsMu sync.Mutex
	var connErrs []error
	go func() {
		defer close(connDone)
		for err := range oConn.ErrorChan() {
			if err != nil {
				connErrsMu.Lock()
				connErrs = append(connErrs, fmt.Errorf(
					"unexpected Ouroboros error: %w", err,
				))
				connErrsMu.Unlock()
			}
		}
	}()
	defer func() {
		if err := oConn.Close(); err != nil {
			t.Errorf("unexpected error when closing Ouroboros object: %s", err)
		}
		select {
		case <-connDone:
		case <-time.After(10 * time.Second):
			t.Error("did not shutdown within timeout")
		}
		connErrsMu.Lock()
		for _, err := range connErrs {
			if expectedErr != nil && strings.Contains(err.Error(), expectedErr.Error()) {
				continue
			}
			t.Error(err)
		}
		connErrsMu.Unlock()
		select {
		case <-mockDone:
		case <-time.After(10 * time.Second):
			t.Error("mock connection did not shut down within timeout")
		}
		for {
			select {
			case err, ok := <-asyncErrChan:
				if !ok {
					return
				}
				if expectedErr != nil && strings.Contains(err.Error(), expectedErr.Error()) {
					continue
				}
				t.Error(err)
			default:
				return
			}
		}
	}()
	// Run test inner function
	innerFunc(t, oConn)
	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok && (expectedErr == nil || !strings.Contains(err.Error(), expectedErr.Error())) {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
}

func TestGetBlock(t *testing.T) {
	var testBlockSlot uint64 = 23456
	var testBlockNumber uint64 = 12345
	// Create basic block and round-trip it through the CBOR encoder to get the hash populated
	// The slot value is one higher than our intersect point and the block height is less than
	// our expected tip
	testBlock := ledger.BabbageBlock{
		BlockHeader: &ledger.BabbageBlockHeader{},
	}
	testBlock.BlockHeader.Body.BlockNumber = testBlockNumber
	testBlock.BlockHeader.Body.Slot = testBlockSlot
	blockCbor, err := cbor.Encode(testBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	if _, err := cbor.Decode(blockCbor, &testBlock); err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	testBlockHash := testBlock.Hash().Bytes()
	wrappedBlock := blockfetch.WrappedBlock{
		Type:     ledger.BlockTypeBabbage,
		RawBlock: cbor.RawMessage(blockCbor),
	}
	wrappedBlockCbor, err := cbor.Encode(wrappedBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(
					wrappedBlockCbor,
				),
				blockfetch.NewMsgBatchDone(),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			blk, err := oConn.BlockFetch().Client.GetBlock(
				pcommon.NewPoint(
					testBlockSlot,
					testBlockHash,
				),
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if blk.Hash() != testBlock.Hash() {
				t.Fatalf(
					"did not receive expected block hash: got %s, wanted %s",
					blk.Hash(),
					testBlock.Hash(),
				)
			}
			if blk.SlotNumber() != testBlockSlot {
				t.Fatalf(
					"did not receive expected block slot: got %d, wanted %d",
					blk.SlotNumber(),
					testBlockSlot,
				)
			}
		},
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
	)
}

func TestGetBlockByronConfiguredEpochLength(t *testing.T) {
	blockCbor, block := byronBlockWithSlotID(t, 2, 17)
	const configuredSlotsPerEpoch = 600
	wantSlot := block.BlockHeader.ConsensusData.SlotId.Epoch*configuredSlotsPerEpoch +
		block.BlockHeader.ConsensusData.SlotId.Slot
	require.NotEqual(t, block.BlockHeader.SlotNumber(), wantSlot)

	wrappedBlockCbor, err := cbor.Encode(blockfetch.WrappedBlock{
		Type:     ledger.BlockTypeByronMain,
		RawBlock: cbor.RawMessage(blockCbor),
	})
	require.NoError(t, err)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(wrappedBlockCbor),
				blockfetch.NewMsgBatchDone(),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			got, err := oConn.BlockFetch().Client.GetBlock(
				pcommon.NewPoint(wantSlot, block.Hash().Bytes()),
			)
			require.NoError(t, err)
			require.Equal(t, block.Hash(), got.Hash())
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			ByronSlotsPerEpoch:  configuredSlotsPerEpoch,
		}),
	)
}

func TestGetBlockByronConfiguredEpochLengthOverflow(t *testing.T) {
	blockCbor, _ := byronBlockWithSlotID(t, math.MaxUint64, 1)
	wrappedBlockCbor, err := cbor.Encode(blockfetch.WrappedBlock{
		Type:     ledger.BlockTypeByronMain,
		RawBlock: cbor.RawMessage(blockCbor),
	})
	require.NoError(t, err)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(wrappedBlockCbor),
				blockfetch.NewMsgBatchDone(),
			},
		},
	)
	runTestWithExpectedProtocolError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.BlockFetch().Client.GetBlock(
				pcommon.NewPoint(1, testPointHash(0xab)),
			)
			require.ErrorIs(t, err, byron.ErrByronSlotNumberOverflow)
		},
		byron.ErrByronSlotNumberOverflow,
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			ByronSlotsPerEpoch:  600,
		}),
	)
}

func byronBlockWithSlotID(t *testing.T, epoch, slot uint64) ([]byte, *byron.ByronMainBlock) {
	t.Helper()
	fixtureHex, err := os.ReadFile("../chainsync/testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex")
	require.NoError(t, err)
	blockCbor, err := hex.DecodeString(strings.TrimSpace(string(fixtureHex)))
	require.NoError(t, err)
	block, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeByronMain,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	byronBlock, ok := block.(*byron.ByronMainBlock)
	require.True(t, ok)
	byronBlock.BlockHeader.ConsensusData.SlotId.Epoch = epoch
	byronBlock.BlockHeader.ConsensusData.SlotId.Slot = slot
	byronBlock.BlockHeader.SetCbor(nil)
	byronBlock.SetCbor(nil)
	blockCbor, err = cbor.Encode(byronBlock)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeByronMain,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return blockCbor, decoded.(*byron.ByronMainBlock)
}

func TestGetBlockNoBlocks(t *testing.T) {
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgNoBlocks(),
			},
		},
	)
	expectedErr := `block(s) not found`
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.BlockFetch().Client.GetBlock(
				pcommon.NewPoint(
					12345,
					testPointHash(0xab),
				),
			)
			if err == nil {
				t.Fatalf("did not receive expected error")
			}
			if err.Error() != expectedErr {
				t.Fatalf(
					"did not receive expected error\n  got:    %s\n  wanted: %s",
					err,
					expectedErr,
				)
			}
		},
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
	)
}

func TestClientStartStopStart(t *testing.T) {
	conversation := append(
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		},
		// Stop() should send ClientDone once started
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  blockfetch.ProtocolId,
			MessageType: blockfetch.MessageTypeClientDone,
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			client := oConn.BlockFetch().Client
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
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
		// Ensure we control protocol startup in the test.
		ouroboros.WithDelayProtocolStart(true),
	)
}

// TestStopAfterConnectionClose verifies that Stop() does not return an error
// when called after the connection has already been closed.
// This tests the fix for not returning error on connection close.
func TestStopAfterConnectionClose(t *testing.T) {
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		// No ClientDone expected because connection will be closed first
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
		ouroboros.WithNodeToNode(true),
		ouroboros.WithDelayProtocolStart(true),
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
	)
	require.NoError(t, err, "unexpected error when creating Ouroboros object")

	client := oConn.BlockFetch().Client
	client.Start()

	// Close the connection first (simulating remote close)
	require.NoError(
		t,
		oConn.Close(),
		"unexpected error when closing connection",
	)

	// Wait for connection to fully close
	select {
	case <-oConn.ErrorChan():
	case <-time.After(5 * time.Second):
		require.Fail(t, "connection did not close within timeout")
	}

	// Now Stop() should not return an error even though connection is closed
	require.NoError(
		t,
		client.Stop(),
		"Stop() should not return error after connection close",
	)

	// Verify IsDone returns true
	require.True(
		t,
		client.IsDone(),
		"IsDone() should return true after connection close",
	)

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

func TestStopAfterConnectionCloseStress(t *testing.T) {
	defer goleak.VerifyNone(t)

	for i := range 20 {
		t.Run(fmt.Sprintf("iteration_%02d", i), func(t *testing.T) {
			conversation := []ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
				ouroboros_mock.ConversationEntryHandshakeNtNResponse,
			}
			mockConn := ouroboros_mock.NewConnection(
				ouroboros_mock.ProtocolRoleClient,
				conversation,
			)
			asyncErrChan := make(chan error, 1)
			go func() {
				err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
				if err != nil {
					asyncErrChan <- fmt.Errorf(
						"received unexpected error: %w",
						err,
					)
				}
				close(asyncErrChan)
			}()

			oConn, err := ouroboros.New(
				ouroboros.WithConnection(mockConn),
				ouroboros.WithNetworkMagic(
					ouroboros_mock.MockNetworkMagic,
				),
				ouroboros.WithNodeToNode(true),
				ouroboros.WithDelayProtocolStart(true),
				ouroboros.WithBlockFetchConfig(
					blockfetch.Config{SkipBlockValidation: true},
				),
			)
			require.NoError(t, err)

			client := oConn.BlockFetch().Client
			client.Start()

			require.NoError(t, oConn.Close())
			select {
			case <-oConn.ErrorChan():
			case <-time.After(5 * time.Second):
				require.Fail(t, "connection did not close within timeout")
			}

			for range 3 {
				require.NoError(t, client.Stop())
			}
			require.True(t, client.IsDone())

			select {
			case err, ok := <-asyncErrChan:
				if ok {
					require.Fail(t, err.Error())
				}
			case <-time.After(2 * time.Second):
				require.Fail(t, "mock connection did not shut down")
			}
		})
	}
}

func TestGetBlockRangeReleasesBusyOnDisconnectBeforeBatchDone(t *testing.T) {
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	mockErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			mockErrChan <- err
		}
		close(mockErrChan)
	}()

	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithBlockFetchConfig(
			blockfetch.Config{SkipBlockValidation: true},
		),
	)
	require.NoError(t, err, "unexpected error when creating Ouroboros object")

	client := oConn.BlockFetch().Client
	point := pcommon.NewPoint(
		12345,
		testPointHash(0xab),
	)
	require.NoError(t, client.GetBlockRange(point, point))

	select {
	case <-client.DoneChan():
	case <-time.After(5 * time.Second):
		require.Fail(t, "blockfetch client did not stop within timeout")
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- client.GetBlockRange(point, point)
	}()
	select {
	case err := <-errChan:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		require.Fail(t, "GetBlockRange blocked on busy mutex")
	}

	select {
	case <-oConn.ErrorChan():
	case <-time.After(5 * time.Second):
		require.Fail(t, "connection did not close within timeout")
	}
	select {
	case err, ok := <-mockErrChan:
		if ok {
			require.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "mock connection did not shut down within timeout")
	}
}
