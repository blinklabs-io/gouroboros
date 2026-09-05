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
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const (
	drepKeyHashHex    = "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"
	drepScriptHashHex = "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"
)

// drepStakeDistrReplyHex is a GetDRepStakeDistr reply carrying four DReps: one
// of each credential kind and the two predefined options. The map is
// cardano-ledger's own golden vector for this query,
// libs/cardano-ledger-api/golden/conway/cbor/queryDRepStakeDistr.cbor, with
// the consensus result wrapper prepended.
//
//	81                     ; result wrapper, array(1)
//	  a4                   ; map(4)
//	    82 00 581c e0a7..  ; [0, addr_keyhash]
//	    1a 3b9aca00        ; 1000000000
//	    82 01 581c a646..  ; [1, scripthash]
//	    00                 ; 0
//	    81 02              ; [2] always-abstain
//	    18 32              ; 50
//	    81 03              ; [3] always-no-confidence
//	    18 64              ; 100
//
// The credential-backed DReps come first because encodeMap walks the map in
// Haskell's derived Ord order over DRep's constructors, not in the RFC 8949
// section 4.2.1 order that would sort the one-element arrays ahead of them.
const drepStakeDistrReplyHex = "81a4" +
	"8200581c" + drepKeyHashHex + "1a3b9aca00" +
	"8201581c" + drepScriptHashHex + "00" +
	"8102" + "1832" +
	"8103" + "1864"

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	data, err := hex.DecodeString(s)
	require.NoError(t, err)
	return data
}

// TestDRepStakeDistrResultDecodesWrappedMap covers the reply shape a node
// sends: the DRep-to-stake map inside the single-element result array the
// Shelley era codec adds, with both credential-backed and predefined DReps.
func TestDRepStakeDistrResultDecodesWrappedMap(t *testing.T) {
	var result DRepStakeDistrResult
	_, err := cbor.Decode(mustDecodeHex(t, drepStakeDistrReplyHex), &result)
	require.NoError(t, err)
	require.Len(t, result, 4)

	require.Equal(t, lcommon.DrepTypeAddrKeyHash, result[0].Drep.Type)
	require.Equal(
		t,
		drepKeyHashHex,
		hex.EncodeToString(result[0].Drep.Credential),
	)
	require.Equal(t, uint64(1000000000), result[0].Stake)

	require.Equal(t, lcommon.DrepTypeScriptHash, result[1].Drep.Type)
	require.Equal(
		t,
		drepScriptHashHex,
		hex.EncodeToString(result[1].Drep.Credential),
	)
	require.Equal(t, uint64(0), result[1].Stake)

	require.Equal(t, lcommon.DrepTypeAbstain, result[2].Drep.Type)
	require.Empty(t, result[2].Drep.Credential)
	require.Equal(t, uint64(50), result[2].Stake)

	require.Equal(t, lcommon.DrepTypeNoConfidence, result[3].Drep.Type)
	require.Empty(t, result[3].Drep.Credential)
	require.Equal(t, uint64(100), result[3].Stake)
}

// TestDRepStakeDistrResultDecodesEmptyWrappedMap covers a node with no
// registered DReps, which answers with the wrapper around an empty map rather
// than omitting the reply.
func TestDRepStakeDistrResultDecodesEmptyWrappedMap(t *testing.T) {
	var result DRepStakeDistrResult
	_, err := cbor.Decode([]byte{0x81, 0xa0}, &result)
	require.NoError(t, err)
	require.Empty(t, result)
}

// TestDRepStakeDistrResultRejectsBareMap pins the wrapper. An unwrapped map is
// the shape GetDRepState was decoded against in #2169, where every reply from
// a node that emits the wrapped form failed to decode.
func TestDRepStakeDistrResultRejectsBareMap(t *testing.T) {
	bare := mustDecodeHex(
		t,
		"a1"+"8200581c"+drepKeyHashHex+"1a000f4240",
	)
	var result DRepStakeDistrResult
	_, err := cbor.Decode(bare, &result)
	require.Error(t, err)
}

