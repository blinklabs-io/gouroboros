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
	"encoding/json"
	"io"
	"math/big"
	"os"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

type DijkstraGenesis struct {
	conway.ConwayGenesis
	MaxRefScriptSizePerBlock         uint32             `json:"maxRefScriptSizePerBlock"`
	MaxRefScriptSizePerTx            uint32             `json:"maxRefScriptSizePerTx"`
	RefScriptCostStride              uint32             `json:"refScriptCostStride"`
	RefScriptCostMultiplier          *common.GenesisRat `json:"refScriptCostMultiplier"`
	MaxPledgeLeverage                *common.GenesisRat `json:"maxPledgeLeverage"`
	MinPoolMargin                    *common.GenesisRat `json:"minPoolMargin"`
	LeiosAnnouncementPeriodLength    uint32             `json:"leiosAnnouncementPeriodLength"`
	LeiosVotePeriodLength            uint32             `json:"leiosVotePeriodLength"`
	LeiosDiffusionPeriodLength       uint32             `json:"leiosDiffusionPeriodLength"`
	LeiosCommitteeSize               uint16             `json:"leiosCommitteeSize"`
	LeiosQuorumStakeThreshold        *common.GenesisRat `json:"leiosQuorumStakeThreshold"`
	MaxEndorserBlockReferencesSize   uint32             `json:"maxEndorserBlockReferencesSize"`
	MaxEndorserBlockTxsSize          uint32             `json:"maxEndorserBlockTxsSize"`
	MaxEndorserBlockExUnits          common.ExUnits     `json:"maxEndorserBlockExecutionUnits"`
	MaxRefScriptSizePerEndorserBlock uint32             `json:"maxRefScriptSizePerEndorserBlock"`
	CommitteeStakeCoverage           *common.GenesisRat `json:"committeeStakeCoverage"`
	QuorumStakeThreshold             *common.GenesisRat `json:"quorumStakeThreshold"`
}

func NewDijkstraGenesisFromReader(r io.Reader) (DijkstraGenesis, error) {
	var ret DijkstraGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewDijkstraGenesisFromFile(path string) (DijkstraGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return DijkstraGenesis{}, err
	}
	defer f.Close()
	return NewDijkstraGenesisFromReader(f)
}

func (p *DijkstraProtocolParameters) UpdateFromGenesis(
	genesis *DijkstraGenesis,
) error {
	if genesis == nil {
		return nil
	}
	committeeStakeCoverage := genesisRatToRat(genesis.CommitteeStakeCoverage)
	quorumStakeThreshold := genesisRatToRat(genesis.QuorumStakeThreshold)
	if err := validateLeiosCommitteeStakeParameters(
		committeeStakeCoverage,
		quorumStakeThreshold,
	); err != nil {
		return err
	}
	if err := p.ConwayProtocolParameters.UpdateFromGenesis(
		&genesis.ConwayGenesis,
	); err != nil {
		return err
	}
	p.MaxRefScriptSizePerBlock = genesis.MaxRefScriptSizePerBlock
	p.MaxRefScriptSizePerTx = genesis.MaxRefScriptSizePerTx
	p.RefScriptCostStride = genesis.RefScriptCostStride
	p.RefScriptCostMultiplier = genesisRatToRat(genesis.RefScriptCostMultiplier)
	p.MaxPledgeLeverage = genesisRatToRat(genesis.MaxPledgeLeverage)
	p.MinPoolMargin = genesisRatToRat(genesis.MinPoolMargin)
	p.LeiosAnnouncementPeriodLength = genesis.LeiosAnnouncementPeriodLength
	p.LeiosVotePeriodLength = genesis.LeiosVotePeriodLength
	p.LeiosDiffusionPeriodLength = genesis.LeiosDiffusionPeriodLength
	p.LeiosCommitteeSize = genesis.LeiosCommitteeSize
	p.LeiosQuorumStakeThreshold = genesisRatToRat(genesis.LeiosQuorumStakeThreshold)
	p.MaxEndorserBlockReferencesSize = genesis.MaxEndorserBlockReferencesSize
	p.MaxEndorserBlockTxsSize = genesis.MaxEndorserBlockTxsSize
	p.MaxEndorserBlockExUnits = genesis.MaxEndorserBlockExUnits
	p.MaxRefScriptSizePerEndorserBlock = genesis.MaxRefScriptSizePerEndorserBlock
	applyConwayRefScriptFeeDefaults(p)
	p.CommitteeStakeCoverage = committeeStakeCoverage
	p.QuorumStakeThreshold = quorumStakeThreshold
	return nil
}

func applyConwayRefScriptFeeDefaults(p *DijkstraProtocolParameters) {
	if p.RefScriptCostStride == 0 {
		p.RefScriptCostStride = uint32(conway.RefScriptCostStride)
	}
	if p.RefScriptCostMultiplier == nil {
		p.RefScriptCostMultiplier = &cbor.Rat{Rat: big.NewRat(6, 5)}
	}
}

func genesisRatToRat(r *common.GenesisRat) *cbor.Rat {
	if r == nil || r.Rat == nil {
		return nil
	}
	return &cbor.Rat{Rat: new(big.Rat).Set(r.Rat)}
}
