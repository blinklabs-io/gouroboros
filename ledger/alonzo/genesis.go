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
		paramNames := costModelParamNamesForMap(langVer, tmpMap)
		for _, param := range paramNames {
			val, ok := costModelParamValue(tmpMap, param)
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

// costModelParamValue accepts historical map-form Alonzo genesis spellings
// while keeping Plutigo's current parameter names canonical. PlutusV1 genesis
// files used these names before later ledger APIs made the builtin names
// explicit.
func costModelParamValue(params map[string]int64, name string) (int64, bool) {
	if value, ok := params[name]; ok {
		return value, true
	}
	var legacyName string
	switch name {
	case "blake2b_256-cpu-arguments-intercept":
		legacyName = "blake2b-cpu-arguments-intercept"
	case "blake2b_256-cpu-arguments-slope":
		legacyName = "blake2b-cpu-arguments-slope"
	case "blake2b_256-memory-arguments":
		legacyName = "blake2b-memory-arguments"
	case "verifyEd25519Signature-cpu-arguments-intercept":
		legacyName = "verifySignature-cpu-arguments-intercept"
	case "verifyEd25519Signature-cpu-arguments-slope":
		legacyName = "verifySignature-cpu-arguments-slope"
	case "verifyEd25519Signature-memory-arguments":
		legacyName = "verifySignature-memory-arguments"
	default:
		return 0, false
	}
	value, ok := params[legacyName]
	return value, ok
}

// costModelParamNamesForMap preserves the ordering of the older Value cost
// models. The canonical model replaces five legacy parameters with nine, so
// those entries cannot be handled as one-for-one aliases. A map using the old
// valueData spelling is decoded in its original order and length; a shorter
// model that ends before this section still stops at its first absent key.
func costModelParamNamesForMap(
	version lang.LanguageVersion,
	params map[string]int64,
) []string {
	paramNames := lang.GetParamNamesForVersion(version)
	if _, ok := params["valueData-cpu-arguments"]; !ok {
		return paramNames
	}
	if _, ok := params["valueData-cpu-arguments-intercept"]; ok {
		return paramNames
	}
	const (
		canonicalStart = "valueData-cpu-arguments-intercept"
		canonicalEnd   = "unValueData-memory-arguments-slope"
	)
	start, end := -1, -1
	for i, name := range paramNames {
		if name == canonicalStart {
			start = i
		}
		if name == canonicalEnd {
			end = i
			break
		}
	}
	// Released Plutigo versions already expose the legacy table, so there is
	// nothing to rewrite when the canonical replacement range is absent.
	if start < 0 || end < start {
		return paramNames
	}
	legacyNames := []string{
		"valueData-cpu-arguments",
		"valueData-memory-arguments",
		"unValueData-cpu-arguments-intercept",
		"unValueData-cpu-arguments-slope",
		"unValueData-memory-arguments",
	}
	ret := make([]string, 0, len(paramNames)-(end-start+1)+len(legacyNames))
	ret = append(ret, paramNames[:start]...)
	ret = append(ret, legacyNames...)
	ret = append(ret, paramNames[end+1:]...)
	return ret
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
	Mem   uint64 `json:"exUnitsMem"`
	Steps uint64 `json:"exUnitsSteps"`
}

func (u *AlonzoGenesisExUnits) UnmarshalJSON(data []byte) error {
	// We need some custom unmarshal logic to handle alternate key names
	tmpData := struct {
		ExUnitsMem   uint64 `json:"exUnitsMem"`
		ExUnitsSteps uint64 `json:"exUnitsSteps"`
		Memory       uint64 `json:"memory"`
		Steps        uint64 `json:"steps"`
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
