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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Start must not return while a callback of the previous protocol instance
// is still running: here a range deadline of that instance is in
// RangeDoneFunc. The previous instance can be left with such a goroutine by
// a Stop whose context ended before its join, or by a Start that runs while a
// concurrent Stop is still joining.
func TestStartWaitsForPreviousInstanceCallbacks(t *testing.T) {
	for _, test := range []struct {
		name string
		// restart stops the client and starts it again, reporting on started
		// once Start has returned.
		restart func(t *testing.T, c *Client, started chan<- struct{})
	}{
		{
			name: "after a Stop whose context ended",
			restart: func(t *testing.T, c *Client, started chan<- struct{}) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				require.ErrorIs(t, c.Stop(ctx), context.Canceled)
				go func() {
					c.Start()
					close(started)
				}()
			},
		},
		{
			name: "during a concurrent Stop",
			restart: func(t *testing.T, c *Client, started chan<- struct{}) {
				c.stopUnlockedHook = func() {
					c.stopUnlockedHook = nil
					go func() {
						c.Start()
						close(started)
					}()
				}
				go func() { _ = c.Stop(context.Background()) }()
			},
		},
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
			// The deadline stopped the protocol. Once its loops are done,
			// Stop has no MsgClientDone to send, so it cannot give up on the
			// connection and leave Start refusing to start.
			oldProto := p.client.ProtocolInstance()
			select {
			case <-oldProto.DoneChan():
			case <-timer.C:
				t.Fatal("the range deadline did not stop the protocol")
			}
			started := make(chan struct{})
			test.restart(t, p.client, started)
			// The old instance's callback is held, so a Start that waits for
			// it cannot return yet.
			window := time.NewTimer(200 * time.Millisecond)
			defer window.Stop()
			select {
			case <-started:
				close(release)
				t.Fatal("Start returned while the previous instance was running RangeDoneFunc")
			case <-window.C:
			}
			close(release)
			select {
			case <-started:
			case <-timer.C:
				t.Fatal("Start did not return after the previous instance's callback")
			}
			require.NotSame(t, oldProto, p.client.ProtocolInstance(),
				"Start did not install a new protocol instance")
		})
	}
}

// awaitStartHeld checks that Start, which reports on started, does not return
// while the previous instance's RangeDoneFunc, which signals entered, waits
// for release, and that it returns once release is closed.
func awaitStartHeld(
	t *testing.T,
	entered <-chan struct{},
	release chan struct{},
	started <-chan struct{},
) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-entered:
	case <-timer.C:
		close(release)
		t.Fatal("the previous instance's RangeDoneFunc did not run")
	}
	window := time.NewTimer(200 * time.Millisecond)
	defer window.Stop()
	select {
	case <-started:
		close(release)
		t.Fatal("Start returned while the previous instance was running RangeDoneFunc")
	case <-window.C:
	}
	close(release)
	select {
	case <-started:
	case <-timer.C:
		t.Fatal("Start did not return after the previous instance's callback")
	}
}

// Stop resolves the ranges of its instance that are still queued once its
// join returns, running RangeDoneFunc with lifecycleMutex released. A Start
// that runs meanwhile must not return before that callback has.
func TestStartWaitsForStopTerminalCallbacks(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	p := newIdlePeer(t, &Config{
		RequestPipelining: true,
		RangeDoneFunc: func(_ CallbackContext, err error) error {
			if errors.Is(err, protocol.ErrProtocolShuttingDown) {
				close(entered)
				<-release
			}
			return nil
		},
	})
	c := p.client
	oldProto := c.ProtocolInstance()
	started := make(chan struct{})
	c.stopUnlockedHook = func() {
		c.stopUnlockedHook = nil
		// Once the old instance's loops are done, its shutdown watcher can
		// no longer take the range, which is left to Stop.
		_ = oldProto.Stop(context.Background())
		c.appendTestRequest(1)
		go func() {
			c.Start()
			close(started)
		}()
	}
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(context.Background()) }()
	awaitStartHeld(t, entered, release, started)
	require.NoError(t, <-stopped)
	require.NotSame(t, oldProto, c.ProtocolInstance(),
		"Start did not install a new protocol instance")
}

// A shutdown drain leaves a range whose MsgRequestRange is being sent to its
// sender, which resolves it on the RequestRange caller's goroutine once the
// send has succeeded. A Start after the instance stopped must not return
// before that callback has.
func TestStartWaitsForDrainedSenderCallback(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	senderParked := make(chan struct{})
	senderGo := make(chan struct{})
	var senderID atomic.Uint64
	p := newIdlePeer(t, &Config{
		RequestPipelining: true,
		RangeTimeout:      300 * time.Millisecond,
		RangeDoneFunc: func(ctx CallbackContext, _ error) error {
			if ctx.RequestId == senderID.Load() {
				close(entered)
				<-release
			}
			return nil
		},
	})
	c := p.client
	oldProto := c.ProtocolInstance()
	_, first := queueTestBlock(t, 100, lcommon.Blake2b256{})
	requestOnePoint(t, p, first)
	_, second := queueTestBlock(t, 101, lcommon.Blake2b256{})
	c.afterRangeSend = func() {
		c.afterRangeSend = nil
		c.queueMutex.Lock()
		senderID.Store(c.nextRequestId)
		c.queueMutex.Unlock()
		close(senderParked)
		<-senderGo
	}
	sent := make(chan error, 1)
	go func() {
		_, err := c.RequestRange(
			context.Background(),
			RangeRequest{Start: second, End: second, ExpectedBytes: 1},
		)
		sent <- err
	}()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-senderParked:
	case <-timer.C:
		t.Fatal("the second MsgRequestRange was not sent")
	}
	// The first range's deadline drains both ranges and stops the protocol,
	// leaving the second to its sender.
	select {
	case <-oldProto.DoneChan():
	case <-timer.C:
		t.Fatal("the range deadline did not stop the protocol")
	}
	require.NoError(t, c.Stop(context.Background()))
	started := make(chan struct{})
	go func() {
		c.Start()
		close(started)
	}()
	close(senderGo)
	awaitStartHeld(t, entered, release, started)
	require.NoError(t, <-sent)
	require.NotSame(t, oldProto, c.ProtocolInstance(),
		"Start did not install a new protocol instance")
}
