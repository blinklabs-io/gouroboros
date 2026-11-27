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

package leiosnotify

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestStopWaitsForRequestSource checks that Stop waits for a RequestNextFunc
// that is still running, bounded by its context, and that no goroutine is
// left once it returns. The reference runs a mini-protocol's work in its own
// thread, which the mux waits for before it returns (network-mux
// Network/Mux.hs:280-287).
func TestStopWaitsForRequestSource(t *testing.T) {
	// Registered first, so it runs after every other cleanup.
	t.Cleanup(func() { goleak.VerifyNone(t) })
	entered := make(chan struct{})
	release := make(chan struct{})
	server, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			close(entered)
			// This source ignores QuitChan.
			<-release
			return nil, errors.New("released")
		}),
	))
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source did not run")
	}
	// Stop cannot finish while the source runs; this bounds how long the
	// test gives it to do so wrongly.
	short, cancelShort := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancelShort()
	require.ErrorIs(
		t, server.Stop(short), context.DeadlineExceeded,
		"Stop returned while the request source was running",
	)
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx))
}

// TestStopWaitsForSourceOfRetiredRun checks that Stop also waits for the
// goroutines of a run the client ended with MsgQuit. The source of that run
// ignores QuitChan, so after the restart it is still running on the retired
// instance and its late notification still goes to ResponseSentFunc; Stop
// must not return before that callback has finished.
func TestStopWaitsForSourceOfRetiredRun(t *testing.T) {
	// Registered first, so it runs after every other cleanup. Goroutines
	// left by earlier tests in the package are not this test's.
	before := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, before) })
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var stopReturned atomic.Bool
	callbackAfterStop := make(chan bool, 1)
	response := NewMsgBlockAnnouncement(
		testAnnouncementEra,
		cbor.RawMessage{0x82, 0x01, 0x02},
	)
	server, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			if calls.Add(1) > 1 {
				return response, nil
			}
			close(entered)
			// This source ignores QuitChan.
			<-release
			return response, nil
		}),
		WithResponseSentFunc(
			func(_ CallbackContext, _ protocol.Message, err error) {
				if errors.Is(err, ErrRequestCanceled) {
					callbackAfterStop <- stopReturned.Load()
				}
			},
		),
	))
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source did not run")
	}
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	// An answer on the next run shows the restart has happened, so Stop
	// below stops the new instance, not the retired one.
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	require.IsType(t, &MsgBlockAnnouncement{}, readQuitTestMessage(t, conn))
	// Stop cannot finish while the retired run's source runs; this bounds
	// how long the test gives it to do so wrongly.
	short, cancelShort := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancelShort()
	err := server.Stop(short)
	if err == nil {
		stopReturned.Store(true)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx))
	stopReturned.Store(true)
	select {
	case after := <-callbackAfterStop:
		require.False(t, after, "ResponseSentFunc ran after Stop returned")
	default:
		t.Fatal("Stop returned before the late ResponseSentFunc ran")
	}
	require.ErrorIs(
		t, err, context.DeadlineExceeded,
		"Stop returned while the retired run's source was running",
	)
}

// TestStopFromCallbackDoesNotWaitForRetiredRun checks that Stop called with
// the Context of a callback returns without waiting, as documented, while a
// run the client ended with MsgQuit still has a source running. Waiting
// there would block the callback on goroutines that need not ever return.
func TestStopFromCallbackDoesNotWaitForRetiredRun(t *testing.T) {
	// Registered first, so it runs after every other cleanup. Goroutines
	// left by earlier tests in the package are not this test's.
	before := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, before) })
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSource := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseSource)
	var calls atomic.Int32
	var srv atomic.Pointer[Server]
	stopped := make(chan error, 1)
	server, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			if calls.Add(1) > 1 {
				stopped <- srv.Load().Stop(ctx.Context())
				return nil, errors.New("stopped")
			}
			close(entered)
			// This source ignores QuitChan.
			<-release
			return nil, errors.New("released")
		}),
	))
	srv.Store(server)
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source did not run")
	}
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from a callback waited for the retired run's source")
	}
	releaseSource()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx))
}

// TestStopFromRetiredRunCallbackDoesNotWait checks that Stop called with the
// Context of a callback of a run the client ended with MsgQuit returns
// without waiting, while the source of that run and of the current run are
// both still running. Waiting on the current run would block the retired
// callback on a source that need not ever return.
func TestStopFromRetiredRunCallbackDoesNotWait(t *testing.T) {
	// Registered first, so it runs after every other cleanup. Goroutines
	// left by earlier tests in the package are not this test's.
	before := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, before) })
	retiredEntered := make(chan struct{})
	currentEntered := make(chan struct{})
	releaseRetired := make(chan struct{})
	releaseCurrent := make(chan struct{})
	var retiredOnce, currentOnce sync.Once
	releaseRetiredSource := func() {
		retiredOnce.Do(func() { close(releaseRetired) })
	}
	releaseCurrentSource := func() {
		currentOnce.Do(func() { close(releaseCurrent) })
	}
	t.Cleanup(releaseRetiredSource)
	t.Cleanup(releaseCurrentSource)
	var calls atomic.Int32
	var srv atomic.Pointer[Server]
	stopped := make(chan error, 1)
	server, conn := newQuitTestServer(t, NewConfig(
		// Both sources ignore QuitChan.
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			if calls.Add(1) > 1 {
				close(currentEntered)
				<-releaseCurrent
				return nil, errors.New("released")
			}
			close(retiredEntered)
			<-releaseRetired
			stopped <- srv.Load().Stop(ctx.Context())
			return nil, errors.New("stopped")
		}),
	))
	srv.Store(server)
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-retiredEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source did not run")
	}
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-currentEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source of the current run did not run")
	}
	// Only the retired run's source is released; the current run's source
	// keeps running while the retired callback calls Stop.
	releaseRetiredSource()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop from a retired run's callback waited for the current run")
	}
	releaseCurrentSource()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx))
}
