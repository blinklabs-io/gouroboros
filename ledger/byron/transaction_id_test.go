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

package byron_test

import (
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestByronTransactionBodyIdConcurrentFirstCall(t *testing.T) {
	var body byron.ByronTransactionBody
	body.TxInputs = make([]byron.ByronTransactionInput, 0)
	body.TxOutputs = make([]byron.ByronTransactionOutput, 0)
	body.Attributes = cbor.RawMessage{0xa0}
	want := body.Id()
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

func TestByronTransactionBodyIdSetCborInvalidatesOnlyCopy(t *testing.T) {
	var body byron.ByronTransactionBody
	body.TxInputs = make([]byron.ByronTransactionInput, 0)
	body.TxOutputs = make([]byron.ByronTransactionOutput, 0)
	body.Attributes = cbor.RawMessage{0xa0}
	wantOriginal := body.Id()

	for name, setCbor := range map[string]func(*byron.ByronTransactionBody, []byte){
		"SetCbor":          (*byron.ByronTransactionBody).SetCbor,
		"SetCborReference": (*byron.ByronTransactionBody).SetCborReference,
	} {
		t.Run(name, func(t *testing.T) {
			copyBody := body
			copyBody.TxInputs = append(
				copyBody.TxInputs,
				byron.ByronTransactionInput{OutputIndex: 1},
			)
			newWire := []byte{0x83, 0x80, 0x80, 0xa0}
			setCbor(&copyBody, newWire)
			if got := body.Id(); got != wantOriginal {
				t.Fatalf("original hash changed after copy invalidation: got %x, want %x", got, wantOriginal)
			}
			encoded, err := cbor.EncodeGeneric(&copyBody)
			if err != nil {
				t.Fatalf("encode copied body: %v", err)
			}
			if got, want := copyBody.Id(), common.Blake2b256Hash(encoded); got != want {
				t.Fatalf("copy hash mismatch after invalidation: got %x, want %x", got, want)
			}

			setCbor(&copyBody, nil)
			if got, want := copyBody.Id(), common.Blake2b256Hash(encoded); got != want {
				t.Fatalf("nil CBOR hash mismatch: got %x, want %x", got, want)
			}
		})
	}
}

func TestByronTransactionBodyIdentityUsesReferenceEncoding(t *testing.T) {
	wire := []byte{0x98, 0x03, 0x80, 0x80, 0xa0}
	var body byron.ByronTransactionBody
	if _, err := cbor.Decode(wire, &body); err != nil {
		t.Fatalf("decode non-shortest body: %v", err)
	}

	wantReferenceEncoding, err := cbor.EncodeGeneric(&body)
	if err != nil {
		t.Fatalf("encode reference body: %v", err)
	}
	wantID := common.Blake2b256Hash(wantReferenceEncoding)
	wantWireHash := common.Blake2b256Hash(wire)
	if wantID == wantWireHash {
		t.Fatal("test vector did not distinguish reference and wire hashes")
	}
	if got := body.Id(); got != wantID {
		t.Fatalf("transaction ID = %x, want reference hash %x", got, wantID)
	}
	if got := body.WireHash(); got != wantWireHash {
		t.Fatalf("wire hash = %x, want original-byte hash %x", got, wantWireHash)
	}
	body.SetCbor(wantReferenceEncoding)
	if got := body.WireHash(); got != body.Id() {
		t.Fatalf(
			"canonical wire hash %x differs from transaction ID %x",
			got,
			body.Id(),
		)
	}
}
