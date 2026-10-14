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

package muxer

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type panicEgressMetrics struct {
	Metrics
	calls atomic.Int32
}

func (p *panicEgressMetrics) EgressWait(
	_ uint16,
	_ EgressClass,
	_ time.Duration,
) {
	p.calls.Add(1)
	panic("injected EgressWait panic")
}

// TestEgressMetricsPanicIsContained checks that a panic in the egress wait
// callback does not escape Send, and that the turn it held is released for
// later sends.
func TestEgressMetricsPanicIsContained(t *testing.T) {
	t.Parallel()
	conn := newGateConn(3)
	m := newTestMuxer(t, conn)
	metrics := &panicEgressMetrics{}
	m.SetMetrics(metrics)

	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { first <- m.Send(newTestSegment(testPraosId)) }()
	require.Eventually(t, func() bool {
		m.egress.mu.Lock()
		defer m.egress.mu.Unlock()
		return m.egress.busy
	}, 5*time.Second, time.Millisecond)
	go func() { second <- m.Send(newTestSegment(testPraosId)) }()
	require.Eventually(t, func() bool {
		return m.egress.waiting() == 1
	}, 5*time.Second, time.Millisecond)
	for range 3 {
		conn.tokens <- struct{}{}
	}
	for _, result := range []chan error{first, second} {
		select {
		case err := <-result:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("send did not complete")
		}
	}
	require.Equal(t, int32(1), metrics.calls.Load())

	third := make(chan error, 1)
	go func() { third <- m.Send(newTestSegment(testPraosId)) }()
	select {
	case err := <-third:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("egress turn was not released after the callback panic")
	}
}
