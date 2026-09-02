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
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/pipeline"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// musashiBlockFixture returns the real Musashi ranking block that
// ledger/dijkstra's decode test uses, along with its point. Its header body
// carries the 12-field Leios extension, so the strict Conway decoder cannot
// represent it, while Musashi peers tag it as Conway (block type 7) on the
// wire. The fixture is read from the dijkstra package rather than copied so
// the two tests cannot drift onto different bytes.
func musashiBlockFixture(t *testing.T) ([]byte, pcommon.Point) {
	t.Helper()
	hexData, err := os.ReadFile(filepath.Join(
		"..", "..", "ledger", "dijkstra", "testdata",
		"musashi_dijkstra_block.hex",
	))
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)
	// A consumer that knows the Dijkstra layout can decode these bytes; that
	// is precisely what the raw callback exists to allow.
	blk, err := dijkstra.NewDijkstraBlockFromCbor(raw)
	require.NoError(t, err)
	// Guard the premise of every test below: if the generic Conway decoder
	// ever learns this layout, these tests stop exercising the gap they were
	// written for and must be revisited rather than silently passing.
	_, err = ledger.NewBlockFromCbor(
		ledger.BlockTypeConway,
		raw,
		lcommon.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.Error(
		t,
		err,
		"fixture decodes as Conway; it no longer covers the raw-callback gap",
	)
	return raw, pcommon.NewPoint(blk.SlotNumber(), blk.Hash().Bytes())
}

// conwayTaggedWrappedBlock wraps raw block bytes the way a Musashi peer does:
// tagged with the Conway block type.
func conwayTaggedWrappedBlock(t *testing.T, raw []byte) []byte {
	t.Helper()
	wrappedBlockCbor, err := cbor.Encode(blockfetch.WrappedBlock{
		Type:     ledger.BlockTypeConway,
		RawBlock: cbor.RawMessage(raw),
	})
	require.NoError(t, err)
	return wrappedBlockCbor
}

// TestGetBlockRangeRawFuncReceivesUndecodableBlock covers a Conway-tagged
// Dijkstra-shaped block reaching BlockRawFunc. The generic type decoder
// cannot represent the wire layout, so decoding it is the raw callback's job;
// the client must not fail the request before the callback ever runs.
func TestGetBlockRangeRawFuncReceivesUndecodableBlock(t *testing.T) {
	raw, point := musashiBlockFixture(t)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBatchDone(),
			},
		},
	)
	type rawBlock struct {
		blockType uint
		block     []byte
	}
	rawChan := make(chan rawBlock, 1)
	doneChan := make(chan struct{}, 1)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(point, point),
			)
			select {
			case got := <-rawChan:
				require.Equal(t, uint(ledger.BlockTypeConway), got.blockType)
				require.True(
					t,
					bytes.Equal(raw, got.block),
					"raw callback did not receive the exact wire bytes",
				)
			case <-time.After(5 * time.Second):
				require.Fail(t, "BlockRawFunc was not called within timeout")
			}
			select {
			case <-doneChan:
			case <-time.After(5 * time.Second):
				require.Fail(t, "BatchDoneFunc was not called within timeout")
			}
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				blockType uint,
				block []byte,
			) error {
				rawChan <- rawBlock{blockType: blockType, block: block}
				return nil
			},
			BatchDoneFunc: func(_ blockfetch.CallbackContext) error {
				doneChan <- struct{}{}
				return nil
			},
		}),
	)
}

// TestGetBlockRangeRawFuncRejectsBlockOutsideRange covers the negative case:
// falling back to raw delivery must not weaken range correlation. A peer that
// answers with a block outside the requested range is still rejected, and the
// callback must not see those bytes.
func TestGetBlockRangeRawFuncRejectsBlockOutsideRange(t *testing.T) {
	raw, point := musashiBlockFixture(t)
	// Request a range that the delivered block does not start.
	requested := pcommon.NewPoint(point.Slot-1, point.Hash)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBatchDone(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	rawCalled := make(chan struct{}, 1)
	connErr := runTestExpectingError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(requested, requested),
			)
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				_ []byte,
			) error {
				rawCalled <- struct{}{}
				return nil
			},
		}),
	)
	require.ErrorContains(t, connErr, "outside requested range")
	select {
	case <-rawCalled:
		require.Fail(t, "BlockRawFunc was called for an out-of-range block")
	default:
	}
}

