// Copyright 2025 Blink Labs Software
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

package leios

import (
	"encoding/json"
	"io"
	"os"
)

type LeiosGenesis struct {
	HeaderDiffusionPeriod           uint  `json:"headerDiffusionPeriod"`
	VotingPeriod                    uint  `json:"votingPeriod"`
	DiffusionPeriod                 uint  `json:"diffusionPeriod"`
	RankingBlockMaxSize             uint  `json:"rankingBlockMaxSize"`
	EndorserBlockReferenceTxMaxSize uint  `json:"endorserBlockRefTxMaxSize"`
	EndorserBlockMaxSize            uint  `json:"endorserBlockMaxSize"`
	MeanCommitteeSize               uint  `json:"meanCommitteeSize"`
	QuorumSize                      uint  `json:"quorumSize"`
	MaxExStepPerEndorserBlock       int64 `json:"maxExStepPerEndorserBlock"`
	MaxExMemPerEndorserBlock        int64 `json:"maxExMemPerEndorserBlock"`
	MaxExStepPerTransaction         int64 `json:"maxExStepPerTransaction"`
	MaxExMemPerTransaction          int64 `json:"maxExMemPerTransaction"`
}

func NewLeiosGenesisFromReader(r io.Reader) (LeiosGenesis, error) {
	var ret LeiosGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewLeiosGenesisFromFile(path string) (LeiosGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return LeiosGenesis{}, err
	}
	defer f.Close()
	return NewLeiosGenesisFromReader(f)
}
