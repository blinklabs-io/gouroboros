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

package localmessagenotification

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestClientDeliversMessagesTheNodeAdmitted checks that the client does not
// apply the per-pool rate limit. The local node applied it when it accepted
// each message (dmq-node a267003c14e9 Validate.hs 124-145), at times the
// client cannot see, so two messages of one pool it accepted a minute apart
// can arrive in one reply, and both are delivered.
func TestClientDeliversMessagesTheNodeAdmitted(t *testing.T) {
	expires := uint32(time.Now().Add(10 * time.Minute).Unix())
	messages, _ := clientTestSignedMessages(
		t,
		[]string{"first", "second"},
		[]uint64{1, 1},
		[]uint32{expires, expires},
	)
	authenticator, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{
			StakeAuthority: clientTestStakeAuthority{},
		},
	)
	require.NoError(t, err)
	var delivered []pcommon.DmqMessage
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithReplyMessagesFunc(
			func(_ CallbackContext, msgs []pcommon.DmqMessage, _ bool) {
				delivered = append(delivered, msgs...)
			},
		),
	)
	client := NewClient(protocol.ProtocolOptions{}, &cfg)

	require.NoError(t, client.messageHandler(testReplyMessages(false, messages)))
	require.Len(t, delivered, 2)
}
