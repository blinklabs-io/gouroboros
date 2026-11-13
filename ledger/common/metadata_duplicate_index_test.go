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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func TestBabbageMetadataSetDuplicateIndexes(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		last byte
	}{
		{"identical", []byte{0xa2, 0, 0xa1, 0, 1, 0, 0xa1, 0, 1}, 1},
		{"different", []byte{0xa2, 0, 0xa1, 0, 1, 0, 0xa1, 0, 2}, 2},
		{"indefinite", []byte{0xbf, 0, 0xa1, 0, 1, 0, 0xa1, 0, 2, 0xff}, 2},
		{"nonadjacent", []byte{0xa3, 0, 0xa1, 0, 1, 1, 0xa0, 0, 0xa1, 0, 2}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var set common.BabbageTransactionMetadataSet
			_, err := cbor.Decode(tc.raw, &set)
			require.NoError(t, err)
			raw, ok := set.GetRawMetadata(0)
			require.True(t, ok)
			require.Equal(t, cbor.RawMessage{0xa1, 0, tc.last}, raw)
			metadata, ok := set.GetMetadata(0)
			require.True(t, ok)
			pairs := metadata.(common.MetaMap).Pairs
			require.Len(t, pairs, 1)
			require.Equal(
				t,
				int64(tc.last),
				pairs[0].Value.(common.MetaInt).Value.Int64(),
			)
			encoded, err := cbor.Encode(set)
			require.NoError(t, err)
			require.Equal(t, tc.raw, encoded)
			require.Equal(t, tc.raw, set.Cbor())

			var strict common.TransactionMetadataSet
			_, err = cbor.Decode(tc.raw, &strict)
			require.ErrorContains(t, err, "duplicate map key")
		})
	}
}

func TestBabbageMetadataSetChecksOverwrittenEntries(t *testing.T) {
	for _, raw := range [][]byte{
		{0xa2, 0, 0xf6, 0, 0xa1, 0, 1}, // invalid first auxiliary value
		{0xa2, 0, 0xa1, 0, 1, 0, 0xf6}, // invalid last auxiliary value
		{0xa2, 0x20, 0xa0, 0, 0xa0},    // negative transaction index
	} {
		var set common.BabbageTransactionMetadataSet
		_, err := cbor.Decode(raw, &set)
		require.Error(t, err)
	}
}

func replaceAuxiliaryIndexMap(t *testing.T, raw, auxiliary []byte) []byte {
	t.Helper()
	var parts, header, body []cbor.RawMessage
	_, err := cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 5)
	parts[3] = auxiliary
	_, err = cbor.Decode(parts[0], &header)
	require.NoError(t, err)
	require.Len(t, header, 2)
	_, err = cbor.Decode(header[0], &body)
	require.NoError(t, err)
	require.Len(t, body, 10)
	var hashes []byte
	for _, part := range parts[1:] {
		hashes = append(hashes, common.Blake2b256Hash(part).Bytes()...)
	}
	body[7] = mustEncodeCBOR(t, common.Blake2b256Hash(hashes).Bytes())
	header[0] = mustEncodeCBOR(t, body)
	parts[0] = mustEncodeCBOR(t, header)
	return mustEncodeCBOR(t, parts)
}

func TestBlockAuxiliaryDuplicateIndexesRespectEra(t *testing.T) {
	for _, indefinite := range []bool{false, true} {
		last := []byte{0xa1, 0, 2}
		aux := []byte{0xa2, 0, 0xa1, 0, 1, 0, 0xa1, 0, 2}
		if indefinite {
			aux[0] = 0xbf
			aux = append(aux, 0xff)
		}
		// These fixtures have updated body commitments, not new block signatures.
		raw, _ := blockAndTransactionWithAuxiliaryData(
			t, testdata.BabbageBlockHex, last, true, false,
		)
		raw = replaceAuxiliaryIndexMap(t, raw, aux)
		block, err := babbage.NewBabbageBlockFromCbor(raw)
		require.NoError(t, err)
		require.Equal(t, aux, block.TransactionMetadataSet.Cbor())
		metadata, ok := block.TransactionMetadataSet.GetRawMetadata(0)
		require.True(t, ok)
		require.Equal(t, cbor.RawMessage(last), metadata)
		encoded, err := cbor.Encode(block)
		require.NoError(t, err)
		require.Equal(t, raw, encoded)

		// Normalizing away the duplicate changes the authenticated body bytes,
		// even though the final transaction metadata stays the same.
		var parts []cbor.RawMessage
		_, err = cbor.Decode(raw, &parts)
		require.NoError(t, err)
		require.Len(t, parts, 5)
		parts[3] = mustEncodeCBOR(t, map[uint]cbor.RawMessage{0: last})
		_, err = babbage.NewBabbageBlockFromCbor(mustEncodeCBOR(t, parts))
		require.ErrorContains(t, err, "body hash mismatch")

		raw, _ = blockAndTransactionWithAuxiliaryData(
			t, testdata.ConwayBlockHex, last, true, false,
		)
		raw = replaceAuxiliaryIndexMap(t, raw, aux)
		_, err = conway.NewConwayBlockFromCbor(raw)
		require.ErrorContains(t, err, "duplicate map key")
	}
}

func TestBabbageDuplicateAuxiliaryIndexesStillCheckBounds(t *testing.T) {
	raw, _ := blockAndTransactionWithAuxiliaryData(
		t, testdata.BabbageBlockHex, []byte{0xa0}, true, false,
	)
	var parts, bodies []cbor.RawMessage
	_, err := cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 5)
	_, err = cbor.Decode(parts[1], &bodies)
	require.NoError(t, err)
	index := mustEncodeCBOR(t, uint(len(bodies)))
	aux := []byte{0xa2}
	for range 2 {
		aux = append(aux, index...)
		aux = append(aux, 0xa0)
	}
	_, err = babbage.NewBabbageBlockFromCbor(
		replaceAuxiliaryIndexMap(t, raw, aux),
	)
	require.ErrorContains(t, err, "outside transaction list length")
}
