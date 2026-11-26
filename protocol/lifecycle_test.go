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

package protocol

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const lifecycleTestTimeout = 5 * time.Second

func requireClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(lifecycleTestTimeout):
		t.Fatal(what)
	}
}

func requireOpen(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(what)
	default:
	}
}

// stopFromOutside calls Stop with a context that carries no callback marker
// on another goroutine and returns a channel
// closed when it returns.
func stopFromOutside(p *Protocol) <-chan struct{} {
	returned := make(chan struct{})
	go func() {
		_ = p.Stop(context.Background())
		close(returned)
	}()
	return returned
}

// TestLifecycleStopCallers stops a running protocol from each place Stop is
// called: from outside, from MessageHandlerFunc on recvLoop, from
// MessageReceivedFunc on readLoop, and through SendError after a message
// fails to decode. In each, Stop returns without the peer, DoneChan closes,
// readLoop exits, and no protocol goroutine is left.
func TestLifecycleStopCallers(t *testing.T) {
	tests := []struct {
		name   string
		config func(p **Protocol) ProtocolConfig
		send   bool
	}{
		{
			name:   "outside",
			config: func(**Protocol) ProtocolConfig { return ProtocolConfig{} },
		},
		{
			name: "message handler",
			config: func(p **Protocol) ProtocolConfig {
				return ProtocolConfig{
					MessageHandlerFunc: func(Message) error {
						return (*p).Stop((*p).CallbackContext())
					},
				}
			},
			send: true,
		},
		{
			name: "message received callback",
			config: func(p **Protocol) ProtocolConfig {
				return ProtocolConfig{
					MessageReceivedFunc: func(Message) {
						_ = (*p).Stop((*p).CallbackContext())
					},
				}
			},
			send: true,
		},
		{
			name: "decode error",
			config: func(**Protocol) ProtocolConfig {
				return ProtocolConfig{
					MessageFromCborFunc: func(uint, []byte) (Message, error) {
						return nil, errInjectedHandler
					},
				}
			},
			send: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ignore := goleak.IgnoreCurrent()
			var p *Protocol
			var sendMessage func()
			p, _, sendMessage = startPanicTestProtocol(t, test.config(&p))
			if test.send {
				sendMessage()
			} else {
				requireClosed(
					t,
					stopFromOutside(p),
					"Stop did not return",
				)
			}
			requireClosed(t, p.DoneChan(), "DoneChan did not close")
			requireReadLoopExited(t, p)
			// Stop is called again by the helper's cleanup; it must be a
			// no-op, and an outside Stop after an inside one must return.
			requireClosed(t, stopFromOutside(p), "a second Stop did not return")
			_ = p.config.Muxer.Stop(context.Background())
			require.NoError(t, goleak.Find(ignore))
		})
	}
}

// TestLifecycleStopWaitsForHandler pins that Stop joins the loops: while a
// MessageHandlerFunc runs, a Stop whose context ends returns that context's
// error and leaves DoneChan open, and a Stop without a deadline returns once
// the handler has returned and DoneChan has closed.
func TestLifecycleStopWaitsForHandler(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	entered := make(chan struct{})
	release := make(chan struct{})
	p, _, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		MessageHandlerFunc: func(Message) error {
			close(entered)
			<-release
			return nil
		},
	})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	// Runs before the helper's cleanup Stop, so a failure here does not
	// leave that Stop waiting on the held handler.
	t.Cleanup(releaseHandler)
	sendMessage()
	requireClosed(t, entered, "handler did not run")
	ctx, cancel := context.WithTimeout(
		context.Background(),
		50*time.Millisecond,
	)
	defer cancel()
	err := p.Stop(ctx)
	require.True(
		t,
		errors.Is(err, context.DeadlineExceeded),
		"Stop returned %v while a MessageHandlerFunc runs",
		err,
	)
	requireOpen(t, p.DoneChan(), "DoneChan closed while a handler runs")
	returned := stopFromOutside(p)
	requireOpen(t, returned, "Stop returned while a handler runs")
	releaseHandler()
	requireClosed(t, returned, "Stop did not return after the handler")
	requireClosed(t, p.DoneChan(), "DoneChan did not close")
	_ = p.config.Muxer.Stop(context.Background())
	require.NoError(t, goleak.Find(ignore))
}

// TestLifecycleHandlerWaitingForStopChan checks the callback behaviour the
// StopChan doc asks for: a handler that waits on StopChan is released by
// Stop, and the protocol then finishes.
func TestLifecycleHandlerWaitingForStopChan(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	entered := make(chan struct{})
	var p *Protocol
	p, _, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		MessageHandlerFunc: func(Message) error {
			close(entered)
			<-p.StopChan()
			return nil
		},
	})
	sendMessage()
	requireClosed(t, entered, "handler did not run")
	requireClosed(t, stopFromOutside(p), "Stop did not return")
	requireClosed(t, p.DoneChan(), "DoneChan did not close")
	requireReadLoopExited(t, p)
	_ = p.config.Muxer.Stop(context.Background())
	require.NoError(t, goleak.Find(ignore))
}

