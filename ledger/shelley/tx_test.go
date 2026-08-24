// Copyright 2025 Blink Labs Software
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

package shelley_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

func TestShelleyTransactionInputToPlutusData(t *testing.T) {
	testTxIdHex := "1639f61ed08f5e489dd64db20f86451a0db06e83d21ea39c73ea0a93b478a370"
	testTxOutputIdx := 2
	testInput := shelley.NewShelleyTransactionInput(
		testTxIdHex,
		testTxOutputIdx,
	)
	testTxId, err := hex.DecodeString(testTxIdHex)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedData := data.NewConstr(
		0,
		data.NewByteString(testTxId),
		data.NewInteger(big.NewInt(int64(testTxOutputIdx))),
	)
	tmpData := testInput.ToPlutusData()
	if !reflect.DeepEqual(tmpData, expectedData) {
		t.Fatalf(
			"did not get expected PlutusData\n     got: %#v\n  wanted: %#v",
			tmpData,
			expectedData,
		)
	}
}

// TestShelleyTransactionBody_MarshalCBOR_PreservesWireBytes reproduces
// https://github.com/blinklabs-io/gouroboros/issues/1990: a decoded
// ShelleyTransactionBody carrying a protocol parameter update must
// re-marshal to its preserved wire bytes instead of falling through to
// the generic encoder, which would emit an explicit CBOR null for each
// of the 15 unset ShelleyProtocolParameterUpdate fields.
func TestShelleyTransactionBody_MarshalCBOR_PreservesWireBytes(t *testing.T) {
	genesisHash := common.NewBlake2b224(bytes.Repeat([]byte{0xAB}, 28))
	// Minimal wire-format body: a fee, a TTL, and a protocol param
	// update proposal with only MinFeeA (key 0) populated, as a real
	// node would emit it.
	rawBody := map[uint64]any{
		2: uint64(206245),  // fee
		3: uint64(5000000), // ttl
		6: []any{
			map[common.Blake2b224]any{
				genesisHash: map[uint64]any{0: uint64(44)}, // MinFeeA only
			},
			uint64(4), // epoch
		},
	}
	wireBytes, err := cbor.Encode(rawBody)
	if err != nil {
		t.Fatalf("unexpected error encoding wire bytes: %s", err)
	}

	body, err := shelley.NewShelleyTransactionBodyFromCbor(wireBytes)
	if err != nil {
		t.Fatalf("unexpected error decoding transaction body: %s", err)
	}
	if body.Update == nil || len(body.Update.ProtocolParamUpdates) != 1 {
		t.Fatalf(
			"expected a single protocol param update, got %#v",
			body.Update,
		)
	}
	update, ok := body.Update.ProtocolParamUpdates[genesisHash]
	if !ok {
		t.Fatalf("expected update entry for genesis hash %s", genesisHash)
	}
	if update.MinFeeA == nil || *update.MinFeeA != 44 {
		t.Fatalf("expected MinFeeA == 44, got %#v", update.MinFeeA)
	}
	if update.MinFeeB != nil {
		t.Fatalf("expected MinFeeB to remain unset, got %#v", update.MinFeeB)
	}

	marshaled, err := body.MarshalCBOR()
	if err != nil {
		t.Fatalf("unexpected error marshaling body: %s", err)
	}
	if !bytes.Equal(marshaled, wireBytes) {
		t.Fatalf(
			"MarshalCBOR() did not return preserved wire bytes: got %d bytes, wanted %d bytes",
			len(marshaled),
			len(wireBytes),
		)
	}

	// Rebuilding a transaction from the decoded body and an empty
	// witness set -- the same pattern ShelleyBlock.Transactions() uses
	// -- must not inflate the encoded transaction size.
	wsBytes, err := cbor.Encode(map[uint64]any{})
	if err != nil {
		t.Fatalf("unexpected error encoding witness set: %s", err)
	}
	var witnessSet shelley.ShelleyTransactionWitnessSet
	if _, err := cbor.Decode(wsBytes, &witnessSet); err != nil {
		t.Fatalf("unexpected error decoding witness set: %s", err)
	}

	wireTxBytes, err := cbor.Encode(
		[]any{cbor.RawMessage(wireBytes), cbor.RawMessage(wsBytes), nil},
	)
	if err != nil {
		t.Fatalf("unexpected error encoding wire transaction: %s", err)
	}

	rebuilt := &shelley.ShelleyTransaction{
		Body:       *body,
		WitnessSet: witnessSet,
	}
	rebuiltBytes := rebuilt.Cbor()
	if !bytes.Equal(rebuiltBytes, wireTxBytes) {
		t.Fatalf(
			"rebuilt transaction CBOR does not match wire bytes: got %d bytes, wanted %d bytes",
			len(rebuiltBytes),
			len(wireTxBytes),
		)
	}
}
