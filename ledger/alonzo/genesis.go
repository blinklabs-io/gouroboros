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

package alonzo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/lang"
)

type AlonzoGenesis struct {
	LovelacePerUtxoWord  uint64                       `json:"lovelacePerUTxOWord"`
	MaxValueSize         uint                         `json:"maxValueSize"`
	CollateralPercentage uint                         `json:"collateralPercentage"`
	MaxCollateralInputs  uint                         `json:"maxCollateralInputs"`
	ExecutionPrices      AlonzoGenesisExecutionPrices `json:"executionPrices"`
	MaxTxExUnits         AlonzoGenesisExUnits         `json:"maxTxExUnits"`
	MaxBlockExUnits      AlonzoGenesisExUnits         `json:"maxBlockExUnits"`
	CostModels           AlonzoGenesisCostModels      `json:"costModels"`
}

type AlonzoGenesisCostModels map[string][]int64

func (c *AlonzoGenesisCostModels) UnmarshalJSON(data []byte) error {
	tmpCostModels := make(map[string][]int64)
	// Decode top-level first
	var tmpData map[string]json.RawMessage
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return err
	}
	var langVer lang.LanguageVersion
	for langKey, data := range tmpData {
		switch langKey {
		case "PlutusV1":
			langVer = lang.LanguageVersionV1
		case "PlutusV2":
			langVer = lang.LanguageVersionV2
		case "PlutusV3":
			langVer = lang.LanguageVersionV3
		default:
			return errors.New("unknown language version key: " + langKey)
		}
		// Try to decode as list first
		var tmpList []int64
		if err := json.Unmarshal(data, &tmpList); err == nil {
			tmpCostModels[langKey] = tmpList
			continue
		}
		// Decode as map
		tmpMap := make(map[string]int64)
		if err := json.Unmarshal(data, &tmpMap); err != nil {
			return fmt.Errorf("decode cost model: %w", err)
		}
		paramNames := lang.GetParamNamesForVersion(langVer)
		for _, param := range paramNames {
			val, ok := tmpMap[param]
			// Stop processing if a param name is not present
			if !ok {
				break
			}
			tmpList = append(tmpList, val)
		}
		tmpCostModels[langKey] = tmpList
	}
	*c = AlonzoGenesisCostModels(tmpCostModels)
	return nil
}

func NewAlonzoGenesisFromReader(r io.Reader) (AlonzoGenesis, error) {
	var ret AlonzoGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewAlonzoGenesisFromFile(path string) (AlonzoGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return AlonzoGenesis{}, err
	}
	defer f.Close()
	return NewAlonzoGenesisFromReader(f)
}

type AlonzoGenesisExUnits struct {
	Mem   uint `json:"exUnitsMem"`
	Steps uint `json:"exUnitsSteps"`
}

func (u *AlonzoGenesisExUnits) UnmarshalJSON(data []byte) error {
	// We need some custom unmarshal logic to handle alternate key names
	tmpData := struct {
		ExUnitsMem   uint `json:"exUnitsMem"`
		ExUnitsSteps uint `json:"exUnitsSteps"`
		Memory       uint `json:"memory"`
		Steps        uint `json:"steps"`
	}{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmpData); err != nil {
		return err
	}
	if tmpData.ExUnitsMem > 0 {
		u.Mem = tmpData.ExUnitsMem
	}
	if tmpData.ExUnitsSteps > 0 {
		u.Steps = tmpData.ExUnitsSteps
	}
	if tmpData.Memory > 0 {
		u.Mem = tmpData.Memory
	}
	if tmpData.Steps > 0 {
		u.Steps = tmpData.Steps
	}
	return nil
}

type AlonzoGenesisExecutionPrices struct {
	Steps *common.GenesisRat `json:"prSteps"`
	Mem   *common.GenesisRat `json:"prMem"`
}

func (p *AlonzoGenesisExecutionPrices) UnmarshalJSON(data []byte) error {
	// We need some custom unmarshal logic to handle alternate key names
	tmpData := struct {
		PrSteps     *common.GenesisRat `json:"prSteps"`
		PrMem       *common.GenesisRat `json:"prMem"`
		PriceSteps  *common.GenesisRat `json:"priceSteps"`
		PriceMemory *common.GenesisRat `json:"priceMemory"`
	}{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmpData); err != nil {
		return err
	}
	if tmpData.PrSteps != nil {
		p.Steps = tmpData.PrSteps
	}
	if tmpData.PrMem != nil {
		p.Mem = tmpData.PrMem
	}
	if tmpData.PriceSteps != nil {
		p.Steps = tmpData.PriceSteps
	}
	if tmpData.PriceMemory != nil {
		p.Mem = tmpData.PriceMemory
	}
	return nil
}
