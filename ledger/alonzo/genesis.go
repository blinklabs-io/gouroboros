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
	"slices"
	"strconv"

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
	// ExtraConfig carries the optional prototype genesis injection. Its cost
	// models are not part of the Alonzo translation context: the reference
	// node applies them once, over the initial protocol parameters of the era
	// a chain starts in. See InjectExtraCostModels.
	ExtraConfig *AlonzoGenesisExtraConfig `json:"extraConfig,omitempty"`
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
		tmpList, err := decodeGenesisCostModel(langVer, data)
		if err != nil {
			return err
		}
		tmpCostModels[langKey] = tmpList
	}
	*c = AlonzoGenesisCostModels(tmpCostModels)
	return nil
}

// decodeGenesisCostModel decodes one language's cost model, given either as a
// list of parameters, kept verbatim, or as a map of parameter names.
func decodeGenesisCostModel(
	langVer lang.LanguageVersion,
	data []byte,
) ([]int64, error) {
	// Try to decode as list first
	var tmpList []int64
	if err := json.Unmarshal(data, &tmpList); err == nil {
		return tmpList, nil
	}
	// Decode as map
	tmpMap := make(map[string]int64)
	if err := json.Unmarshal(data, &tmpMap); err != nil {
		return nil, fmt.Errorf("decode cost model: %w", err)
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
	return tmpList, nil
}

// AlonzoGenesisExtraConfig is the extraConfig block of an Alonzo genesis, as
// used by the Leios prototype testnets to start with full-length cost models
// without an on-chain parameter update.
type AlonzoGenesisExtraConfig struct {
	// CostModels maps a cost model language key (PlutusV1Key to PlutusV4Key,
	// or the language number of an "Unknown" entry) to its parameters, kept
	// as supplied. It is nil when extraConfig has no costModels.
	CostModels map[uint][]int64
}

// extraConfigCostModelLanguages are the language names the reference parser
// reads from extraConfig.costModels.
var extraConfigCostModelLanguages = []struct {
	name    string
	key     uint
	version lang.LanguageVersion
}{
	{"PlutusV1", PlutusV1Key, lang.LanguageVersionV1},
	{"PlutusV2", PlutusV2Key, lang.LanguageVersionV2},
	{"PlutusV3", PlutusV3Key, lang.LanguageVersionV3},
	{"PlutusV4", PlutusV4Key, lang.LanguageVersionV4},
}

const extraConfigUnknownCostModels = "Unknown"

// UnmarshalJSON follows the reference AlonzoExtraConfig parser: it reads only
// "costModels", and parses it leniently, so a model may have any number of
// parameters, every Plutus language is accepted, models for languages the
// node does not know go under "Unknown" keyed by language number, and other
// keys are ignored. An "Unknown" entry for a known language number replaces
// the named entry, as in the reference parser.
func (c *AlonzoGenesisExtraConfig) UnmarshalJSON(data []byte) error {
	var tmp struct {
		CostModels json.RawMessage `json:"costModels"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return fmt.Errorf("decode extraConfig: %w", err)
	}
	*c = AlonzoGenesisExtraConfig{}
	if isJSONNull(tmp.CostModels) {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(tmp.CostModels, &raw); err != nil {
		return fmt.Errorf("decode extraConfig cost models: %w", err)
	}
	costModels := make(map[uint][]int64)
	for _, language := range extraConfigCostModelLanguages {
		modelData, ok := raw[language.name]
		if !ok || isJSONNull(modelData) {
			continue
		}
		model, err := decodeGenesisCostModel(language.version, modelData)
		if err != nil {
			return fmt.Errorf(
				"decode extraConfig %s cost model: %w",
				language.name,
				err,
			)
		}
		costModels[language.key] = model
	}
	if unknownData, ok := raw[extraConfigUnknownCostModels]; ok &&
		!isJSONNull(unknownData) {
		var unknown map[string][]int64
		if err := json.Unmarshal(unknownData, &unknown); err != nil {
			return fmt.Errorf("decode extraConfig Unknown cost models: %w", err)
		}
		for langStr, model := range unknown {
			langNum, err := strconv.ParseUint(langStr, 10, 8)
			if err != nil {
				return fmt.Errorf(
					"decode extraConfig Unknown cost model language %q: %w",
					langStr,
					err,
				)
			}
			costModels[uint(langNum)] = model
		}
	}
	c.CostModels = costModels
	return nil
}

// MarshalJSON writes the cost models back in the genesis form, with known
// languages by name and the others under "Unknown".
func (c AlonzoGenesisExtraConfig) MarshalJSON() ([]byte, error) {
	if c.CostModels == nil {
		return []byte("{}"), nil
	}
	costModels := make(map[string]any, len(c.CostModels))
	unknown := make(map[string][]int64)
	for key, model := range c.CostModels {
		known := false
		for _, language := range extraConfigCostModelLanguages {
			if language.key == key {
				costModels[language.name] = model
				known = true
				break
			}
		}
		if !known {
			unknown[strconv.FormatUint(uint64(key), 10)] = model
		}
	}
	if len(unknown) > 0 {
		costModels[extraConfigUnknownCostModels] = unknown
	}
	return json.Marshal(map[string]any{"costModels": costModels})
}

// InjectExtraCostModels returns a copy of costModels with the extraConfig
// cost models applied over it, language by language: a language present in
// extraConfig replaces the existing model and every other language is kept.
// The result shares no slices with costModels or the genesis.
//
// The reference node does this once, over the protocol parameters of the
// initial ledger state when a chain starts in the Alonzo era or later, after
// the era translations (and so after the Conway and Dijkstra genesis cost
// models) have been applied. It is not part of the Alonzo era translation, so
// UpdateFromGenesis does not apply it.
func (g *AlonzoGenesis) InjectExtraCostModels(
	costModels map[uint][]int64,
) map[uint][]int64 {
	var ret map[uint][]int64
	if costModels != nil {
		ret = make(map[uint][]int64, len(costModels))
		for key, model := range costModels {
			ret[key] = slices.Clone(model)
		}
	}
	if g == nil || g.ExtraConfig == nil || len(g.ExtraConfig.CostModels) == 0 {
		return ret
	}
	if ret == nil {
		ret = make(map[uint][]int64, len(g.ExtraConfig.CostModels))
	}
	for key, model := range g.ExtraConfig.CostModels {
		ret[key] = slices.Clone(model)
	}
	return ret
}

func isJSONNull(data json.RawMessage) bool {
	return len(bytes.TrimSpace(data)) == 0 ||
		bytes.Equal(bytes.TrimSpace(data), []byte("null"))
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
