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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pool entry inside the distribution map is not addressable, so it must
// encode through a value receiver to be written as the ledger's tuple.
func TestIndividualPoolStakeEncodesAsTupleInMap(t *testing.T) {
	for name, blsKey := range map[string]cbor.RawMessage{
		"without key": nil,
		"with key":    cbor.RawMessage{0x41, 0x01},
	} {
		t.Run(name, func(t *testing.T) {
			result := StakeDistribution2Result{
				Distribution: PoolDistribution{
					Pools: map[ledger.PoolId]IndividualPoolStake{
						{0x01}: {
							StakeFraction:  &cbor.Rat{Rat: big.NewRat(1, 2)},
							TotalPoolStake: 7,
							BlsKey:         blsKey,
						},
					},
					TotalActiveStake: 9,
				},
			}
			wire, err := cbor.Encode(result)
			require.NoError(t, err)

			var decoded StakeDistribution2Result
			_, err = cbor.Decode(wire, &decoded)
			require.NoError(t, err)
			require.Len(t, decoded.Distribution.Pools, 1)
			pool := decoded.Distribution.Pools[ledger.PoolId{0x01}]
			assert.Zero(t, pool.StakeFraction.Cmp(big.NewRat(1, 2)))
			assert.Equal(t, uint64(7), pool.TotalPoolStake)
			assert.Equal(t, len(blsKey), len(pool.BlsKey))

			// The entry is a 3 or 4 item array, never a map of field names
			var generic []any
			_, err = cbor.Decode(wire, &generic)
			require.NoError(t, err)
			require.Len(t, generic, 1)
			distribution, ok := generic[0].([]any)
			require.True(t, ok, "distribution is %T", generic[0])
			require.Len(t, distribution, 2)
			pools, ok := distribution[0].(map[any]any)
			require.True(t, ok, "pools is %T", distribution[0])
			for _, entry := range pools {
				items, isArray := entry.([]any)
				require.True(t, isArray, "entry is %T", entry)
				wantItems := 3
				if blsKey != nil {
					wantItems = 4
				}
				assert.Len(t, items, wantItems)
			}
		})
	}
}
