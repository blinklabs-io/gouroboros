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

type ConwayGenesis struct {
	PoolVotingThresholds       ConwayGenesisPoolVotingThresholds
	DRepVotingThresholds       ConwayGenesisDRepVotingThresholds
	MinCommitteeSize           uint   `json:"committeeMinSize"`
	CommitteeTermLimit         uint64 `json:"committeeMaxTermLength"`
	GovActionValidityPeriod    uint64 `json:"govActionLifetime"`
	GovActionDeposit           uint64
	DRepDeposit                uint64 `json:"dRepDeposit"`
	DRepInactivityPeriod       uint64 `json:"dRepActivity"`
	MinFeeRefScriptCostPerByte float32
	PlutusV3CostModel          []int `json:"plutusV3CostModel"`
	Constitution               ConwayGenesisConstitution
	Committee                  ConwayGenesisCommittee
}

type ConwayGenesisPoolVotingThresholds struct {
	CommitteeNormal       float32
	CommitteeNoConfidence float32
	HardForkInitiation    float32
	MotionNoConfidence    float32
	PpSecurityGroup       float32
}

type ConwayGenesisDRepVotingThresholds struct {
	MotionNoConfidence    float32
	CommitteeNormal       float32
	CommitteeNoConfidence float32
	UpdateToConstitution  float32
	HardForkInitiation    float32
	PpNetworkGroup        float32
	PpEconomicGroup       float32
	PpTechnicalGroup      float32
	PpGovGroup            float32
	TreasuryWithdrawal    float32
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
