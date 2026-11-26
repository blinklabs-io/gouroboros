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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// tipWorkerBusyLockTimeout replaces Stop's 5 s wait for busyMutex in these
// tests. A Stop that returns well within it did not wait it out.
const tipWorkerBusyLockTimeout = 3 * time.Second

// newTipWorkerPair connects a node-to-client Client to a Server over a pipe.
// Cleanup is bounded so that a test that fails with the client wedged still
// returns.
func newTipWorkerPair(
	t *testing.T,
	serverCfg Config,
	clientCfg Config,
) (*Client, chan error) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	serverErrors := make(chan error, 8)
	clientErrors := make(chan error, 8)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: serverErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: testConnectionId(),
		Muxer:        clientMuxer,
		ErrorChan:    clientErrors,
		Mode:         protocol.ProtocolModeNodeToClient,
	}, &clientCfg)
	client.testStopBusyLockTimeout = tipWorkerBusyLockTimeout
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Stop(ctx)
		_ = server.Stop(ctx)
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = clientMuxer.Stop(ctx)
		_ = serverMuxer.Stop(ctx)
	})
	return client, clientErrors
}

// A query holds busyMutex while the peer is silent; Stop stops waiting for
// the lock once its ctx is done.
func TestStopReturnsOnCtxWhileBusyMutexHeld(t *testing.T) {
	client, cleanup := newStartedTestClient(t, nil)
	defer cleanup()
	client.testStopBusyLockTimeout = tipWorkerBusyLockTimeout
	requestSent := make(chan struct{})
	client.testIntersectRequestSent = func() { close(requestSent) }
	rangeDone := make(chan error, 1)
	go func() {
		_, _, err := client.GetAvailableBlockRange(nil)
		rangeDone <- err
	}()
	recv(t, requestSent, "the range query's MsgFindIntersect")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := client.Stop(ctx)
	elapsed := time.Since(start)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, tipWorkerBusyLockTimeout/2,
		"Stop kept waiting for busyMutex after its ctx was done")
	require.ErrorIs(t, recv(t, rangeDone, "the range query"),
		protocol.ErrProtocolShuttingDown)
}

// Jump holds busyMutex until the answer whose callback is running is
// delivered, so a Stop from that callback must not wait for the lock.
func TestStopFromCallbackDoesNotWaitForBusyMutex(t *testing.T) {
	type stopResult struct {
		err     error
		elapsed time.Duration
	}
	stopped := make(chan stopResult, 1)
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			return points[0], Tip{BlockNumber: 9}, nil
		}),
	)
	clientCfg := NewConfig(
		WithIntersectFoundFunc(func(
			ctx CallbackContext, _ pcommon.Point, _ Tip,
		) error {
			start := time.Now()
			err := ctx.Client.Stop(ctx.Context())
			stopped <- stopResult{err, time.Since(start)}
			return nil
		}),
	)
	client, _ := newTipWorkerPair(t, serverCfg, clientCfg)

	jumpDone := make(chan error, 1)
	go func() {
		_, _, _, err := client.Jump(
			[]pcommon.Point{pcommon.NewPoint(5, testPointHash(0x05))},
		)
		jumpDone <- err
	}()
	res := recv(t, stopped, "Stop called from IntersectFoundFunc")
	require.NoError(t, res.err)
	require.Less(t, res.elapsed, tipWorkerBusyLockTimeout/2,
		"Stop from a callback waited for busyMutex held by Jump")
	recv(t, jumpDone, "Jump")
}

// A callback that calls GetCurrentTip blocks the goroutine that would
// deliver its answer; Stop must still release it and return, which waiting
// on DoneChan alone cannot do because DoneChan closes only after that
// goroutine returns.
func TestStopReleasesGetCurrentTipCalledFromCallback(t *testing.T) {
	entered := make(chan struct{}, 1)
	tipErr := make(chan error, 1)
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			if len(points) == 0 {
				return pcommon.Point{}, Tip{BlockNumber: 3}, ErrIntersectNotFound
			}
			return points[0], Tip{BlockNumber: 3}, nil
		}),
		WithRequestNextFunc(func(ctx CallbackContext) error {
			return ctx.Server.RollBackward(
				pcommon.NewPointOrigin(), Tip{BlockNumber: 3},
			)
		}),
	)
	var client *Client
	clientCfg := NewConfig(
		WithPipelineLimit(1),
		WithRollBackwardFunc(func(CallbackContext, pcommon.Point, Tip) error {
			entered <- struct{}{}
			_, err := client.GetCurrentTip()
			tipErr <- err
			return nil
		}),
	)
	client, _ = newTipWorkerPair(t, serverCfg, clientCfg)
	client.testStopBusyLockTimeout = 10 * time.Millisecond

	require.NoError(t, client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	recv(t, entered, "the RollBackward callback")
	require.Eventually(t, func() bool {
		return len(client.wantIntersectFoundChan) == 1
	}, 5*time.Second, time.Millisecond, "GetCurrentTip never sent its request")

	stopDone := make(chan error, 1)
	go func() { stopDone <- client.Stop(context.Background()) }()
	require.ErrorIs(t, recv(t, tipErr, "GetCurrentTip in the callback"),
		protocol.ErrProtocolShuttingDown)
	recv(t, stopDone, "Stop")
}