// TestGetBlockRangeBlockFuncStillFailsUndecodableBlock covers the other half
// of the contract: a caller that asks for decoded blocks gets unchanged
// behavior. Without a raw callback there is nothing to deliver the block to,
// so the decode failure still fails the request.
func TestGetBlockRangeBlockFuncStillFailsUndecodableBlock(t *testing.T) {
	raw, point := musashiBlockFixture(t)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBatchDone(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	blockCalled := make(chan struct{}, 1)
	connErr := runTestExpectingError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(point, point),
			)
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				_ ledger.Block,
			) error {
				blockCalled <- struct{}{}
				return nil
			},
		}),
	)
	require.ErrorContains(t, connErr, "decode Conway block error")
	select {
	case <-blockCalled:
		require.Fail(t, "BlockFunc was called for an undecodable block")
	default:
	}
}

// TestGetBlockUndecodableBlockStillFails covers single-block delivery, which
// hands the caller a decoded ledger.Block and therefore has no raw fallback
// available even when a raw callback is configured.
func TestGetBlockUndecodableBlockStillFails(t *testing.T) {
	raw, point := musashiBlockFixture(t)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBatchDone(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	connErr := runTestExpectingError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.BlockFetch().Client.GetBlock(point)
			require.Error(t, err)
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				_ []byte,
			) error {
				return nil
			},
		}),
	)
	require.ErrorContains(t, connErr, "decode Conway block error")
}

// TestGetBlockRangePipelineStillFailsUndecodableBlock covers the remaining
// consumer. The block pipeline takes precedence over BlockRawFunc and runs its
// own typed decode, so configuring a raw callback alongside it must not make
// the client accept a block the pipeline cannot decode either.
func TestGetBlockRangePipelineStillFailsUndecodableBlock(t *testing.T) {
	raw, point := musashiBlockFixture(t)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBatchDone(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	blockPipeline := pipeline.NewBlockPipeline(
		pipeline.WithSkipBodyHashValidation(true),
		pipeline.WithApplyFunc(func(_ *pipeline.BlockItem) error {
			return nil
		}),
	)
	require.NoError(t, blockPipeline.Start(t.Context()))
	defer func() { _ = blockPipeline.Stop() }()
	rawCalled := make(chan struct{}, 1)
	connErr := runTestExpectingError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(point, point),
			)
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			Pipeline:            blockPipeline,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				_ []byte,
			) error {
				rawCalled <- struct{}{}
				return nil
			},
		}),
	)
	require.ErrorContains(t, connErr, "decode Conway block error")
	select {
	case <-rawCalled:
		require.Fail(t, "BlockRawFunc was called while a pipeline was set")
	default:
	}
}

// chainedMusashiBlock rewrites the fixture's slot and prev_hash so it follows
// the given point, producing a second Leios-extended block for range tests.
// Only the header fields BlockFetch correlates on are changed; the block stays
// undecodable by the generic Conway decoder, which is the point. Nothing here
// re-signs the header, so this is only valid with SkipBlockValidation.
func chainedMusashiBlock(
	t *testing.T,
	raw []byte,
	prev pcommon.Point,
	slot uint64,
) ([]byte, pcommon.Point) {
	t.Helper()
	var blockElems []cbor.RawMessage
	_, err := cbor.Decode(raw, &blockElems)
	require.NoError(t, err)
	require.NotEmpty(t, blockElems, "block should hold a header")
	var headerElems []cbor.RawMessage
	_, err = cbor.Decode(blockElems[0], &headerElems)
	require.NoError(t, err)
	require.NotEmpty(t, headerElems, "header should hold a body")
	var bodyElems []cbor.RawMessage
	_, err = cbor.Decode(headerElems[0], &bodyElems)
	require.NoError(t, err)
	require.Len(t, bodyElems, 12, "fixture should carry the Leios extension")
	slotCbor, err := cbor.Encode(slot)
	require.NoError(t, err)
	prevCbor, err := cbor.Encode(prev.Hash)
	require.NoError(t, err)
	bodyElems[1] = slotCbor
	bodyElems[2] = prevCbor
	headerCbor, err := cbor.Encode(bodyElems)
	require.NoError(t, err)
	headerElems[0] = headerCbor
	newHeader, err := cbor.Encode(headerElems)
	require.NoError(t, err)
	blockElems[0] = newHeader
	newBlock, err := cbor.Encode(blockElems)
	require.NoError(t, err)
	hash := lcommon.Blake2b256Hash(newHeader)
	return newBlock, pcommon.NewPoint(slot, hash.Bytes())
}

