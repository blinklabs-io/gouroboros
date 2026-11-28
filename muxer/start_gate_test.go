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
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// passedGateConn reports each time the read loop passes the start gate: the
// read loop sets the read deadline right after the gate, before it reads.
type passedGateConn struct {
	net.Conn
	passed chan struct{}
}

func (c *passedGateConn) SetReadDeadline(d time.Time) error {
	c.passed <- struct{}{}
	return c.Conn.SetReadDeadline(d)
}

type gateStep int

const (
	gateStartOnce gateStep = iota
	gateStart
	// gatePassed waits until the read loop has taken what opened the gate
	// and is about to read.
	gatePassed
)

// Each row is a sequence of Start and StartOnce calls and the number of
// segments the read loop may then read out of three the peer sends. Without
// Start, each StartOnce lets one more segment through; once Start has been
// called all three are read, including when a StartOnce is still pending.
func TestStartGateTransitions(t *testing.T) {
	const sent = 3
	tests := []struct {
		name  string
		steps []gateStep
		reads int
	}{
		{"no call reads nothing", nil, 0},
		{"StartOnce reads one", []gateStep{gateStartOnce}, 1},
		{"StartOnce after the first is taken reads two",
			[]gateStep{gateStartOnce, gatePassed, gateStartOnce}, 2},
		{"Start reads all", []gateStep{gateStart}, sent},
		{"Start then StartOnce reads all",
			[]gateStep{gateStart, gateStartOnce}, sent},
		{"Start while a StartOnce is pending reads all",
			[]gateStep{gateStartOnce, gatePassed, gateStartOnce, gateStart},
			sent},
		{"Start twice reads all", []gateStep{gateStart, gateStart}, sent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			local, remote := net.Pipe()
			conn := &passedGateConn{Conn: local, passed: make(chan struct{}, 8)}
			m := New(conn)
			t.Cleanup(func() {
				_ = remote.Close()
				ctx, cancel := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)
				defer cancel()
				require.NoError(t, m.Stop(ctx))
			})
			_, recv, _ := m.RegisterProtocol(1, ProtocolRoleResponder)
			require.NotNil(t, recv)
			for _, step := range tc.steps {
				switch step {
				case gateStartOnce:
					m.StartOnce()
				case gateStart:
					m.Start()
				case gatePassed:
					select {
					case <-conn.passed:
					case <-time.After(5 * time.Second):
						t.Fatal("read loop did not pass the gate")
					}
				}
			}
			go func() {
				for range sent {
					if writeUnregisterTestSegment(
						remote,
						newTestSegment(1),
					) != nil {
						return
					}
				}
			}()
			got := 0
			for got < tc.reads {
				select {
				case <-recv:
					got++
				case <-time.After(5 * time.Second):
					t.Fatalf("read %d segments, want %d", got, tc.reads)
				}
			}
			select {
			case <-recv:
				t.Fatalf("read more than %d segments", tc.reads)
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

// Stop before Start or StartOnce ends the read loop, which has read nothing.
func TestStopBeforeStartEndsReadLoop(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	conn := &passedGateConn{Conn: local, passed: make(chan struct{}, 1)}
	m := New(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, m.Stop(ctx))
	require.Empty(t, conn.passed, "the read loop passed the gate")
}
