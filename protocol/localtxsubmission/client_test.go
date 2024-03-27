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

package localtxsubmission_test

import (
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

var conversationHandshakeSubmitTx = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localtxsubmission.ProtocolId,
		MessageType: localtxsubmission.MessageTypeSubmitTx,
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

func TestSubmitTxAccept(t *testing.T) {
	testTx := test.DecodeHexString("abcdef0123456789")
	conversation := append(
		conversationHandshakeSubmitTx,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxsubmission.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxsubmission.NewMsgAcceptTx(),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			err := oConn.LocalTxSubmission().Client.SubmitTx(ledger.TxTypeBabbage, testTx)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
		},
	)
}

func TestSubmitTxRject(t *testing.T) {
	testTx := test.DecodeHexString("abcdef0123456789")
	expectedErr := localtxsubmission.TransactionRejectedError{
		// [0, [1, ["foo"]]]
		ReasonCbor: test.DecodeHexString("820082018163666f6f"),
		Reason:     fmt.Errorf("GenericError ([0 [1 [foo]]])"),
	}
	conversation := append(
		conversationHandshakeSubmitTx,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localtxsubmission.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localtxsubmission.NewMsgRejectTx(expectedErr.ReasonCbor),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			err := oConn.LocalTxSubmission().Client.SubmitTx(ledger.TxTypeBabbage, testTx)
			if err == nil {
				t.Fatalf("did not receive expected error")
			}
			if err.Error() != expectedErr.Error() {
				t.Fatalf("did not receive expected error\n  got:    %s\n  wanted: %s", err, expectedErr)
			}
		},
	)
}
