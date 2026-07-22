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

package cbor

import (
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
)

type genericTypeCache struct {
	mu    sync.Mutex
	types atomic.Pointer[map[reflect.Type]reflect.Type]
}

func newGenericTypeCache() *genericTypeCache {
	cache := &genericTypeCache{}
	types := make(map[reflect.Type]reflect.Type)
	cache.types.Store(&types)
	return cache
}

func (c *genericTypeCache) getOrCreate(
	srcType reflect.Type,
	create func() (reflect.Type, error),
) (reflect.Type, error) {
	if cachedType, ok := c.get(srcType); ok {
		return cachedType, nil
	}

	createdType, err := create()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Publish a fresh map on misses so cache hits can read without a lock.
	currentTypes := *c.types.Load()
	if cachedType, ok := currentTypes[srcType]; ok {
		return cachedType, nil
	}

	nextTypes := make(map[reflect.Type]reflect.Type, len(currentTypes)+1)
	maps.Copy(nextTypes, currentTypes)
	nextTypes[srcType] = createdType
	c.types.Store(&nextTypes)
	return createdType, nil
}

func (c *genericTypeCache) get(srcType reflect.Type) (reflect.Type, bool) {
	cachedType, ok := (*c.types.Load())[srcType]
	return cachedType, ok
}
