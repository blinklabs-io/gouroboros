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

package common

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type ProtocolParameterUpdate interface {
	IsProtocolParameterUpdate()
	Cbor() []byte
}

// ProtocolParameterUpdateVersionValidator validates fields whose reference
// domain depends on the active protocol version.
type ProtocolParameterUpdateVersionValidator interface {
	ValidateProtocolParameterUpdateVersion(ProtocolParametersProtocolVersion) error
}

// ProtocolParameterUpdateCostModelProvider reports cost models requiring
// protocol-version-dependent validation.
type ProtocolParameterUpdateCostModelProvider interface {
	ProtocolParameterUpdateCostModels() map[uint][]int64
}

type ProtocolParameterVersionUpdateProvider interface {
	ProtocolParameterVersionUpdate() *ProtocolParametersProtocolVersion
}

type ProtocolParametersProtocolVersion struct {
	cbor.StructAsArray
	Major uint
	Minor uint
}

type ProtocolParametersProtocolVersionProvider interface {
	ProtocolParametersProtocolVersion() ProtocolParametersProtocolVersion
}

type ProtocolParameters interface {
	Utxorpc() (*cardano.PParams, error)
}

// ValidateCostModelLanguageIDs enforces the Word8 wire domain used by
// cardano-ledger while retaining unknown language IDs for forward
// compatibility.
func ValidateCostModelLanguageIDs(models map[uint][]int64) error {
	for languageID := range models {
		if languageID > 255 {
			return fmt.Errorf(
				"cost-model language ID %d exceeds Word8 maximum 255",
				languageID,
			)
		}
	}
	return nil
}

// ValidateNonNegativeBoundedRatCBOR verifies the raw tag-30 integer pair used
// by reference NonNegativeInterval and UnitInterval values. It checks the
// unreduced CBOR numerator and denominator before cbor.Rat normalizes them.
func ValidateNonNegativeBoundedRatCBOR(raw []byte) error {
	var tagged cbor.RawTag
	if _, err := cbor.Decode(raw, &tagged); err != nil {
		return err
	}
	if tagged.Number != cbor.CborTagRational {
		return fmt.Errorf("expected CBOR rational tag, got %d", tagged.Number)
	}
	var components []cbor.RawMessage
	if _, err := cbor.Decode(tagged.Content, &components); err != nil {
		return err
	}
	if len(components) != 2 {
		return errors.New("expected rational numerator and denominator")
	}
	numerator, err := rawCBORInteger(components[0])
	if err != nil {
		return fmt.Errorf("decode rational numerator: %w", err)
	}
	denominator, err := rawCBORInteger(components[1])
	if err != nil {
		return fmt.Errorf("decode rational denominator: %w", err)
	}
	if !numerator.IsUint64() {
		return errors.New("rational numerator must be in Word64")
	}
	if !denominator.IsUint64() || denominator.Sign() == 0 {
		return errors.New("rational denominator must be positive Word64")
	}
	return nil
}

// ValidateNonNegativeBoundedRatArrayCBOR validates every raw tag-30 rational
// in a struct-as-array CBOR value.
func ValidateNonNegativeBoundedRatArrayCBOR(raw []byte) error {
	if len(raw) == 0 || raw[0]>>5 != 4 {
		return errors.New("expected CBOR array")
	}
	var values []cbor.RawMessage
	if _, err := cbor.Decode(raw, &values); err != nil {
		return err
	}
	for index, value := range values {
		if err := ValidateNonNegativeBoundedRatCBOR(value); err != nil {
			return fmt.Errorf("rational at array index %d: %w", index, err)
		}
	}
	return nil
}

func rawCBORInteger(raw []byte) (*big.Int, error) {
	var value any
	if _, err := cbor.Decode(raw, &value); err != nil {
		return nil, err
	}
	result := new(big.Int)
	switch integer := value.(type) {
	case int64:
		result.SetInt64(integer)
	case uint64:
		result.SetUint64(integer)
	case big.Int:
		result.Set(&integer)
	default:
		return nil, fmt.Errorf("expected integer, got %T", value)
	}
	return result, nil
}

