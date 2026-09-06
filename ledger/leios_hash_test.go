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

// Tests that pin the LeiosHash contract for every era transaction type:
// LeiosHash is the Blake2b-256 hash of the transaction's current CBOR, it is
// recomputed on every call rather than memoized, the transaction types stay
// copyable, and concurrent callers on one shared transaction agree. They live
// in package ledger_test rather than in each era package so that a new era
// cannot be added without being listed here.

package ledger_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// leiosHashTx is the part of common.Transaction these tests exercise. Every
// era transaction type satisfies it through its pointer type.
type leiosHashTx interface {
	SetCbor([]byte)
	Cbor() []byte
	LeiosHash() lcommon.Blake2b256
}

// leiosHashEras lists one entry per era transaction type. Adding an era
// without adding it here leaves that era's LeiosHash unpinned.
var leiosHashEras = []struct {
	name  string
	newTx func() leiosHashTx
}{
	{"byron", func() leiosHashTx { return &byron.ByronTransaction{} }},
	{"shelley", func() leiosHashTx { return &shelley.ShelleyTransaction{} }},
	{"allegra", func() leiosHashTx { return &allegra.AllegraTransaction{} }},
	{"mary", func() leiosHashTx { return &mary.MaryTransaction{} }},
	{"alonzo", func() leiosHashTx { return &alonzo.AlonzoTransaction{} }},
	{"babbage", func() leiosHashTx { return &babbage.BabbageTransaction{} }},
	{"conway", func() leiosHashTx { return &conway.ConwayTransaction{} }},
	{"dijkstra", func() leiosHashTx { return &dijkstra.DijkstraTransaction{} }},
}

// Two distinct, well-formed transaction-shaped CBOR payloads. The bytes only
// have to differ: LeiosHash hashes the stored CBOR without interpreting it.
var (
	leiosHashCborA = []byte{0x83, 0xa0, 0xa0, 0xa0}
	leiosHashCborB = []byte{0x83, 0xa0, 0xa0, 0xf6}
)

// TestLeiosHashMatchesCbor pins that LeiosHash is the Blake2b-256 hash of the
// transaction's CBOR and is stable across repeated calls.
func TestLeiosHashMatchesCbor(t *testing.T) {
	want := lcommon.Blake2b256Hash(leiosHashCborA)
	for _, era := range leiosHashEras {
		t.Run(era.name, func(t *testing.T) {
			tx := era.newTx()
			tx.SetCbor(leiosHashCborA)
			first := tx.LeiosHash()
			if first != want {
				t.Fatalf(
					"LeiosHash = %s, want Blake2b256Hash(Cbor()) = %s",
					first,
					want,
				)
			}
			for i := range 4 {
				if got := tx.LeiosHash(); got != first {
					t.Fatalf("call %d returned %s, want %s", i+2, got, first)
				}
			}
		})
	}
}

// TestLeiosHashTracksCbor pins that LeiosHash follows the transaction's
// current CBOR. A memo populated on the first call and never invalidated
// would keep returning the first payload's hash.
func TestLeiosHashTracksCbor(t *testing.T) {
	wantA := lcommon.Blake2b256Hash(leiosHashCborA)
	wantB := lcommon.Blake2b256Hash(leiosHashCborB)
	if wantA == wantB {
		t.Fatal("test payloads must hash differently")
	}
	for _, era := range leiosHashEras {
		t.Run(era.name, func(t *testing.T) {
			tx := era.newTx()
			tx.SetCbor(leiosHashCborA)
			if got := tx.LeiosHash(); got != wantA {
				t.Fatalf("first payload: LeiosHash = %s, want %s", got, wantA)
			}
			tx.SetCbor(leiosHashCborB)
			if got := tx.LeiosHash(); got != wantB {
				t.Fatalf(
					"second payload: LeiosHash = %s, want %s (stale cache from the first payload?)",
					got,
					wantB,
				)
			}
		})
	}
}

