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
	CostModels           map[string]CostModel         `json:"costModels"`
}

type CostModel map[string]int

// NormalizeCostModels converts all cost model keys to consistent paramX format
func (g *AlonzoGenesis) NormalizeCostModels() error {
	if g.CostModels == nil {
		return nil
	}

	for version, model := range g.CostModels {
		normalized := make(CostModel)
		for k, v := range model {
			// Check if key is already in paramX format
			var index int
			if _, err := fmt.Sscanf(k, "param%d", &index); err == nil {
				normalized[k] = v // Keep existing paramX keys
				continue
			}

			// Check if key is a numeric index (from array format)
			if _, err := fmt.Sscanf(k, "%d", &index); err == nil {
				normalized[fmt.Sprintf("param%d", index)] = v
				continue
			}
			normalized[k] = v
		}
		g.CostModels[version] = normalized
	}
	return nil
}

func (c *CostModel) UnmarshalJSON(data []byte) error {
	tmpMap := make(map[string]interface{})
	if err := json.Unmarshal(data, &tmpMap); err != nil {
		// Try to unmarshal as array first
		var tmpArray []interface{}
		if arrayErr := json.Unmarshal(data, &tmpArray); arrayErr == nil {
			*c = make(CostModel)
			for i, v := range tmpArray {
				num, err := toInt(v)
				if err != nil {
					return fmt.Errorf("array index %d: %w", i, err)
				}
				(*c)[fmt.Sprintf("%d", i)] = num
			}
			return nil
		}
		return err
	}

	*c = make(CostModel)
	for k, v := range tmpMap {
		num, err := toInt(v)
		if err != nil {
			return fmt.Errorf("key %s: %w", k, err)
		}
		(*c)[k] = num
	}
	return nil
}

func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case json.Number:
		intVal, err := val.Int64()
		return int(intVal), err
	case int64:
		return int(val), nil
	case uint64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("unsupported numeric type: %T", v)
	}
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
