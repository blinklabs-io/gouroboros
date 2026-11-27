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
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestServerSlowRequestRangeIsNotTimedOut holds the server's RequestRange
// callback well past BatchStartTimeout. The server holds agency in Busy, and
// the reference times a state only on the side waiting for the peer, so the
// slow server must still answer rather than time itself out.
func TestServerSlowRequestRangeIsNotTimedOut(t *testing.T) {
	t.Parallel()
	const batchStartTimeout = 20 * time.Millisecond
	h := restartwindow.New(t, blockfetch.ProtocolId)
	errorChan := make(chan error, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	server := blockfetch.NewServer(
		protocol.ProtocolOptions{
			Muxer:     h.Server,
			ErrorChan: errorChan,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&blockfetch.Config{
			BatchStartTimeout: batchStartTimeout,
			RequestRangeFunc: func(
				ctx blockfetch.CallbackContext,
				_ pcommon.Point,
				_ pcommon.Point,
			) error {
				close(entered)
				<-release
				return ctx.Server.NoBlocks()
			},
		},
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(blockfetch.NewMsgRequestRange(
		pcommon.NewPointOrigin(),
		pcommon.NewPointOrigin(),
	))
	select {
	case <-entered:
	case <-time.After(restartwindow.Timeout):
		t.Fatal("server never called RequestRangeFunc")
	}
	select {
	case err := <-errorChan:
		close(release)
		t.Fatalf("server timed itself out while holding agency: %v", err)
	case <-time.After(10 * batchStartTimeout):
	}
	close(release)

	require.Equal(
		t,
		uint(blockfetch.MessageTypeNoBlocks),
		h.RecvType("NoBlocks"),
	)
	select {
	case err := <-errorChan:
		t.Fatalf("unexpected server error: %v", err)
	default:
	}
}
