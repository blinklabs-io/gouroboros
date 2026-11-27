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
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/stretchr/testify/require"
)

// overlapWindow is how long the first request's source waits to see whether
// the second request's source starts while it is still running. A correct
// server never starts it, so this only bounds how long the passing case
// takes.
const overlapWindow = 500 * time.Millisecond

// TestServerAnswersPipelinedRequestsInOrder pipelines two requests. The
// reference server (LeiosDemoOnlyTestNotify.hs, leiosNotifyServerPeerLookahead)
// runs its notification source for one request at a time and answers them
// in the order they arrived, so the source for the second request must not
// start until the first has its answer, and the answers go out in order even
// when the second would be ready first.
func TestServerAnswersPipelinedRequestsInOrder(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	secondStarted := make(chan struct{})
	var overlapped atomic.Bool
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
				if calls.Add(1) == 1 {
					select {
					case <-secondStarted:
						overlapped.Store(true)
					case <-time.After(overlapWindow):
					}
					return leiosnotify.NewMsgBlockOffer(
						pcommon.NewPointOrigin(),
						1,
					), nil
				}
				close(secondStarted)
				return leiosnotify.NewMsgBlockTxsOffer(
					pcommon.NewPointOrigin(),
				), nil
			},
		},
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(leiosnotify.NewMsgNotificationRequestNext())
	h.Send(leiosnotify.NewMsgNotificationRequestNext())

	first := h.RecvType("first reply")
	second := h.RecvType("second reply")
	require.False(
		t,
		overlapped.Load(),
		"the second request's source ran while the first's was running",
	)
	require.Equal(
		t,
		[]uint{
			leiosnotify.MessageTypeBlockOffer,
			leiosnotify.MessageTypeBlockTxsOffer,
		},
		[]uint{first, second},
	)
}