// TestLeiosHashCopyIsIndependent pins that a value copy of a transaction
// carries no state shared with its source: replacing the copy's CBOR must
// change only the copy's LeiosHash.
func TestLeiosHashCopyIsIndependent(t *testing.T) {
	wantA := lcommon.Blake2b256Hash(leiosHashCborA)
	wantB := lcommon.Blake2b256Hash(leiosHashCborB)
	for _, era := range leiosHashEras {
		t.Run(era.name, func(t *testing.T) {
			tx := era.newTx()
			tx.SetCbor(leiosHashCborA)
			// Populate anything the original might cache before copying.
			if got := tx.LeiosHash(); got != wantA {
				t.Fatalf("original: LeiosHash = %s, want %s", got, wantA)
			}
			// Copy the transaction by value, the way the tree passes these
			// types around, and give the copy different CBOR.
			src := reflect.ValueOf(tx).Elem()
			cp, ok := reflect.New(src.Type()).Interface().(leiosHashTx)
			if !ok {
				t.Fatalf("copy of %s does not satisfy leiosHashTx", src.Type())
			}
			reflect.ValueOf(cp).Elem().Set(src)
			cp.SetCbor(leiosHashCborB)
			if got := cp.LeiosHash(); got != wantB {
				t.Fatalf(
					"copy: LeiosHash = %s, want %s (state shared with the original?)",
					got,
					wantB,
				)
			}
			if got := tx.LeiosHash(); got != wantA {
				t.Fatalf(
					"original after copy was modified: LeiosHash = %s, want %s",
					got,
					wantA,
				)
			}
		})
	}
}

// TestLeiosHashConcurrent pins that concurrent LeiosHash calls on one shared
// transaction are safe and agree. Run under -race, this fails if LeiosHash
// writes to the receiver.
func TestLeiosHashConcurrent(t *testing.T) {
	const goroutines = 16
	want := lcommon.Blake2b256Hash(leiosHashCborA)
	for _, era := range leiosHashEras {
		t.Run(era.name, func(t *testing.T) {
			tx := era.newTx()
			tx.SetCbor(leiosHashCborA)
			results := make([]lcommon.Blake2b256, goroutines)
			var start sync.WaitGroup
			var done sync.WaitGroup
			start.Add(1)
			for i := range goroutines {
				done.Add(1)
				go func(i int) {
					defer done.Done()
					start.Wait()
					results[i] = tx.LeiosHash()
				}(i)
			}
			start.Done()
			done.Wait()
			for i, got := range results {
				if got != want {
					t.Fatalf("goroutine %d: LeiosHash = %s, want %s", i, got, want)
				}
			}
		})
	}
}

// TestLeiosHashNoCacheField pins that no era transaction type carries a hash
// cache or a synchronization primitive of its own. Either one would reopen
// this contract: a cache needs invalidating on every CBOR change, and a
// synchronization primitive -- including one held behind a pointer, which
// go vet's copylocks check does not flag -- makes copying the transaction
// unsafe.
func TestLeiosHashNoCacheField(t *testing.T) {
	lockerType := reflect.TypeOf((*sync.Locker)(nil)).Elem()
	for _, era := range leiosHashEras {
		t.Run(era.name, func(t *testing.T) {
			txType := reflect.TypeOf(era.newTx()).Elem()
			for i := range txType.NumField() {
				field := txType.Field(i)
				if field.Name == "hash" {
					t.Errorf(
						"%s has a %q field: LeiosHash is contractually recomputed, not cached",
						txType,
						field.Name,
					)
				}
				fieldType := field.Type
				for fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				if reflect.PointerTo(fieldType).Implements(lockerType) {
					t.Errorf(
						"%s field %q is a synchronization primitive (%s); era transaction types must stay copyable",
						txType,
						field.Name,
						field.Type,
					)
				}
			}
		})
	}
}
