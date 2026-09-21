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

package babbage_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/stretchr/testify/require"
)

const (
	zeroPolicyHex = "11111111111111111111111111111111111111111111111111111111"
	// { policy: { "A": 0 } }
	maZeroQuantity = "a1581c" + zeroPolicyHex + "a1414100"
	// { policy: {} }
	maEmptyAssets = "a1581c" + zeroPolicyHex + "a0"
	// {}
	maEmptyOuter    = "a0"
	zeroTestAddrHex = "40000000000000000000000000000000000000000000000000000000008198bd431b03"
)

// TestBabbageAcceptsAndPrunesZeroAssets pins pre-Conway decoding: at decoder
// versions below 9 cardano-ledger uses pruneZeroMultiAsset instead of failing
// (eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs), so a zero quantity, an
// empty per-policy asset map, and an empty outer map all decode and prune
// away. gouroboros #1961 established that dropping the pruned entry is what
// keeps script execution budgets matching the producing node, so the Conway
// and Dijkstra rejections must not reach back into this era.
func TestBabbageAcceptsAndPrunesZeroAssets(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		asset string
	}{
		{name: "zero quantity", asset: maZeroQuantity},
		{name: "empty assets", asset: maEmptyAssets},
		{name: "empty multiasset map", asset: maEmptyOuter},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// { inputs: [], outputs: [ output ], fee: 0, mint: multiasset }
			output := "a2" + "00" + "5823" + zeroTestAddrHex +
				"01" + "82" + "1a000f4240" + test.asset
			cborData, err := hex.DecodeString(
				"a4" + "0080" + "01" + "81" + output + "0200" + "09" + test.asset,
			)
			require.NoError(t, err)

			var body babbage.BabbageTransactionBody
			require.NoError(t, body.UnmarshalCBOR(cborData))
			require.NotNil(t, body.TxMint)
			require.Empty(
				t,
				body.TxMint.Policies(),
				"pre-Conway mint must prune to no policies",
			)
			require.Len(t, body.TxOutputs, 1)
			require.Empty(
				t,
				body.TxOutputs[0].Assets().Policies(),
				"pre-Conway output value must prune to no policies",
			)
		})
	}
}
