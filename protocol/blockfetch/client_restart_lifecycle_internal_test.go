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
	"testing"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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
