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

package localstatequery

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// poolKeyHashHex is an arbitrary 28-byte block-issuer (pool cold) key hash
// used across the chain-dep-state fixtures.
const poolKeyHashHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c"

// poolKeyHashHex2 is a second, distinct 28-byte key hash for multi-pool cases.
const poolKeyHashHex2 = "1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201"

func blake224(t *testing.T, s string) ledger.Blake2b224 {
	t.Helper()
	var h ledger.Blake2b224
	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode key hash %q: %s", s, err)
	}
	if len(raw) != len(h) {
		t.Fatalf("key hash %q: expected %d bytes, got %d", s, len(h), len(raw))
	}
	copy(h[:], raw)
	return h
}

// hash32 returns a 32-byte slice filled with b, for use as a Nonce value.
func hash32(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}

// The following builders assemble the raw CBOR terms for chain-dep-state
// fixtures independently of the type under test, following the encodings
// verified against ouroboros-consensus / cardano-ledger:
//
//	WithOrigin SlotNo : Origin -> [0]            At s -> [1, s]
//	Nonce             : Neutral -> [0]           Nonce h -> [1, h]

func slotAt(s uint64) []any { return []any{1, s} }

var slotOrigin = []any{0}

func nonceHash(b []byte) []any { return []any{1, b} }

var nonceNeutral = []any{0}

// TestDebugChainDepStateQueryBuildShelleyQuery verifies the DebugChainDepState
// (sub-query 13) leaf wraps through buildShelleyQuery into the full
// BlockQuery -> ShelleyQuery -> era -> [13] envelope cardano-node expects.
func TestDebugChainDepStateQueryBuildShelleyQuery(t *testing.T) {
	const era = 6 // Conway
	q := buildShelleyQuery(era, QueryTypeShelleyDebugChainDepState)
	data, err := cbor.Encode(q)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	// [0, [0, [6, [13]]]]  =>  82 00 82 00 82 06 81 0d
	want := "820082008206810d"
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("built query mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestDebugChainDepStatePraos decodes a Praos (Babbage+/Conway) chain-dep-state
// result and verifies the opcert counters, slot, protocol tag, and nonces.
func TestDebugChainDepStatePraos(t *testing.T) {
	pool1 := blake224(t, poolKeyHashHex)
	pool2 := blake224(t, poolKeyHashHex2)

	inner := []any{
		slotAt(9_000_000),
		map[ledger.Blake2b224]uint64{pool1: 7, pool2: 42},
		nonceHash(hash32(0xaa)), // evolving
		nonceNeutral,            // candidate
		nonceHash(hash32(0xbb)), // epoch
		nonceNeutral,            // previous epoch
		nonceNeutral,            // lab
		nonceNeutral,            // last epoch block
	}
	wire, err := cbor.Encode([]any{chainDepStateVersionPraos, inner})
	if err != nil {
		t.Fatalf("encode fixture: %s", err)
	}

	var got DebugChainDepStateResult
	if _, err := cbor.Decode(wire, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}

	if got.Protocol != ChainDepStateProtocolPraos {
		t.Fatalf("protocol: got %d want Praos", got.Protocol)
	}
	if !got.LastSlot.HasSlot || got.LastSlot.Slot != 9_000_000 {
		t.Fatalf("last slot: got %#v want At 9000000", got.LastSlot)
	}
	if v, ok := got.OpCertCounter(pool1); !ok || v != 7 {
		t.Fatalf("pool1 counter: got (%d,%v) want (7,true)", v, ok)
	}
	if v, ok := got.OpCertCounter(pool2); !ok || v != 42 {
		t.Fatalf("pool2 counter: got (%d,%v) want (42,true)", v, ok)
	}
	if got.EvolvingNonce.Type != lcommon.NonceTypeNonce {
		t.Fatalf("evolving nonce type: got %d want Nonce", got.EvolvingNonce.Type)
	}
	if hex.EncodeToString(got.EvolvingNonce.Value[:]) != hex.EncodeToString(hash32(0xaa)) {
		t.Fatalf("evolving nonce value mismatch")
	}
	if got.CandidateNonce.Type != lcommon.NonceTypeNeutral {
		t.Fatalf("candidate nonce: got type %d want Neutral", got.CandidateNonce.Type)
	}
	// Praos-only nonces must be populated.
	if got.EpochNonce == nil || got.EpochNonce.Type != lcommon.NonceTypeNonce {
		t.Fatalf("epoch nonce: got %#v want non-nil Nonce", got.EpochNonce)
	}
	if got.PreviousEpochNonce == nil || got.LabNonce == nil ||
		got.LastEpochBlockNonce == nil {
		t.Fatal("Praos-only nonce pointers must be non-nil")
	}
}

// TestDebugChainDepStateTPraos decodes a TPraos (Shelley..Alonzo)
// chain-dep-state result, whose PrtclState nests the counters and the two
// shared nonces, and verifies the Praos-only nonce pointers stay nil.
func TestDebugChainDepStateTPraos(t *testing.T) {
	pool1 := blake224(t, poolKeyHashHex)

	inner := []any{
		slotAt(123),
		[]any{ // PrtclState: [counters, evolvingNonce, candidateNonce]
			map[ledger.Blake2b224]uint64{pool1: 3},
			nonceHash(hash32(0xcc)),
			nonceNeutral,
		},
	}
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, inner})
	if err != nil {
		t.Fatalf("encode fixture: %s", err)
	}

	var got DebugChainDepStateResult
	if _, err := cbor.Decode(wire, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}

	if got.Protocol != ChainDepStateProtocolTPraos {
		t.Fatalf("protocol: got %d want TPraos", got.Protocol)
	}
	if !got.LastSlot.HasSlot || got.LastSlot.Slot != 123 {
		t.Fatalf("last slot: got %#v want At 123", got.LastSlot)
	}
	if v, ok := got.OpCertCounter(pool1); !ok || v != 3 {
		t.Fatalf("pool1 counter: got (%d,%v) want (3,true)", v, ok)
	}
	if got.EvolvingNonce.Type != lcommon.NonceTypeNonce {
		t.Fatalf("evolving nonce: got type %d want Nonce", got.EvolvingNonce.Type)
	}
	if got.CandidateNonce.Type != lcommon.NonceTypeNeutral {
		t.Fatalf("candidate nonce: got type %d want Neutral", got.CandidateNonce.Type)
	}
	if got.EpochNonce != nil || got.PreviousEpochNonce != nil ||
		got.LabNonce != nil || got.LastEpochBlockNonce != nil {
		t.Fatal("TPraos result must leave Praos-only nonce pointers nil")
	}
}

