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

package conway_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/conway"
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

// zeroTestMintBody is { inputs: [], outputs: [], fee: 0, mint: multiasset }.
func zeroTestMintBody(t *testing.T, multiasset string) []byte {
	t.Helper()
	return zeroTestHex(t, "a4"+"0080"+"0180"+"0200"+"09"+multiasset)
}

// zeroTestOutputBody carries the multiasset in an output value instead.
func zeroTestOutputBody(t *testing.T, multiasset string) []byte {
	t.Helper()
	return zeroTestHex(
		t,
		"a3"+"0080"+"01"+"81"+zeroTestOutput(multiasset)+"0200",
	)
}

// zeroTestCollateralReturnBody carries it in the collateral return (key 16).
func zeroTestCollateralReturnBody(t *testing.T, multiasset string) []byte {
	t.Helper()
	return zeroTestHex(
		t,
		"a4"+"0080"+"0180"+"0200"+"10"+zeroTestOutput(multiasset),
	)
}

func zeroTestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// TestConwayRejectsZeroAndEmptyAssets covers cardano-ledger's Conway-era
// MultiAsset decoder, decodeMap decCBOR (decodeNonEmptyMap decodeNonZeroAmount)
// from protocol version 9: a zero asset quantity fails with "MultiAsset cannot
// contain zeros" and a policy with no assets fails with "Empty Assets are not
// allowed". Both reach every MultiAsset position in the body — mint, output
// value, and collateral return value. An empty outer map stays legal until
// Dijkstra, so it is a control here.
func TestConwayRejectsZeroAndEmptyAssets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     func(*testing.T, string) []byte
		asset    string
		wantErr  string
		accepted bool
	}{
		{
			name:    "mint zero quantity",
			body:    zeroTestMintBody,
			asset:   maZeroQuantity,
			wantErr: "multiasset cannot contain zeros",
		},
		{
			name:    "mint empty assets",
			body:    zeroTestMintBody,
			asset:   maEmptyAssets,
			wantErr: "empty assets are not allowed",
		},
		{
			name:    "output value zero quantity",
			body:    zeroTestOutputBody,
			asset:   maZeroQuantity,
			wantErr: "multiasset cannot contain zeros",
		},
		{
			name:    "output value empty assets",
			body:    zeroTestOutputBody,
			asset:   maEmptyAssets,
			wantErr: "empty assets are not allowed",
		},
		{
			name:    "collateral return zero quantity",
			body:    zeroTestCollateralReturnBody,
			asset:   maZeroQuantity,
			wantErr: "multiasset cannot contain zeros",
		},
		{
			name:    "collateral return empty assets",
			body:    zeroTestCollateralReturnBody,
			asset:   maEmptyAssets,
			wantErr: "empty assets are not allowed",
		},
		{
			// decodeMultiAsset's outer map only becomes non-empty-checked at
			// Dijkstra, and an output value has no field-level emptiness
			// guard, so [coin, {}] decodes here. The mint field is not an
			// equivalent case: Conway's body decoder rejects an explicitly
			// empty key 9 through a separate guard ("TxBody: 'Mint' must be
			// non-empty when supplied"), which is tracked by issue #2396.
			name:     "output value empty outer map accepted until Dijkstra",
			body:     zeroTestOutputBody,
			asset:    maEmptyOuter,
			accepted: true,
		},
		{
			name:     "mint nonzero quantity accepted",
			body:     zeroTestMintBody,
			asset:    maValid,
			accepted: true,
		},
		{
			name:     "output value nonzero quantity accepted",
			body:     zeroTestOutputBody,
			asset:    maValid,
			accepted: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var body conway.ConwayTransactionBody
			err := body.UnmarshalCBOR(test.body(t, test.asset))
			if test.accepted {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
