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

package localstatequery_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

var conversationHandshakeAcquire = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeAcquireNoPoint,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgAcquired(),
		},
	},
}

var conversationCurrentEra = append(
	conversationHandshakeAcquire,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeQuery,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgResult([]byte{0x5}),
		},
	},
)

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

func TestGetCurrentEra(t *testing.T) {
	runTest(
		t,
		conversationCurrentEra,
		func(t *testing.T, oConn *ouroboros.Connection) {
			// Run query
			currentEra, err := oConn.LocalStateQuery().Client.GetCurrentEra()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if currentEra != 5 {
				t.Fatalf("did not receive expected result: got %d, expected %d", currentEra, 5)
			}
		},
	)
}

func TestGetChainPoint(t *testing.T) {
	expectedPoint := ocommon.NewPoint(123, []byte{0xa, 0xb, 0xc})
	cborData, err := cbor.Encode(expectedPoint)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			chainPoint, err := oConn.LocalStateQuery().Client.GetChainPoint()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(chainPoint, &expectedPoint) {
				t.Fatalf("did not receive expected result\n  got:    %#v\n  wanted: %#v", chainPoint, expectedPoint)
			}
		},
	)
}

func TestGetEpochNo(t *testing.T) {
	expectedEpochNo := 123456
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(
					// [123456]
					test.DecodeHexString("811a0001e240"),
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			epochNo, err := oConn.LocalStateQuery().Client.GetEpochNo()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if epochNo != expectedEpochNo {
				t.Fatalf("did not receive expected result, got %d, wanted %d", epochNo, expectedEpochNo)
			}
		},
	)
}

func TestGetUTxOByAddress(t *testing.T) {
	testAddress, err := ledger.NewAddress("addr_test1vrk294czhxhglflvxla7vxj2cjz7wyrdpxl3fj0vych5wws77xuc7")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedUtxoId := localstatequery.UtxoId{
		Hash: ledger.NewBlake2b256([]byte{0x1, 0x2}),
		Idx:  7,
	}
	expectedResult := localstatequery.UTxOByAddressResult{
		Results: map[localstatequery.UtxoId]ledger.BabbageTransactionOutput{
			expectedUtxoId: ledger.BabbageTransactionOutput{
				OutputAddress: testAddress,
				OutputAmount: ledger.MaryTransactionOutputValue{
					Amount: 234567,
				},
			},
		},
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			utxos, err := oConn.LocalStateQuery().Client.GetUTxOByAddress([]ledger.Address{testAddress})
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			// Set stored CBOR to nil to make comparison easier
			for k, v := range utxos.Results {
				v.SetCbor(nil)
				utxos.Results[k] = v
			}
			if !reflect.DeepEqual(utxos, &expectedResult) {
				t.Fatalf("did not receive expected result\n got:    %#v\n  wanted: %#v", utxos, expectedResult)
			}
		},
	)
}
