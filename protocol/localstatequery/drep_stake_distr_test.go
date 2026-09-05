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
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const (
	drepKeyHashHex    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	drepScriptHashHex = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// drepStakeDistrReplyHex is a GetDRepStakeDistr reply carrying four DReps: the
// two predefined options and one of each credential kind.
//
//	81                     ; result wrapper, array(1)
//	  a4                   ; map(4)
//	    81 02              ; [2] always-abstain
//	    19 01f4            ; 500
//	    81 03              ; [3] always-no-confidence
//	    18 64              ; 100
//	    82 00 581c aa..    ; [0, addr_keyhash]
//	    1a 000f4240        ; 1000000
//	    82 01 581c bb..    ; [1, scripthash]
//	    1a 001e8480        ; 2000000
//
// The keys are in the deterministic order of RFC 8949 section 4.2.1, which is
// the order a node emits them in: the one-element arrays sort ahead of the
// two-element ones.
const drepStakeDistrReplyHex = "81a4" +
	"8102" + "1901f4" +
	"8103" + "1864" +
	"8200581c" + drepKeyHashHex + "1a000f4240" +
	"8201581c" + drepScriptHashHex + "1a001e8480"

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

	require.Equal(t, lcommon.DrepTypeAbstain, result[0].Drep.Type)
	require.Empty(t, result[0].Drep.Credential)
	require.Equal(t, uint64(500), result[0].Stake)

	require.Equal(t, lcommon.DrepTypeNoConfidence, result[1].Drep.Type)
	require.Empty(t, result[1].Drep.Credential)
	require.Equal(t, uint64(100), result[1].Stake)

	require.Equal(t, lcommon.DrepTypeAddrKeyHash, result[2].Drep.Type)
	require.Equal(
		t,
		drepKeyHashHex,
		hex.EncodeToString(result[2].Drep.Credential),
	)
	require.Equal(t, uint64(1000000), result[2].Stake)

	require.Equal(t, lcommon.DrepTypeScriptHash, result[3].Drep.Type)
	require.Equal(
		t,
		drepScriptHashHex,
		hex.EncodeToString(result[3].Drep.Credential),
	)
	require.Equal(t, uint64(2000000), result[3].Stake)
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

// TestDRepStakeDistrResultRejectsTruncatedMap covers a map header that claims
// more entries than the reply can hold.
func TestDRepStakeDistrResultRejectsTruncatedMap(t *testing.T) {
	var result DRepStakeDistrResult
	_, err := cbor.Decode([]byte{0x81, 0xbb, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, &result)
	require.Error(t, err)
}

// TestDRepStakeDistrResultMarshalsNodeShape proves the encoder emits the
// wrapper and orders the map keys the way a node does, independent of the
// order the entries were built in.
func TestDRepStakeDistrResultMarshalsNodeShape(t *testing.T) {
	result := DRepStakeDistrResult{
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeScriptHash,
				Credential: mustDecodeHex(t, drepScriptHashHex),
			},
			Stake: 2000000,
		},
		{
			Drep: lcommon.Drep{
				Type:       lcommon.DrepTypeAddrKeyHash,
				Credential: mustDecodeHex(t, drepKeyHashHex),
			},
			Stake: 1000000,
		},
		{
			Drep:  lcommon.Drep{Type: lcommon.DrepTypeNoConfidence},
			Stake: 100,
		},
		{
			Drep:  lcommon.Drep{Type: lcommon.DrepTypeAbstain},
			Stake: 500,
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
