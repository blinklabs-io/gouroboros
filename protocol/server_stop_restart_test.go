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

package protocol_test

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/stretchr/testify/assert"
)

// restartingServer is a mini-protocol server that replaces its protocol
// instance when the client ends the protocol.
type restartingServer interface {
	Start()
	Stop()
	ProtocolInstance() *protocol.Protocol
}

type restartingServerCase struct {
	name       string
	protocolId uint16
	// hold is the log message the server writes when its restart begins.
	hold string
	// done is the client message that makes the server restart.
	done      any
	newServer func(protocol.ProtocolOptions) restartingServer
}

var restartingServerCases = []restartingServerCase{
	{
		name:       "blockfetch",
		protocolId: blockfetch.ProtocolId,
		hold:       "client done",
		done:       blockfetch.NewMsgClientDone(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToNode
			return blockfetch.NewServer(o, nil)
		},
	},
	{
		name:       "chainsync",
		protocolId: chainsync.ProtocolIdNtC,
		hold:       "done",
		done:       chainsync.NewMsgDone(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToClient
			return chainsync.NewServer(o, &chainsync.Config{})
		},
	},
	{
		name:       "leiosfetch",
		protocolId: leiosfetch.ProtocolId,
		hold:       "client done",
		done:       leiosfetch.NewMsgDone(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToNode
			return leiosfetch.NewServer(o, nil)
		},
	},
	// leiosnotify sends its MsgDone reply before restarting, and the send
	// fails on a stopped instance, so this hold point (before the reply)
	// cannot reach the window after it. The case still guards the Stop path.
	{
		name:       "leiosnotify",
		protocolId: leiosnotify.ProtocolId,
		hold:       "client quit",
		done:       leiosnotify.NewMsgQuit(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToNode
			return leiosnotify.NewServer(o, nil)
		},
	},
	{
		name:       "leiosvotes",
		protocolId: leiosvotes.ProtocolId,
		hold:       "client done",
		done:       leiosvotes.NewMsgDone(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToNode
			return leiosvotes.NewServer(o, nil)
		},
	},
	{
		name:       "peersharing",
		protocolId: peersharing.ProtocolId,
		hold:       "done",
		done:       peersharing.NewMsgDone(),
		newServer: func(o protocol.ProtocolOptions) restartingServer {
			o.Mode = protocol.ProtocolModeNodeToNode
			return peersharing.NewServer(o, nil)
		},
	},
}

// TestServerStopDuringRestartStopsServer stops each restarting server while
// its Done handler is running, before it has replaced its protocol instance.
// Stop must stop the server: the restart must not install and start a new
// instance, and nothing may keep receiving the client's segments.
func TestServerStopDuringRestartStopsServer(t *testing.T) {
	t.Parallel()
	for _, tc := range restartingServerCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := restartwindow.New(t, tc.protocolId)
			server := tc.newServer(protocol.ProtocolOptions{
				Muxer:  h.Server,
				Logger: h.Logger(tc.hold),
			})
			server.Start()
			t.Cleanup(server.Stop)
			h.Start()

			h.Send(tc.done)
			h.WaitHeld()
			stopped := server.ProtocolInstance()
			server.Stop()
			h.Release()

			restarted := func() bool { return server.ProtocolInstance() != stopped }
			if !assert.Never(t, restarted, 200*time.Millisecond, 5*time.Millisecond,
				"a stopped server installed a new protocol instance") {
				return
			}
			// More segments than a receiver's channel and forwarder hold,
			// so any that are still being received stay charged.
			for range 20 {
				h.Send(tc.done)
			}
			h.Settle()
			held := func() bool { return h.Server.IngressInUse() > 0 }
			assert.Never(t, held, 200*time.Millisecond, 5*time.Millisecond,
				"a stopped server is still receiving the client's segments")
		})
	}
}

// TestCancelRestartReleasesSuspendedReceiver suspends a server's muxer
// receiver with StopForRestart, as a restart does, then cancels the restart.
// The receiver must be released: a suspended one keeps buffering the peer's
// segments for a next instance that never comes.
func TestCancelRestartReleasesSuspendedReceiver(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, chainsync.ProtocolIdNtC)
	server := chainsync.NewServer(
		protocol.ProtocolOptions{
			Muxer: h.Server,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		&chainsync.Config{},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	p := server.ProtocolInstance()
	p.StopForRestart()
	p.CancelRestart()
	// More segments than a receiver's channel and forwarder hold.
	for range 20 {
		h.Send(chainsync.NewMsgDone())
	}
	h.Settle()
	held := func() bool { return h.Server.IngressInUse() > 0 }
	assert.Never(t, held, 200*time.Millisecond, 5*time.Millisecond,
		"a cancelled restart's receiver is still buffering ingress")
}
