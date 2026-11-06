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

	// ExtraConfig is the optional extraConfig object of the genesis file. It
	// is nil when the file has none. A consumer building protocol parameters
	// must let ExtraConfig.CostModels override the cost models derived from
	// the genesis (including the models of later eras) once the era
	// transition has been applied. AlonzoProtocolParameters.UpdateFromGenesis
	// does this for the Alonzo parameters; a consumer that derives later-era
	// models afterwards re-applies the overlay with ApplyExtraConfigCostModels.
	ExtraConfig *AlonzoGenesisExtraConfig `json:"extraConfig,omitempty"`
}

// AlonzoGenesisExtraConfig mirrors AlonzoExtraConfig in the ledger. Its cost
// models replace, per language, the cost models translated from the genesis
// files, so a test network can carry full-length PlutusV1, PlutusV2 and
// PlutusV3 models without an on-chain parameter update. Each model is a list
// of values or a map keyed by parameter name.
type AlonzoGenesisExtraConfig struct {
	CostModels AlonzoGenesisCostModels `json:"costModels"`
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

// NewAlonzoGenesisFromReader decodes an Alonzo genesis JSON document. It
// accepts what the reference FromJSON AlonzoGenesis accepts: the required
// keys must be present and in range, and unknown keys are ignored.
func NewAlonzoGenesisFromReader(r io.Reader) (AlonzoGenesis, error) {
	var ret AlonzoGenesis
	var data json.RawMessage
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return ret, err
	}
	normalized, err := validateAlonzoGenesisJSON(data)
	if err != nil {
		return ret, fmt.Errorf("invalid Alonzo genesis: %w", err)
	}
	if err := json.Unmarshal(normalized, &ret); err != nil {
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

// decodeAlias decodes the first of names that is present, not null and
// decodes as T, matching the reference's (o .: a <|> o .: b). A spelling that
// is not selected is never decoded, so an invalid value under it cannot fail
// the document. It returns nil when no name is present.
func decodeAlias[T any](
	obj map[string]json.RawMessage,
	names ...string,
) (*T, error) {
	var lastErr error
	for _, name := range names {
		raw, ok := obj[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			lastErr = fmt.Errorf("%s: %w", name, err)
			continue
		}
		return &v, nil
	}
	return nil, lastErr
}

func (u *AlonzoGenesisExUnits) UnmarshalJSON(data []byte) error {
	// We need some custom unmarshal logic to handle alternate key names
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	// The reference reads "memory" <|> "exUnitsMem": a present primary key
	// wins even when it is zero.
	mem, err := decodeAlias[uint64](obj, "memory", "exUnitsMem")
	if err != nil {
		return err
	}
	steps, err := decodeAlias[uint64](obj, "steps", "exUnitsSteps")
	if err != nil {
		return err
	}
	if mem != nil {
		u.Mem = *mem
	}
	if steps != nil {
		u.Steps = *steps
	}
	return nil
}

type AlonzoGenesisExecutionPrices struct {
	Steps *common.GenesisRat `json:"prSteps"`
	Mem   *common.GenesisRat `json:"prMem"`
}

func (p *AlonzoGenesisExecutionPrices) UnmarshalJSON(data []byte) error {
	// We need some custom unmarshal logic to handle alternate key names
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	steps, err := decodeAlias[common.GenesisRat](obj, "priceSteps", "prSteps")
	if err != nil {
		return err
	}
	mem, err := decodeAlias[common.GenesisRat](obj, "priceMemory", "prMem")
	if err != nil {
		return err
	}
	if steps != nil {
		p.Steps = steps
	}
	if mem != nil {
		p.Mem = mem
	}
	return nil
}
