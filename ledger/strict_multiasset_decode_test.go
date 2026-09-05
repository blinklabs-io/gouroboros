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
	"math"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeMultiAsset(
	t *testing.T,
	quantity *big.Int,
) map[common.Blake2b224]map[cbor.ByteString]*big.Int {
	t.Helper()
	return map[common.Blake2b224]map[cbor.ByteString]*big.Int{
		{0x11}: {
			cbor.NewByteString([]byte("asset")): quantity,
		},
	}
}

func encodeMintBody(t *testing.T, quantity *big.Int) []byte {
	t.Helper()
	wire, err := cbor.Encode(map[uint64]any{
		9: encodeMultiAsset(t, quantity),
	})
	require.NoError(t, err)
	return wire
}

func TestOutputQuantityBounds(t *testing.T) {
	maxUint64 := new(big.Int).SetUint64(math.MaxUint64)
	tests := []struct {
		name     string
		quantity *big.Int
		wantErr  bool
	}{
		{name: "negative", quantity: big.NewInt(-1), wantErr: true},
		{name: "zero pruned", quantity: new(big.Int)},
		{name: "one", quantity: big.NewInt(1)},
		{name: "maximum uint64", quantity: maxUint64},
		{
			name:     "above uint64",
			quantity: new(big.Int).Add(maxUint64, big.NewInt(1)),
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := cbor.Encode([]any{
				uint64(1),
				encodeMultiAsset(t, test.quantity),
			})
			require.NoError(t, err)

			var value mary.MaryTransactionOutputValue
			err = value.UnmarshalCBOR(wire)
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if test.quantity.Sign() == 0 {
				assert.Empty(t, value.Assets.Policies())
			}
		})
	}
}

func TestMintQuantityBoundsAcrossEras(t *testing.T) {
	decoders := []struct {
		name   string
		decode func([]byte) error
	}{
		{
			name: "Mary",
			decode: func(wire []byte) error {
				_, err := mary.NewMaryTransactionBodyFromCbor(wire)
				return err
			},
		},
		{
			name: "Alonzo",
			decode: func(wire []byte) error {
				_, err := alonzo.NewAlonzoTransactionBodyFromCbor(wire)
				return err
			},
		},
		{
			name: "Babbage",
			decode: func(wire []byte) error {
				_, err := babbage.NewBabbageTransactionBodyFromCbor(wire)
				return err
			},
		},
		{
			name: "Conway",
			decode: func(wire []byte) error {
				_, err := conway.NewConwayTransactionBodyFromCbor(wire)
				return err
			},
		},
		{
			name: "Dijkstra",
			decode: func(wire []byte) error {
				_, err := dijkstra.NewDijkstraTransactionBodyFromCbor(wire)
				return err
			},
		},
		{
			name: "Dijkstra subtransaction",
			decode: func(wire []byte) error {
				var body dijkstra.DijkstraSubTransactionBody
				_, err := cbor.Decode(wire, &body)
				return err
			},
		},
	}
	minInt64 := big.NewInt(math.MinInt64)
	maxInt64 := big.NewInt(math.MaxInt64)
	quantities := []struct {
		name     string
		quantity *big.Int
		wantErr  bool
	}{
		{
			name:     "below int64",
			quantity: new(big.Int).Sub(minInt64, big.NewInt(1)),
			wantErr:  true,
		},
		{name: "minimum int64", quantity: minInt64},
		{name: "negative one", quantity: big.NewInt(-1)},
		{name: "zero pruned", quantity: new(big.Int)},
		{name: "one", quantity: big.NewInt(1)},
		{name: "maximum int64", quantity: maxInt64},
		{
			name:     "above int64",
			quantity: new(big.Int).Add(maxInt64, big.NewInt(1)),
			wantErr:  true,
		},
	}
	for _, decoder := range decoders {
		for _, quantity := range quantities {
			t.Run(decoder.name+"/"+quantity.name, func(t *testing.T) {
				err := decoder.decode(encodeMintBody(t, quantity.quantity))
				if quantity.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}
