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

package consensus

import (
	"container/list"
	"sync"
)

// The pure stake/coefficient/mode tuple needs no epoch invalidation. Bound
// retained states so processing historical pools cannot grow memory forever.
const leaderEligibilityCacheMaxEntries = 4096

type leaderEligibilityCacheKey struct {
	poolStake   uint64
	totalStake  uint64
	numerator   uint64
	denominator uint64
	mode        ConsensusMode
}

type leaderEligibilityCacheEntry struct {
	key   leaderEligibilityCacheKey
	state *leaderEligibilityState
}

type leaderEligibilityCache struct {
	mu      sync.Mutex
	entries map[leaderEligibilityCacheKey]*list.Element
	order   *list.List
	maxSize int
}

func newLeaderEligibilityCache(maxSize int) *leaderEligibilityCache {
	return &leaderEligibilityCache{
		entries: make(map[leaderEligibilityCacheKey]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
	}
}

func (c *leaderEligibilityCache) get(
	key leaderEligibilityCacheKey,
) (*leaderEligibilityState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	entry := elem.Value.(*leaderEligibilityCacheEntry)
	return entry.state, true
}

func (c *leaderEligibilityCache) put(
	key leaderEligibilityCacheKey,
	state *leaderEligibilityState,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*leaderEligibilityCacheEntry)
		entry.state = state
		return
	}
	entry := &leaderEligibilityCacheEntry{key: key, state: state}
	c.entries[key] = c.order.PushFront(entry)
	if c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			oldestEntry := oldest.Value.(*leaderEligibilityCacheEntry)
			delete(c.entries, oldestEntry.key)
		}
	}
}

// Preparation runs outside the mutex; concurrent misses may duplicate work.
// Only successfully prepared, immutable private states enter the cache.
var leaderEligibilityMemo = newLeaderEligibilityCache(
	leaderEligibilityCacheMaxEntries,
)
