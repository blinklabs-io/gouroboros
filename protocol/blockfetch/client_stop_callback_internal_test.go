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

package blockfetch

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func setLifecycleState(c *Client, state clientLifecycleState) {
	c.lifecycleMutex.Lock()
	c.lifecycleState = state
	c.lifecycleMutex.Unlock()
}

// A Stop that finds the client not running (never started, or already
// stopped by a Stop still waiting for the protocol to exit) resolves the
// outstanding ranges itself. RangeDoneFunc must then run with no client lock
// held, as the reference hands blocks to the consumer on the protocol thread
// with none (ouroboros-network/lib/Ouroboros/Network/BlockFetch/Client.hs,
// addFetchedBlock), so a callback may call Stop again.
func TestStopNotRunningCallbackMayCallStop(t *testing.T) {
	for _, test := range []struct {
		name  string
		state clientLifecycleState
	}{
		{"new", clientStateNew},
		{"stopped", clientStateStopped},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
			var c *Client
			innerStop := make(chan error, 1)
			c = newQueueTestClient(&Config{
				RequestPipelining: true,
				RangeDoneFunc: func(cb CallbackContext, err error) error {
					require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
					innerStop <- c.Stop(cb.Context())
					return nil
				},
			})
			c.appendTestRequest(1)
			setLifecycleState(c, test.state)

			outerStop := make(chan error, 1)
			go func() { outerStop <- c.Stop(context.Background()) }()
			for name, stopped := range map[string]chan error{
				"Stop called from RangeDoneFunc": innerStop,
				"Stop":                           outerStop,
			} {
				select {
				case err := <-stopped:
					require.NoError(t, err)
				case <-time.After(2 * time.Second):
					t.Fatalf("%s did not return: RangeDoneFunc ran under lifecycleMutex", name)
				}
			}
		})
	}
}

// The same Stop must not report a range retired while the receive loop is
// still delivering one of its blocks: the terminal report waits for the block
// callback, as it does when a range deadline retires the request.
func TestStopNotRunningWaitsForActiveDelivery(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var delivering atomic.Bool
	var doneDuringDelivery atomic.Bool
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	rangeDone := make(chan error, 1)
	block, point := queueTestBlock(t, 100, lcommon.Blake2b256{})
	c := newQueueTestClient(&Config{
		RequestPipelining:   true,
		SkipBlockValidation: true,
		BlockRawFunc: func(CallbackContext, uint, []byte) error {
			delivering.Store(true)
			close(callbackStarted)
			<-releaseCallback
			delivering.Store(false)
			return nil
		},
		RangeDoneFunc: func(_ CallbackContext, err error) error {
			doneDuringDelivery.Store(delivering.Load())
			rangeDone <- err
			return nil
		},
	})
	req := c.appendTestRequest(1024)
	req.start = point
	req.end = point
	req.started = true
	req.retired = make(chan struct{})

	handlerDone := make(chan error, 1)
	go func() { handlerDone <- c.handleBlock(block) }()
	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("block callback did not start")
	}
	setLifecycleState(c, clientStateStopped)
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(context.Background()) }()

	// Give a Stop that does not wait for the delivery the chance to report
	// the range early; one that waits stays blocked until the release.
	var doneErr error
	reported := false
	select {
	case doneErr = <-rangeDone:
		reported = true
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseCallback)
	if !reported {
		select {
		case doneErr = <-rangeDone:
		case <-time.After(time.Second):
			t.Fatal("range was not reported after its block callback returned")
		}
	}
	require.False(t, doneDuringDelivery.Load(),
		"RangeDoneFunc ran while the range's block callback was active")
	require.ErrorIs(t, doneErr, protocol.ErrProtocolShuttingDown)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop did not return")
	}
	require.NoError(t, <-handlerDone)
}

// requestOnePoint sends a RangeRequest for one point and waits until the peer
// has read it, so the peer's answers follow the request.
func requestOnePoint(t *testing.T, p *idlePeer, point pcommon.Point) {
	t.Helper()
	start := p.outboundRead.value()
	_, err := p.client.RequestRange(
		context.Background(),
		RangeRequest{Start: point, End: point, ExpectedBytes: 1},
	)
	require.NoError(t, err)
	p.waitForRequestRange(t, start, point)
}

func (p *idlePeer) waitForRequestRange(
	t *testing.T,
	start int,
	point pcommon.Point,
) {
	t.Helper()
	data, err := cbor.Encode(NewMsgRequestRange(point, point))
	require.NoError(t, err)
	p.waitForOutboundBytes(t, start+binary.Size(muxer.SegmentHeader{})+len(data))
}

