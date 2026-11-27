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
	"context"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
)

// TestServerMethodsDuringRestart calls the server's protocol methods from a
// goroutine of the caller's while the receive loop replaces the protocol
// instance after the client's done message. Under the race detector it fails
// if those methods read the instance without the server's lock. The reader
// starts before the hold is released and synchronises with nothing the
// receive loop does, so the race is reported whichever runs first.
func TestServerMethodsDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, chainsync.ProtocolIdNtC)
	server := chainsync.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("done"),
			Mode:   protocol.ProtocolModeNodeToClient,
		},
		&chainsync.Config{},
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(chainsync.NewMsgDone())
	h.WaitHeld()
	read := make(chan struct{})
	go func() {
		defer close(read)
		_ = server.DoneChan()
	}()
	h.Release()
	<-read
}