// TestDRepStakeDistrResultRejectsEraMismatchWrapper covers the other arm of
// the era codec's wrapper, which carries a mismatch rather than a result and
// must not be read as the first element of a distribution.
func TestDRepStakeDistrResultRejectsEraMismatchWrapper(t *testing.T) {
	var result DRepStakeDistrResult
	_, err := cbor.Decode([]byte{0x82, 0x00, 0xa0}, &result)
	require.ErrorContains(t, err, "single-element result array")
}

// TestDRepStakeDistrResultRejectsTruncatedMap pins that a reply whose map
// header declares more pairs than follow it is rejected rather than decoded
// into a short distribution. The wrapper decode is what rejects it: the result
// array's element is read as a cbor.RawMessage, which requires a complete
// well-formed item, so the over-declared map never reaches the entry loop.
func TestDRepStakeDistrResultRejectsTruncatedMap(t *testing.T) {
	for name, reply := range map[string][]byte{
		// map(2) carrying a single pair
		"over-declared header": {
			0x81, 0xa2, 0x81, 0x02, 0x19, 0x01, 0xf4,
		},
		// map(2^64-1) carrying nothing
		"huge header, empty body": {
			0x81, 0xbb,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var result DRepStakeDistrResult
			_, err := cbor.Decode(reply, &result)
			require.Error(t, err)
			require.Empty(t, result)
		})
	}
}

// TestDRepStakeDistrResultMarshalsNodeShape proves the encoder emits the
// wrapper and orders the map keys the way a node does, independent of the
// order the entries were built in.
func TestDRepStakeDistrResultMarshalsNodeShape(t *testing.T) {
	result := DRepStakeDistrResult{
		{
			Drep:  lcommon.Drep{Type: lcommon.DrepTypeNoConfidence},
			Stake: 100,
		},
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeScriptHash,
				Credential: mustDecodeHex(t, drepScriptHashHex),
			},
			Stake: 0,
		},
		{
			Drep:  lcommon.Drep{Type: lcommon.DrepTypeAbstain},
			Stake: 50,
		},
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeAddrKeyHash,
				Credential: mustDecodeHex(t, drepKeyHashHex),
			},
			Stake: 1000000000,
		},
	}
	encoded, err := cbor.Encode(result)
	require.NoError(t, err)
	require.Equal(t, drepStakeDistrReplyHex, hex.EncodeToString(encoded))
}

// TestDRepStakeDistrResultRejectsDuplicateDRep covers the one input that
// cannot be expressed as a CBOR map.
func TestDRepStakeDistrResultRejectsDuplicateDRep(t *testing.T) {
	result := DRepStakeDistrResult{
		{Drep: lcommon.Drep{Type: lcommon.DrepTypeAbstain}, Stake: 1},
		{Drep: lcommon.Drep{Type: lcommon.DrepTypeAbstain}, Stake: 2},
	}
	_, err := cbor.Encode(result)
	require.ErrorContains(t, err, "duplicate DRep")
}

// TestDRepStakeDistrResultRejectsDuplicateAfterEncoding covers two entries
// that differ as Go values but not on the wire: an Abstain DRep carries no
// credential, so both encode to the same map key.
func TestDRepStakeDistrResultRejectsDuplicateAfterEncoding(t *testing.T) {
	result := DRepStakeDistrResult{
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeAbstain,
				Credential: []byte{0x01},
			},
			Stake: 1,
		},
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeAbstain,
				Credential: []byte{0x02},
			},
			Stake: 2,
		},
	}
	_, err := cbor.Encode(result)
	require.ErrorContains(t, err, "duplicate DRep")
}

