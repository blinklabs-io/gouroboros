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

package dijkstra_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// Wire fragments for the three MultiAsset encodings cardano-ledger's
// decodeMultiAsset (eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs) treats
// differently by decoder version. Hand-written because an encoder cannot be
// made to emit an empty inner map once decode-time pruning exists.
const (
	zeroPolicyHex = "11111111111111111111111111111111111111111111111111111111"
	// { policy: { "A": 0 } }
	maZeroQuantity = "a1581c" + zeroPolicyHex + "a1414100"
	// { policy: {} }
	maEmptyAssets = "a1581c" + zeroPolicyHex + "a0"
	// {}
	maEmptyOuter = "a0"
	// { policy: { "A": 1 } }
	maValid = "a1581c" + zeroPolicyHex + "a1414101"
	// Enterprise address reused from the existing Conway output tests.
	zeroTestAddrHex = "40000000000000000000000000000000000000000000000000000000008198bd431b03"
)

// zeroTestOutput is a Babbage-form output whose value is [1000000, multiasset].
func zeroTestOutput(multiasset string) string {
	return "a2" + "00" + "5823" + zeroTestAddrHex +
		"01" + "82" + "1a000f4240" + multiasset
}

func zeroTestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// TestDijkstraRejectsZeroAndEmptyMultiAsset covers cardano-ledger's Dijkstra
// MultiAsset decoder, decodeNonEmptyMap (decodeNonEmptyMap decodeNonZeroAmount)
// from protocol version 12. It adds the empty outer map to the Conway-era zero
// quantity and empty per-policy map rejections, and applies to every
// MultiAsset position: mint, output value, collateral return, and the same
// two positions inside a sub-transaction body.
func TestDijkstraRejectsZeroAndEmptyMultiAsset(t *testing.T) {
	t.Parallel()
	// { inputs: [], outputs: [], fee: 0, mint: multiasset }
	mintBody := func(multiasset string) string {
		return "a4" + "0080" + "0180" + "0200" + "09" + multiasset
	}
	// { inputs: [], outputs: [ output ], fee: 0 }
	outputBody := func(multiasset string) string {
		return "a3" + "0080" + "01" + "81" + zeroTestOutput(multiasset) + "0200"
	}
	// { inputs: [], outputs: [], fee: 0, collateral return: output }
	collateralReturnBody := func(multiasset string) string {
		return "a4" + "0080" + "0180" + "0200" + "10" + zeroTestOutput(multiasset)
	}
	// Sub-transaction bodies carry no fee field.
	subMintBody := func(multiasset string) string {
		return "a3" + "0080" + "0180" + "09" + multiasset
	}
	subOutputBody := func(multiasset string) string {
		return "a2" + "0080" + "01" + "81" + zeroTestOutput(multiasset)
	}

	assets := []struct {
		name    string
		hex     string
		wantErr string
	}{
		{
			name:    "zero quantity",
			hex:     maZeroQuantity,
			wantErr: "multiasset cannot contain zeros",
		},
		{
			name:    "empty assets",
			hex:     maEmptyAssets,
			wantErr: "empty assets are not allowed",
		},
		{
			name:    "empty multiasset map",
			hex:     maEmptyOuter,
			wantErr: "empty multiasset map is not allowed",
		},
	}
	positions := []struct {
		name string
		body func(string) string
		sub  bool
	}{
		{name: "mint", body: mintBody},
		{name: "output value", body: outputBody},
		{name: "collateral return", body: collateralReturnBody},
		{name: "sub-transaction mint", body: subMintBody, sub: true},
		{name: "sub-transaction output value", body: subOutputBody, sub: true},
	}
	decode := func(t *testing.T, sub bool, cborData []byte) error {
		t.Helper()
		if sub {
			var body dijkstra.DijkstraSubTransactionBody
			return body.UnmarshalCBOR(cborData)
		}
		var body dijkstra.DijkstraTransactionBody
		return body.UnmarshalCBOR(cborData)
	}

	for _, position := range positions {
		for _, asset := range assets {
			t.Run(position.name+" "+asset.name, func(t *testing.T) {
				t.Parallel()
				err := decode(
					t,
					position.sub,
					zeroTestHex(t, position.body(asset.hex)),
				)
				require.ErrorContains(t, err, asset.wantErr)
			})
		}
		t.Run(position.name+" nonzero quantity accepted", func(t *testing.T) {
			t.Parallel()
			require.NoError(
				t,
				decode(t, position.sub, zeroTestHex(t, position.body(maValid))),
			)
		})
	}
}
