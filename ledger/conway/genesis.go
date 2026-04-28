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

package conway

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

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
	Delegs                     ConwayGenesisDelegs               `json:"delegs"`
	InitialDReps               ConwayGenesisInitialDReps         `json:"initialDReps"`
}

type ConwayGenesisPoolVotingThresholds struct {
	CommitteeNormal       *common.GenesisRat `json:"committeeNormal"`
	CommitteeNoConfidence *common.GenesisRat `json:"committeeNoConfidence"`
	HardForkInitiation    *common.GenesisRat `json:"hardForkInitiation"`
	MotionNoConfidence    *common.GenesisRat `json:"motionNoConfidence"`
	PpSecurityGroup       *common.GenesisRat `json:"ppSecurityGroup"`
}

type ConwayGenesisDRepVotingThresholds struct {
	MotionNoConfidence    *common.GenesisRat `json:"motionNoConfidence"`
	CommitteeNormal       *common.GenesisRat `json:"committeeNormal"`
	CommitteeNoConfidence *common.GenesisRat `json:"committeeNoConfidence"`
	UpdateToConstitution  *common.GenesisRat `json:"updateToConstitution"`
	HardForkInitiation    *common.GenesisRat `json:"hardForkInitiation"`
	PpNetworkGroup        *common.GenesisRat `json:"ppNetworkGroup"`
	PpEconomicGroup       *common.GenesisRat `json:"ppEconomicGroup"`
	PpTechnicalGroup      *common.GenesisRat `json:"ppTechnicalGroup"`
	PpGovGroup            *common.GenesisRat `json:"ppGovGroup"`
	TreasuryWithdrawal    *common.GenesisRat `json:"treasuryWithdrawal"`
}

type ConwayGenesisConstitution struct {
	Anchor ConwayGenesisConstitutionAnchor `json:"anchor"`
	Script string                          `json:"script"`
}

type ConwayGenesisConstitutionAnchor struct {
	DataHash string `json:"dataHash"`
	Url      string `json:"url"`
}

type ConwayGenesisCommittee struct {
	Members   map[string]int     `json:"members"`
	Threshold *common.GenesisRat `json:"threshold"`
}

type ConwayGenesisDelegs map[*common.Credential]ConwayGenesisDelegatee

func (d *ConwayGenesisDelegs) UnmarshalJSON(data []byte) error {
	var tmpData map[string]ConwayGenesisDelegatee
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return fmt.Errorf("decode Conway genesis delegs: %w", err)
	}
	tmpDeleg := make(map[*common.Credential]ConwayGenesisDelegatee, len(tmpData))
	for k, v := range tmpData {
		var tmpCred common.Credential
		// Wrap the key in quotes so that it can be processed as raw JSON
		tmpKeyData := slices.Concat([]byte(`"`), []byte(k), []byte(`"`))
		if err := json.Unmarshal(tmpKeyData, &tmpCred); err != nil {
			return fmt.Errorf("decode Conway genesis deleg credential: %w", err)
		}
		tmpDeleg[&tmpCred] = v
	}
	*d = ConwayGenesisDelegs(tmpDeleg)
	return nil
}

const (
	ConwayGenesisDelegateeTypeStake     = 0
	ConwayGenesisDelegateeTypeVote      = 1
	ConwayGenesisDelegateeTypeStakeVote = 2
)

type ConwayGenesisDelegatee struct {
	Type   int
	PoolId common.PoolId
	DRep   common.Drep
}

func (d *ConwayGenesisDelegatee) UnmarshalJSON(data []byte) error {
	tmpData := struct {
		PoolId *string      `json:"poolId"`
		DRep   *common.Drep `json:"dRep"`
	}{}
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return fmt.Errorf("decode delegatee: %w", err)
	}
	var hasPoolId bool
	var hasDRep bool
	if tmpData.PoolId != nil {
		poolId, err := hex.DecodeString(*tmpData.PoolId)
		if err != nil {
			return fmt.Errorf("decode pool ID: %w", err)
		}
		if len(poolId) != 28 {
			return errors.New("invalid pool ID length")
		}
		d.PoolId = common.PoolId(poolId)
		hasPoolId = true
	}
	if tmpData.DRep != nil {
		d.DRep = *tmpData.DRep
		hasDRep = true
	}
	if hasPoolId && hasDRep {
		d.Type = ConwayGenesisDelegateeTypeStakeVote
	} else if hasPoolId {
		d.Type = ConwayGenesisDelegateeTypeStake
	} else if hasDRep {
		d.Type = ConwayGenesisDelegateeTypeVote
	} else {
		return errors.New("unknown delegatee type")
	}
	return nil
}

type ConwayGenesisInitialDReps map[*common.Credential]ConwayGenesisDRepState

func (i *ConwayGenesisInitialDReps) UnmarshalJSON(data []byte) error {
	var tmpData map[string]ConwayGenesisDRepState
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return fmt.Errorf("decode Conway genesis initial dreps: %w", err)
	}
	tmpDreps := make(map[*common.Credential]ConwayGenesisDRepState, len(tmpData))
	for k, v := range tmpData {
		var tmpCred common.Credential
		// Wrap the key in quotes so that it can be processed as raw JSON
		tmpKeyData := slices.Concat([]byte(`"`), []byte(k), []byte(`"`))
		if err := json.Unmarshal(tmpKeyData, &tmpCred); err != nil {
			return fmt.Errorf("decode Conway genesis initial drep credential: %w", err)
		}
		tmpDreps[&tmpCred] = v
	}
	*i = ConwayGenesisInitialDReps(tmpDreps)
	return nil
}

type ConwayGenesisDRepState struct {
	Expiry  uint64            `json:"expiry"`
	Deposit uint64            `json:"deposit"`
	Anchor  *common.GovAnchor `json:"anchor"`
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
