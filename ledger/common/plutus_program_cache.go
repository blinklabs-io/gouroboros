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
	"math/big"
	"reflect"
	"sync"

	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

const (
	// programCacheMaxBytes bounds the estimated memory retained by
	// defaultProgramCache. A cached program keeps its whole decode arena
	// alive, so the bound is in bytes rather than entries.
	programCacheMaxBytes = 64 << 20
	// programCacheEntryOverhead is the fixed per-entry cost, which also
	// charges cached decode failures.
	programCacheEntryOverhead = 1024
	// programTermChunkSlots and programDataChunkSlots are the slot counts of
	// plutigo's syn and data decode arenas. Each arena kind a program uses
	// reserves whole chunks regardless of how few nodes it holds, so a tiny
	// script still keeps tens of kilobytes. They must not be smaller than
	// plutigo's values (decodeTermChunkSize, dataDecodeChunkSize).
	programTermChunkSlots = 384
	programDataChunkSlots = 64
	// programSlotBytes is the size of one interface slot in a term, constant
	// or data list.
	programSlotBytes = 16
	// programNodeSlack covers per-node allocations the walk does not model
	// exactly (type descriptors, arena slice-chunk padding, slice growth).
	programNodeSlack = 64
	// programEstimateFloor is added to every successfully decoded program.
	programEstimateFloor = 16 << 10
)

func sizeOf[T any]() int64 {
	return int64(reflect.TypeFor[T]().Size())
}

// chunked returns the bytes reserved for n values of elemSize when allocated
// from an arena that hands out whole chunks of chunkSlots values.
func chunked(n int, chunkSlots int, elemSize int64) int64 {
	if n <= 0 {
		return 0
	}
	return int64((n+chunkSlots-1)/chunkSlots*chunkSlots) * elemSize
}

// programSizer accumulates the node counts and payload bytes that determine
// how much memory a decoded program keeps alive.
type programSizer struct {
	vars, delays, forces, lambdas, applies int
	constrs, cases, errs, builtins         int
	termSlots                              int
	constants                              int
	integers, byteStrings, strs            int
	units, bools                           int
	protoLists, protoArrays, protoPairs    int
	values, datas                          int
	constantSlots                          int
	payload                                int64
	dInts, dBytes, dLists, dMaps, dConstrs int
	dValues                                int
	dSlots                                 int
	typs                                   int
	unknownBytes                           int64
}

// programEntrySize estimates the bytes a decoded program keeps alive by
// walking it and charging every arena for the whole chunks it reserves, so
// the estimate does not depend on script length. A failed decode retains only
// the error.
func programEntrySize(program *syn.Program[syn.DeBruijn]) int64 {
	size := int64(programCacheEntryOverhead)
	if program == nil {
		return size
	}
	var s programSizer
	s.walkTerms(program.Term)
	return size + s.bytes()
}

func (s *programSizer) walkTerms(root syn.Term[syn.DeBruijn]) {
	stack := []syn.Term[syn.DeBruijn]{root}
	for len(stack) > 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch t := term.(type) {
		case nil:
		case *syn.Var[syn.DeBruijn]:
			s.vars++
		case *syn.Delay[syn.DeBruijn]:
			s.delays++
			stack = append(stack, t.Term)
		case *syn.Force[syn.DeBruijn]:
			s.forces++
			stack = append(stack, t.Term)
		case *syn.Lambda[syn.DeBruijn]:
			s.lambdas++
			stack = append(stack, t.Body)
		case *syn.Apply[syn.DeBruijn]:
			s.applies++
			stack = append(stack, t.Function, t.Argument)
		case *syn.Constr[syn.DeBruijn]:
			s.constrs++
			s.termSlots += len(t.Fields)
			stack = append(stack, t.Fields...)
		case *syn.Case[syn.DeBruijn]:
			s.cases++
			s.termSlots += len(t.Branches)
			stack = append(stack, t.Constr)
			stack = append(stack, t.Branches...)
		case *syn.Error:
			s.errs++
		case *syn.Builtin:
			s.builtins++
		case *syn.Constant:
			s.constants++
			s.walkConstantType(t.Con)
			s.walkConstants(t.Con)
		default:
			s.unknownBytes += programNodeSlack * 16
		}
	}
}

// walkConstantType counts the type nodes a constant term keeps alive. The
// decoder builds each constant term's type on the heap and nested values
// share subtrees of it, so only the top-level value's types are walked.
func (s *programSizer) walkConstantType(con syn.IConstant) {
	var stack []syn.Typ
	switch c := con.(type) {
	case *syn.ProtoList:
		stack = append(stack, c.LTyp)
	case *syn.ProtoArray:
		stack = append(stack, c.ATyp)
	case *syn.ProtoPair:
		stack = append(stack, c.FstType, c.SndType)
	}
	for len(stack) > 0 {
		typ := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if typ == nil {
			continue
		}
		s.typs++
		switch t := typ.(type) {
		case *syn.TList:
			stack = append(stack, t.Typ)
		case *syn.TArray:
			stack = append(stack, t.Typ)
		case *syn.TPair:
			stack = append(stack, t.First, t.Second)
		}
	}
}

