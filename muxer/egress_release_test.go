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
	"testing"

	"github.com/stretchr/testify/require"
)

// release gives the turn to the head of the Praos queue unless a Leios
// segment waits and either no Praos segment waits or praosBurst Praos turns
// have passed it; an empty queue frees the turn. Each row is one release
// state.
func TestEgressReleaseGrantsOneTurn(t *testing.T) {
	tests := []struct {
		name         string
		praos, leios int
		praosRun     int
		want         EgressClass
		wantRun      int
		wantFree     bool
	}{
		{"nothing waits", 0, 0, 3, 0, 0, true},
		{"only Praos waits", 2, 0, 0, EgressClassPraos, 0, false},
		{"only Leios waits", 0, 2, 0, EgressClassLeios, 0, false},
		{"both wait, run below burst", 1, 1, 0, EgressClassPraos, 1, false},
		{"both wait, run one short of burst", 1, 1, praosBurst - 1,
			EgressClassPraos, praosBurst, false},
		{
			"both wait, run at burst",
			1,
			1,
			praosBurst,
			EgressClassLeios,
			0,
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := egress{busy: true, praosRun: tc.praosRun}
			var praos, leios []chan struct{}
			for range tc.praos {
				praos = append(praos, make(chan struct{}))
			}
			for range tc.leios {
				leios = append(leios, make(chan struct{}))
			}
			e.praos = append([]chan struct{}(nil), praos...)
			e.leios = append([]chan struct{}(nil), leios...)
			e.release()
			require.Equal(t, tc.wantRun, e.praosRun)
			require.Equal(t, !tc.wantFree, e.busy)
			if tc.wantFree {
				return
			}
			granted, other := praos, leios
			if tc.want == EgressClassLeios {
				granted, other = leios, praos
			}
			require.True(
				t,
				isClosed(granted[0]),
				"head of the class is granted",
			)
			for _, w := range append(granted[1:], other...) {
				require.False(t, isClosed(w), "one grant per release")
			}
		})
	}
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// Segments of one class are granted in the
// order they queued, so with one segment in flight per mini-protocol each
// waits at most one turn per other mini-protocol of its class, the
// round-robin of Network/Mux/Egress.hs.
func TestEgressPraosGrantedInQueueOrder(t *testing.T) {
	var e egress
	_, err := e.acquire(newTestSegment(2), nil)
	require.NoError(t, err)
	var waiters []<-chan error
	for _, id := range []uint16{2, 3, 4} {
		waiters = append(waiters, queueWaiter(t, &e, newTestSegment(id)))
	}
	for i, w := range waiters {
		e.release()
		requireGranted(t, w)
		for _, later := range waiters[i+1:] {
			requireNotGranted(t, later)
		}
	}
	e.release()
	require.False(t, e.busy)
}
