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
package common

import (
	"bytes"
	"encoding/json"
	"math/big"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func multiAssetOrderFixture() MultiAsset[*big.Int] {
	data := map[Blake2b224]map[cbor.ByteString]*big.Int{}
	for i := 0; i < 16; i++ {
		var policy Blake2b224
		policy[0] = byte((i * 37) % 251)
		policy[27] = byte(i)
		names := map[cbor.ByteString]*big.Int{}
		for j := 0; j < 8; j++ {
			name := []byte{byte((j * 53) % 241), byte(j)}
			names[cbor.NewByteString(name)] = big.NewInt(int64(i + j + 1))
		}
		// Shorter name that sorts before its extensions byte-wise.
		names[cbor.NewByteString([]byte{0x00})] = big.NewInt(1)
		data[policy] = names
	}
	return NewMultiAsset(data)
}

func TestMultiAssetPoliciesAndAssetsCanonicalOrder(t *testing.T) {
	t.Parallel()
	ma := multiAssetOrderFixture()
	wantPolicies := ma.Policies()
	slices.SortFunc(wantPolicies, func(a, b Blake2b224) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})
	for i := 0; i < 50; i++ {
		got := ma.Policies()
		require.Equal(t, wantPolicies, got, "call %d", i)
		for _, policy := range got {
			names := ma.Assets(policy)
			require.True(
				t,
				slices.IsSortedFunc(names, bytes.Compare),
				"assets of policy %x not sorted on call %d",
				policy.Bytes(),
				i,
			)
		}
	}
}

func TestMultiAssetMarshalJSONCanonicalOrder(t *testing.T) {
	t.Parallel()
	ma := multiAssetOrderFixture()
	first, err := json.Marshal(&ma)
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		again, err := json.Marshal(&ma)
		require.NoError(t, err)
		require.Equal(t, string(first), string(again), "call %d", i)
	}
}
