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
	"encoding/hex"
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

func TestByronTransactionBodyIdMatchesWireIdForCanonicalWireForm(t *testing.T) {
	t.Parallel()
	// Mainnet transaction 6497b33b...4d9e (slot 3313): inputs and outputs
	// carry their tag-24 wrappers on the wire.
	const wantId = "6497b33b10fa2619c6efbd9f874ecd1c91badb10bf70850732aab45b90524d9e"
	bodyCbor, err := hex.DecodeString(
		"839f8200d8185824825820a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a200700ff" +
			"9f8282d818584283581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633ca101581e581c010d876783fb2b4d0d17c86df29af8d35356ed3d1827bf4744f06700001a8dc672c11a000f4240ffa0",
	)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	var body byron.ByronTransactionBody
	if err := body.UnmarshalCBOR(bodyCbor); err != nil {
		t.Fatalf("decode Byron transaction body: %v", err)
	}
	if got := body.WireId().String(); got != wantId {
		t.Fatalf("WireId = %s, want %s", got, wantId)
	}
	if got := body.Id().String(); got != wantId {
		t.Fatalf("Id = %s, want %s (WireId)", got, wantId)
	}
}

func TestByronInputOutputMarshalCBORRoundTripsWireForm(t *testing.T) {
	t.Parallel()
	address, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	if err != nil {
		t.Fatalf("create Byron address: %v", err)
	}
	input, err := byron.NewByronTransactionInput(
		"a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a2007", 3,
	)
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	inputCbor, err := cbor.Encode(input)
	if err != nil {
		t.Fatalf("encode input: %v", err)
	}
	// [0, #6.24(...)]
	if len(inputCbor) < 3 || inputCbor[0] != 0x82 || inputCbor[1] != 0x00 || inputCbor[2] != 0xd8 {
		t.Fatalf("input not in wire form: %x", inputCbor)
	}
	var gotInput byron.ByronTransactionInput
	if err := gotInput.UnmarshalCBOR(inputCbor); err != nil {
		t.Fatalf("decode encoded input: %v", err)
	}
	if gotInput.Id() != input.Id() || gotInput.Index() != input.Index() {
		t.Fatalf("input round trip mismatch: %v != %v", gotInput, input)
	}
	output := byron.ByronTransactionOutput{OutputAddress: address, OutputAmount: 1}
	outputCbor, err := cbor.Encode(output)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	// [#6.24(...), amount]: tag 24 follows the array header.
	if len(outputCbor) < 2 || outputCbor[0] != 0x82 || outputCbor[1] != 0xd8 {
		t.Fatalf("output address not tag 24 wrapped: %x", outputCbor)
	}
	var gotOutput byron.ByronTransactionOutput
	if err := gotOutput.UnmarshalCBOR(outputCbor); err != nil {
		t.Fatalf("decode encoded output: %v", err)
	}
	if gotOutput.Address().String() != address.String() || gotOutput.Amount().Uint64() != 1 {
		t.Fatalf("output round trip mismatch: %v", gotOutput)
	}
}
