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

package dijkstra

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// dijkstraHeaderWithBodyFields rebuilds the captured header with the given
// trailing body fields after the ten Babbage fields.
func dijkstraHeaderWithBodyFields(
	t *testing.T,
	extra ...any,
) []byte {
	t.Helper()
	full, err := hex.DecodeString(leiosExtendedHeaderHex)
	require.NoError(t, err)
	var top []cbor.RawMessage
	_, err = cbor.Decode(full, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	var bodyElems []cbor.RawMessage
	_, err = cbor.Decode(top[0], &bodyElems)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(bodyElems), 10)
	body := make([]any, 0, 10+len(extra))
	for _, e := range bodyElems[:10] {
		body = append(body, e)
	}
	body = append(body, extra...)
	out, err := cbor.Encode([]any{body, top[1]})
	require.NoError(t, err)
	return out
}

func TestDijkstraBlockHeaderRequiresExactlyTwelveBodyFields(t *testing.T) {
	t.Parallel()
	hash32 := make([]byte, 32)
	announcement := []any{hash32, uint64(0xffffffff)}

	valid := map[string][]any{
		"null announcement": {true, nil},
		"announcement":      {false, announcement},
		"max uint32 size":   {true, announcement},
	}
	for name, extra := range valid {
		t.Run("valid "+name, func(t *testing.T) {
			t.Parallel()
			raw := dijkstraHeaderWithBodyFields(t, extra...)
			var h DijkstraBlockHeader
			_, err := cbor.Decode(raw, &h)
			require.NoError(t, err)
			require.Len(t, h.LeiosHeaderExtension, 2)
			out, err := h.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, raw, out)
			var top []cbor.RawMessage
			_, err = cbor.Decode(raw, &top)
			require.NoError(t, err)
			require.Equal(t, []byte(top[0]), h.Body.Cbor())
		})
	}

	invalid := map[string][]any{
		"10 fields":             {},
		"11 fields bool":        {true},
		"11 fields legacy pair": {announcement},
		"13 fields":             {true, nil, nil},
		"16 fields":             {true, nil, 1, 2, 3, 4},
		"certified not bool":    {uint64(1), nil},
		"certified null":        {nil, nil},
		"announcement uint":     {true, uint64(7)},
		"announcement empty":    {true, []any{}},
		"announcement 1 elem":   {true, []any{hash32}},
		"announcement 3 elems":  {true, []any{hash32, uint64(1), uint64(2)}},
		"announcement short eb": {true, []any{make([]byte, 31), uint64(1)}},
		"announcement long eb":  {true, []any{make([]byte, 33), uint64(1)}},
		"announcement eb text":  {true, []any{"x", uint64(1)}},
		"announcement size neg": {true, []any{hash32, int64(-1)}},
		"announcement size >u32": {
			true, []any{hash32, uint64(0x100000000)},
		},
		"announcement size text": {true, []any{hash32, "x"}},
	}
	for name, extra := range invalid {
		t.Run("reject "+name, func(t *testing.T) {
			t.Parallel()
			raw := dijkstraHeaderWithBodyFields(t, extra...)
			var h DijkstraBlockHeader
			_, err := cbor.Decode(raw, &h)
			require.Error(t, err)
		})
	}
}

func TestDijkstraBlockHeaderMarshalRejectsMalformedLeiosExtension(t *testing.T) {
	t.Parallel()
	invalid := map[string][]cbor.RawMessage{
		"one field":           {{0xf5}},
		"three fields":        {{0xf5}, {0xf6}, {0xf6}},
		"certified not bool":  {{0x01}, {0xf6}},
		"announcement uint":   {{0xf5}, {0x07}},
		"announcement 1 elem": {{0xf5}, {0x81, 0xf6}},
	}
	for name, ext := range invalid {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var src DijkstraBlockHeader
			_, err := cbor.Decode(dijkstraHeaderWithBodyFields(t, true, nil), &src)
			require.NoError(t, err)
			h := DijkstraBlockHeader{BabbageBlockHeader: src.BabbageBlockHeader}
			h.SetCbor(nil)
			h.LeiosHeaderExtension = ext
			_, err = h.MarshalCBOR()
			require.Error(t, err)
		})
	}
}
