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
	"testing"
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
