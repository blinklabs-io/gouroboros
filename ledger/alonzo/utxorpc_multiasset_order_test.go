// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package alonzo_test

import (
	"bytes"
	"math/big"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestOutputUtxorpcMultiAssetCanonicalOrder(t *testing.T) {
	t.Parallel()
	data := map[common.Blake2b224]map[cbor.ByteString]*big.Int{}
	for i := 0; i < 16; i++ {
		var policy common.Blake2b224
		policy[0] = byte((i * 37) % 251)
		names := map[cbor.ByteString]*big.Int{}
		for j := 0; j < 8; j++ {
			names[cbor.NewByteString(
				[]byte{byte((j * 53) % 241), byte(j)},
			)] = big.NewInt(int64(i + j + 1))
		}
		data[policy] = names
	}
	ma := common.NewMultiAsset(data)
	output := alonzo.AlonzoTransactionOutput{
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: 1000,
			Assets: &ma,
		},
	}
	for i := 0; i < 50; i++ {
		got, err := output.Utxorpc()
		require.NoError(t, err)
		require.Len(t, got.Assets, 16)
		require.True(
			t,
			slices.IsSortedFunc(
				got.Assets,
				func(a, b *utxorpc.Multiasset) int {
					return bytes.Compare(a.PolicyId, b.PolicyId)
				},
			),
			"policies not sorted on call %d",
			i,
		)
		for _, p := range got.Assets {
			require.True(
				t,
				slices.IsSortedFunc(
					p.Assets,
					func(a, b *utxorpc.Asset) int {
						return bytes.Compare(a.Name, b.Name)
					},
				),
				"assets not sorted on call %d",
				i,
			)
		}
	}
}
