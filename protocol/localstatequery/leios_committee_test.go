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
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// MsgQuery(Block(Shelley(era=7, GetLeiosCommittee))), the message
// cardano-cli sends for `dijkstra query leios-committee`. Before query 41
// was registered in shelleyQueryTypes, decoding it failed with "unknown Block
// query type: 41" and the connection closed.
func TestDecodeLeiosCommitteeQuery(t *testing.T) {
	payload, err := hex.DecodeString("8203820082008207811829")
	require.NoError(t, err)

	msg, err := NewMsgFromCbor(MessageTypeQuery, payload)
	require.NoError(t, err)
	blockQuery := msg.(*MsgQuery).Query.Query.(*BlockQuery)
	shelleyQuery := blockQuery.Query.(*ShelleyQuery)
	require.Equal(t, uint(7), shelleyQuery.Era)
	query, ok := shelleyQuery.Query.(*ShelleyLeiosCommitteeQuery)
	require.True(t, ok)
	require.Equal(t, QueryTypeShelleyLeiosCommittee, query.Type)

	encoded, err := cbor.Encode(query)
	require.NoError(t, err)
	require.Equal(t, "811829", hex.EncodeToString(encoded))

	queryType, ok := shelleyLeafQueryType(msg.(*MsgQuery).Query)
	require.True(t, ok)
	require.Equal(t, QueryTypeShelleyLeiosCommittee, queryType)
}

func TestLeiosCommitteeResultEncoding(t *testing.T) {
	half := big.NewRat(1, 2)
	third := big.NewRat(1, 3)
	key := bytes.Repeat([]byte{0xaa}, 96)
	tests := []struct {
		name   string
		result LeiosCommitteeResult
		// Bytes the reference ledger writes: [[current, StrictMaybe next]],
		// weights as tag 30, keys as StrictMaybe ByteString.
		wantHex string
	}{
		{
			name: "next committee not available",
			result: LeiosCommitteeResult{
				Current: []LeiosCommitteeSeat{{Weight: half}},
			},
			wantHex: "81" + "82" + "81" + "82d81e820102" + "80" + "80",
		},
		{
			name: "seat with key and next committee",
			result: LeiosCommitteeResult{
				Current: []LeiosCommitteeSeat{{Weight: half, VKey: key}},
				Next: []LeiosCommitteeSeat{
					{Weight: third},
					{Weight: half, VKey: key},
				},
			},
			wantHex: "81" + "82" +
				"81" + "82d81e820102" + "81" + "5860" + strings.Repeat("aa", 96) +
				"81" + "82" + "82d81e820103" + "80" +
				"82d81e820102" + "81" + "5860" + strings.Repeat("aa", 96),
		},
		{
			name: "empty committees",
			result: LeiosCommitteeResult{
				Current: nil,
				Next:    []LeiosCommitteeSeat{},
			},
			wantHex: "81" + "82" + "80" + "81" + "80",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := cbor.Encode(tc.result)
			require.NoError(t, err)
			require.Equal(t, tc.wantHex, hex.EncodeToString(encoded))

			var decoded LeiosCommitteeResult
			require.NoError(t, decoded.UnmarshalCBOR(encoded))
			require.Len(t, decoded.Current, len(tc.result.Current))
			for i, seat := range tc.result.Current {
				require.Zero(t, seat.Weight.Cmp(decoded.Current[i].Weight))
				require.Equal(t, seat.VKey, decoded.Current[i].VKey)
			}
			require.Equal(t, tc.result.Next == nil, decoded.Next == nil)
			require.Len(t, decoded.Next, len(tc.result.Next))
		})
	}
}

// The ledger writes a list of more than 23 items indefinite-length, and the
// decoder accepts either form.
func TestLeiosCommitteeResultLargeCommittee(t *testing.T) {
	seats := make([]LeiosCommitteeSeat, 24)
	for i := range seats {
		seats[i] = LeiosCommitteeSeat{
			Weight: big.NewRat(int64(i+1), 100),
			VKey:   bytes.Repeat([]byte{byte(i)}, 96),
		}
	}
	encoded, err := cbor.Encode(LeiosCommitteeResult{Current: seats})
	require.NoError(t, err)
	// [ [ _ seat..., break ], [] ]
	require.Equal(t, "8182", hex.EncodeToString(encoded[:2]))
	require.Equal(t, byte(0x9f), encoded[2])

	var decoded LeiosCommitteeResult
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
	require.Len(t, decoded.Current, 24)
	require.Nil(t, decoded.Next)
	for i, seat := range decoded.Current {
		require.Zero(t, seats[i].Weight.Cmp(seat.Weight))
		require.Equal(t, seats[i].VKey, seat.VKey)
	}
}

func TestLeiosCommitteeResultRejectsMalformed(t *testing.T) {
	tests := map[string]string{
		"era mismatch reply":      "82" + "80" + "80",
		"empty wrapper":           "80",
		"one committee":           "81" + "81" + "80",
		"three fields":            "81" + "83" + "80" + "80" + "80",
		"next holds two":          "81" + "82" + "80" + "82" + "80" + "80",
		"seat missing key field":  "81" + "82" + "81" + "81" + "d81e820102" + "80",
		"weight not a rational":   "81" + "82" + "81" + "82" + "01" + "80" + "80",
		"key holds two":           "81" + "82" + "81" + "82d81e820102" + "824041" + "80",
		"key wrong length":        "81" + "82" + "81" + "82d81e820102" + "814100" + "80",
		"key not bytes":           "81" + "82" + "81" + "82d81e820102" + "8101" + "80",
		"committee not a list":    "81" + "82" + "01" + "80",
		"zero denominator weight": "81" + "82" + "81" + "82d81e820100" + "80" + "80",
	}
	for name, payloadHex := range tests {
		t.Run(name, func(t *testing.T) {
			payload, err := hex.DecodeString(payloadHex)
			require.NoError(t, err)
			var decoded LeiosCommitteeResult
			require.Error(t, decoded.UnmarshalCBOR(payload))
		})
	}
}