func (s *programSizer) walkConstants(root syn.IConstant) {
	stack := []syn.IConstant{root}
	for len(stack) > 0 {
		con := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch c := con.(type) {
		case nil:
		case *syn.Integer:
			s.integers++
			if c.Inner != nil {
				s.payload += int64(len(c.Inner.Bits())) * 8
			}
		case *syn.ByteString:
			s.byteStrings++
			s.payload += int64(len(c.Inner))
		case *syn.String:
			s.strs++
			s.payload += int64(len(c.Inner))
		case *syn.Unit:
			s.units++
		case *syn.Bool:
			s.bools++
		case *syn.ProtoList:
			s.protoLists++
			s.constantSlots += len(c.List)
			stack = append(stack, c.List...)
		case *syn.ProtoArray:
			s.protoArrays++
			s.constantSlots += len(c.Array)
			stack = append(stack, c.Array...)
		case *syn.ProtoPair:
			s.protoPairs++
			stack = append(stack, c.First, c.Second)
		case *syn.Value:
			s.values++
			s.constantSlots += len(c.Entries)
			stack = append(stack, c.Entries...)
		case *syn.Data:
			s.datas++
			s.walkData(c.Inner)
		default:
			s.unknownBytes += 4096
		}
	}
}

func (s *programSizer) walkData(root data.PlutusData) {
	stack := []data.PlutusData{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch d := node.(type) {
		case nil:
		case *data.Integer:
			s.dInts++
			if d.Inner != nil {
				s.payload += int64(len(d.Inner.Bits())) * 8
			}
		case *data.ByteString:
			s.dBytes++
			s.payload += int64(len(d.Inner))
		case *data.List:
			s.dLists++
			s.dSlots += len(d.Items)
			stack = append(stack, d.Items...)
		case *data.Map:
			s.dMaps++
			s.dSlots += 2 * len(d.Pairs)
			for _, pair := range d.Pairs {
				stack = append(stack, pair[0], pair[1])
			}
		case *data.Constr:
			s.dConstrs++
			s.dSlots += len(d.Fields)
			stack = append(stack, d.Fields...)
		case *data.Value:
			s.dValues++
			if d.Inner != nil {
				stack = append(stack, d.Inner)
			}
		default:
			s.unknownBytes += 4096
		}
	}
}

func (s *programSizer) bytes() int64 {
	total := s.unknownBytes + s.payload*2
	total += chunked(s.vars, programTermChunkSlots, sizeOf[syn.Var[syn.DeBruijn]]())
	total += chunked(s.delays, programTermChunkSlots, sizeOf[syn.Delay[syn.DeBruijn]]())
	total += chunked(s.forces, programTermChunkSlots, sizeOf[syn.Force[syn.DeBruijn]]())
	total += chunked(s.lambdas, programTermChunkSlots, sizeOf[syn.Lambda[syn.DeBruijn]]())
	total += chunked(s.applies, programTermChunkSlots, sizeOf[syn.Apply[syn.DeBruijn]]())
	total += chunked(s.constrs, programTermChunkSlots, sizeOf[syn.Constr[syn.DeBruijn]]())
	total += chunked(s.cases, programTermChunkSlots, sizeOf[syn.Case[syn.DeBruijn]]())
	total += chunked(s.errs, programTermChunkSlots, sizeOf[syn.Error]())
	total += chunked(s.builtins, programTermChunkSlots, sizeOf[syn.Builtin]())
	total += chunked(s.constants, programTermChunkSlots, sizeOf[syn.Constant]())
	total += chunked(s.termSlots, programTermChunkSlots, programSlotBytes)

	total += chunked(s.integers, programTermChunkSlots, sizeOf[syn.Integer]()+sizeOf[big.Int]())
	total += chunked(s.byteStrings, programTermChunkSlots, sizeOf[syn.ByteString]())
	total += chunked(s.strs, programTermChunkSlots, sizeOf[syn.String]())
	total += chunked(s.units, programTermChunkSlots, programSlotBytes)
	total += chunked(s.bools, programTermChunkSlots, programSlotBytes)
	total += chunked(s.protoLists, programTermChunkSlots, sizeOf[syn.ProtoList]())
	total += chunked(s.protoArrays, programTermChunkSlots, sizeOf[syn.ProtoArray]())
	total += chunked(s.protoPairs, programTermChunkSlots, sizeOf[syn.ProtoPair]())
	total += chunked(s.values, programTermChunkSlots, sizeOf[syn.Value]())
	total += chunked(s.datas, programTermChunkSlots, sizeOf[syn.Data]())
	total += chunked(s.constantSlots, programTermChunkSlots, programSlotBytes)

	total += chunked(s.dInts, programDataChunkSlots, sizeOf[data.Integer]()+sizeOf[big.Int]())
	total += chunked(s.dBytes, programDataChunkSlots, sizeOf[data.ByteString]())
	total += chunked(s.dLists, programDataChunkSlots, sizeOf[data.List]())
	total += chunked(s.dMaps, programDataChunkSlots, sizeOf[data.Map]())
	total += chunked(s.dConstrs, programDataChunkSlots, sizeOf[data.Constr]())
	total += chunked(s.dValues, programDataChunkSlots, sizeOf[data.Value]())
	total += chunked(s.dSlots, programTermChunkSlots, 2*programSlotBytes)
	// Type nodes are individual heap allocations, the largest a TPair.
	total += int64(s.typs) * (sizeOf[syn.TPair]() + programSlotBytes)

	nodes := s.vars + s.delays + s.forces + s.lambdas + s.applies +
		s.constrs + s.cases + s.constants + s.protoLists + s.protoArrays +
		s.protoPairs + s.values + s.datas + s.dInts + s.dBytes + s.dLists +
		s.dMaps + s.dConstrs + s.dValues
	total += int64(nodes) * programNodeSlack
	// Slice-chunk padding and the decoder's own bookkeeping are not modeled;
	// measured retention stays within a few percent of the walk, so a 50%
	// margin plus a fixed floor keeps the charge an upper bound.
	return total + total/2 + programEstimateFloor
}

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
		size:    programEntrySize(program),
	})
	return stored.program, stored.err
}