// TestLifecycleStopBeforeStart checks that Stop before Start closes DoneChan
// and that the later Start starts no goroutine.
func TestLifecycleStopBeforeStart(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	p := New(ProtocolConfig{
		Name:         "lifecycle",
		InitialState: panicTestInitialState,
		StateMap: StateMap{
			panicTestInitialState: {Agency: AgencyClient},
		},
	})
	require.NoError(t, p.Stop(context.Background()))
	requireClosed(t, p.DoneChan(), "DoneChan did not close")
	p.Start()
	require.True(t, p.IsStopping())
	require.NoError(t, goleak.Find(ignore))
}

// TestLifecycleRestartAfterTerminalMessage runs a server's restart the way
// the mini-protocol servers do: the handler for the message that ends the
// protocol calls StopForRestart on recvLoop. readLoop has queued that
// message only after MessageReceivedFunc returned for it, and stops reading
// after queueing it, so StopForRestart returns even though
// MessageReceivedFunc was held until then. The handler passes its callback
// context, so StopForRestart does not wait for recvLoop, which runs it.
func TestLifecycleRestartAfterTerminalMessage(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	doneState := NewState(3, "Done")
	received := make(chan struct{})
	releaseReceived := make(chan struct{})
	restarted := make(chan struct{})
	var p *Protocol
	p, _, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		StateMap: StateMap{
			panicTestInitialState: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType:  1,
					NewState: doneState,
				}},
			},
			doneState: {Agency: AgencyNone},
		},
		MessageReceivedFunc: func(Message) {
			close(received)
			<-releaseReceived
		},
		MessageHandlerFunc: func(Message) error {
			_ = p.StopForRestart(p.CallbackContext())
			close(restarted)
			return nil
		},
	})
	sendMessage()
	requireClosed(t, received, "MessageReceivedFunc did not run")
	requireOpen(t, restarted, "the handler ran before MessageReceivedFunc returned")
	close(releaseReceived)
	requireClosed(t, restarted, "StopForRestart did not return")
	requireReadLoopExited(t, p)
	requireClosed(t, p.DoneChan(), "DoneChan did not close")
	p.CancelRestart()
	_ = p.config.Muxer.Stop(context.Background())
	require.NoError(t, goleak.Find(ignore))
}

// TestLifecycleRestartFromReceivedCallback restarts a server from
// MessageReceivedFunc, on readLoop, with its callback context: StopForRestart
// returns there, readLoop exits, and the next instance of the role receives
// the messages readLoop had read and not queued.
func TestLifecycleRestartFromReceivedCallback(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	returned := make(chan struct{})
	var once sync.Once
	var p *Protocol
	p, _, _ = startPanicTestProtocol(t, ProtocolConfig{
		StateMap: StateMap{
			panicTestInitialState: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType:  1,
					NewState: panicTestInitialState,
				}},
			},
		},
		MessageReceivedFunc: func(Message) {
			once.Do(func() {
				_ = p.StopForRestart(p.CallbackContext())
				close(returned)
			})
		},
	})
	// Two messages in one segment, so readLoop holds the second when the
	// first one's callback restarts the server.
	segment := muxer.NewSegment(0, []byte{0x81, 0x01, 0x81, 0x01}, false)
	require.NotNil(t, segment)
	select {
	case p.muxerRecvChan <- segment:
	case <-time.After(lifecycleTestTimeout):
		t.Fatal("protocol never accepted the peer's messages")
	}
	requireClosed(t, returned, "StopForRestart from MessageReceivedFunc did not return")
	requireReadLoopExited(t, p)
	requireClosed(t, p.DoneChan(), "DoneChan did not close")

	handled := make(chan struct{}, 2)
	nextConfig := p.config
	nextConfig.MessageReceivedFunc = nil
	nextConfig.MessageHandlerFunc = func(Message) error {
		handled <- struct{}{}
		return nil
	}
	next := New(nextConfig)
	next.Start()
	for range 2 {
		requireReceived(t, handled, "the next instance did not get the carried messages")
	}
	require.NoError(t, next.Stop(context.Background()))
	_ = p.config.Muxer.Stop(context.Background())
	require.NoError(t, goleak.Find(ignore))
}

func requireReceived(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(lifecycleTestTimeout):
		t.Fatal(what)
	}
}

func requireReadLoopExited(t *testing.T, p *Protocol) {
	t.Helper()
	require.Eventually(t, func() bool {
		p.restartMu.Lock()
		defer p.restartMu.Unlock()
		return p.readExited
	}, lifecycleTestTimeout, time.Millisecond, "readLoop did not exit")
}
