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

package leiosvotes

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersRequestNextSentDuringRestart sends a client's
// VotesRequestNext, the first message of a restarted LeiosVotes, while the
// server is still handling the preceding MsgDone, and requires the restarted
// server to answer it.
func TestServerAnswersRequestNextSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, ProtocolId)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("client done"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		&Config{
			RequestNextFunc: func(CallbackContext, uint64) ([]Vote, error) {
				return []Vote{testVote()}, nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(NewMsgDone())
	h.WaitHeld()
	h.Send(NewMsgVotesRequestNext(1))
	h.Settle()
	h.Release()

	require.Equal(t, uint(MessageTypeVote), h.RecvType("Vote"))
}
