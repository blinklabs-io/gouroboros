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
	body.SetCbor([]byte{0x82, 0x80, 0x80})
	want := common.Blake2b256Hash(body.Cbor())
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if got := body.WireId(); got != want {
				t.Errorf("unexpected hash: got %x, want %x", got, want)
			}
		}()
	}
	close(start)
	workers.Wait()
}

func TestByronTransactionBodyIdSetCborInvalidatesOnlyCopy(t *testing.T) {
	var body byron.ByronTransactionBody
	body.SetCbor([]byte{0x82, 0x80, 0x80})
	wantOriginal := body.WireId()

	for name, setCbor := range map[string]func(*byron.ByronTransactionBody, []byte){
		"SetCbor":          (*byron.ByronTransactionBody).SetCbor,
		"SetCborReference": (*byron.ByronTransactionBody).SetCborReference,
	} {
		t.Run(name, func(t *testing.T) {
			copyBody := body
			newWire := []byte{0x82, 0x81, 0x80, 0x80}
			setCbor(&copyBody, newWire)
			if got := body.WireId(); got != wantOriginal {
				t.Fatalf("original hash changed after copy invalidation: got %x, want %x", got, wantOriginal)
			}
			if got, want := copyBody.WireId(), common.Blake2b256Hash(newWire); got != want {
				t.Fatalf("copy hash mismatch after invalidation: got %x, want %x", got, want)
			}

			setCbor(&copyBody, nil)
			if got, want := copyBody.WireId(), common.Blake2b256Hash(nil); got != want {
				t.Fatalf("nil CBOR hash mismatch: got %x, want %x", got, want)
			}
		})
	}
}

func TestByronTransactionIdsSeparateCanonicalAndWireHashes(t *testing.T) {
	address, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	if err != nil {
		t.Fatalf("create Byron address: %v", err)
	}
	encodedAddress, err := cbor.Encode(address)
	if err != nil {
		t.Fatalf("encode Byron address: %v", err)
	}
	// Byron uses indefinite-length input and output lists. The output amount
	// also uses a valid non-shortest integer encoding.
	wireBody := append([]byte{0x83, 0x9f, 0xff, 0x9f, 0x82}, encodedAddress...)
	wireBody = append(wireBody, 0x18, 0x01, 0xff, 0xa0)
	canonicalBody := append([]byte{0x83, 0x9f, 0xff, 0x9f, 0x82}, encodedAddress...)
	canonicalBody = append(canonicalBody, 0x01, 0xff, 0xa0)
	var body byron.ByronTransactionBody
	if err := body.UnmarshalCBOR(wireBody); err != nil {
		t.Fatalf("decode Byron transaction body: %v", err)
	}
	if got, want := body.WireId(), common.Blake2b256Hash(wireBody); got != want {
		t.Fatalf("wire hash mismatch: got %x, want %x", got, want)
	}
	if got, want := body.Id(), common.Blake2b256Hash(canonicalBody); got != want {
		t.Fatalf("canonical transaction ID mismatch: got %x, want %x", got, want)
	}
	if body.Id() == body.WireId() {
		t.Fatal("non-shortest integer encoding produced identical hashes")
	}
	tx := &byron.ByronTransaction{Body: body}
	produced := tx.Produced()
	if len(produced) != 1 || produced[0].Id.Id() != body.Id() {
		t.Fatalf("produced UTxO did not use canonical transaction ID: %+v", produced)
	}
	var canonical byron.ByronTransactionBody
	if err := canonical.UnmarshalCBOR(canonicalBody); err != nil {
		t.Fatalf("decode canonical transaction body: %v", err)
	}
	if canonical.Id() != canonical.WireId() {
		t.Fatal("canonical transaction hashes differ")
	}
}