// Every callback that runs on the receive loop may stop the client with
// Stop and its callback context. Stop then cannot wait for the
// receive loop, which is its own caller: it requests shutdown and returns,
// and the shutdown watcher resolves what is left.
func TestStopFromReceiveLoopCallbackReturns(t *testing.T) {
	block, point := queueTestBlock(t, 100, lcommon.Blake2b256{})
	for _, test := range []struct {
		name  string
		setup func(cfg *Config, stop func(CallbackContext))
		drive func(t *testing.T, p *idlePeer)
	}{
		{
			name: "BlockRawFunc",
			setup: func(cfg *Config, stop func(CallbackContext)) {
				cfg.BlockRawFunc = func(cb CallbackContext, _ uint, _ []byte) error {
					stop(cb)
					return nil
				}
			},
			drive: func(t *testing.T, p *idlePeer) {
				requestOnePoint(t, p, point)
				p.send(t, NewMsgStartBatch())
				p.send(t, block)
			},
		},
		{
			name: "BlockFunc",
			setup: func(cfg *Config, stop func(CallbackContext)) {
				cfg.BlockFunc = func(cb CallbackContext, _ uint, _ ledger.Block) error {
					stop(cb)
					return nil
				}
			},
			drive: func(t *testing.T, p *idlePeer) {
				requestOnePoint(t, p, point)
				p.send(t, NewMsgStartBatch())
				p.send(t, block)
			},
		},
		{
			name: "RangeDoneFunc",
			setup: func(cfg *Config, stop func(CallbackContext)) {
				cfg.RangeDoneFunc = func(cb CallbackContext, _ error) error {
					stop(cb)
					return nil
				}
			},
			drive: func(t *testing.T, p *idlePeer) {
				requestOnePoint(t, p, point)
				p.send(t, NewMsgNoBlocks())
			},
		},
		{
			name: "BatchDoneFunc",
			setup: func(cfg *Config, stop func(CallbackContext)) {
				cfg.RequestPipelining = false
				cfg.BlockRawFunc = func(CallbackContext, uint, []byte) error {
					return nil
				}
				cfg.BatchDoneFunc = func(cb CallbackContext) error {
					stop(cb)
					return nil
				}
			},
			drive: func(t *testing.T, p *idlePeer) {
				start := p.outboundRead.value()
				started := make(chan error, 1)
				go func() { started <- p.client.GetBlockRange(point, point) }()
				p.waitForRequestRange(t, start, point)
				p.send(t, NewMsgStartBatch())
				require.NoError(t, <-started)
				p.send(t, block)
				p.send(t, NewMsgBatchDone())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var p *idlePeer
			stopped := make(chan error, 1)
			cfg := &Config{
				RequestPipelining:   true,
				SkipBlockValidation: true,
				RangeDoneFunc:       func(CallbackContext, error) error { return nil },
			}
			test.setup(cfg, func(cb CallbackContext) {
				stopped <- p.client.Stop(cb.Context())
			})
			p = newIdlePeer(t, cfg)
			test.drive(t, p)
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-stopped:
			case <-timer.C:
				t.Fatal("Stop called from a receive-loop callback did not return")
			}
			select {
			case <-p.client.DoneChan():
			case <-timer.C:
				t.Fatal("the protocol did not finish after Stop")
			}
		})
	}
}

// When Stop returns, the goroutines the client started have exited, so no
// callback of the stopped protocol can still be running: here a range
// deadline is resolving its request when Stop is called. Stop given the
// context of another client's callback is a stop from outside this client's
// callbacks, so it waits too.
func TestStopWaitsForDeadlineWatcher(t *testing.T) {
	other := newQueueTestClient(nil)
	otherCallback := other.callbackContextFor(
		&rangeRequest{id: 1, protocol: other.ProtocolInstance()},
	).Context()
	for _, test := range []struct {
		name string
		stop func(*Client) error
	}{
		{"Stop", func(c *Client) error { return c.Stop(context.Background()) }},
		{"Stop from another client's callback", func(c *Client) error {
			return c.Stop(otherCallback)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			p := newIdlePeer(t, &Config{
				RequestPipelining: true,
				RangeTimeout:      50 * time.Millisecond,
				RangeDoneFunc: func(_ CallbackContext, err error) error {
					if errors.Is(err, ErrRangeTimeout) {
						close(entered)
						<-release
					}
					return nil
				},
			})
			_, point := queueTestBlock(t, 100, lcommon.Blake2b256{})
			requestOnePoint(t, p, point)
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-entered:
			case <-timer.C:
				t.Fatal("the range deadline did not fire")
			}
			stopped := make(chan error, 1)
			go func() { stopped <- test.stop(p.client) }()
			// The callback is held, so a stop that joins the deadline watcher
			// cannot return yet; one that returns now left it running.
			window := time.NewTimer(200 * time.Millisecond)
			defer window.Stop()
			select {
			case <-stopped:
				close(release)
				t.Fatal("stop returned while the deadline watcher was running RangeDoneFunc")
			case <-window.C:
			}
			close(release)
			select {
			case <-stopped:
			case <-timer.C:
				t.Fatal("stop did not return after the deadline watcher finished")
			}
		})
	}
}

// A Start can install a new protocol instance while a running Stop waits for
// the old one with lifecycleMutex released. That Stop then resolves only the
// old instance's ranges and releases only the busy lock taken before it:
// a range queued on the new instance, and the new instance's busy lock,
// are left alone.
func TestStopLeavesRestartedInstanceRequests(t *testing.T) {
	p := newIdlePeer(t, &Config{
		RequestPipelining:   true,
		SkipBlockValidation: true,
		RangeDoneFunc:       func(CallbackContext, error) error { return nil },
	})
	c := p.client
	var (
		newReq   *rangeRequest
		newToken uint64
		oldProto = c.ProtocolInstance()
	)
	c.stopUnlockedHook = func() {
		c.stopUnlockedHook = nil
		c.Start()
		require.NotSame(t, oldProto, c.ProtocolInstance(),
			"Start did not install a new protocol instance")
		newReq = c.appendTestRequest(1)
		newToken, _ = c.acquireBusy()
	}
	require.NoError(t, c.Stop(context.Background()))
	require.NotNil(t, newReq, "the hook did not run")
	select {
	case err := <-newReq.doneChan:
		t.Fatalf("Stop of the old instance resolved the new instance's range: %v", err)
	default:
	}
	c.queueMutex.Lock()
	queued := slices.Contains(c.queue, newReq)
	c.queueMutex.Unlock()
	require.True(t, queued, "Stop of the old instance removed the new instance's range")
	c.busyStateMutex.Lock()
	held := c.busyLocked && c.busyToken == newToken
	c.busyStateMutex.Unlock()
	require.True(t, held, "Stop of the old instance released the new instance's busy lock")
	c.releaseBusy(newToken)
	require.NoError(t, c.Stop(context.Background()))
}
