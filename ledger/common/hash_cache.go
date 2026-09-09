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
	"runtime"
	"sync/atomic"
	"unsafe"
)

const (
	hashCacheEmpty uint32 = iota
	hashCacheComputing
	hashCacheReady
)

type Blake2b256Cache struct {
	state unsafe.Pointer
}

func (c *Blake2b256Cache) Get(compute func() Blake2b256) Blake2b256 {
	state := atomic.LoadPointer(&c.state)
	if state == nil {
		newState := &blake2b256CacheState{}
		if atomic.CompareAndSwapPointer(&c.state, nil, unsafe.Pointer(newState)) {
			state = unsafe.Pointer(newState)
		} else {
			state = atomic.LoadPointer(&c.state)
		}
	}
	cacheState := (*blake2b256CacheState)(state)
	for {
		switch atomic.LoadUint32(&cacheState.state) {
		case hashCacheReady:
			return cacheState.value
		case hashCacheEmpty:
			if atomic.CompareAndSwapUint32(&cacheState.state, hashCacheEmpty, hashCacheComputing) {
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							atomic.StoreUint32(&cacheState.state, hashCacheEmpty)
							panic(recovered)
						}
					}()
					cacheState.value = compute()
				}()
				atomic.StoreUint32(&cacheState.state, hashCacheReady)
				return cacheState.value
			}
		default:
			runtime.Gosched()
		}
	}
}

type blake2b256CacheState struct {
	value Blake2b256
	state uint32
}