// TestDebugChainDepStateOriginSlot verifies a chain-dep-state taken at chain
// origin (no block applied) and an empty counter map decode cleanly.
func TestDebugChainDepStateOriginSlot(t *testing.T) {
	inner := []any{
		slotOrigin,
		map[ledger.Blake2b224]uint64{},
		nonceNeutral,
		nonceNeutral,
		nonceNeutral,
		nonceNeutral,
		nonceNeutral,
		nonceNeutral,
	}
	wire, err := cbor.Encode([]any{chainDepStateVersionPraos, inner})
	if err != nil {
		t.Fatalf("encode fixture: %s", err)
	}
	var got DebugChainDepStateResult
	if _, err := cbor.Decode(wire, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got.LastSlot.HasSlot {
		t.Fatalf("expected origin slot, got %#v", got.LastSlot)
	}
	if len(got.OpCertCounters) != 0 {
		t.Fatalf("expected empty counters, got %d", len(got.OpCertCounters))
	}
	if _, ok := got.OpCertCounter(blake224(t, poolKeyHashHex)); ok {
		t.Fatal("absent pool must return ok=false")
	}
}

// TestDebugChainDepStateGolden pins the exact Praos wire bytes so the decoded
// field mapping cannot silently drift from the ouroboros-consensus layout.
func TestDebugChainDepStateGolden(t *testing.T) {
	// [0, [ [1,1], {hash:1}, [0],[0],[0],[0],[0],[0] ]]
	//   82 00                          version 0, outer list-2
	//   88                             inner list-8
	//     82 01 01                     lastSlot = At 1
	//     a1 581c <28-byte hash> 01    opcert counters {hash: 1}
	//     81 00  (x6)                  six neutral nonces
	wire, err := hex.DecodeString(
		"820088820101a1581c" + poolKeyHashHex + "01" +
			"810081008100810081008100",
	)
	if err != nil {
		t.Fatalf("hex: %s", err)
	}
	var got DebugChainDepStateResult
	if _, err := cbor.Decode(wire, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got.Protocol != ChainDepStateProtocolPraos {
		t.Fatalf("protocol: got %d want Praos", got.Protocol)
	}
	if !got.LastSlot.HasSlot || got.LastSlot.Slot != 1 {
		t.Fatalf("last slot: got %#v want At 1", got.LastSlot)
	}
	if v, ok := got.OpCertCounter(blake224(t, poolKeyHashHex)); !ok || v != 1 {
		t.Fatalf("counter: got (%d,%v) want (1,true)", v, ok)
	}
	for _, n := range []lcommon.Nonce{got.EvolvingNonce, got.CandidateNonce} {
		if n.Type != lcommon.NonceTypeNeutral {
			t.Fatalf("expected neutral nonce, got type %d", n.Type)
		}
	}
}

// TestDebugChainDepStateRejectsUnknownVersion ensures a serialisation version
// the decoder does not understand is surfaced as an error rather than silently
// producing an empty/mislabelled result.
func TestDebugChainDepStateRejectsUnknownVersion(t *testing.T) {
	wire, err := cbor.Encode([]any{uint64(2), []any{}})
	if err != nil {
		t.Fatalf("encode fixture: %s", err)
	}
	var got DebugChainDepStateResult
	if _, err := cbor.Decode(wire, &got); err == nil {
		t.Fatal("expected decode error for unknown version, got nil")
	}
}

// TestDebugChainDepStateOpCertCounterLookup exercises the per-pool lookup
// helper for present and absent pools.
func TestDebugChainDepStateOpCertCounterLookup(t *testing.T) {
	present := blake224(t, poolKeyHashHex)
	absent := blake224(t, poolKeyHashHex2)
	r := DebugChainDepStateResult{
		OpCertCounters: map[ledger.Blake2b224]uint64{present: 99},
	}
	cases := []struct {
		name    string
		pool    ledger.Blake2b224
		wantVal uint64
		wantOk  bool
	}{
		{"present", present, 99, true},
		{"absent", absent, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := r.OpCertCounter(tc.pool)
			if v != tc.wantVal || ok != tc.wantOk {
				t.Fatalf("got (%d,%v) want (%d,%v)", v, ok, tc.wantVal, tc.wantOk)
			}
		})
	}
}
