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

package ledger_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func blockWithAuxiliaryDataIndex(
	t *testing.T,
	blockData []byte,
	index uint,
) ([]byte, int) {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(blockData, &parts)
	require.NoError(t, err)
	if len(parts) < 4 {
		t.Fatalf("block has %d components, want at least 4", len(parts))
		return nil, 0
	}
	var bodies []cbor.RawMessage
	_, err = cbor.Decode(parts[1], &bodies)
	require.NoError(t, err)
	metadata := map[uint]cbor.RawMessage{}
	if index != ^uint(0) {
		metadata[index] = cbor.RawMessage{0xa0}
	}
	parts[3], err = cbor.Encode(metadata)
	require.NoError(t, err)
	blockData, err = cbor.Encode(parts)
	require.NoError(t, err)
	return blockData, len(bodies)
}

func TestBlockDecodersRejectOutOfRangeAuxiliaryDataIndexes(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		decode func([]byte) error
	}{
		{
			name: "Shelley",
			raw:  testdata.ShelleyBlockHex,
			decode: func(data []byte) error {
				_, err := shelley.NewShelleyBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
		{
			name: "Allegra",
			raw:  testdata.AllegraBlockHex,
			decode: func(data []byte) error {
				_, err := allegra.NewAllegraBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
		{
			name: "Mary",
			raw:  testdata.MaryBlockHex,
			decode: func(data []byte) error {
				_, err := mary.NewMaryBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
		{
			name: "Alonzo",
			raw:  testdata.AlonzoBlockHex,
			decode: func(data []byte) error {
				_, err := alonzo.NewAlonzoBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
		{
			name: "Babbage",
			raw:  testdata.BabbageBlockHex,
			decode: func(data []byte) error {
				_, err := babbage.NewBabbageBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
		{
			name: "Conway",
			raw:  testdata.ConwayBlockHex,
			decode: func(data []byte) error {
				_, err := conway.NewConwayBlockFromCbor(data, common.VerifyConfig{
					SkipBodyHashValidation: true,
				})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockData, err := hex.DecodeString(strings.TrimSpace(tt.raw))
			require.NoError(t, err)
			blockData, transactionCount := blockWithAuxiliaryDataIndex(
				t,
				blockData,
				^uint(0),
			)
			require.NoError(t, tt.decode(blockData), "empty metadata set")
			blockData, _ = blockWithAuxiliaryDataIndex(
				t,
				blockData,
				uint(transactionCount-1),
			)
			require.NoError(t, tt.decode(blockData), "last in-range index")
			blockData, _ = blockWithAuxiliaryDataIndex(
				t,
				blockData,
				uint(transactionCount),
			)
			require.ErrorContains(
				t,
				tt.decode(blockData),
				"auxiliary-data index",
			)
		})
	}
}
