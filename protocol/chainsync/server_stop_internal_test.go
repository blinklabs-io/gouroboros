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

package chainsync

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerStopAfterRestartBeganReleasesReceiver stops the server after
// StopForRestart has suspended its muxer receiver and before the restart
// installs a new instance. The cancelled restart must release the receiver:
// a suspended one keeps buffering the client's segments, charged against the
// connection's ingress budget, until the connection closes.
func TestServerStopAfterRestartBeganReleasesReceiver(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, ProtocolIdNtC)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer: h.Server,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		&Config{},
	)
	stopped := make(chan struct{})
	server.restartHook = func() {
		server.Stop()
		close(stopped)
	}
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	original := server.ProtocolInstance()
	h.Send(NewMsgDone())
	select {
	case <-stopped:
	case <-time.After(restartwindow.Timeout):
		t.Fatal("the restart never reached its hook")
	}
	require.Same(t, original, server.ProtocolInstance(),
		"a stopped server installed a new protocol instance")
	// More segments than a receiver's channel and forwarder hold, so any
	// that are still being received stay charged.
	for range 20 {
		h.Send(NewMsgDone())
	}
	h.Settle()
	held := func() bool { return h.Server.IngressInUse() > 0 }
	assert.Never(t, held, 200*time.Millisecond, 5*time.Millisecond,
		"a stopped server's receiver is still buffering ingress")
}
