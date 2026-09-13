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

package common_test

import (
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestTransactionBodyBaseIdConcurrentFirstCall(t *testing.T) {
	var body common.TransactionBodyBase
	body.SetCbor([]byte{0xa0})
	want := common.Blake2b256Hash(body.Cbor())
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if got := body.Id(); got != want {
				t.Errorf("unexpected hash: got %x, want %x", got, want)
			}
		}()
	}
	close(start)
	workers.Wait()
}

func TestTransactionBodyBaseIdSetCborInvalidatesOnlyCopy(t *testing.T) {
	var body common.TransactionBodyBase
	body.SetCbor([]byte{0xa0})
	wantOriginal := body.Id()

	for name, setCbor := range map[string]func(*common.TransactionBodyBase, []byte){
		"SetCbor":          (*common.TransactionBodyBase).SetCbor,
		"SetCborReference": (*common.TransactionBodyBase).SetCborReference,
	} {
		t.Run(name, func(t *testing.T) {
			copyBody := body
			newWire := []byte{0xa1, 0x00, 0x01}
			setCbor(&copyBody, newWire)
			if got := body.Id(); got != wantOriginal {
				t.Fatalf("original hash changed after copy invalidation: got %x, want %x", got, wantOriginal)
			}
			if got, want := copyBody.Id(), common.Blake2b256Hash(newWire); got != want {
				t.Fatalf("copy hash mismatch after invalidation: got %x, want %x", got, want)
			}

			setCbor(&copyBody, nil)
			if got, want := copyBody.Id(), common.Blake2b256Hash(nil); got != want {
				t.Fatalf("nil CBOR hash mismatch: got %x, want %x", got, want)
			}
		})
	}
}

func BenchmarkTransactionBodyBaseIdMemo(b *testing.B) {
	var body common.TransactionBodyBase
	body.SetCbor(make([]byte, 256))
	b.ResetTimer()
	for range b.N {
		_ = body.Id()
	}
}

func BenchmarkTransactionBodyBaseIdRecompute(b *testing.B) {
	var body common.TransactionBodyBase
	wire := make([]byte, 256)
	b.ResetTimer()
	for range b.N {
		body.SetCborReference(wire)
		_ = body.Id()
	}
}
