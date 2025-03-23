// Copyright 2024 Blink Labs Software
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

package conway

import (
	"encoding/json"
	"io"
	"os"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ConwayGenesis struct {
	PoolVotingThresholds       ConwayGenesisPoolVotingThresholds `json:"poolVotingThresholds"`
	DRepVotingThresholds       ConwayGenesisDRepVotingThresholds `json:"dRepVotingThresholds"`
	MinCommitteeSize           uint                              `json:"committeeMinSize"`
	CommitteeTermLimit         uint64                            `json:"committeeMaxTermLength"`
	GovActionValidityPeriod    uint64                            `json:"govActionLifetime"`
	GovActionDeposit           uint64                            `json:"govActionDeposit"`
	DRepDeposit                uint64                            `json:"dRepDeposit"`
	DRepInactivityPeriod       uint64                            `json:"dRepActivity"`
	MinFeeRefScriptCostPerByte *common.GenesisRat                `json:"minFeeRefScriptCostPerByte"`
	PlutusV3CostModel          []int64                           `json:"plutusV3CostModel"`
	Constitution               ConwayGenesisConstitution         `json:"constitution"`
	Committee                  ConwayGenesisCommittee            `json:"committee"`
}

type ConwayGenesisPoolVotingThresholds struct {
	CommitteeNormal       *common.GenesisRat
	CommitteeNoConfidence *common.GenesisRat
	HardForkInitiation    *common.GenesisRat
	MotionNoConfidence    *common.GenesisRat
	PpSecurityGroup       *common.GenesisRat
}

type ConwayGenesisDRepVotingThresholds struct {
	MotionNoConfidence    *common.GenesisRat
	CommitteeNormal       *common.GenesisRat
	CommitteeNoConfidence *common.GenesisRat
	UpdateToConstitution  *common.GenesisRat
	HardForkInitiation    *common.GenesisRat
	PpNetworkGroup        *common.GenesisRat
	PpEconomicGroup       *common.GenesisRat
	PpTechnicalGroup      *common.GenesisRat
	PpGovGroup            *common.GenesisRat
	TreasuryWithdrawal    *common.GenesisRat
}

type ConwayGenesisConstitution struct {
	Anchor ConwayGenesisConstitutionAnchor
	Script string
}

type ConwayGenesisConstitutionAnchor struct {
	DataHash string
	Url      string
}

type ConwayGenesisCommittee struct {
	Members   map[string]int
	Threshold map[string]int
}

func NewConwayGenesisFromReader(r io.Reader) (ConwayGenesis, error) {
	var ret ConwayGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewConwayGenesisFromFile(path string) (ConwayGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return ConwayGenesis{}, err
	}
	defer f.Close()
	return NewConwayGenesisFromReader(f)
}
