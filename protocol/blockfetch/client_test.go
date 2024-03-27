// Copyright 2024 Blink Labs Software
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
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
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

func runTest(t *testing.T, conversation []ouroboros_mock.ConversationEntry, innerFunc testInnerFunc) {
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
			asyncErrChan <- fmt.Errorf("received unexpected error: %s", err)
		}
		close(asyncErrChan)
	}()
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithNodeToNode(true),
	)
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

func TestGetBlock(t *testing.T) {
	var testBlockSlot uint64 = 23456
	var testBlockNumber uint64 = 12345
	// Create basic block and round-trip it through the CBOR encoder to get the hash populated
	// The slot value is one higher than our intersect point and the block height is less than
	// our expected tip
	testBlock := ledger.BabbageBlock{
		Header: &ledger.BabbageBlockHeader{},
	}
	testBlock.Header.Body.BlockNumber = testBlockNumber
	testBlock.Header.Body.Slot = testBlockSlot
	blockCbor, err := cbor.Encode(testBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	if _, err := cbor.Decode(blockCbor, &testBlock); err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	testBlockHash := test.DecodeHexString(testBlock.Hash())
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
				ocommon.NewPoint(
					testBlockSlot,
					testBlockHash,
				),
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if blk.Hash() != testBlock.Hash() {
				t.Fatalf("did not receive expected block hash: got %s, wanted %s", blk.Hash(), testBlock.Hash())
			}
			if blk.SlotNumber() != testBlockSlot {
				t.Fatalf("did not receive expected block slot: got %d, wanted %d", blk.SlotNumber(), testBlockSlot)
			}
		},
	)
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
				ocommon.NewPoint(
					12345,
					test.DecodeHexString("abcdef0123456789"),
				),
			)
			if err == nil {
				t.Fatalf("did not receive expected error")
			}
			if err.Error() != expectedErr {
				t.Fatalf("did not receive expected error\n  got:    %s\n  wanted: %s", err, expectedErr)
			}
		},
	)
}
