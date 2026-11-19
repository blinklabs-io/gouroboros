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
	"context"
	"errors"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestCompressedBlocksThroughPipelinedRange(t *testing.T) {
	t.Parallel()
	for _, encoding := range []uint64{blockfetch.CompressionRaw, blockfetch.CompressionZstandard} {
		t.Run(string(rune('0'+encoding)), func(t *testing.T) {
			t.Parallel()
			wrapped, point := testBlock(t, 100)
			data := wrapped
			if encoding == blockfetch.CompressionZstandard {
				enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
				require.NoError(t, err)
				data = enc.EncodeAll(wrapped, nil)
				enc.Close()
			}
			handshakeReply := ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId, IsResponse: true,
				Messages: []protocol.Message{handshake.NewMsgAcceptVersion(
					15,
					protocol.VersionDataNtN13andUp{
						VersionDataNtN11to12: protocol.VersionDataNtN11to12{
							CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
						},
					},
				)},
			}
			msg := &blockfetch.MsgCompressedBlocks{
				MessageBase: protocol.MessageBase{
					MessageType: blockfetch.MessageTypeCompressedBlocks,
				},
				Encoding: encoding, EncodedData: data,
			}
			h := newPipelineHarness(t, []ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric, handshakeReply,
				requestRangeInput(), requestRangeInput(),
				batchOutput(
					blockfetch.NewMsgStartBatch(),
					msg,
					blockfetch.NewMsgBatchDone(),
				),
				batchOutput(
					blockfetch.NewMsgStartBatch(),
					msg,
					blockfetch.NewMsgBatchDone(),
				),
			}, blockfetch.WithCompressionEnabled(true))
			defer h.close(t)
			ctx, cancel := context.WithTimeout(
				context.Background(),
				testTimeout,
			)
			defer cancel()
			ids := make([]uint64, 0, 2)
			for range 2 {
				id, err := h.client.RequestRange(ctx, blockfetch.RangeRequest{
					Start: point, End: point, ExpectedBytes: 4096,
				})
				require.NoError(t, err)
				ids = append(ids, id)
			}
			for _, id := range ids {
				require.Equal(
					t,
					deliveredBlock{requestId: id, slot: 100},
					h.nextBlock(t),
				)
				result := h.nextDone(t)
				require.Equal(t, id, result.requestId)
				require.NoError(t, result.err)
			}
		})
	}
}

func TestCompressedBlocksRejectDisabledAndInvalidRanges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		enabled bool
		version uint16
		payload []byte
		limit   uint64
	}{
		{name: "disabled", version: 15},
		{name: "before Dijkstra", enabled: true, version: 13},
		{name: "wrong block point", enabled: true, version: 15, payload: []byte{0x82, 0x00, 0x80}},
		{name: "malformed sequence", enabled: true, version: 15, payload: []byte{0x82}},
		{name: "wire and expanded budget", enabled: true, version: 15, limit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wrapped, point := testBlock(t, 100)
			payload := wrapped
			if tc.payload != nil {
				payload = tc.payload
			}
			msg := &blockfetch.MsgCompressedBlocks{
				MessageBase: protocol.MessageBase{
					MessageType: blockfetch.MessageTypeCompressedBlocks,
				},
				Encoding: blockfetch.CompressionRaw, EncodedData: payload,
			}
			reply := ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId, IsResponse: true,
				Messages: []protocol.Message{
					handshake.NewMsgAcceptVersion(
						tc.version,
						protocol.VersionDataNtN13andUp{
							VersionDataNtN11to12: protocol.VersionDataNtN11to12{
								CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
							},
						},
					),
				},
			}
			h := newPipelineHarness(t, []ouroboros_mock.ConversationEntry{
				ouroboros_mock.ConversationEntryHandshakeRequestGeneric, reply,
				requestRangeInput(), batchOutput(blockfetch.NewMsgStartBatch(), msg),
			}, blockfetch.WithCompressionEnabled(tc.enabled), blockfetch.WithMaxRangeBytes(tc.limit))
			defer h.close(t)
			ctx, cancel := context.WithTimeout(
				context.Background(),
				testTimeout,
			)
			defer cancel()
			_, err := h.client.RequestRange(
				ctx,
				blockfetch.RangeRequest{
					Start:         point,
					End:           point,
					ExpectedBytes: 1,
				},
			)
			require.NoError(t, err)
			var failure error
			select {
			case result := <-h.done:
				failure = result.err
			case failure = <-h.connErrs:
			case <-ctx.Done():
				t.Fatal("expected compressed range rejection")
			}
			require.Error(t, failure)
			if tc.limit > 0 {
				require.True(
					t,
					errors.Is(failure, blockfetch.ErrRangeByteLimit),
				)
			}
		})
	}
}

