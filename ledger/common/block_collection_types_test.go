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
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// These fixtures have matching body commitments but are not newly signed blocks.
func TestBlockCollectionsRequireMapAndArray(t *testing.T) {
	for _, raw := range [][]byte{{0xf6}, {0xf7}, {0xa0}} {
		var set common.TransactionMetadataSet
		err := set.UnmarshalCBOR(raw)
		if raw[0] == 0xa0 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "must be a CBOR map")
		}
	}
	for _, era := range []struct {
		name, fixture string
		decode        func([]byte) error
	}{
		{"Alonzo", testdata.AlonzoBlockHex, func(b []byte) error { _, err := alonzo.NewAlonzoBlockFromCbor(b); return err }},
		{"Babbage", testdata.BabbageBlockHex, func(b []byte) error { _, err := babbage.NewBabbageBlockFromCbor(b); return err }},
		{"Conway", testdata.ConwayBlockHex, func(b []byte) error { _, err := conway.NewConwayBlockFromCbor(b); return err }},
	} {
		for _, tc := range []struct {
			name  string
			index int
			raw   []byte
		}{
			{"empty control", 3, []byte{0xa0}},
			{"indefinite empty map", 3, []byte{0xbf, 0xff}},
			{"indefinite empty array", 4, []byte{0x9f, 0xff}},
			{"aux null", 3, []byte{0xf6}},
			{"aux undefined", 3, []byte{0xf7}},
			{"invalid null", 4, []byte{0xf6}},
			{"invalid undefined", 4, []byte{0xf7}},
		} {
			t.Run(era.name+"/"+tc.name, func(t *testing.T) {
				fixture, err := hex.DecodeString(strings.TrimSpace(era.fixture))
				require.NoError(t, err)
				var block, header, body []cbor.RawMessage
				_, err = cbor.Decode(fixture, &block)
				require.NoError(t, err)
				require.Len(t, block, 5)
				_, err = cbor.Decode(block[0], &header)
				require.NoError(t, err)
				require.Len(t, header, 2)
				_, err = cbor.Decode(header[0], &body)
				require.NoError(t, err)
				hashIndex := 7
				if era.name == "Alonzo" {
					require.Len(t, body, 15)
					hashIndex = 8
				} else {
					require.Len(t, body, 10)
				}
				block[1] = []byte{0x80}
				block[2] = []byte{0x80}
				block[3] = []byte{0xa0}
				block[4] = []byte{0x80}
				block[tc.index] = tc.raw
				var hashes []byte
				var size uint64
				for _, part := range block[1:] {
					hashes = append(
						hashes,
						common.Blake2b256Hash(part).Bytes()...)
					size += uint64(len(part))
				}
				body[hashIndex-1], err = cbor.Encode(size)
				require.NoError(t, err)
				hash := common.Blake2b256Hash(hashes)
				body[hashIndex], err = cbor.Encode(hash.Bytes())
				require.NoError(t, err)
				header[0], err = cbor.Encode(body)
				require.NoError(t, err)
				block[0], err = cbor.Encode(header)
				require.NoError(t, err)
				encoded, err := cbor.Encode(block)
				require.NoError(t, err)
				require.NoError(
					t,
					common.ValidateBlockBodyHash(encoded, hash, era.name, 5),
				)
				gotSize, err := common.BlockBodySizeFromCbor(encoded)
				require.NoError(t, err)
				require.Equal(t, size, gotSize)
				err = era.decode(encoded)
				require.NotEmpty(t, tc.raw)
				if tc.raw[0] == 0xf6 || tc.raw[0] == 0xf7 {
					if tc.index == 3 {
						require.ErrorContains(t, err, "must be a CBOR map")
					} else {
						require.ErrorContains(t, err, "must be a CBOR array")
					}
				} else {
					require.NoError(t, err)
				}
				block[3] = []byte{0xa1, 0x00, 0xa0}
				corrupted, err := cbor.Encode(block)
				require.NoError(t, err)
				require.Error(
					t,
					common.ValidateBlockBodyHash(corrupted, hash, era.name, 5),
					"negative hash control",
				)
			})
		}
	}
}

func TestStandaloneTransactionAuxiliaryDataCanBeNull(t *testing.T) {
	// This is a decoder fixture, not a ledger-valid transaction.
	raw := []byte{
		0x84,
		0xa3,
		0x00,
		0x80,
		0x01,
		0x80,
		0x02,
		0x00,
		0xa0,
		0xf5,
		0xf6,
	}
	_, err := alonzo.NewAlonzoTransactionFromCbor(raw)
	require.NoError(t, err)
	_, err = babbage.NewBabbageTransactionFromCbor(raw)
	require.NoError(t, err)
	_, err = conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
}
