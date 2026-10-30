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
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
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
//	StrictMaybe SlotNo: Nothing -> []            Just s -> [s]
//	Nonce             : Neutral -> [0]           Nonce h -> [1, h]

func slotAt(s uint64) []any { return []any{1, s} }

var slotOrigin = []any{0}

func strictSlot(s uint64) []any { return []any{s} }

// tpraosLedgerState builds the TPraos ledger chain-dependent state:
// [[counters, evolving, candidate], [epoch, previousEpoch], lab].
func tpraosLedgerState(
	counters map[ledger.Blake2b224]uint64,
	evolving, candidate, epoch, previousEpoch, lab []any,
) []any {
	return []any{
		[]any{counters, evolving, candidate},
		[]any{epoch, previousEpoch},
		lab,
	}
}

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
// chain-dep-state result and checks every field lands where the layout puts it.
func TestDebugChainDepStateTPraos(t *testing.T) {
	pool1 := blake224(t, poolKeyHashHex)

	inner := []any{
		strictSlot(123),
		tpraosLedgerState(
			map[ledger.Blake2b224]uint64{pool1: 3},
			nonceHash(hash32(0xcc)),
			nonceNeutral,
			nonceHash(hash32(0xdd)),
			nonceNeutral,
			nonceHash(hash32(0xee)),
		),
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
	assert.Equal(t, hash32(0xcc), got.EvolvingNonce.Value[:])
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.CandidateNonce.Type)
	require.NotNil(t, got.EpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EpochNonce.Type)
	assert.Equal(t, hash32(0xdd), got.EpochNonce.Value[:])
	require.NotNil(t, got.PreviousEpochNonce)
	assert.Equal(
		t,
		uint(lcommon.NonceTypeNeutral),
		got.PreviousEpochNonce.Type,
	)
	require.NotNil(t, got.LabNonce)
	assert.Equal(t, hash32(0xee), got.LabNonce.Value[:])
	assert.Nil(t, got.LastEpochBlockNonce)
}

// TestDebugChainDepStateTPraosOrigin decodes a TPraos state with no block
// applied, whose last slot is the empty StrictMaybe.
func TestDebugChainDepStateTPraosOrigin(t *testing.T) {
	inner := []any{
		[]any{},
		tpraosLedgerState(
			map[ledger.Blake2b224]uint64{},
			nonceNeutral, nonceNeutral, nonceNeutral, nonceNeutral,
			nonceNeutral,
		),
	}
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, inner})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")
	assert.False(t, got.LastSlot.HasSlot)
	assert.Empty(t, got.OpCertCounters)
}

// TestDebugChainDepStateTPraosRejectsLongSlot ensures a last slot that is not
// a StrictMaybe is rejected rather than read as a slot.
func TestDebugChainDepStateTPraosRejectsLongSlot(t *testing.T) {
	inner := []any{
		[]any{1, 2},
		tpraosLedgerState(
			map[ledger.Blake2b224]uint64{},
			nonceNeutral, nonceNeutral, nonceNeutral, nonceNeutral,
			nonceNeutral,
		),
	}
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, inner})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.ErrorContains(t, err, "tpraos last slot")
}

// TestDebugChainDepStateTPraosRejectsNull ensures a null in any required
// TPraos field is an error, not a zero value.
func TestDebugChainDepStateTPraosRejectsNull(t *testing.T) {
	pool1 := blake224(t, poolKeyHashHex)
	counters := map[ledger.Blake2b224]uint64{pool1: 1}
	nonce := nonceHash(hash32(0x01))
	state := func(
		slot any,
		prtcl any,
		tickn any,
		lab any,
	) []any {
		return []any{slot, []any{prtcl, tickn, lab}}
	}
	goodPrtcl := []any{counters, nonce, nonce}
	goodTickn := []any{nonce, nonce}
	cases := map[string][]any{
		"null slot item": state([]any{nil}, goodPrtcl, goodTickn, nonce),
		"null slot":      state(nil, goodPrtcl, goodTickn, nonce),
		"null protocol":  state(strictSlot(1), nil, goodTickn, nonce),
		"null counters": state(
			strictSlot(1),
			[]any{nil, nonce, nonce},
			goodTickn,
			nonce,
		),
		"null evolving": state(
			strictSlot(1),
			[]any{counters, nil, nonce},
			goodTickn,
			nonce,
		),
		"null candidate": state(
			strictSlot(1),
			[]any{counters, nonce, nil},
			goodTickn,
			nonce,
		),
		"null tickn": state(strictSlot(1), goodPrtcl, nil, nonce),
		"null epoch": state(
			strictSlot(1),
			goodPrtcl,
			[]any{nil, nonce},
			nonce,
		),
		"null previous": state(
			strictSlot(1),
			goodPrtcl,
			[]any{nonce, nil},
			nonce,
		),
		"null lab":    state(strictSlot(1), goodPrtcl, goodTickn, nil),
		"null ledger": {strictSlot(1), nil},
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			wire, err := cbor.Encode(
				[]any{chainDepStateVersionTPraos, inner},
			)
			require.NoError(t, err, "encode fixture")
			var got DebugChainDepStateResult
			_, err = cbor.Decode(wire, &got)
			require.Error(t, err)
		})
	}
}

