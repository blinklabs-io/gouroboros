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

package localtxmonitor

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// TestDeliverResultReturnsAfterStopWithoutReceiver checks that a reply
// arriving after Stop, with no caller left to take it, does not block the
// receive loop. DoneChan closes only after that loop returns, so Stop is the
// only signal that can release it; the protocol is never started here, so
// DoneChan stays open throughout.
func TestDeliverResultReturnsAfterStopWithoutReceiver(t *testing.T) {
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	t.Cleanup(func() {
		m.Stop(context.Background())
		connA.Close()
		connB.Close()
	})
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		ErrorChan: make(chan error, 1),
		Mode:      protocol.ProtocolModeNodeToClient,
	}, nil)
	close(client.stopRequested)
	delivered := make(chan struct{})
	go func() {
		deliverResult(client, client.acquireResultChan, true)
		close(delivered)
	}()
	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("deliverResult blocked after Stop with no receiver")
	}
}
