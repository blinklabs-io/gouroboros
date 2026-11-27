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

package muxer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// heldMetrics runs onFirst from its first depth report for protocol 0x01,
// which arrives on the muxer's read loop.
type heldMetrics struct {
	fakeMetrics
	once    sync.Once
	onFirst func()
}

func (h *heldMetrics) IngressQueueDepth(
	id uint16,
	role muxer.ProtocolRole,
	n int,
) {
	h.fakeMetrics.IngressQueueDepth(id, role, n)
	if id == 0x01 {
		h.once.Do(h.onFirst)
	}
}

// startHeldMuxer starts a muxer whose first metrics callback runs onFirst,
// and returns once that callback has begun.
func startHeldMuxer(t *testing.T, onFirst func(*muxer.Muxer)) *muxer.Muxer {
	t.Helper()
	conn := newMockConn()
	m := muxer.New(conn)
	entered := make(chan struct{})
	m.SetMetrics(&heldMetrics{onFirst: func() {
		close(entered)
		onFirst(m)
	}})
	_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.Start()
	writePanicTestSegments(t, conn, 1, "payload")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("metrics callback did not run")
	}
	return m
}

// TestMuxerStopWaitsForMetricsCallback checks that Stop returns only once
// the muxer's goroutines, which run the Metrics callbacks, have returned,
// and that a ctx ending first makes it return ctx.Err() without stopping
// the shutdown.
func TestMuxerStopWaitsForMetricsCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	m := startHeldMuxer(t, func(*muxer.Muxer) { <-release })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	require.ErrorIs(
		t,
		m.Stop(ctx),
		context.DeadlineExceeded,
		"Stop returned while a metrics callback was running",
	)

	releaseCallback()
	require.NoError(t, m.Stop(context.Background()))
	select {
	case _, ok := <-m.ErrorChan():
		require.False(t, ok, "ErrorChan open after Stop returned")
	default:
		t.Fatal("ErrorChan open after Stop returned")
	}
}

// TestMuxerStopFromMetricsCallback checks that a Metrics callback stopping
// its own muxer with the muxer's CallbackContext returns at once, since
// Stop would otherwise wait for the goroutine running the callback, and
// that shutdown then completes.
func TestMuxerStopFromMetricsCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	stopped := make(chan error, 1)
	m := startHeldMuxer(t, func(m *muxer.Muxer) {
		stopped <- m.Stop(m.CallbackContext())
	})
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from a metrics callback waited for its own goroutine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, m.Stop(ctx))
}