// chainDepStateTPraosDiskGolden is the TPraos chain-dependent state of the
// last telescope entry of ouroboros-consensus's Shelley and Alonzo disk
// goldens, byte for byte (the two entries are identical), from
// ouroboros-consensus at commit 840e789b1cd112e161820a20d30d4546f30e6209:
// ouroboros-consensus-cardano/golden/cardano/disk/ChainDepState_Shelley and
// ChainDepState_Alonzo.
const chainDepStateTPraosDiskGolden = "" +
	"82018281018383a2581ca646474b8f5431261506b6c273d307c7569a4eb6c96b" +
	"42dd4a29520a02581ce0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b" +
	"047b082541018201582012dd0a6a7d0e222a97926da03adb5a7768d31cc7c5c2" +
	"bd6828e14a7d25fa3a608201582012dd0a6a7d0e222a97926da03adb5a7768d3" +
	"1cc7c5c2bd6828e14a7d25fa3a608281008201582012dd0a6a7d0e222a97926d" +
	"a03adb5a7768d31cc7c5c2bd6828e14a7d25fa3a608201582012dd0a6a7d0e22" +
	"2a97926da03adb5a7768d31cc7c5c2bd6828e14a7d25fa3a60"

// TestDebugChainDepStateTPraosDiskGolden decodes the state consensus itself
// serialised for the Shelley and Alonzo eras, so the layout is the node's
// rather than this package's idea of it.
func TestDebugChainDepStateTPraosDiskGolden(t *testing.T) {
	t.Parallel()
	wire, err := hex.DecodeString(chainDepStateTPraosDiskGolden)
	require.NoError(t, err, "hex")
	sum := sha256.Sum256(wire)
	require.Equal(
		t,
		"eb9f2ff2652456f4972ff97233746f22bf7693ebcaf5122eb0e0609310020d37",
		hex.EncodeToString(sum[:]),
		"fixture is not the consensus golden",
	)

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolTPraos, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(1), got.LastSlot.Slot)
	require.Len(t, got.OpCertCounters, 2)
	v, ok := got.OpCertCounter(
		blake224(t, "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"),
	)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), v)
	v, ok = got.OpCertCounter(
		blake224(t, "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"),
	)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), v)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EvolvingNonce.Type)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.CandidateNonce.Type)
	require.NotNil(t, got.EpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.EpochNonce.Type)
	require.NotNil(t, got.PreviousEpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.PreviousEpochNonce.Type)
	require.NotNil(t, got.LabNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.LabNonce.Type)
	assert.Nil(t, got.LastEpochBlockNonce)
	assert.Nil(t, got.Announcement)
}

// TestDebugChainDepStateReusedResultClearsPraosNonce guards against stale
// state: decoding a TPraos result into a value previously populated by a Praos
// decode must leave the Praos-only last-epoch-block nonce nil.
func TestDebugChainDepStateReusedResultClearsPraosNonce(t *testing.T) {
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
		strictSlot(20),
		tpraosLedgerState(
			map[ledger.Blake2b224]uint64{pool1: 2},
			nonceHash(hash32(0x77)),
			nonceNeutral, nonceNeutral, nonceNeutral, nonceNeutral,
		),
	}
	tpraosWire, err := cbor.Encode(
		[]any{chainDepStateVersionTPraos, tpraosInner},
	)
	require.NoError(t, err)

	_, err = cbor.Decode(tpraosWire, &result)
	require.NoError(t, err)

	assert.Equal(t, ChainDepStateProtocolTPraos, result.Protocol)
	require.NotNil(t, result.EpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), result.EpochNonce.Type)
	require.NotNil(t, result.PreviousEpochNonce)
	assert.Equal(
		t,
		uint(lcommon.NonceTypeNeutral),
		result.PreviousEpochNonce.Type,
	)
	require.NotNil(t, result.LabNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), result.LabNonce.Type)
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

// praos2Inner builds the state of a Praos2 (Leios) chain-dep state, with the
// announcement of the endorser block the last applied header carried.
func praos2Inner(
	pool ledger.Blake2b224,
	announcement []any,
) []any {
	return []any{
		slotAt(42),
		map[ledger.Blake2b224]uint64{pool: 5},
		nonceHash(hash32(0xaa)), // evolving
		nonceNeutral,            // candidate
		nonceHash(hash32(0xbb)), // epoch
		nonceNeutral,            // previous epoch
		nonceNeutral,            // lab
		nonceNeutral,            // last epoch block
		announcement,
	}
}

