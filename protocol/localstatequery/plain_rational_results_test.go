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

package localstatequery

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// Stake fractions and reward-provenance ratios are plain rationals, not
// bounded ones: before protocol version 9 they are written as an untagged
// [numerator, denominator], from 9 under tag 30. Both forms must decode.
var plainRationalEncodings = map[string]string{
	"untagged":            "820102",
	"untagged indefinite": "9f0102ff",
	"tag 30":              "d81e820102",
}

func TestStakeDistributionResultsAcceptPlainRationals(t *testing.T) {
	poolID := ledger.PoolId(ledger.NewBlake2b224(mustDecodeHex(t, poolIdLowHex)))
	for name, fraction := range plainRationalEncodings {
		reply := mustDecodeHex(
			t,
			"81a1581c"+poolIdLowHex+"82"+fraction+"5820"+vrfHashHex,
		)
		t.Run("stake distribution/"+name, func(t *testing.T) {
			var result StakeDistributionResult
			_, err := cbor.Decode(reply, &result)
			require.NoError(t, err)
			require.Equal(
				t,
				0,
				result.Results[poolID].StakeFraction.Cmp(big.NewRat(1, 2)),
			)
		})
		t.Run("pool distribution/"+name, func(t *testing.T) {
			var result PoolDistrResult
			_, err := cbor.Decode(reply, &result)
			require.NoError(t, err)
			require.Equal(
				t,
				0,
				result.Results[poolID].StakeFraction.Cmp(big.NewRat(1, 2)),
			)
		})
	}
	// A tag other than 30, or a second tag, is still refused.
	for _, fraction := range []string{"d81f820102", "d81ed81e820102"} {
		reply := mustDecodeHex(
			t,
			"81a1581c"+poolIdLowHex+"82"+fraction+"5820"+vrfHashHex,
		)
		var result StakeDistributionResult
		_, err := cbor.Decode(reply, &result)
		require.Error(t, err, fraction)
	}
}

func TestRewardProvenanceResultAcceptsPlainRationals(t *testing.T) {
	for name, ratio := range plainRationalEncodings {
		t.Run(name, func(t *testing.T) {
			raw := cbor.RawMessage(mustDecodeHex(t, ratio))
			inner := []any{
				uint64(432000), map[ledger.Blake2b224]uint64{},
				uint64(1), uint64(2), uint64(3), uint64(4), uint64(5),
				uint64(6), raw, uint64(7), raw, uint64(8), uint64(9),
				uint64(10), cbor.RawMessage{0xa0}, cbor.RawMessage{0xa0},
			}
			data, err := cbor.Encode([]any{inner})
			require.NoError(t, err)
			var result RewardProvenanceResult
			_, err = cbor.Decode(data, &result)
			require.NoError(t, err)
			require.Equal(t, 0, result.Decentralization.Cmp(big.NewRat(1, 2)))
			require.Equal(t, 0, result.Eta.Cmp(big.NewRat(1, 2)))
			require.Equal(t, uint64(7), result.ExpectedBlocks)
		})
	}
}
