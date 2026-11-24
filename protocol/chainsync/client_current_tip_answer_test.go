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
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestGetCurrentTipWaitsForItsOwnAnswer runs GetCurrentTip while a sync at
// the chain tip has a MsgRequestNext outstanding. The roll that answers it
// carries a tip and arrives before GetCurrentTip's MsgFindIntersect is even
// sent. GetCurrentTip must still wait for the MsgIntersectNotFound answering
// its own request: if it returned on the roll, that answer would be given to
// the next call waiting for an intersect answer, and the Jump below would
// report no intersection for a point the server has, then fail on its own
// MsgIntersectFound.
func TestGetCurrentTipWaitsForItsOwnAnswer(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	requested := make(chan struct{}, 1)
	releaseRoll := make(chan struct{})
	releaseNotFound := make(chan struct{})
	var releaseOnce sync.Once
	releaseAnswer := func() { releaseOnce.Do(func() { close(releaseNotFound) }) }
	t.Cleanup(releaseAnswer)
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			if len(points) == 0 {
				<-releaseNotFound
				return pcommon.Point{}, Tip{}, ErrIntersectNotFound
			}
			return points[0], Tip{}, nil
		}),
		WithRequestNextFunc(func(ctx CallbackContext) error {
			if err := ctx.Server.AwaitReply(); err != nil {
				return err
			}
			requested <- struct{}{}
			go func() {
				<-releaseRoll
				_ = ctx.Server.RollBackward(pcommon.NewPointOrigin(), Tip{})
			}()
			return nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: make(chan error, 4),
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &serverCfg)

	clientErrors := make(chan error, 4)
	clientCfg := NewConfig(
		WithPipelineLimit(1),
		// Stopping the sync on its roll lets Jump run afterwards.
		WithRollBackwardFunc(func(CallbackContext, pcommon.Point, Tip) error {
			return ErrStopSyncProcess
		}),
	)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &clientCfg)
	server.Start()
	t.Cleanup(server.Stop)
	client.Start()
	t.Cleanup(func() { _ = client.Stop() })
	serverMuxer.Start()
	clientMuxer.Start()

	require.NoError(t, client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received MsgRequestNext")
	}

	var tipReturned atomic.Bool
	tipErr := make(chan error, 1)
	go func() {
		_, err := client.GetCurrentTip()
		tipReturned.Store(true)
		tipErr <- err
	}()
	require.Eventually(t, func() bool {
		return len(client.wantCurrentTipChan)+
			len(client.wantIntersectFoundChan) == 1
	}, 5*time.Second, time.Millisecond, "GetCurrentTip never sent its request")

	close(releaseRoll)
	require.Never(t, tipReturned.Load, 300*time.Millisecond, time.Millisecond,
		"GetCurrentTip returned on the roll's tip before its own answer")

	releaseAnswer()
	select {
	case err := <-tipErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("GetCurrentTip did not return after its answer")
	}

	jumpPoint := pcommon.NewPoint(5, testPointHash(0x05))
	require.Eventually(t, func() bool {
		return client.prepareIdleProtocol(make(chan bool)) == nil
	}, 5*time.Second, time.Millisecond, "sync did not stop")
	point, _, found, err := client.Jump([]pcommon.Point{jumpPoint})
	require.NoError(t, err)
	require.True(t, found, "Jump received another call's MsgIntersectNotFound")
	require.Equal(t, jumpPoint.Slot, point.Slot)
	select {
	case err := <-clientErrors:
		t.Fatalf("client protocol error: %v", err)
	default:
	}
}