// PoolRuleProtocolParameters is the protocol-parameter view required by the
// Shelley POOL rules. Every protocol parameter type in this repository from
// Shelley onwards implements it, so it is an internal accessor rather than a
// capability a consumer has to supply. Byron has no POOL rule.
//
// Reference: poolTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs reads
// ppProtocolVersionL, ppMinPoolCostL and ppEMaxL.
type PoolRuleProtocolParameters interface {
	// ProtocolMajorVersion returns the active major protocol version, used
	// to gate the era-dependent POOL predicates.
	ProtocolMajorVersion() uint
	// MinPoolCostValue returns the minPoolCost protocol parameter.
	MinPoolCostValue() uint64
	// PoolRetirementMaxEpoch returns the eMax protocol parameter, the
	// number of epochs after the current one within which a scheduled pool
	// retirement must fall.
	PoolRetirementMaxEpoch() uint64
}

// CommitteeMaxTermLengthProvider is the optional protocol-parameter
// capability exposing the constitutional committee maximum term length, used
// by the Conway committee-term ratification predicate. The boolean is false
// when the parameter is unavailable; zero is a present, valid limit.
type CommitteeMaxTermLengthProvider interface {
	CommitteeMaxTermLength() (uint64, bool)
}

type ExUnitPrice struct {
	cbor.StructAsArray
	MemPrice  *cbor.Rat
	StepPrice *cbor.Rat
}

// ConvertToUtxorpcCardanoCostModels converts a map of cost models for Plutus
// scripts into cardano.CostModels.
//
// NOTE: the map keys follow the real cardano-ledger wire convention for the
// protocol-parameter cost-models map (`Map language pv -> CostModel`), which
// is 0-indexed: PlutusV1=0, PlutusV2=1, PlutusV3=2, PlutusV4=3. This matches
// the same 0-indexed keys used throughout this repo's own genesis loading
// (e.g. ledger/alonzo/pparams.go's PlutusV1Key/PlutusV2Key/PlutusV3Key
// constants) and script-execution cost-model lookups (e.g.
// ledger/conway/rules.go, ledger/dijkstra/rules.go), and is verified against
// a real on-chain transaction fixture in
// ledger/babbage/script_data_hash_used_languages_test.go. Do not shift this
// back to 1-indexed keys.
func ConvertToUtxorpcCardanoCostModels(
	models map[uint][]int64,
) *cardano.CostModels {
	costModels := &cardano.CostModels{}
	for k, v := range models {
		costModel := &cardano.CostModel{Values: v}
		switch k {
		case 0:
			costModels.PlutusV1 = costModel
		case 1:
			costModels.PlutusV2 = costModel
		case 2:
			costModels.PlutusV3 = costModel
		case 3:
			costModels.PlutusV4 = costModel
		default:
			slog.Warn("unsupported cost model version", "version", k)
		}
	}
	return costModels
}

// CostModelsToPlutusData converts ledger cost-model updates to a PlutusData map.
func CostModelsToPlutusData(models map[uint][]int64) data.PlutusData {
	if err := ValidateCostModelLanguageIDs(models); err != nil {
		panic(err)
	}
	keys := slices.Collect(maps.Keys(models))
	slices.Sort(keys)
	pairs := make([][2]data.PlutusData, 0, len(keys))
	for _, key := range keys {
		values := models[key]
		valueItems := make([]data.PlutusData, len(values))
		for i, value := range values {
			valueItems[i] = data.NewInteger(big.NewInt(value))
		}
		pairs = append(
			pairs,
			[2]data.PlutusData{
				data.NewInteger(new(big.Int).SetUint64(uint64(key))),
				data.NewList(valueItems...),
			},
		)
	}
	return data.NewMap(pairs)
}
