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

package chainsync_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersFindIntersectSentDuringRestart sends a client's
// FindIntersect, the first message of a restarted ChainSync, while the server
// is still handling the preceding MsgDone, and requires the restarted server
// to answer it.
func TestServerAnswersFindIntersectSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, chainsync.ProtocolIdNtC)
	server := chainsync.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("done"),
			Mode:   protocol.ProtocolModeNodeToClient,
		},
		&chainsync.Config{
			FindIntersectFunc: func(
				chainsync.CallbackContext,
				[]pcommon.Point,
			) (pcommon.Point, chainsync.Tip, error) {
				return pcommon.NewPointOrigin(), chainsync.Tip{}, nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(chainsync.NewMsgDone())
	h.WaitHeld()
	h.Send(chainsync.NewMsgFindIntersect(
		[]pcommon.Point{pcommon.NewPointOrigin()},
	))
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(chainsync.MessageTypeIntersectFound),
		h.RecvType("IntersectFound"),
	)
}

// TestServerAnswersBackToBackRestarts sends MsgDone and the next run's
// FindIntersect without waiting between them, repeatedly.
func TestServerAnswersBackToBackRestarts(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, chainsync.ProtocolIdNtC)
	server := chainsync.NewServer(
		protocol.ProtocolOptions{
			Muxer: h.Server,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		&chainsync.Config{
			FindIntersectFunc: func(
				chainsync.CallbackContext,
				[]pcommon.Point,
			) (pcommon.Point, chainsync.Tip, error) {
				return pcommon.NewPointOrigin(), chainsync.Tip{}, nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	for range 200 {
		h.Send(chainsync.NewMsgFindIntersect(
			[]pcommon.Point{pcommon.NewPointOrigin()},
		))
		require.Equal(
			t,
			uint(chainsync.MessageTypeIntersectFound),
			h.RecvType("IntersectFound"),
		)
		h.Send(chainsync.NewMsgDone())
	}
}
