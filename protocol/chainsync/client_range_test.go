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

package chainsync_test

import (
	"reflect"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
)

// testRangeOneBlockAfterIntersect drives GetAvailableBlockRange where the only
// block after the intersection is at blockSlot and is also the peer's tip.
func testRangeOneBlockAfterIntersect(
	t *testing.T,
	intersect pcommon.Point,
	blockSlot uint64,
) {
	t.Helper()
	testBlock := ledger.BabbageBlock{
		BlockHeader: &ledger.BabbageBlockHeader{},
	}
	testBlock.BlockHeader.Body.BlockNumber = 12001
	testBlock.BlockHeader.Body.Slot = blockSlot
	blockCbor, err := cbor.Encode(testBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	if _, err := cbor.Decode(blockCbor, &testBlock); err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	blockPoint := pcommon.NewPoint(
		testBlock.SlotNumber(),
		testBlock.Hash().Bytes(),
	)
	tip := chainsync.Tip{BlockNumber: 12001, Point: blockPoint}
	rollForwardMsg, err := chainsync.NewMsgRollForwardNtC(
		ledger.BlockTypeBabbage,
		blockCbor,
		tip,
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
				chainsync.NewMsgIntersectFound(intersect, tip),
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
				chainsync.NewMsgRollBackward(intersect, tip),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{rollForwardMsg},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange(
				[]pcommon.Point{intersect},
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(start, blockPoint) ||
				!reflect.DeepEqual(end, blockPoint) {
				t.Fatalf(
					"expected one-block range %#v\n  got start: %#v\n  got end:   %#v",
					blockPoint,
					start,
					end,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}

// Not t.Parallel: runTest uses goleak.VerifyNone, which is process-wide.
func TestGetAvailableBlockRangeTipIsNextBlock(t *testing.T) {
	testRangeOneBlockAfterIntersect(
		t,
		pcommon.NewPoint(20001, testPointHash(0x12)),
		20002,
	)
}

// Not t.Parallel: runTest uses goleak.VerifyNone, which is process-wide.
func TestGetAvailableBlockRangeTipSharesIntersectSlot(t *testing.T) {
	testRangeOneBlockAfterIntersect(
		t,
		pcommon.NewPoint(20001, testPointHash(0x12)),
		20001,
	)
}

// Not t.Parallel: runTest uses goleak.VerifyNone, which is process-wide.
func TestGetAvailableBlockRangeIntersectIsTip(t *testing.T) {
	intersect := pcommon.NewPoint(20001, testPointHash(0x12))
	tip := chainsync.Tip{BlockNumber: 12001, Point: intersect}
	// No RequestNext entry: the client must not send one when already at tip
	conversation := append(
		conversationHandshakeFindIntersect,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectFound(intersect, tip),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange(
				[]pcommon.Point{intersect},
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if start.Slot != 0 || end.Slot != 0 ||
				len(start.Hash) != 0 || len(end.Hash) != 0 {
				t.Fatalf(
					"expected empty range\n  got start: %#v\n  got end:   %#v",
					start,
					end,
				)
			}
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: true},
		),
	)
}
