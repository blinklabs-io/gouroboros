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

type ProtocolParametersProtocolVersion struct {
	cbor.StructAsArray
	Major uint
	Minor uint
}

type ProtocolParameters interface {
	Utxorpc() (*cardano.PParams, error)
}

type ExUnitPrice struct {
	cbor.StructAsArray
	MemPrice  *cbor.Rat
	StepPrice *cbor.Rat
}

// ConvertToUtxorpcCardanoCostModels converts a map of cost models for Plutus scripts into cardano.CostModels
// Only PlutusV(keys 1, 2, and 3) are supported.
func ConvertToUtxorpcCardanoCostModels(
	models map[uint][]int64,
) *cardano.CostModels {
	costModels := &cardano.CostModels{}
	for k, v := range models {
		costModel := &cardano.CostModel{Values: v}
		switch k {
		case 1:
			costModels.PlutusV1 = costModel
		case 2:
			costModels.PlutusV2 = costModel
		case 3:
			costModels.PlutusV3 = costModel
		default:
			slog.Warn("unsupported cost model version", "version", k)
		}
	}
	return costModels
}

// CostModelsToPlutusData converts ledger cost-model updates to a PlutusData map.
func CostModelsToPlutusData(models map[uint][]int64) data.PlutusData {
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