// Jump holds busyMutex until the answer whose IntersectFoundFunc is running
// is delivered, so a GetCurrentTip from that callback waits for the lock
// rather than for its own answer. Stop must release that wait as well.
func TestStopReleasesGetCurrentTipWaitingForBusyMutexInCallback(t *testing.T) {
	entered := make(chan struct{}, 1)
	tipErr := make(chan error, 1)
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			return points[0], Tip{BlockNumber: 3}, nil
		}),
	)
	var client *Client
	clientCfg := NewConfig(
		WithIntersectFoundFunc(func(CallbackContext, pcommon.Point, Tip) error {
			entered <- struct{}{}
			_, err := client.GetCurrentTip()
			tipErr <- err
			return nil
		}),
	)
	client, _ = newTipWorkerPair(t, serverCfg, clientCfg)
	client.testStopBusyLockTimeout = 10 * time.Millisecond

	jumpDone := make(chan error, 1)
	go func() {
		_, _, _, err := client.Jump(
			[]pcommon.Point{pcommon.NewPoint(5, testPointHash(0x05))},
		)
		jumpDone <- err
	}()
	recv(t, entered, "IntersectFoundFunc")

	stopDone := make(chan error, 1)
	go func() { stopDone <- client.Stop(context.Background()) }()
	require.ErrorIs(t, recv(t, tipErr, "GetCurrentTip in the callback"),
		protocol.ErrProtocolShuttingDown)
	recv(t, stopDone, "Stop")
	recv(t, jumpDone, "Jump")
}

// Each reply shape the server may give GetAvailableBlockRange yields the
// range from that call's own replies, and leaves no request, ready signal or
// registration behind.
func TestGetAvailableBlockRangeReplyShapes(t *testing.T) {
	intersect := pcommon.NewPoint(10, testPointHash(0x0b))
	tip := Tip{BlockNumber: 70, Point: pcommon.NewPoint(1000, testPointHash(0xff))}
	blockCbor := rangeTestBlockCbor(t)
	tests := []struct {
		name          string
		rollBackFirst bool
		serverTip     Tip
		wantEmpty     bool
		wantRequests  int
	}{
		{name: "roll back then forward", rollBackFirst: true, serverTip: tip, wantRequests: 2},
		{name: "roll forward at once", rollBackFirst: false, serverTip: tip, wantRequests: 1},
		{name: "intersection is the tip", serverTip: Tip{BlockNumber: 1, Point: intersect}, wantEmpty: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			serverCfg := NewConfig(
				WithFindIntersectFunc(func(
					_ CallbackContext, points []pcommon.Point,
				) (pcommon.Point, Tip, error) {
					return points[0], tc.serverTip, nil
				}),
				WithRequestNextFunc(func(ctx CallbackContext) error {
					requests++
					if tc.rollBackFirst && requests == 1 {
						return ctx.Server.RollBackward(intersect, tc.serverTip)
					}
					return ctx.Server.RollForward(
						ledger.BlockTypeBabbage, blockCbor, tc.serverTip,
					)
				}),
			)
			clientCfg := NewConfig(WithPipelineLimit(3))
			clientCfg.SkipBlockValidation = true
			client, clientErrors := newTipWorkerPair(t, serverCfg, clientCfg)

			start, end, err := client.GetAvailableBlockRange(
				[]pcommon.Point{intersect},
			)
			require.NoError(t, err)
			if tc.wantEmpty {
				require.Equal(t, pcommon.Point{}, start)
				require.Equal(t, pcommon.Point{}, end)
			} else {
				require.Equal(t, uint64(50), start.Slot,
					"start is not the block after the intersection")
				require.Equal(t, tc.serverTip.Point, end)
			}
			require.Equal(t, tc.wantRequests, requests)

			client.lifecycleMutex.Lock()
			outstanding := client.outstandingRequestNext
			ready := len(client.readyForNextBlockChan)
			client.lifecycleMutex.Unlock()
			require.Zero(t, outstanding, "a MsgRequestNext is still counted")
			require.Zero(t, ready, "a ready signal was left for the next sync")
			require.Zero(t, len(client.wantCurrentTipChan))
			require.Zero(t, len(client.wantFirstBlockChan))
			require.Zero(t, len(client.wantIntersectFoundChan))
			select {
			case err := <-clientErrors:
				t.Fatalf("client protocol error: %v", err)
			default:
			}
		})
	}
}
