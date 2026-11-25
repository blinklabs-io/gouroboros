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

package messagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestServerRefusesOnlyTheTooFrequentMessage checks a reply holding two
// messages of one pool and one of another, with the rate limit on. dmq-node
// a267003c14e9 judges each message alone (Validate.hs 84-121) and adds the
// valid ones before it throws SigTooFrequent, closing the connection
// (app/Main.hs 199-221; ouroboros-network Mempool/Simple.hs 183-245): the
// first message of each pool is accepted, and the second of the first pool
// ends the connection.
func TestServerRefusesOnlyTheTooFrequentMessage(t *testing.T) {
	a, _ := serverTestSignedMessages(t, []string{"a1", "a2"}, []uint64{1, 1})
	b, _ := serverTestSignedMessages(t, []string{"b1"}, []uint64{1})
	authenticator, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{
			StakeAuthority: serverTestStakeAuthority{},
		},
	)
	require.NoError(t, err)
	var delivered [][]byte
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithReplyMessagesFunc(
			func(_ CallbackContext, msgs []pcommon.DmqMessage) {
				for i := range msgs {
					delivered = append(delivered, msgs[i].ID())
				}
			},
		),
	)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	reply := []pcommon.DmqMessage{a[0], a[1], b[0]}
	serverTestRequest(server, reply...)

	err = server.handleReplyMessages(NewMsgReplyMessages(reply))
	require.ErrorIs(t, err, pcommon.ErrMessageTooFrequent)
	require.Equal(t, [][]byte{a[0].ID(), b[0].ID()}, delivered)
	require.NoError(
		t,
		authenticator.VerifyMessages([]pcommon.DmqMessage{a[0], b[0]}),
		"the accepted messages are recorded and verify again",
	)
}
