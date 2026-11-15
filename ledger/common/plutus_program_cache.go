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
	"bytes"
	"container/list"
	"hash/maphash"
	"sync"

	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/lang"
)

const (
	// programCacheMaxBytes bounds the estimated memory retained by
	// defaultProgramCache. A cached program keeps its whole decode arena
	// alive, so the bound is in bytes rather than entries; each entry is
	// charged cek.PreparedProgram.RetainedBytes.
	programCacheMaxBytes = 64 << 20
	// programCacheEntryOverhead is the fixed per-entry cost, which also
	// charges cached decode failures.
	programCacheEntryOverhead = 1024
)

// programCacheKey holds everything the decoded and validated program depends
// on: the script bytes, the ledger language, and the protocol major version,
// which gates builtin availability and the protocol-version-11 limits checked
// by syn.DecodeDeBruijnWithContext and syn.ValidateTermVersionForExecution.
// The script is keyed by a maphash under a per-process random seed, which is
// far cheaper than a cryptographic hash of every evaluated script; an entry
// keeps the script bytes and a hit compares them, so a collision is a miss.
type programCacheKey struct {
	hash           uint64
	ledgerLanguage lang.LanguageVersion
	protoMajor     uint
}

var programCacheSeed = maphash.MakeSeed()

type programCacheEntry struct {
	key programCacheKey
	// script is a copy of the script bytes the entry was prepared from.
	script []byte
	// prepared is the program decoded and checked once by
	// cek.PrepareProgram, so evaluations skip plutigo's per-run structure
	// walk.
	prepared *cek.PreparedProgram
	err      error
	size     int64
}

// programCache is a byte-bounded LRU of decoded programs. A cached program is
// shared by every caller and goroutine that hits it, so it is read-only:
// evaluation wraps arguments in new Apply and Constant nodes and never writes
// through the shared term graph, and each evaluation uses its own
// cek.Machine. Programs must come from a one-shot decode, never from a reused
// syn.DeBruijnDecoder, whose next Decode zeroes the previous program's nodes.
// Decode failures are cached under the same key as successes.
type programCache struct {
	mu       sync.Mutex
	maxBytes int64
	bytes    int64
	entries  map[programCacheKey]*list.Element // value: *programCacheEntry
	recency  list.List                         // front: most recently used
}

func newProgramCache(maxBytes int64) *programCache {
	return &programCache{
		maxBytes: maxBytes,
		entries:  make(map[programCacheKey]*list.Element),
	}
}

var defaultProgramCache = newProgramCache(programCacheMaxBytes)

// get returns the entry for script. An entry is immutable once stored, so
// its script is compared outside the lock, which is then taken again only to
// mark the entry recently used if it is still cached.
func (c *programCache) get(
	key programCacheKey,
	script []byte,
) (*programCacheEntry, bool) {
	c.mu.Lock()
	elem, ok := c.entries[key]
	c.mu.Unlock()
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*programCacheEntry)
	if !bytes.Equal(entry.script, script) {
		return nil, false
	}
	c.mu.Lock()
	if current, ok := c.entries[key]; ok && current == elem {
		c.recency.MoveToFront(elem)
	}
	c.mu.Unlock()
	return entry, true
}

// put stores entry unless a concurrent caller stored one first, and returns
// whichever is cached so racing callers on one miss share a program.
func (c *programCache) put(entry *programCacheEntry) *programCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[entry.key]; ok {
		cached := elem.Value.(*programCacheEntry)
		if !bytes.Equal(cached.script, entry.script) {
			// A different script with the same hash keeps its slot.
			return entry
		}
		c.recency.MoveToFront(elem)
		return cached
	}
	if entry.size > c.maxBytes {
		return entry
	}
	c.entries[entry.key] = c.recency.PushFront(entry)
	c.bytes += entry.size
	for c.bytes > c.maxBytes {
		oldest := c.recency.Back()
		evicted := c.recency.Remove(oldest).(*programCacheEntry)
		delete(c.entries, evicted.key)
		c.bytes -= evicted.size
	}
	return entry
}

// decodePrepared returns the prepared program for innerScript, preparing it
// with prepareFn on a miss.
func (c *programCache) decodePrepared(
	innerScript []byte,
	ledgerLanguage lang.LanguageVersion,
	protoMajor uint,
	prepareFn func() (*cek.PreparedProgram, error),
) (*cek.PreparedProgram, error) {
	entry := c.lookup(innerScript, ledgerLanguage, protoMajor, prepareFn)
	return entry.prepared, entry.err
}

func (c *programCache) lookup(
	innerScript []byte,
	ledgerLanguage lang.LanguageVersion,
	protoMajor uint,
	prepareFn func() (*cek.PreparedProgram, error),
) *programCacheEntry {
	key := programCacheKey{
		hash:           maphash.Bytes(programCacheSeed, innerScript),
		ledgerLanguage: ledgerLanguage,
		protoMajor:     protoMajor,
	}
	if hit, ok := c.get(key, innerScript); ok {
		return hit
	}
	prepared, err := prepareFn()
	if err != nil {
		prepared = nil
	}
	return c.put(&programCacheEntry{
		key:      key,
		script:   bytes.Clone(innerScript),
		prepared: prepared,
		err:      err,
		size:     programEntrySize(prepared) + int64(len(innerScript)),
	})
}

// programEntrySize is the bytes charged for a cache entry: a fixed cost,
// which also charges a cached failure, plus what the prepared program keeps
// alive.
func programEntrySize(prepared *cek.PreparedProgram) int64 {
	size := int64(programCacheEntryOverhead)
	if prepared != nil {
		size += prepared.RetainedBytes()
	}
	return size
}
