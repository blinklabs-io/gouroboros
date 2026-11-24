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

package localmessagesubmission

import (
	"sync"
	"testing"
	"time"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestConcurrentSubmitsGetTheirOwnAnswers submits one message the node
// accepts and one it rejects from two goroutines at once. As in the
// reference, where the result of a submission goes to that submission's
// continuation (LocalTxSubmission/Client.hs, SendMsgSubmitTx), each call
// must return the node's answer to its own message.
func TestConcurrentSubmitsGetTheirOwnAnswers(t *testing.T) {
	t.Parallel()
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithSubmitMessageFunc(func(
			_ CallbackContext,
			msg *pcommon.DmqMessage,
		) pcommon.RejectReason {
			if string(msg.MessageID) == "reject" {
				return pcommon.InvalidReason{Message: "rejected by test"}
			}
			return nil
		}),
	)
	session := newSubmissionTestSession(t, &serverCfg)
	expires := uint32(time.Now().Add(time.Minute).Unix())
	submit := func(id string) error {
		return session.client.SubmitMessage(&pcommon.DmqMessage{
			MessageID: []byte(id),
			Payload:   pcommon.DmqMessagePayload{ExpiresAt: expires},
		})
	}
	var acceptErr, rejectErr error
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Go(func() { <-start; acceptErr = submit("accept") })
	wg.Go(func() { <-start; rejectErr = submit("reject") })
	close(start)
	wg.Wait()

	require.NoError(t, acceptErr)
	var rejected MessageRejectedError
	require.ErrorAs(t, rejectErr, &rejected)
	require.Equal(
		t,
		pcommon.InvalidReason{Message: "rejected by test"},
		rejected.Reason,
	)
}
