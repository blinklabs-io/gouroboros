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

package localmessagenotification

import (
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestServerStopBeforeStartCompletesLifecycle(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)

	require.NoError(t, server.Stop())
	select {
	case <-server.DoneChan():
	default:
		t.Fatal("DoneChan was not closed after stopping before Start")
	}

	require.NoError(t, server.Stop())
	require.NotPanics(t, server.Start)
	select {
	case <-server.DoneChan():
	default:
		t.Fatal("Start restarted a stopped protocol")
	}
}

func TestServerRepeatedStopAfterStartCompletesLifecycle(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		_ = remoteConn.Close()
	})
	protocolMuxer := muxer.New(localConn)
	t.Cleanup(protocolMuxer.Stop)
	server := NewServer(
		protocol.ProtocolOptions{Muxer: protocolMuxer},
		nil,
	)
	server.Start()

	require.NoError(t, server.Stop())
	require.NoError(t, server.Stop())
	select {
	case <-server.DoneChan():
	case <-time.After(time.Second):
		t.Fatal("started server did not complete shutdown")
	}
}

func TestServerStopUnblocksWaitingRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server := NewServer(protocol.ProtocolOptions{}, nil)
		result := make(chan error, 1)
		go func() { result <- server.WaitForMessage(0) }()

		// Wait until WaitForMessage is durably blocked in its select before
		// stopping the server.
		synctest.Wait()
		require.NoError(t, server.Stop())
		select {
		case err := <-result:
			require.ErrorContains(t, err, "server shutting down")
		case <-time.After(time.Second):
			t.Fatal("waiting request was not cancelled")
		}
	})
}

func TestServerStopStopsExpirationCleanerBeforeStart(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	require.NoError(t, server.Stop())

	select {
	case <-server.expirationStopChan:
	default:
		t.Fatal("expiration cleaner was not stopped")
	}
}
