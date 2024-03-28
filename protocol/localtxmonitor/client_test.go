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

package localtxmonitor_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

var conversationHandshakeAcquire = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localtxmonitor.ProtocolId,
		MessageType: localtxmonitor.MessageTypeAcquire,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localtxmonitor.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localtxmonitor.NewMsgAcquired(12345),
		},
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

func TestHasTxTrue(t *testing.T) {
	testTxId := test.DecodeHexString("abcdef0123456789")
	expectedResult := true
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localtxmonitor.ProtocolId,
			MessageType: localtxmonitor.MessageTypeHasTx,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxmonitor.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxmonitor.NewMsgReplyHasTx(expectedResult),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			hasTx, err := oConn.LocalTxMonitor().Client.HasTx(testTxId)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if hasTx != expectedResult {
				t.Fatalf("did not receive expected HasTx result: got %v, wanted %v", hasTx, expectedResult)
			}
		},
	)
}

func TestHasTxFalse(t *testing.T) {
	testTxId := test.DecodeHexString("abcdef0123456789")
	expectedResult := false
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localtxmonitor.ProtocolId,
			MessageType: localtxmonitor.MessageTypeHasTx,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxmonitor.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxmonitor.NewMsgReplyHasTx(expectedResult),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			hasTx, err := oConn.LocalTxMonitor().Client.HasTx(testTxId)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if hasTx != expectedResult {
				t.Fatalf("did not receive expected HasTx result: got %v, wanted %v", hasTx, expectedResult)
			}
		},
	)
}

func TestGetSizes(t *testing.T) {
	var expectedCapacity uint32 = 100000
	var expectedSize uint32 = 12345
	var expectedTxCount uint32 = 5
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localtxmonitor.ProtocolId,
			MessageType: localtxmonitor.MessageTypeGetSizes,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxmonitor.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxmonitor.NewMsgReplyGetSizes(
					expectedCapacity,
					expectedSize,
					expectedTxCount,
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			capacity, size, txCount, err := oConn.LocalTxMonitor().Client.GetSizes()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if capacity != expectedCapacity {
				t.Fatalf("did not receive expected capacity result: got %d, wanted %d", capacity, expectedCapacity)
			}
			if size != expectedSize {
				t.Fatalf("did not receive expected size result: got %d, wanted %d", size, expectedSize)
			}
			if txCount != expectedTxCount {
				t.Fatalf("did not receive expected TX count result: got %d, wanted %d", txCount, expectedTxCount)
			}
		},
	)
}

func TestNextTx(t *testing.T) {
	expectedTxEra := ledger.TxTypeBabbage
	expectedTx1 := test.DecodeHexString("abcdef0123456789")
	expectedTx2 := test.DecodeHexString("bcdef0123456789a")
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localtxmonitor.ProtocolId,
			MessageType: localtxmonitor.MessageTypeNextTx,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxmonitor.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxmonitor.NewMsgReplyNextTx(
					uint8(expectedTxEra),
					expectedTx1,
				),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localtxmonitor.ProtocolId,
			MessageType: localtxmonitor.MessageTypeNextTx,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxmonitor.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxmonitor.NewMsgReplyNextTx(
					uint8(expectedTxEra),
					expectedTx2,
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			tx1, err := oConn.LocalTxMonitor().Client.NextTx()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tx1, expectedTx1) {
				t.Fatalf("did not get expected TX content\n  got:    %x\n  wanted: %x", tx1, expectedTx1)
			}
			tx2, err := oConn.LocalTxMonitor().Client.NextTx()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tx2, expectedTx2) {
				t.Fatalf("did not get expected TX content\n  got:    %x\n  wanted: %x", tx2, expectedTx2)
			}
		},
	)
}
