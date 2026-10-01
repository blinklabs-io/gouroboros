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
	"container/list"
	"crypto/sha256"
	"sync"

	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

const (
	// programCacheMaxBytes bounds the estimated memory retained by
	// defaultProgramCache. A cached program keeps its whole decode arena
	// alive, so the bound is in bytes rather than entries.
	programCacheMaxBytes = 64 << 20
	// programArenaBytesPerScriptByte estimates the arena retained per byte of
	// flat-encoded script. Measured arenas run roughly 95-400 KB for scripts
	// of a few KB to a few tens of KB.
	programArenaBytesPerScriptByte = 48
	// programCacheEntryOverhead is the fixed per-entry cost, which also
	// charges cached decode failures.
	programCacheEntryOverhead = 1024
)

// programCacheKey holds everything the decoded and validated program depends
// on: the script bytes, the ledger language, and the protocol major version,
// which gates builtin availability and the protocol-version-11 limits checked
// by syn.DecodeDeBruijnWithContext and syn.ValidateTermVersionForExecution.
type programCacheKey struct {
	sum            [sha256.Size]byte
	ledgerLanguage lang.LanguageVersion
	protoMajor     uint
}

type programCacheEntry struct {
	key     programCacheKey
	program *syn.Program[syn.DeBruijn]
	err     error
	size    int64
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

func (c *programCache) get(
	key programCacheKey,
) (*programCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.recency.MoveToFront(elem)
	return elem.Value.(*programCacheEntry), true
}

// put stores entry unless a concurrent caller stored one first, and returns
// whichever is cached so racing callers on one miss share a program.
func (c *programCache) put(entry *programCacheEntry) *programCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[entry.key]; ok {
		c.recency.MoveToFront(elem)
		return elem.Value.(*programCacheEntry)
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

// decode returns the decoded program for innerScript, decoding it with
// decodeFn on a miss.
func (c *programCache) decode(
	innerScript []byte,
	ledgerLanguage lang.LanguageVersion,
	protoMajor uint,
	decodeFn func() (*syn.Program[syn.DeBruijn], error),
) (*syn.Program[syn.DeBruijn], error) {
	key := programCacheKey{
		sum:            sha256.Sum256(innerScript),
		ledgerLanguage: ledgerLanguage,
		protoMajor:     protoMajor,
	}
	if hit, ok := c.get(key); ok {
		return hit.program, hit.err
	}
	program, err := decodeFn()
	stored := c.put(&programCacheEntry{
		key:     key,
		program: program,
		err:     err,
		size: programCacheEntryOverhead +
			int64(len(innerScript))*programArenaBytesPerScriptByte,
	})
	return stored.program, stored.err
}