// TestDebugChainDepStatePraos2 decodes a Praos2 (Leios) chain-dep-state
// result, which carries the Praos fields plus the announcement of the endorser
// block the last applied header carried.
func TestDebugChainDepStatePraos2(t *testing.T) {
	pool := blake224(t, poolKeyHashHex)
	issuer := blake224(t, poolKeyHashHex2)
	ebHash := hash32(0xee)
	announcement := []any{
		[]any{issuer, ebHash, 4096},
	}

	inner := praos2Inner(pool, announcement)
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, inner})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos2, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(42), got.LastSlot.Slot)

	v, ok := got.OpCertCounter(pool)
	assert.True(t, ok)
	assert.Equal(t, uint64(5), v)

	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EvolvingNonce.Type)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.CandidateNonce.Type)
	require.NotNil(t, got.EpochNonce)
	require.NotNil(t, got.PreviousEpochNonce)
	require.NotNil(t, got.LabNonce)
	require.NotNil(t, got.LastEpochBlockNonce)

	require.NotNil(t, got.Announcement)
	assert.Equal(t, issuer, got.Announcement.Issuer)
	assert.Equal(t, ebHash, got.Announcement.EbHash[:])
	assert.Equal(t, uint32(4096), got.Announcement.EbSize)
}

// TestDebugChainDepStatePraos2WithoutAnnouncement covers the header that
// carried no announcement: the ledger writes an empty list for it.
func TestDebugChainDepStatePraos2WithoutAnnouncement(t *testing.T) {
	pool := blake224(t, poolKeyHashHex)

	inner := praos2Inner(pool, []any{})
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, inner})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos2, got.Protocol)
	assert.Nil(t, got.Announcement)
}

// TestDebugChainDepStatePraos2Golden pins the exact Praos2 wire bytes so the
// decoded field mapping cannot silently drift from the ouroboros-consensus
// layout.
func TestDebugChainDepStatePraos2Golden(t *testing.T) {
	// [1, [ [1,1], {hash:1}, [0], [0], [0], [0], [0], [0], [[hash, hash, 0]] ]]
	//   82 01                          version 1, outer list-2
	//   89                             inner list-9
	//     82 01 01                     lastSlot = At 1
	//     a1 581c <28-byte hash> 01    opcert counters {hash: 1}
	//     81 00  (x6)                  six neutral nonces
	//     81 83 581c <28-byte hash>    the announcement
	//          5820 <32-byte hash> 00
	wire, err := hex.DecodeString(
		"820189" +
			"820101" +
			"a1581c" + poolKeyHashHex + "01" +
			"810081008100810081008100" +
			"8183581c" + poolKeyHashHex2 +
			"5820" + strings.Repeat("ab", 32) + "00",
	)
	require.NoError(t, err, "hex")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos2, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(1), got.LastSlot.Slot)

	v, ok := got.OpCertCounter(blake224(t, poolKeyHashHex))
	assert.True(t, ok)
	assert.Equal(t, uint64(1), v)

	require.NotNil(t, got.Announcement)
	assert.Equal(t, blake224(t, poolKeyHashHex2), got.Announcement.Issuer)
	assert.Equal(
		t,
		strings.Repeat("ab", 32),
		hex.EncodeToString(got.Announcement.EbHash[:]),
	)
	assert.Equal(t, uint32(0), got.Announcement.EbSize)
}

// TestDebugChainDepStateRejectsUnknownLayout ensures a state whose layout
// matches no protocol is surfaced as an error rather than silently producing
// an empty or mislabelled result.
func TestDebugChainDepStateRejectsUnknownLayout(t *testing.T) {
	wire, err := cbor.Encode([]any{chainDepStateVersionTPraos, []any{1, 2, 3}})
	require.NoError(t, err, "encode fixture")

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.Error(t, err, "expected decode error for unknown layout")
}

// TestDebugChainDepStatePraos2RejectsLongAnnouncement keeps the StrictMaybe
// of the announcement to its two shapes: the ledger writes the empty list or
// a one-element list, and decodeStrictMaybe rejects any other length.
func TestDebugChainDepStatePraos2RejectsLongAnnouncement(t *testing.T) {
	t.Parallel()
	pool := blake224(t, poolKeyHashHex)
	issuer := blake224(t, poolKeyHashHex2)
	entry := []any{issuer, hash32(0xee), 4096}
	for _, announcement := range [][]any{
		{entry, entry},
		{entry, entry, entry},
	} {
		wire, err := cbor.Encode(
			[]any{
				chainDepStateVersionTPraos,
				praos2Inner(pool, announcement),
			},
		)
		require.NoError(t, err, "encode fixture")
		var got DebugChainDepStateResult
		_, err = cbor.Decode(wire, &got)
		require.Errorf(
			t,
			err,
			"announcement of %d entries must be rejected",
			len(announcement),
		)
	}
}

