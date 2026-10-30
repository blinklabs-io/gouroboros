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

package blockfetch_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersRequestRangeSentDuringRestart sends a client's
// RequestRange, the first message of a restarted BlockFetch, while the server
// is still handling the preceding MsgClientDone, and requires the restarted
// server to answer it.
func TestServerAnswersRequestRangeSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, blockfetch.ProtocolId)
	server := blockfetch.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("client done"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		&blockfetch.Config{
			RequestRangeFunc: func(
				ctx blockfetch.CallbackContext,
				_ pcommon.Point,
				_ pcommon.Point,
			) error {
				return ctx.Server.NoBlocks()
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(blockfetch.NewMsgClientDone())
	h.WaitHeld()
	h.Send(blockfetch.NewMsgRequestRange(
		pcommon.NewPointOrigin(),
		pcommon.NewPointOrigin(),
	))
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(blockfetch.MessageTypeNoBlocks),
		h.RecvType("NoBlocks"),
	)
}
