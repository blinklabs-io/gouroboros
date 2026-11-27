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
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestGetCurrentTipWithRequestOutstandingSendsNothing runs GetCurrentTip
// while a sync at the chain tip has a MsgRequestNext outstanding. It must
// return the tip of the last reply without sending MsgFindIntersect: that
// message would wait for the roll answering the sync's request, and an
// answer the roll overtook would go to the next call waiting for an
// intersect answer, so the Jump below would report no intersection for a
// point the server has.
func TestGetCurrentTipWithRequestOutstandingSendsNothing(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(func() { _ = clientMuxer.Stop(context.Background()) })
	t.Cleanup(func() { _ = serverMuxer.Stop(context.Background()) })

	// The sync's intersect answer and the roll carry different tips, so the
	// test can tell which one GetCurrentTip returned.
	syncTip := Tip{BlockNumber: 5}
	rollTip := Tip{BlockNumber: 7}
	answerTip := Tip{BlockNumber: 42}
	var tipQueries atomic.Int32
	requested := make(chan struct{}, 1)
	releaseRoll := make(chan struct{})
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			if len(points) == 0 {
				tipQueries.Add(1)
				return pcommon.Point{}, answerTip, ErrIntersectNotFound
			}
			return points[0], syncTip, nil
		}),
		WithRequestNextFunc(func(ctx CallbackContext) error {
			if err := ctx.Server.AwaitReply(); err != nil {
				return err
			}
			requested <- struct{}{}
			go func() {
				<-releaseRoll
				_ = ctx.Server.RollBackward(pcommon.NewPointOrigin(), rollTip)
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
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	serverMuxer.Start()
	clientMuxer.Start()

	require.NoError(t, client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received MsgRequestNext")
	}

	tip := getCurrentTipWithin(t, client, 5*time.Second)
	require.Equal(t, syncTip, tip,
		"GetCurrentTip did not return the last reply's tip")

	close(releaseRoll)
	require.Eventually(t, func() bool {
		return client.prepareIdleProtocol(make(chan bool)) == nil
	}, 5*time.Second, time.Millisecond, "sync did not stop")
	require.Zero(t, tipQueries.Load(),
		"GetCurrentTip sent MsgFindIntersect while a request was outstanding")
	// With nothing outstanding it asks the server, and takes its own answer.
	tip = getCurrentTipWithin(t, client, 5*time.Second)
	require.Equal(t, answerTip, tip)
	require.Equal(t, int32(1), tipQueries.Load())

	jumpPoint := pcommon.NewPoint(5, testPointHash(0x05))
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
