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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// callDuringStopMetrics blocks in its first report for protocol 0x01 until
// released, then runs call against the muxer.
type callDuringStopMetrics struct {
	fakeMetrics
	m    *muxer.Muxer
	call func(*muxer.Muxer)
	// onDelivery selects the delivery hook, which runs on the forwarder
	// goroutine, instead of the depth hook, which first runs on the read
	// loop.
	onDelivery bool
	once       sync.Once
	// entered is closed once the hook is running; proceed releases it.
	entered chan struct{}
	proceed chan struct{}
	// returned is closed when call has returned.
	returned chan struct{}
}

func (r *callDuringStopMetrics) hook() {
	r.once.Do(func() {
		close(r.entered)
		<-r.proceed
		r.call(r.m)
		close(r.returned)
	})
}

func (r *callDuringStopMetrics) IngressQueueDepth(
	id uint16,
	role muxer.ProtocolRole,
	n int,
) {
	r.fakeMetrics.IngressQueueDepth(id, role, n)
	if !r.onDelivery && id == 0x01 {
		r.hook()
	}
}

func (r *callDuringStopMetrics) IngressDeliveryBlocked(
	id uint16,
	role muxer.ProtocolRole,
	d time.Duration,
) {
	r.fakeMetrics.IngressDeliveryBlocked(id, role, d)
	if r.onDelivery && id == 0x01 {
		r.hook()
	}
}

// connClosed reports whether the muxer has closed conn, which its cleanup
// does as soon as a stop is requested, immediately before it waits for the
// muxer's goroutines.
func connClosed(conn *mockConn) bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.closed
}

// runCallbackDuringStop runs call from a metrics callback that is released
// only after a stop was requested and the cleanup has begun waiting for the
// muxer's goroutines, then checks the call returns and shutdown completes.
func runCallbackDuringStop(
	t *testing.T,
	onDelivery bool,
	call func(*muxer.Muxer),
) {
	t.Helper()
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &callDuringStopMetrics{
		m:          m,
		call:       call,
		onDelivery: onDelivery,
		entered:    make(chan struct{}),
		proceed:    make(chan struct{}),
		returned:   make(chan struct{}),
	}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, _, _ = m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	m.Start()

	if onDelivery {
		// Fill the receive channel so a delivery has to wait, then free one
		// place so it completes and reports its wait.
		writePanicTestSegments(t, conn, 15, "payload")
		require.Eventually(t, func() bool {
			return len(recvChan) == cap(recvChan)
		}, 2*time.Second, time.Millisecond)
		<-time.After(20 * time.Millisecond)
		<-recvChan
	} else {
		writePanicTestSegments(t, conn, 1, "payload")
	}
	select {
	case <-metrics.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("metrics callback did not run")
	}

	// Request the stop while the callback is still running, let the cleanup
	// reach its wait for the muxer's goroutines, and only then let the
	// callback call into the muxer.
	m.Stop()
	require.Eventually(t, func() bool {
		return connClosed(conn)
	}, 2*time.Second, time.Millisecond, "cleanup did not start")
	<-time.After(50 * time.Millisecond)
	close(metrics.proceed)

	select {
	case <-metrics.returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the muxer call did not return during shutdown")
	}
	waitMuxerFinished(t, m)
}

// TestMetricsCallbackRegisterDuringStop checks that a metrics callback which
// calls RegisterProtocol after a stop was requested gets nil channels back
// and cannot hold up shutdown. Registration used to wait on a mutex that the
// cleanup held while waiting for the callback's own goroutine to exit, so
// ErrorChan never closed.
func TestMetricsCallbackRegisterDuringStop(t *testing.T) {
	for _, onDelivery := range []bool{false, true} {
		name := "read loop callback"
		if onDelivery {
			name = "forwarder callback"
		}
		t.Run(name, func(t *testing.T) {
			var registered atomic.Bool
			runCallbackDuringStop(t, onDelivery, func(m *muxer.Muxer) {
				send, recv, done := m.RegisterProtocol(
					0x03,
					muxer.ProtocolRoleResponder,
				)
				registered.Store(send != nil || recv != nil || done != nil)
			})
			require.False(t, registered.Load(), "registered during shutdown")
		})
	}
}

// TestMetricsCallbackMuxerCallsDuringStop checks that the muxer calls a
// metrics callback may make remain safe, and cannot hold up shutdown, when a
// stop is requested while the callback is running.
func TestMetricsCallbackMuxerCallsDuringStop(t *testing.T) {
	calls := map[string]func(*muxer.Muxer){
		"unregister reported role": func(m *muxer.Muxer) {
			m.UnregisterProtocol(0x01, muxer.ProtocolRoleResponder)
		},
		"unregister other protocol": func(m *muxer.Muxer) {
			m.UnregisterProtocol(0x02, muxer.ProtocolRoleResponder)
		},
		"limits and accounting": func(m *muxer.Muxer) {
			m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000)
			_ = m.IngressLimit(0x01, muxer.ProtocolRoleResponder)
			_ = m.IngressInUse()
			_ = m.IngressBudget()
		},
		"read buffer budget": func(m *muxer.Muxer) {
			if m.ReserveReadBuffer(1) {
				m.ReleaseReadBuffer(1)
			}
			_ = m.ReadBufferInUse()
		},
		"send": func(m *muxer.Muxer) {
			_ = m.Send(muxer.NewSegment(0x02, []byte("sent"), false))
		},
		"stop": func(m *muxer.Muxer) { m.Stop() },
	}
	for name, call := range calls {
		for _, onDelivery := range []bool{false, true} {
			hook := "read loop callback"
			if onDelivery {
				hook = "forwarder callback"
			}
			t.Run(name+"/"+hook, func(t *testing.T) {
				runCallbackDuringStop(t, onDelivery, call)
			})
		}
	}
}