// TestGetBlockRangeRawFuncMultiBlockContinuity covers the path the raw
// fallback newly makes reachable: a range of more than one undecodable block,
// where continuity between them is established from each block's previous
// hash read out of the header CBOR.
func TestGetBlockRangeRawFuncMultiBlockContinuity(t *testing.T) {
	raw, first := musashiBlockFixture(t)
	second, secondPoint := chainedMusashiBlock(t, raw, first, first.Slot+1)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, second)),
				blockfetch.NewMsgBatchDone(),
			},
		},
	)
	rawChan := make(chan []byte, 2)
	doneChan := make(chan struct{}, 1)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(first, secondPoint),
			)
			for _, want := range [][]byte{raw, second} {
				select {
				case got := <-rawChan:
					require.True(
						t,
						bytes.Equal(want, got),
						"raw callback received blocks out of order or altered",
					)
				case <-time.After(5 * time.Second):
					require.Fail(t, "BlockRawFunc was not called within timeout")
				}
			}
			select {
			case <-doneChan:
			case <-time.After(5 * time.Second):
				require.Fail(t, "BatchDoneFunc was not called within timeout")
			}
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				block []byte,
			) error {
				rawChan <- block
				return nil
			},
			BatchDoneFunc: func(_ blockfetch.CallbackContext) error {
				doneChan <- struct{}{}
				return nil
			},
		}),
	)
}

// TestGetBlockRangeRawFuncRejectsBrokenContinuity is the negative twin of the
// test above. A second block that does not extend the first is rejected even
// though both are in the requested range, so raw delivery cannot be used to
// slip a foreign block into the middle of a batch.
func TestGetBlockRangeRawFuncRejectsBrokenContinuity(t *testing.T) {
	raw, first := musashiBlockFixture(t)
	unrelated := pcommon.NewPoint(first.Slot, make([]byte, 32))
	second, secondPoint := chainedMusashiBlock(
		t,
		raw,
		unrelated,
		first.Slot+1,
	)
	conversation := append(
		conversationHandshakeRequestRange,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: blockfetch.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				blockfetch.NewMsgStartBatch(),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, raw)),
				blockfetch.NewMsgBlock(conwayTaggedWrappedBlock(t, second)),
				blockfetch.NewMsgBatchDone(),
			},
		},
		ouroboros_mock.ConversationEntrySleep{Duration: 100 * time.Millisecond},
		ouroboros_mock.ConversationEntryClose{},
	)
	rawChan := make(chan []byte, 2)
	connErr := runTestExpectingError(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			require.NoError(
				t,
				oConn.BlockFetch().Client.GetBlockRange(first, secondPoint),
			)
		},
		ouroboros.WithBlockFetchConfig(blockfetch.Config{
			SkipBlockValidation: true,
			BlockRawFunc: func(
				_ blockfetch.CallbackContext,
				_ uint,
				block []byte,
			) error {
				rawChan <- block
				return nil
			},
		}),
	)
	require.ErrorContains(t, connErr, "does not follow previous range point")
	require.Len(t, rawChan, 1, "only the first block should be delivered")
}

// runTestExpectingError drives a mock conversation whose Ouroboros connection
// is expected to fail, and returns that failure. runTest panics on a
// connection error, so error-path tests use this instead.
func runTestExpectingError(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
	options ...ouroboros.ConnectionOptionFunc,
) error {
	t.Helper()
	// Scope the leak check to goroutines this helper starts. A caller may
	// have set up long-lived goroutines of its own (a block pipeline, say)
	// that it stops after this returns.
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	opts := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
	}
	opts = append(opts, options...)
	oConn, err := ouroboros.New(opts...)
	require.NoError(t, err, "unexpected error when creating Ouroboros object")
	innerFunc(t, oConn)
	var connErr error
	select {
	case connErr = <-oConn.ErrorChan():
	case <-time.After(5 * time.Second):
		require.Fail(t, "connection did not report an error within timeout")
	}
	_ = oConn.Close()
	return connErr
}
