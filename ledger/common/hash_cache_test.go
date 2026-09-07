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