func TestLeiosCommitteeResultRejectsOversizedCommittee(t *testing.T) {
	// A list header declaring 2^32 seats, with no items behind it.
	payload, err := hex.DecodeString("8182" + "9affffffff" + "80")
	require.NoError(t, err)
	var decoded LeiosCommitteeResult
	require.Error(t, decoded.UnmarshalCBOR(payload))
}

func TestLeiosCommitteeResultRejectsOverLimitCommittee(t *testing.T) {
	// One seat over the 65536 a vote index can address, all keyless.
	seat := "82d81e82010280"
	count := lcommon.MaxLeiosCommitteeSize + 1
	payload, err := hex.DecodeString(
		"8182" + "9a" + fmt.Sprintf("%08x", count) +
			strings.Repeat(seat, count) + "80",
	)
	require.NoError(t, err)
	var decoded LeiosCommitteeResult
	require.Error(t, decoded.UnmarshalCBOR(payload))
}

func TestLeiosCommitteeResultRejectsWrongKeyLengthOnEncode(t *testing.T) {
	_, err := cbor.Encode(LeiosCommitteeResult{
		Current: []LeiosCommitteeSeat{
			{Weight: big.NewRat(1, 2), VKey: []byte{0x01}},
		},
	})
	require.Error(t, err)
}

func TestLeiosCommitteeResultRejectsMissingWeight(t *testing.T) {
	_, err := cbor.Encode(LeiosCommitteeResult{
		Current: []LeiosCommitteeSeat{{}},
	})
	require.Error(t, err)
}

// leiosCommitteeGoldenHex is cardano-ledger's golden CBOR for
// queryLeiosCommittee (libs/cardano-ledger-api/golden/dijkstra/cbor, at
// 9df9f98cce86c5f2195142b2c02dd9b39dd9dc7c, identical for Conway): a list of
// three QueryResultLeiosCommittee values, written without the era wrapper
// that the consensus layer adds around a query reply.
//
//  1. empty current committee, next committee present and empty
//  2. current and next committee of two seats, one keyed and one keyless
//  3. the same current committee, next committee absent
const leiosCommitteeGoldenHex = "8382808180828282d81e820308815860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b79cc425db08f8780732d0ea9bc85f" +
	"c6175f272b2344bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d6b0f21b372169ee761938c28914ddc" +
	"b9663de54db288e760a8e14f82d81e82010480818282d81e820308815860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b7" +
	"9cc425db08f8780732d0ea9bc85fc6175f272b2344bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d6b" +
	"0f21b372169ee761938c28914ddcb9663de54db288e760a8e14f82d81e82010480828282d81e820308815860a57565542eaa" +
	"01ef2b910bf0eaba4d98a1e5b8b79cc425db08f8780732d0ea9bc85fc6175f272b2344bb27bc572ebf14022e52689dcedfcc" +
	"f44a00e5bd1aa59db44517217d6b0f21b372169ee761938c28914ddcb9663de54db288e760a8e14f82d81e8201048080"

func TestLeiosCommitteeResultMatchesLedgerGolden(t *testing.T) {
	golden, err := hex.DecodeString(leiosCommitteeGoldenHex)
	require.NoError(t, err)
	var examples []cbor.RawMessage
	_, err = cbor.Decode(golden, &examples)
	require.NoError(t, err)
	require.Len(t, examples, 3)

	// The consensus layer wraps a reply in a one-element array.
	wrap := func(result []byte) []byte {
		return append([]byte{0x81}, result...)
	}

	t.Run("empty committees", func(t *testing.T) {
		var got LeiosCommitteeResult
		require.NoError(t, got.UnmarshalCBOR(wrap(examples[0])))
		require.Empty(t, got.Current)
		require.NotNil(t, got.Next)
		require.Empty(t, got.Next)
	})

	for i, wantNext := range map[int]bool{1: true, 2: false} {
		t.Run(fmt.Sprintf("example %d", i+1), func(t *testing.T) {
			var got LeiosCommitteeResult
			require.NoError(t, got.UnmarshalCBOR(wrap(examples[i])))
			require.Len(t, got.Current, 2)
			require.Zero(t, big.NewRat(3, 8).Cmp(got.Current[0].Weight))
			require.Len(t, got.Current[0].VKey, 96)
			require.Zero(t, big.NewRat(1, 4).Cmp(got.Current[1].Weight))
			require.Nil(t, got.Current[1].VKey)
			require.Equal(t, wantNext, got.Next != nil)
			if wantNext {
				require.Len(t, got.Next, 2)
				require.Equal(t, got.Current[0].VKey, got.Next[0].VKey)
			}
		})
	}

	// Encoding reproduces the reference bytes exactly.
	for i, raw := range examples {
		var got LeiosCommitteeResult
		require.NoError(t, got.UnmarshalCBOR(wrap(raw)))
		encoded, err := cbor.Encode(got)
		require.NoError(t, err)
		require.Equal(
			t,
			hex.EncodeToString(wrap(raw)),
			hex.EncodeToString(encoded),
			"example %d",
			i+1,
		)
	}
}
