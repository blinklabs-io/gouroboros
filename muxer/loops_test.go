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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const loopsTestTimeout = 5 * time.Second

func requireReturns(t *testing.T, f func(), what string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(loopsTestTimeout):
		t.Fatal(what)
	}
}

// TestProtocolLoopsRefusedAfterClose checks that no loop starts once the
// connection has begun waiting for its loops.
func TestProtocolLoopsRefusedAfterClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := &Muxer{}
	m.CloseProtocolLoops()
	started := false
	require.False(t, m.StartProtocolLoops(m, func() { started = true }))
	m.WaitProtocolLoops()
	require.False(t, started)
}

// TestWaitProtocolLoopsWaitsForRunningLoop checks the join: the wait
// returns only after a running loop returns.
func TestWaitProtocolLoopsWaitsForRunningLoop(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := &Muxer{}
	entered := make(chan struct{})
	release := make(chan struct{})
	require.True(t, m.StartProtocolLoops(m, func() {
		close(entered)
		<-release
	}))
	<-entered
	waited := make(chan struct{})
	go func() {
		m.WaitProtocolLoops()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("WaitProtocolLoops returned while a loop ran")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	requireReturns(t, func() { <-waited }, "WaitProtocolLoops did not return")
}

// TestExcusedLoopIsNotWaitedFor checks that an owner's excused loops, as
// those of a protocol whose callback closes its own connection, are skipped
// by WaitProtocolLoops and by WaitProtocolLoopsExcept for that owner, while
// a wait excepting another owner still waits for them.
func TestExcusedLoopIsNotWaitedFor(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := &Muxer{}
	owner, other := new(int), new(int)
	entered := make(chan struct{})
	release := make(chan struct{})
	require.True(t, m.StartProtocolLoops(owner, func() {
		close(entered)
		<-release
	}))
	<-entered
	require.False(t, m.ExcuseProtocolLoops(other), "excused an owner with no loops")
	require.True(t, m.ExcuseProtocolLoops(owner))
	requireReturns(t, m.WaitProtocolLoops, "WaitProtocolLoops waited for an excused loop")
	requireReturns(
		t,
		func() { _ = m.WaitProtocolLoopsExcept(context.Background(), owner) },
		"WaitProtocolLoopsExcept waited for its own owner's loop",
	)
	waited := make(chan struct{})
	go func() {
		_ = m.WaitProtocolLoopsExcept(context.Background(), other)
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("a wait excepting another owner skipped the loop")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	requireReturns(t, func() { <-waited }, "WaitProtocolLoopsExcept did not return")
}