func TestCompressedBlocksPreserveMultiBlockOrder(t *testing.T) {
	t.Parallel()
	first, firstPoint := testBlock(t, 100)
	secondBlock := newTestBabbageBlock(101, 101)
	copy(secondBlock.BlockHeader.Body.PrevHash[:], firstPoint.Hash)
	encoded, err := cbor.Encode(secondBlock)
	require.NoError(t, err)
	_, err = cbor.Decode(encoded, &secondBlock)
	require.NoError(t, err)
	second, err := cbor.Encode(
		blockfetch.WrappedBlock{
			Type:     ledger.BlockTypeBabbage,
			RawBlock: encoded,
		},
	)
	require.NoError(t, err)
	lastPoint := pcommon.NewPoint(101, secondBlock.Hash().Bytes())
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	defer enc.Close()
	msg := &blockfetch.MsgCompressedBlocks{
		MessageBase: protocol.MessageBase{
			MessageType: blockfetch.MessageTypeCompressedBlocks,
		},
		Encoding: blockfetch.CompressionZstandard, EncodedData: enc.EncodeAll(append(first, second...), nil),
	}
	reply := ouroboros_mock.ConversationEntryOutput{
		ProtocolId: handshake.ProtocolId, IsResponse: true,
		Messages: []protocol.Message{handshake.NewMsgAcceptVersion(
			15,
			protocol.VersionDataNtN13andUp{
				VersionDataNtN11to12: protocol.VersionDataNtN11to12{
					CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
				},
			},
		)},
	}
	h := newPipelineHarness(t, []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric, reply,
		requestRangeInput(), batchOutput(blockfetch.NewMsgStartBatch(), msg, blockfetch.NewMsgBatchDone()),
	}, blockfetch.WithCompressionEnabled(true))
	defer h.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	id, err := h.client.RequestRange(
		ctx,
		blockfetch.RangeRequest{
			Start:         firstPoint,
			End:           lastPoint,
			ExpectedBytes: 8192,
		},
	)
	require.NoError(t, err)
	require.Equal(t, deliveredBlock{requestId: id, slot: 100}, h.nextBlock(t))
	require.Equal(t, deliveredBlock{requestId: id, slot: 101}, h.nextBlock(t))
	result := h.nextDone(t)
	require.NoError(t, result.err)
}

func TestCompressionServerEmitsOptInWireMessage(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "legacy", true: "compressed"}[enabled],
			func(t *testing.T) {
				t.Parallel()
				wrapped, point := testBlock(t, 100)
				var block blockfetch.WrappedBlock
				_, err := cbor.Decode(wrapped, &block)
				require.NoError(t, err)
				vd := protocol.VersionDataNtN13andUp{
					VersionDataNtN11to12: protocol.VersionDataNtN11to12{
						CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
					},
				}
				serverVD := vd
				serverVD.CborInitiatorAndResponderDiffusionMode = true
				conversation := ouroboros_mock.NewConversationHandshakeNtNAsClient(
					15,
					vd,
					serverVD,
				)
				typ := uint(blockfetch.MessageTypeBlock)
				if enabled {
					typ = blockfetch.MessageTypeCompressedBlocks
				}
				conversation = append(
					conversation,
					ouroboros_mock.ConversationEntryOutput{
						ProtocolId: blockfetch.ProtocolId,
						Messages: []protocol.Message{
							blockfetch.NewMsgRequestRange(point, point),
						},
					},
					ouroboros_mock.ConversationEntryInput{
						ProtocolId:  blockfetch.ProtocolId,
						IsResponse:  true,
						MessageType: blockfetch.MessageTypeStartBatch,
					},
					ouroboros_mock.ConversationEntryInput{
						ProtocolId:  blockfetch.ProtocolId,
						IsResponse:  true,
						MessageType: typ,
					},
					ouroboros_mock.ConversationEntryInput{
						ProtocolId:  blockfetch.ProtocolId,
						IsResponse:  true,
						MessageType: blockfetch.MessageTypeBatchDone,
					},
				)
				mock := ouroboros_mock.NewConnection(
					ouroboros_mock.ProtocolRoleServer,
					conversation,
				)
				cfg, err := blockfetch.NewConfig(
					blockfetch.WithCompressionEnabled(enabled),
					blockfetch.WithRequestRangeFunc(
						func(ctx blockfetch.CallbackContext, _, _ pcommon.Point) error {
							if err := ctx.Server.StartBatch(); err != nil {
								return err
							}
							if err := ctx.Server.Block(block.Type, block.RawBlock); err != nil {
								return err
							}
							return ctx.Server.BatchDone()
						},
					),
				)
				require.NoError(t, err)
				conn, err := ouroboros.New(
					ouroboros.WithConnection(mock),
					ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
					ouroboros.WithNodeToNode(true),
					ouroboros.WithServer(true),
					ouroboros.WithBlockFetchConfig(cfg),
				)
				require.NoError(t, err)
				defer conn.Close()
				ctx, cancel := context.WithTimeout(
					context.Background(),
					testTimeout,
				)
				defer cancel()
				select {
				case err, ok := <-mock.(*ouroboros_mock.Connection).ErrorChan():
					if ok {
						require.NoError(t, err)
					}
				case <-ctx.Done():
					t.Fatal("server did not emit expected block wire type")
				}
			},
		)
	}
}
