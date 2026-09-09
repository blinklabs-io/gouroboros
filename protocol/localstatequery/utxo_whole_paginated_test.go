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

package localstatequery_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
)

// TestUtxoWholePaginatedQueryIsDispatchable checks the vendor-extension
// paginated query reaches a server through the same dispatch table a real
// connection uses -- the same regression class TestPoolDistr2QueryIsDispatchable
// covers: registering a result type without registering the query's tag in
// shelleyQueryTypes leaves a server failing the whole connection on decode
// rather than just rejecting the one query.
func TestUtxoWholePaginatedQueryIsDispatchable(t *testing.T) {
	cursorTxId := bytes.Repeat([]byte{0xAB}, 32)
	blockQuery, err := cbor.Encode(
		[]any{
			localstatequery.QueryTypeShelleyUtxoWholePaginated,
			cursorTxId,
			uint32(3),
			uint32(50000),
		},
	)
	if err != nil {
		t.Fatalf("encoding query: %v", err)
	}
	// A Shelley leaf query travels wrapped in its era and block-query envelopes.
	wrapped, err := cbor.Encode(
		[]any{
			localstatequery.QueryTypeBlock,
			[]any{
				localstatequery.QueryTypeShelley,
				[]any{ledger.EraIdConway, cbor.RawMessage(blockQuery)},
			},
		},
	)
	if err != nil {
		t.Fatalf("encoding wrapper: %v", err)
	}
	var query localstatequery.QueryWrapper
	if _, err := cbor.Decode(wrapped, &query); err != nil {
		t.Fatalf("decoding wrapped query: %v", err)
	}
	blockQ, ok := query.Query.(*localstatequery.BlockQuery)
	if !ok {
		t.Fatalf("expected a block query, got %T", query.Query)
	}
	shelleyQ, ok := blockQ.Query.(*localstatequery.ShelleyQuery)
	if !ok {
		t.Fatalf("expected a shelley query, got %T", blockQ.Query)
	}
	paginatedQ, ok := shelleyQ.Query.(*localstatequery.ShelleyUtxoWholePaginatedQuery)
	if !ok {
		t.Fatalf(
			"expected a GetUTxOWholePaginated query, got %T",
			shelleyQ.Query,
		)
	}
	if !bytes.Equal(paginatedQ.CursorTxId, cursorTxId) {
		t.Errorf(
			"cursor tx id: got %x, want %x",
			paginatedQ.CursorTxId,
			cursorTxId,
		)
	}
	if paginatedQ.CursorIdx != 3 {
		t.Errorf("cursor idx: got %d, want 3", paginatedQ.CursorIdx)
	}
	if paginatedQ.Limit != 50000 {
		t.Errorf("limit: got %d, want 50000", paginatedQ.Limit)
	}
}

// TestUtxoWholePaginatedResultDecodes proves UTxOWholePaginatedResult
// decodes the shape the server actually sends -- see
// ledger.queryShelleyUtxoWholePaginated on the dingo side, which destructures
// its fields into a bare []any the way queryShelleyUtxoWhole already does for
// UTxOsResult, rather than encoding the Go struct directly.
func TestUtxoWholePaginatedResultDecodes(t *testing.T) {
	var txId [32]byte
	copy(txId[:], bytes.Repeat([]byte{0xCD}, 32))
	utxoId := localstatequery.UtxoId{
		Hash: ledger.NewBlake2b256(txId[:]),
		Idx:  2,
	}
	nextCursorTxId := bytes.Repeat([]byte{0xEF}, 32)

	addr, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeKeyNone,
		lcommon.AddressNetworkTestnet,
		bytes.Repeat([]byte{0xAA}, lcommon.AddressHashSize),
		nil,
	)
	if err != nil {
		t.Fatalf("building fixture address: %v", err)
	}
	output := babbage.BabbageTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 5_000_000},
	}
	outputCbor, err := cbor.Encode(&output)
	if err != nil {
		t.Fatalf("encoding fixture output: %v", err)
	}

	encoded, err := cbor.Encode(
		[]any{
			map[any]any{
				utxoId: cbor.RawMessage(outputCbor),
			},
			true,
			nextCursorTxId,
			uint32(7),
		},
	)
	if err != nil {
		t.Fatalf("encoding result: %v", err)
	}
	var result localstatequery.UTxOWholePaginatedResult
	if _, err := cbor.Decode(encoded, &result); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if !result.HasMore {
		t.Errorf("has more: got false, want true")
	}
	if !bytes.Equal(result.NextCursorTxId, nextCursorTxId) {
		t.Errorf(
			"next cursor tx id: got %x, want %x",
			result.NextCursorTxId,
			nextCursorTxId,
		)
	}
	if result.NextCursorIdx != 7 {
		t.Errorf("next cursor idx: got %d, want 7", result.NextCursorIdx)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results: got %d entries, want 1", len(result.Results))
	}
}
