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

package leiosfetch_test

import (
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/stretchr/testify/require"
)

// TestServerMethodsDuringRestart calls the server's protocol methods from a
// goroutine of the caller's while the receive loop replaces the protocol
// instance after the client ends its run. Under the race detector it fails
// if those methods read the instance without the server's lock. The reader
// starts before the hold is released and synchronises with nothing the
// receive loop does, so the race is reported whichever runs first.
func TestServerMethodsDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, leiosfetch.ProtocolId)
	server := leiosfetch.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("client done"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	old := server.ProtocolInstance()
	h.Send(leiosfetch.NewMsgDone())
	h.WaitHeld()
	// The reader keeps reading until the restart is over: the receive loop
	// reads the field many times itself, which can push a single earlier
	// read out of the race detector's history.
	restarted := make(chan struct{})
	read := make(chan struct{})
	go func() {
		defer close(read)
		for {
			_ = server.DoneChan()
			_ = server.IsDone()
			select {
			case <-restarted:
				return
			default:
			}
		}
	}()
	h.Release()
	require.Eventually(t, func() bool {
		return server.ProtocolInstance() != old
	}, restartwindow.Timeout, time.Millisecond)
	close(restarted)
	<-read
}
