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

import "sync"

// protocolLoops tracks the goroutines that run the mini-protocols on this
// muxer's connection, so the connection can wait for them when it closes.
// The reference mux runs every mini-protocol thread in a job pool and
// cancels and waits for all of them before its run returns (network-mux
// Network/Mux.hs:280-287, Control/Concurrent/JobPool.hs:105-109).
type protocolLoops struct {
	mu      sync.Mutex
	closed  bool
	running map[*protocolLoop]struct{}
	// changed is closed, and replaced, whenever a loop exits or is
	// excused, waking waiters to re-check.
	changed chan struct{}
}

type protocolLoop struct {
	// owner identifies the protocol instance the loop belongs to.
	owner any
	// excused loops are not waited for by WaitProtocolLoops: a callback
	// on their protocol instance is itself waiting for the connection to
	// close.
	excused bool
}

func (l *protocolLoops) notifyLocked() {
	if l.changed != nil {
		close(l.changed)
	}
	l.changed = make(chan struct{})
}

// waitUntil blocks until done, evaluated with mu held, is true.
func (l *protocolLoops) waitUntil(done func() bool) {
	for {
		l.mu.Lock()
		if done() {
			l.mu.Unlock()
			return
		}
		if l.changed == nil {
			l.changed = make(chan struct{})
		}
		changed := l.changed
		l.mu.Unlock()
		<-changed
	}
}

// StartProtocolLoops starts each of loops on its own goroutine, tracked as
// belonging to owner so that WaitProtocolLoops waits for it. It starts none
// and returns false once CloseProtocolLoops has been called.
func (m *Muxer) StartProtocolLoops(owner any, loops ...func()) bool {
	l := &m.protocolLoops
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	if l.running == nil {
		l.running = make(map[*protocolLoop]struct{})
	}
	for _, loop := range loops {
		entry := &protocolLoop{owner: owner}
		l.running[entry] = struct{}{}
		go func() {
			defer func() {
				l.mu.Lock()
				delete(l.running, entry)
				l.notifyLocked()
				l.mu.Unlock()
			}()
			loop()
		}()
	}
	return true
}

// CloseProtocolLoops makes every later StartProtocolLoops start nothing.
func (m *Muxer) CloseProtocolLoops() {
	l := &m.protocolLoops
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
}

// WaitProtocolLoops returns once every protocol loop has exited, except the
// loops excused by ExcuseProtocolLoops.
func (m *Muxer) WaitProtocolLoops() {
	l := &m.protocolLoops
	l.waitUntil(func() bool {
		for entry := range l.running {
			if !entry.excused {
				return false
			}
		}
		return true
	})
}

// ExcuseProtocolLoops excuses owner's running loops from WaitProtocolLoops,
// for a callback of owner that closes the connection: the wait would
// otherwise wait for the callback's own caller. Owners are compared by
// identity; it reports whether owner has a running loop on this muxer.
func (m *Muxer) ExcuseProtocolLoops(owner any) bool {
	l := &m.protocolLoops
	l.mu.Lock()
	defer l.mu.Unlock()
	found := false
	for entry := range l.running {
		if entry.owner == owner {
			entry.excused = true
			found = true
		}
	}
	if found {
		l.notifyLocked()
	}
	return found
}

// WaitProtocolLoopsExcept returns once every protocol loop that does not
// belong to owner has exited, excused or not. A nil owner excepts nothing.
func (m *Muxer) WaitProtocolLoopsExcept(owner any) {
	l := &m.protocolLoops
	l.waitUntil(func() bool {
		for entry := range l.running {
			if owner == nil || entry.owner != owner {
				return false
			}
		}
		return true
	})
}