// TestDebugChainDepStatePraos2EbSizeBoundary pins the width of the endorser
// block size: EbReferencesAnnouncement holds a Word32 (cardano-ledger
// Cardano.Ledger.Block), so the largest value is 2^32-1.
func TestDebugChainDepStatePraos2EbSizeBoundary(t *testing.T) {
	t.Parallel()
	pool := blake224(t, poolKeyHashHex)
	issuer := blake224(t, poolKeyHashHex2)
	decode := func(size uint64) (DebugChainDepStateResult, error) {
		wire, err := cbor.Encode([]any{
			chainDepStateVersionTPraos,
			praos2Inner(pool, []any{[]any{issuer, hash32(0xee), size}}),
		})
		require.NoError(t, err, "encode fixture")
		var got DebugChainDepStateResult
		_, err = cbor.Decode(wire, &got)
		return got, err
	}
	got, err := decode(math.MaxUint32)
	require.NoError(t, err)
	require.NotNil(t, got.Announcement)
	assert.Equal(t, uint32(math.MaxUint32), got.Announcement.EbSize)
	_, err = decode(math.MaxUint32 + 1)
	require.Error(t, err, "a size above Word32 must be rejected")
}

// chainDepStateDijkstraDiskGolden is the Praos2 chain-dependent state of the
// last telescope entry of ouroboros-consensus's Dijkstra disk golden, byte for
// byte, from ouroboros-consensus at commit
// 840e789b1cd112e161820a20d30d4546f30e6209:
// ouroboros-consensus-cardano/golden/cardano/disk/ChainDepState_Dijkstra.
// It is the version-1 wrapper and the nine-field state, with no announcement.
const chainDepStateDijkstraDiskGolden = "" +
	"820189820101a2581ca646474b8f5431261506b6c273d307c7569a4eb6c96b42" +
	"dd4a29520a02581ce0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b04" +
	"7b082541018201582012dd0a6a7d0e222a97926da03adb5a7768d31cc7c5c2bd" +
	"6828e14a7d25fa3a608201582012dd0a6a7d0e222a97926da03adb5a7768d31c" +
	"c7c5c2bd6828e14a7d25fa3a60810081008201582012dd0a6a7d0e222a97926d" +
	"a03adb5a7768d31cc7c5c2bd6828e14a7d25fa3a608201582012dd0a6a7d0e22" +
	"2a97926da03adb5a7768d31cc7c5c2bd6828e14a7d25fa3a6080"

// TestDebugChainDepStatePraos2DijkstraDiskGolden decodes the state consensus
// itself serialised, so the Praos2 layout is the node's rather than this
// package's idea of it. The result has no encoder, so this is decode-only.
func TestDebugChainDepStatePraos2DijkstraDiskGolden(t *testing.T) {
	t.Parallel()
	wire, err := hex.DecodeString(chainDepStateDijkstraDiskGolden)
	require.NoError(t, err, "hex")
	sum := sha256.Sum256(wire)
	require.Equal(
		t,
		"a1e56ebf3518919d0fb62b3baefbb622e8cf985931a4ff014088dd802cfb59a2",
		hex.EncodeToString(sum[:]),
		"fixture is not the consensus golden",
	)

	var got DebugChainDepStateResult
	_, err = cbor.Decode(wire, &got)
	require.NoError(t, err, "decode")

	assert.Equal(t, ChainDepStateProtocolPraos2, got.Protocol)
	assert.True(t, got.LastSlot.HasSlot)
	assert.Equal(t, uint64(1), got.LastSlot.Slot)
	require.Len(t, got.OpCertCounters, 2)
	v, ok := got.OpCertCounter(
		blake224(t, "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"),
	)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), v)
	v, ok = got.OpCertCounter(
		blake224(t, "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"),
	)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), v)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.EvolvingNonce.Type)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.CandidateNonce.Type)
	require.NotNil(t, got.EpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.EpochNonce.Type)
	require.NotNil(t, got.PreviousEpochNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNeutral), got.PreviousEpochNonce.Type)
	require.NotNil(t, got.LabNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.LabNonce.Type)
	require.NotNil(t, got.LastEpochBlockNonce)
	assert.Equal(t, uint(lcommon.NonceTypeNonce), got.LastEpochBlockNonce.Type)
	assert.Nil(t, got.Announcement)
}
