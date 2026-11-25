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
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// hookedStakeAuthority runs onLookup at the first stake lookup once it is
// set, which happens while a reply is being verified and before it is
// committed.
type hookedStakeAuthority struct {
	once     sync.Once
	onLookup func()
}

func (h *hookedStakeAuthority) PoolActiveStake(
	pcommon.PoolKeyHash,
) (uint64, error) {
	if h.onLookup != nil {
		h.once.Do(h.onLookup)
	}
	return 1, nil
}

// TestServerCommitRefusesOnlyTheTooFrequentMessage checks a reply holding
// one message of pool A and one of pool B, while another submission of pool
// A is committed between the reply's verification and its commit. dmq-node
// a267003c14e9 judges each message alone, under its mempool's lock
// (Validate.hs 84-121; ouroboros-network 4646039f Mempool/Simple.hs
// 183-245): A's message is refused as too frequent, B's is accepted, and
// the connection ends once B's has been delivered.
func TestServerCommitRefusesOnlyTheTooFrequentMessage(t *testing.T) {
	a, _ := serverTestSignedMessages(t, []string{"a1", "a2"}, []uint64{1, 1})
	b, _ := serverTestSignedMessages(t, []string{"b1"}, []uint64{1})
	stake := &hookedStakeAuthority{}
	authenticator, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{StakeAuthority: stake},
	)
	require.NoError(t, err)
	// The other submission of pool A is prepared before the hook is set,
	// and committed from it while the reply's first message is verified.
	commitOther, err := authenticator.PrepareMessages(a[:1])
	require.NoError(t, err)
	stake.onLookup = func() { require.NoError(t, commitOther()) }

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
	reply := []pcommon.DmqMessage{a[1], b[0]}
	serverTestRequest(server, reply...)

	err = server.handleReplyMessages(
		NewMsgReplyMessages(protocol.ProtocolVersionDMQNtN1, reply),
	)
	require.ErrorIs(t, err, pcommon.ErrMessageTooFrequent)
	require.Equal(t, [][]byte{b[0].ID()}, delivered)
	require.ErrorIs(
		t,
		authenticator.VerifyMessages(a[1:2]),
		pcommon.ErrMessageTooFrequent,
		"the refused message is not recorded",
	)
	require.NoError(
		t,
		authenticator.VerifyMessages(b[:1]),
		"the accepted message is recorded and verifies again",
	)
}
