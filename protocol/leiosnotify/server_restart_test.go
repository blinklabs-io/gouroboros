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

package leiosnotify_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersRequestNextSentAfterServerDone ends a LeiosNotify run with
// the server's MsgDone and sends the client's next NotificationRequestNext as
// soon as that MsgDone arrives. The server sends MsgDone before it restarts,
// so the request reaches the server while the finished instance is still
// registered, and the restarted server must answer it.
func TestServerAnswersRequestNextSentAfterServerDone(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, leiosnotify.ProtocolId)
	server := leiosnotify.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("unused hold"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		&leiosnotify.Config{
			RequestNextFunc: func(
				leiosnotify.CallbackContext,
			) (protocol.Message, error) {
				return leiosnotify.NewMsgBlockOffer(
					pcommon.NewPointOrigin(),
					1,
				), nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(leiosnotify.NewMsgQuit())
	require.Equal(
		t,
		uint(leiosnotify.MessageTypeDone),
		h.RecvType("Done"),
	)
	h.Send(leiosnotify.NewMsgNotificationRequestNext())

	require.Equal(
		t,
		uint(leiosnotify.MessageTypeBlockOffer),
		h.RecvType("BlockOffer"),
	)
}
