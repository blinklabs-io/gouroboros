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

package dijkstra

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func TestDijkstraProtocolParametersRewardParameters(t *testing.T) {
	protocolParams := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			A0:          &cbor.Rat{Rat: big.NewRat(3, 10)},
			MinPoolCost: 340_000_000,
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 12,
				Minor: 1,
			},
		},
		MaxPledgeLeverage: &cbor.Rat{Rat: big.NewRat(100, 1)},
		MinPoolMargin:     &cbor.Rat{Rat: big.NewRat(3, 100)},
	}

	got := protocolParams.RewardParams()
	require.Equal(t, protocolParams.ProtocolVersion, got.ProtocolVersion)
	require.Equal(t, protocolParams.MinPoolCost, got.MinPoolCost)
	require.Equal(t, big.NewRat(3, 10), got.PoolInfluence)
	require.Equal(t, big.NewRat(100, 1), got.MaxPledgeLeverage)
	require.Equal(t, big.NewRat(3, 100), got.MinPoolMargin)

	got.MaxPledgeLeverage.SetInt64(5)
	require.Equal(t, big.NewRat(100, 1), protocolParams.MaxPledgeLeverage.Rat)
}
