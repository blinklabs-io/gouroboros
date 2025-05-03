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
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
)

type AlonzoGenesis struct {
	LovelacePerUtxoWord  uint64                       `json:"lovelacePerUTxOWord"`
	MaxValueSize         uint                         `json:"maxValueSize"`
	CollateralPercentage uint                         `json:"collateralPercentage"`
	MaxCollateralInputs  uint                         `json:"maxCollateralInputs"`
	ExecutionPrices      AlonzoGenesisExecutionPrices `json:"executionPrices"`
	MaxTxExUnits         AlonzoGenesisExUnits         `json:"maxTxExUnits"`
	MaxBlockExUnits      AlonzoGenesisExUnits         `json:"maxBlockExUnits"`
	CostModels           map[string]interface{}       `json:"costModels"`
}

func NewAlonzoGenesisFromReader(r io.Reader) (AlonzoGenesis, error) {
	var ret AlonzoGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	if err := ret.NormalizeCostModels(); err != nil {
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

type AlonzoGenesisExecutionPrices struct {
	Steps *AlonzoGenesisExecutionPricesRat `json:"prSteps"`
	Mem   *AlonzoGenesisExecutionPricesRat `json:"prMem"`
}

type AlonzoGenesisExecutionPricesRat struct {
	*big.Rat
}

func (r *AlonzoGenesisExecutionPricesRat) UnmarshalJSON(data []byte) error {
	var tmpData struct {
		Numerator   int64 `json:"numerator"`
		Denominator int64 `json:"denominator"`
	}
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return err
	}
	r.Rat = big.NewRat(tmpData.Numerator, tmpData.Denominator)
	return nil
}

func (a *AlonzoGenesis) NormalizeCostModels() error {
	if a.CostModels == nil {
		return nil
	}

	normalized := make(map[string]map[string]int)
	for version, model := range a.CostModels {
		if modelMap, ok := model.(map[string]interface{}); ok {
			versionMap := make(map[string]int)
			for k, v := range modelMap {
				switch val := v.(type) {
				case float64:
					versionMap[k] = int(val)
				case int:
					versionMap[k] = val
				case json.Number:
					intVal, err := val.Int64()
					if err != nil {
						floatVal, err := val.Float64()
						if err != nil {
							return fmt.Errorf("invalid number in cost model: %v", val)
						}
						intVal = int64(floatVal)
					}
					versionMap[k] = int(intVal)
				default:
					return fmt.Errorf("invalid cost model value type: %T", v)
				}
			}
			normalized[version] = versionMap
		}
	}
	a.CostModels = make(map[string]interface{})
	for k, v := range normalized {
		a.CostModels[k] = v
	}
	return nil
}
