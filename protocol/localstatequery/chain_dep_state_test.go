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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// poolKeyHashHex is an arbitrary 28-byte block-issuer (pool cold) key hash
// used across the chain-dep-state fixtures.
const poolKeyHashHex = "0102030405060708090a0b0c0d0e0f10" +
	"1112131415161718191a1b1c"

// poolKeyHashHex2 is a second, distinct 28-byte key hash for multi-pool cases.
const poolKeyHashHex2 = "1c1b1a191817161514131211100f0e0d" +
	"0c0b0a090807060504030201"

func blake224(t *testing.T, s string) ledger.Blake2b224 {
	t.Helper()
	var h ledger.Blake2b224
	raw, err := hex.DecodeString(s)
	require.NoError(t, err, "decode key hash %q", s)
	require.Len(t, raw, len(h), "key hash %q wrong length", s)
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
	require.NoError(t, err)
	// [0, [0, [6, [13]]]]  =>  82 00 82 00 82 06 81 0d
	require.Equal(t, "820082008206810d", hex.EncodeToString(data))
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
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(9_000_000), got.LastSlot.Slot)

	v, ok := got.OpCertCounter(pool1)
	assert.True(t, ok)
	assert.Equal(t, uint64(7), v)
	v, ok = got.OpCertCounter(pool2)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), v)

	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EvolvingNonce.Type)
	assert.Equal(t, hash32(0xaa), got.EvolvingNonce.Value[:])
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.CandidateNonce.Type)

	// Praos-only nonces must be populated.
	require.NotNil(t, got.EpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EpochNonce.Type)
	assert.NotNil(t, got.PreviousEpochNonce)
	assert.NotNil(t, got.LabNonce)
	assert.NotNil(t, got.LastEpochBlockNonce)
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
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolTPraos, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(123), got.LastSlot.Slot)

	v, ok := got.OpCertCounter(pool1)
	assert.True(t, ok)
	assert.Equal(t, uint64(3), v)

	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EvolvingNonce.Type)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.CandidateNonce.Type)

	assert.Nil(t, got.EpochNonce)
	assert.Nil(t, got.PreviousEpochNonce)
	assert.Nil(t, got.LabNonce)
	assert.Nil(t, got.LastEpochBlockNonce)
}

// TestDebugChainDepStateReusedResultClearsPraosNonces guards against stale
// state: decoding a TPraos result into a value previously populated by a Praos
// decode must leave every Praos-only nonce nil.
func TestDebugChainDepStateReusedResultClearsPraosNonces(t *testing.T) {
	pool1 := blake224(t, poolKeyHashHex)

	praosInner := []any{
		slotAt(10),
		map[ledger.Blake2b224]uint64{pool1: 1},
		nonceHash(hash32(0x11)),
		nonceHash(hash32(0x22)),
		nonceHash(hash32(0x33)),
		nonceHash(hash32(0x44)),
		nonceHash(hash32(0x55)),
		nonceHash(hash32(0x66)),
	}
	praosWire, err := cbor.Encode([]any{chainDepStateVersionPraos, praosInner})
	require.NoError(t, err)

	// Reuse the same result value for the second decode.
	var result DebugChainDepStateResult
	_, err = cbor.Decode(praosWire, &result)
	require.NoError(t, err)
	require.NotNil(t, result.EpochNonce, "Praos decode should populate nonces")

	tpraosInner := []any{
		slotAt(20),
		[]any{
			map[ledger.Blake2b224]uint64{pool1: 2},
			nonceHash(hash32(0x77)),
			nonceNeutral,
		},
	}
	tpraosWire, err := cbor.Encode(
		[]any{chainDepStateVersionTPraos, tpraosInner},
	)
	require.NoError(t, err)

	_, err = cbor.Decode(tpraosWire, &result)
	require.NoError(t, err)

	assert.Equal(t, ChainDepStateProtocolTPraos, result.Protocol)
	assert.Nil(t, result.EpochNonce)
	assert.Nil(t, result.PreviousEpochNonce)
	assert.Nil(t, result.LabNonce)
	assert.Nil(t, result.LastEpochBlockNonce)
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
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.False(t, got.LastSlot.HasSlot)
	assert.Empty(t, got.OpCertCounters)
	_, ok := got.OpCertCounter(blake224(t, poolKeyHashHex))
	assert.False(t, ok, "absent pool must return ok=false")
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
	require.NoError(t, err, "hex")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(1), got.LastSlot.Slot)

	v, ok := got.OpCertCounter(blake224(t, poolKeyHashHex))
	assert.True(t, ok)
	assert.Equal(t, uint64(1), v)

	for _, n := range []lcommon.Nonce{got.EvolvingNonce, got.CandidateNonce} {
		assert.Equal(t, uint(lcommon.NonceTypeNeutral), n.Type)
	}
}

// TestDebugChainDepStateRejectsUnknownVersion ensures a serialisation version
// the decoder does not understand is surfaced as an error rather than silently
// producing an empty/mislabelled result.
func TestDebugChainDepStateRejectsUnknownVersion(t *testing.T) {
	wire, err := cbor.Encode([]any{uint64(2), []any{}})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.Error(t, err, "expected decode error for unknown version")
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
			assert.Equal(t, tc.wantVal, v)
			assert.Equal(t, tc.wantOk, ok)
		})
	}
}
