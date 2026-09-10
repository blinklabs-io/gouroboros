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
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// w36GenesisPath holds the Dijkstra genesis published for the
// prototype-2026w36 Leios devnet, copied verbatim from
// https://book.play.dev.cardano.org/environments-pre/leios/dijkstra-genesis.json
// on 2026-09-10.
const w36GenesisPath = "testdata/dijkstra-genesis-prototype-2026w36.json"

// w36GenesisSha256 pins the fixture, so that a silent respin of the published
// genesis is caught here rather than at node startup.
const w36GenesisSha256 = "e3a0f8edd086e0c1fccc9fa04b58fa76b8f735ce1214f854072078f7c823e8be"

func TestDijkstraGenesisW36FixtureMatchesPublishedHash(t *testing.T) {
	data, err := os.ReadFile(w36GenesisPath)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	require.Equal(t, w36GenesisSha256, hex.EncodeToString(sum[:]))
}

// TestDijkstraGenesisDecodesPrototype2026w36 decodes the published
// prototype-2026w36 genesis in full. The decoder rejects unknown fields, so
// every parameter in the published file has to be known: any single missing
// one stops a node from starting against the w36 devnet.
func TestDijkstraGenesisDecodesPrototype2026w36(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromFile(w36GenesisPath)
	require.NoError(t, err)

	require.Equal(t, uint32(1000), genesis.LeiosAnnouncementPeriodLength)
	require.Equal(t, uint32(4000), genesis.LeiosVotePeriodLength)
	require.Equal(t, uint32(7000), genesis.LeiosDiffusionPeriodLength)
	require.Equal(t, uint16(900), genesis.LeiosCommitteeSize)
	require.NotNil(t, genesis.LeiosQuorumStakeThreshold)
	require.Equal(
		t,
		0,
		genesis.LeiosQuorumStakeThreshold.Cmp(big.NewRat(3, 4)),
	)

	require.Equal(t, uint32(100000), genesis.MaxEndorserBlockReferencesSize)
	require.Equal(t, uint32(1000000), genesis.MaxEndorserBlockTxsSize)
	require.Equal(
		t,
		common.ExUnits{Memory: 310000000, Steps: 100000000000},
		genesis.MaxEndorserBlockExUnits,
	)
	require.Equal(
		t,
		uint32(4000000),
		genesis.MaxRefScriptSizePerEndorserBlock,
	)

	// maxPledgeLeverage is explicitly null in the published w36 genesis,
	// meaning no leverage cap, and has to decode to a nil ratio rather than
	// erroring.
	require.Nil(t, genesis.MaxPledgeLeverage)
	require.NotNil(t, genesis.MinPoolMargin)
	require.Equal(t, 0, genesis.MinPoolMargin.Cmp(big.NewRat(3, 200)))

	require.Len(t, genesis.PlutusV4CostModel, 251)
	require.Equal(t, int64(100788), genesis.PlutusV4CostModel[0])
	require.Equal(t, int64(1), genesis.PlutusV4CostModel[250])

	// Parameters carried over unchanged from the earlier prototypes.
	require.Equal(t, uint32(1048576), genesis.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(204800), genesis.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(25600), genesis.RefScriptCostStride)
	require.NotNil(t, genesis.RefScriptCostMultiplier)
	require.Equal(
		t,
		0,
		genesis.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)),
	)
}

// TestDijkstraProtocolParametersUpdateFromW36Genesis asserts the published w36
// values reach the protocol parameters, including the PlutusV4 cost model,
// which lands under the zero-based Plutus language index the PlutusV4
// evaluation context is built from.
func TestDijkstraProtocolParametersUpdateFromW36Genesis(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromFile(w36GenesisPath)
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))

	require.Equal(t, uint32(1000), pparams.LeiosAnnouncementPeriodLength)
	require.Equal(t, uint32(4000), pparams.LeiosVotePeriodLength)
	require.Equal(t, uint32(7000), pparams.LeiosDiffusionPeriodLength)
	require.Equal(t, uint16(900), pparams.LeiosCommitteeSize)
	require.NotNil(t, pparams.LeiosQuorumStakeThreshold)
	require.Equal(
		t,
		0,
		pparams.LeiosQuorumStakeThreshold.Cmp(big.NewRat(3, 4)),
	)
	require.Equal(
		t,
		common.ExUnits{Memory: 310000000, Steps: 100000000000},
		pparams.MaxEndorserBlockExUnits,
	)
	require.Equal(t, uint32(100000), pparams.MaxEndorserBlockReferencesSize)
	require.Equal(t, uint32(1000000), pparams.MaxEndorserBlockTxsSize)
	require.Equal(
		t,
		uint32(4000000),
		pparams.MaxRefScriptSizePerEndorserBlock,
	)
	require.Nil(t, pparams.MaxPledgeLeverage)
	require.NotNil(t, pparams.MinPoolMargin)
	require.Equal(t, 0, pparams.MinPoolMargin.Cmp(big.NewRat(3, 200)))

	require.Contains(t, pparams.CostModels, uint(3))
	require.Len(t, pparams.CostModels[3], 251)
	require.Equal(t, int64(100788), pparams.CostModels[3][0])
}

// TestDijkstraGenesisWithoutPlutusV4CostModelLeavesCostModelsAlone asserts a
// genesis with no plutusV4CostModel does not create an empty cost model entry,
// which would otherwise be handed to the PlutusV4 evaluation context.
func TestDijkstraGenesisWithoutPlutusV4CostModelLeavesCostModelsAlone(
	t *testing.T,
) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "maxRefScriptSizePerBlock": 1048576
}`))
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))
	require.NotContains(t, pparams.CostModels, uint(3))
}
