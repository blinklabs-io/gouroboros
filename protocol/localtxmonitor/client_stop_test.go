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
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

func TestClientStopReleasesSnapshotAndWaitsForDone(t *testing.T) {
	client, peer, errors := startAcquiredTestClient(t)
	stopped := make(chan error, 1)
	go func() { stopped <- client.Stop(context.Background()) }()
	requireQueryTestMessageType(t, peer, MessageTypeRelease)
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(peer, binary.BigEndian, &header))
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before Done payload delivery: %v", err)
	default:
	}
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(peer, payload)
	require.NoError(t, err)
	require.Equal(t, ProtocolId, header.GetProtocolId())
	require.Equal(t, []byte{0x81, MessageTypeDone}, payload)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after delivery")
	}
	require.NoError(t, client.Stop(context.Background()))
	select {
	case <-client.DoneChan():
	default:
		t.Fatal("Stop returned before the protocol stopped")
	}
	select {
	case err := <-errors:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

func TestClientStopReportsDeliveryFailure(t *testing.T) {
	for _, stage := range []string{"release", "done"} {
		for _, failure := range []string{"disconnect", "timeout"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				client, peer, _ := startAcquiredTestClient(t)
				stopped := make(chan error, 1)
				go func() { stopped <- client.Stop(context.Background()) }()
				if stage == "done" {
					requireQueryTestMessageType(t, peer, MessageTypeRelease)
				}
				var header muxer.SegmentHeader
				require.NoError(t, binary.Read(peer, binary.BigEndian, &header))
				if failure == "disconnect" {
					require.NoError(t, peer.Close())
				}
				select {
				case err := <-stopped:
					require.Error(t, err)
					if failure == "timeout" {
						require.ErrorIs(t, err, context.DeadlineExceeded)
					}
					require.NoError(t, client.Stop(context.Background()))
				case <-time.After(2 * time.Second):
					t.Fatal("Stop did not report failed delivery")
				}
			})
		}
	}
}