// TestDRepStakeDistrResultRejectsDuplicateKeyInReply covers a reply whose map
// repeats a DRep. The cbor package's decode modes reject a repeated map key
// (DupMapKeyEnforcedAPF), which every map-shaped result in this package
// inherits; walking this map by hand has to reject it too, or a caller summing
// the entries counts one DRep's stake twice.
func TestDRepStakeDistrResultRejectsDuplicateKeyInReply(t *testing.T) {
	duplicate := "81a2" + "8102" + "1901f4" + "8102" + "1864"
	var result DRepStakeDistrResult
	_, err := cbor.Decode(mustDecodeHex(t, duplicate), &result)
	require.ErrorContains(t, err, "duplicate DRep")
}

// ledgerStyleReply builds a GetDRepStakeDistr reply the way cardano-ledger's
// encodeMap does: n credential-backed DReps in Ord order, under a
// definite-length header at or below the threshold and an indefinite-length
// map with a break above it.
func ledgerStyleReply(n int) []byte {
	body := []byte{}
	for i := range n {
		key := append(
			[]byte{0x82, 0x00, 0x58, 0x1c},
			bytes.Repeat([]byte{byte(i)}, 28)...,
		)
		body = append(body, key...)
		body = append(body, 0x0a) // stake = 10
	}
	out := []byte{0x81}
	if n <= 23 {
		out = append(out, 0xa0|byte(n))
		out = append(out, body...)
		return out
	}
	out = append(out, 0xbf)
	out = append(out, body...)
	return append(out, 0xff)
}

// TestDRepStakeDistrResultDecodesIndefiniteMap covers the reply a real
// distribution arrives in. cardano-ledger's encodeMap uses
// variableMapLenEncoding, which switches from a definite-length header to an
// indefinite-length map above 23 pairs, so every mainnet-sized reply to this
// query is the 0xbf ... 0xff form. Decoding only the definite form would read
// no distribution with more than 23 DReps in it.
func TestDRepStakeDistrResultDecodesIndefiniteMap(t *testing.T) {
	for _, n := range []int{0, 1, 22, 23, 24, 25, 64} {
		t.Run(fmt.Sprintf("%d_dreps", n), func(t *testing.T) {
			reply := ledgerStyleReply(n)
			if n > 23 {
				require.Equal(t, byte(0xbf), reply[1],
					"ledger encodes this size as an indefinite map")
				require.Equal(t, byte(0xff), reply[len(reply)-1])
			}
			var result DRepStakeDistrResult
			_, err := cbor.Decode(reply, &result)
			require.NoError(t, err)
			require.Len(t, result, n)
			for i, entry := range result {
				require.Equal(
					t,
					lcommon.DrepTypeAddrKeyHash,
					entry.Drep.Type,
				)
				require.Equal(
					t,
					bytes.Repeat([]byte{byte(i)}, 28),
					entry.Drep.Credential,
				)
				require.Equal(t, uint64(10), entry.Stake)
			}
		})
	}
}

// TestDRepStakeDistrResultMarshalsIndefiniteAboveThreshold pins the encoder to
// the same rule, so a re-encoded distribution is byte-for-byte what the node
// sent it as.
func TestDRepStakeDistrResultMarshalsIndefiniteAboveThreshold(t *testing.T) {
	for _, n := range []int{23, 24} {
		t.Run(fmt.Sprintf("%d_dreps", n), func(t *testing.T) {
			reply := ledgerStyleReply(n)
			var result DRepStakeDistrResult
			_, err := cbor.Decode(reply, &result)
			require.NoError(t, err)
			encoded, err := cbor.Encode(result)
			require.NoError(t, err)
			require.Equal(
				t,
				hex.EncodeToString(reply),
				hex.EncodeToString(encoded),
			)
		})
	}
}

// TestDRepStakeDistrResultRejectsUnterminatedIndefiniteMap covers an
// indefinite map whose break byte never arrives.
func TestDRepStakeDistrResultRejectsUnterminatedIndefiniteMap(t *testing.T) {
	var result DRepStakeDistrResult
	_, err := cbor.Decode(
		[]byte{0x81, 0xbf, 0x81, 0x02, 0x18, 0x32},
		&result,
	)
	require.Error(t, err)
	require.Empty(t, result)
}
