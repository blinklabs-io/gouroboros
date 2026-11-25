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

package common

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlake2b256CacheConcurrentFirstFill(t *testing.T) {
	var cache Blake2b256Cache
	var want Blake2b256
	for i := range want {
		want[i] = byte(i)
	}

	var start sync.WaitGroup
	start.Add(1)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			start.Wait()
			if got := cache.Get(func() Blake2b256 { return want }); got != want {
				t.Errorf("unexpected hash: got %x, want %x", got, want)
			}
		}()
	}
	start.Done()
	workers.Wait()
}

func TestBlake2b256CacheRetriesAfterPanic(t *testing.T) {
	var cache Blake2b256Cache
	called := 0
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected compute panic")
			}
		}()
		cache.Get(func() Blake2b256 {
			called++
			panic("compute failed")
		})
	}()

	want := Blake2b256{1}
	if got := cache.Get(func() Blake2b256 {
		called++
		return want
	}); got != want {
		t.Fatalf("unexpected hash: got %x, want %x", got, want)
	}
	if called != 2 {
		t.Fatalf("unexpected compute count: got %d, want 2", called)
	}
}

func TestBlake2b256CacheCopyDuringCompute(t *testing.T) {
	var cache Blake2b256Cache
	started := make(chan struct{})
	release := make(chan struct{})
	want := Blake2b256{2}
	done := make(chan Blake2b256)
	go func() {
		done <- cache.Get(func() Blake2b256 {
			close(started)
			<-release
			return want
		})
	}()
	<-started

	copy := cache
	close(release)
	if got := <-done; got != want {
		t.Fatalf("unexpected original hash: got %x, want %x", got, want)
	}
	if got := copy.Get(func() Blake2b256 {
		t.Fatal("copied cache recomputed while original was in flight")
		return Blake2b256{}
	}); got != want {
		t.Fatalf("unexpected copied hash: got %x, want %x", got, want)
	}
}

func TestBlake2b256CacheConcurrentReset(t *testing.T) {
	var cache Blake2b256Cache
	want := Blake2b256{3}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 1000 {
				if got := cache.Get(func() Blake2b256 { return want }); got != want {
					t.Errorf("unexpected hash: got %x, want %x", got, want)
				}
				cache.Reset()
			}
		}()
	}
	close(start)
	workers.Wait()
}

// Callers that find a computation in progress wait for it: none returns
// before the value is stored, none computes, and all get the same hash.
func TestBlake2b256CacheWaitersWaitForReady(t *testing.T) {
	var cache Blake2b256Cache
	want := Blake2b256{4}
	started := make(chan struct{})
	release := make(chan struct{})
	first := make(chan Blake2b256, 1)
	go func() {
		first <- cache.Get(func() Blake2b256 {
			close(started)
			<-release
			return want
		})
	}()
	<-started

	const waiters = 8
	results := make(chan Blake2b256, waiters)
	var extraComputes atomic.Int32
	for range waiters {
		go func() {
			results <- cache.Get(func() Blake2b256 {
				extraComputes.Add(1)
				return Blake2b256{5}
			})
		}()
	}
	select {
	case got := <-results:
		t.Fatalf("a waiter returned %x before the computation finished", got)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if got := <-first; got != want {
		t.Fatalf("unexpected computed hash: got %x, want %x", got, want)
	}
	for range waiters {
		select {
		case got := <-results:
			if got != want {
				t.Fatalf("unexpected waiter hash: got %x, want %x", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a waiter did not return after the computation finished")
		}
	}
	if n := extraComputes.Load(); n != 0 {
		t.Fatalf("waiters computed %d times, want 0", n)
	}
}

// A computation that panics hands the claim back, so a caller that was
// waiting for it computes the hash itself instead of waiting forever.
func TestBlake2b256CachePanicHandsOverToWaiter(t *testing.T) {
	var cache Blake2b256Cache
	want := Blake2b256{6}
	started := make(chan struct{})
	panicNow := make(chan struct{})
	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		cache.Get(func() Blake2b256 {
			close(started)
			<-panicNow
			panic("compute failed")
		})
	}()
	<-started

	var computes atomic.Int32
	waiter := make(chan Blake2b256, 1)
	go func() {
		waiter <- cache.Get(func() Blake2b256 {
			computes.Add(1)
			return want
		})
	}()
	close(panicNow)
	if r := <-panicked; r == nil {
		t.Fatal("expected the first computation to panic")
	}
	select {
	case got := <-waiter:
		if got != want {
			t.Fatalf("unexpected hash: got %x, want %x", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter did not take over after the panic")
	}
	if n := computes.Load(); n != 1 {
		t.Fatalf("waiter computed %d times, want 1", n)
	}
}

// Racing first callers claim the computation with one compare-and-swap, so
// exactly one of them computes.
func TestBlake2b256CacheConcurrentFirstFillComputesOnce(t *testing.T) {
	for range 50 {
		var cache Blake2b256Cache
		var computes atomic.Int32
		start := make(chan struct{})
		var workers sync.WaitGroup
		for range 16 {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				cache.Get(func() Blake2b256 {
					computes.Add(1)
					return Blake2b256{7}
				})
			}()
		}
		close(start)
		workers.Wait()
		if n := computes.Load(); n != 1 {
			t.Fatalf("compute ran %d times, want 1", n)
		}
	}
}
