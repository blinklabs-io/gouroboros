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
	"sync"
)

// goroutineGroup tracks the goroutines the client starts for one protocol
// instance, so that Stop can wait for them to exit.
//
// A sync.WaitGroup does not fit: a request whose MsgRequestRange was queued
// just before the protocol stopped starts its deadline watcher after Stop may
// already be waiting, and WaitGroup forbids an Add from zero during a Wait.
// Instead the group is closed before the wait, and Go then refuses.
type goroutineGroup struct {
	mu      sync.Mutex
	running int
	closed  bool
	idle    chan struct{}
}

func newGoroutineGroup() *goroutineGroup {
	idle := make(chan struct{})
	close(idle)
	return &goroutineGroup{idle: idle}
}

// Go runs f on a new goroutine and reports true, or reports false without
// running f once the group has been closed.
func (g *goroutineGroup) Go(f func()) bool {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return false
	}
	if g.running == 0 {
		g.idle = make(chan struct{})
	}
	g.running++
	g.mu.Unlock()
	go func() {
		defer g.done()
		f()
	}()
	return true
}

func (g *goroutineGroup) done() {
	g.mu.Lock()
	g.running--
	if g.running == 0 {
		close(g.idle)
	}
	g.mu.Unlock()
}

// closeAndWait refuses further goroutines and waits for the running ones to
// return, or until ctx is done. Several callers may wait at once.
func (g *goroutineGroup) closeAndWait(ctx context.Context) error {
	g.mu.Lock()
	g.closed = true
	idle := g.idle
	g.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
