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

package chainsync_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
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
						Point:       ocommon.NewPointOrigin(),
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
			err := oConn.ChainSync().Client.Sync([]ocommon.Point{})
			if err == nil {
				t.Fatalf("did not receive expected error")
			}
			if err != chainsync.IntersectNotFoundError {
				t.Fatalf("did not receive expected error\n  got:    %s\n  wanted: %s", err, chainsync.IntersectNotFoundError)
			}
		},
	)
}

func TestGetCurrentTip(t *testing.T) {
	expectedTip := chainsync.Tip{
		BlockNumber: 12345,
		Point: ocommon.NewPoint(
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
				t.Fatalf("did not receive expected tip\n  got:    %#v\n  wanted: %#v", tip, expectedTip)
			}
		},
	)
}

func TestGetAvailableBlockRange(t *testing.T) {
	expectedIntersect := ocommon.NewPoint(
		20001,
		test.DecodeHexString("123456789abcdef0"),
	)
	expectedTip := chainsync.Tip{
		BlockNumber: 12345,
		Point: ocommon.NewPoint(
			23456,
			test.DecodeHexString("0123456789abcdef"),
		),
	}
	// Create basic block and round-trip it through the CBOR encoder to get the hash populated
	// The slot value is one higher than our intersect point and the block height is less than
	// our expected tip
	testBlock := ledger.BabbageBlock{
		Header: &ledger.BabbageBlockHeader{},
	}
	testBlock.Header.Body.BlockNumber = 12001
	testBlock.Header.Body.Slot = 20002
	blockCbor, err := cbor.Encode(testBlock)
	if err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	if _, err := cbor.Decode(blockCbor, &testBlock); err != nil {
		t.Fatalf("received unexpected error: %s", err)
	}
	expectedStart := ocommon.NewPoint(
		testBlock.SlotNumber(),
		test.DecodeHexString(testBlock.Hash()),
	)
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
				chainsync.NewMsgRollForwardNtC(
					ledger.BlockTypeBabbage,
					blockCbor,
					expectedTip,
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange([]ocommon.Point{expectedIntersect})
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(start, expectedStart) {
				t.Fatalf("did not receive expected start point\n  got:    %#v\n  wanted: %#v", start, expectedStart)
			}
			if !reflect.DeepEqual(end, expectedTip.Point) {
				t.Fatalf("did not receive expected end point\n  got:    %#v\n  wanted: %#v", end, expectedTip.Point)
			}
		},
	)
}
