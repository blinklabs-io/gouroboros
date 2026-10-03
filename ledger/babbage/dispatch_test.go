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

package babbage

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestDatumOptionAcceptsListLengthEncodings(t *testing.T) {
	hash := common.Blake2b256{1, 2, 3}
	canonical, err := cbor.Encode([]any{DatumOptionTypeHash, hash})
	require.NoError(t, err)
	for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
		t.Run(encoding.Name, func(t *testing.T) {
			var decoded BabbageTransactionOutputDatumOption
			require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
			require.NotNil(t, decoded.hash)
			require.Equal(t, hash, *decoded.hash)
		})
	}
}

func TestDatumOptionRoundTripPreservesBytes(t *testing.T) {
	t.Parallel()
	hash := common.Blake2b256{1, 2, 3}
	hashCbor, err := cbor.Encode([]any{DatumOptionTypeHash, hash})
	require.NoError(t, err)
	inlineCbor := func(datumHex string) []byte {
		inner, err := hex.DecodeString(datumHex)
		require.NoError(t, err)
		ret, err := cbor.Encode(
			[]any{DatumOptionTypeData, cbor.Tag{Number: 24, Content: inner}},
		)
		require.NoError(t, err)
		return ret
	}
	tests := []struct {
		name     string
		input    []byte
		wantHash bool
	}{
		{"hash", hashCbor, true},
		// 121([42]) with the indefinite-length field list Plutus emits
		{"inline", inlineCbor("d8799f182aff"), false},
		// 121([42]) with 42 in a non-shortest uint16 head, which re-encoding
		// the decoded Plutus data would not reproduce
		{"inline non-shortest datum", inlineCbor("d8798119002a"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var decoded BabbageTransactionOutputDatumOption
			require.NoError(t, decoded.UnmarshalCBOR(tc.input))
			require.Equal(t, tc.wantHash, decoded.hash != nil)
			require.Equal(t, !tc.wantHash, decoded.data != nil)
			out, err := decoded.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, tc.input, out)
		})
	}
}

func TestDatumOptionVariantAccessors(t *testing.T) {
	t.Parallel()
	hash := common.Blake2b256{1, 2, 3}
	tests := []struct {
		name       string
		option     *BabbageTransactionOutputDatumOption
		wantHash   bool
		wantInline bool
	}{
		{"hash", &BabbageTransactionOutputDatumOption{hash: &hash}, true, false},
		{
			"inline",
			&BabbageTransactionOutputDatumOption{data: &common.Datum{}},
			false,
			true,
		},
		{"empty", &BabbageTransactionOutputDatumOption{}, false, false},
		{"nil", nil, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.wantHash, tc.option.IsDatumHash())
			require.Equal(t, tc.wantInline, tc.option.IsInlineDatum())
		})
	}
}
