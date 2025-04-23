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

package alonzo

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"

	//"github.com/blinklabs-io/gouroboros/ledger/mary"
	cardano "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type PlutusVersion string

const (
	PlutusV1 PlutusVersion = "PlutusV1"
	PlutusV2 PlutusVersion = "PlutusV2"
	PlutusV3 PlutusVersion = "PlutusV3"
)

type CostModel struct {
	Version    PlutusVersion
	Parameters map[string]int64
	Order      []string
}

type AlonzoProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA              uint
	MinFeeB              uint
	MaxBlockBodySize     uint
	MaxTxSize            uint
	MaxBlockHeaderSize   uint
	KeyDeposit           uint
	PoolDeposit          uint
	MaxEpoch             uint
	NOpt                 uint
	A0                   *cbor.Rat
	Rho                  *cbor.Rat
	Tau                  *cbor.Rat
	Decentralization     *cbor.Rat
	ExtraEntropy         common.Nonce
	ProtocolMajor        uint
	ProtocolMinor        uint
	MinUtxoValue         uint
	MinPoolCost          uint64
	AdaPerUtxoByte       uint64
	CostModels           map[PlutusVersion]*CostModel
	ExecutionCosts       common.ExUnitPrice
	MaxTxExUnits         common.ExUnits
	MaxBlockExUnits      common.ExUnits
	MaxValueSize         uint
	CollateralPercentage uint
	MaxCollateralInputs  uint
}

func (p *AlonzoProtocolParameters) Update(
	paramUpdate *AlonzoProtocolParameterUpdate,
) {
	if paramUpdate.MinFeeA != nil {
		p.MinFeeA = *paramUpdate.MinFeeA
	}
	if paramUpdate.MinFeeB != nil {
		p.MinFeeB = *paramUpdate.MinFeeB
	}
	if paramUpdate.MaxBlockBodySize != nil {
		p.MaxBlockBodySize = *paramUpdate.MaxBlockBodySize
	}
	if paramUpdate.MaxTxSize != nil {
		p.MaxTxSize = *paramUpdate.MaxTxSize
	}
	if paramUpdate.MaxBlockHeaderSize != nil {
		p.MaxBlockHeaderSize = *paramUpdate.MaxBlockHeaderSize
	}
	if paramUpdate.KeyDeposit != nil {
		p.KeyDeposit = *paramUpdate.KeyDeposit
	}
	if paramUpdate.PoolDeposit != nil {
		p.PoolDeposit = *paramUpdate.PoolDeposit
	}
	if paramUpdate.MaxEpoch != nil {
		p.MaxEpoch = *paramUpdate.MaxEpoch
	}
	if paramUpdate.NOpt != nil {
		p.NOpt = *paramUpdate.NOpt
	}
	if paramUpdate.A0 != nil {
		p.A0 = paramUpdate.A0
	}
	if paramUpdate.Rho != nil {
		p.Rho = paramUpdate.Rho
	}
	if paramUpdate.Tau != nil {
		p.Tau = paramUpdate.Tau
	}
	if paramUpdate.Decentralization != nil {
		p.Decentralization = paramUpdate.Decentralization
	}
	if paramUpdate.ProtocolVersion != nil {
		p.ProtocolMajor = paramUpdate.ProtocolVersion.Major
		p.ProtocolMinor = paramUpdate.ProtocolVersion.Minor
	}
	if paramUpdate.ExtraEntropy != nil {
		p.ExtraEntropy = *paramUpdate.ExtraEntropy
	}
	if paramUpdate.MinUtxoValue != nil {
		p.MinUtxoValue = *paramUpdate.MinUtxoValue
	}
	if paramUpdate.MinPoolCost != nil {
		p.MinPoolCost = *paramUpdate.MinPoolCost
	}
	if paramUpdate.AdaPerUtxoByte != nil {
		p.AdaPerUtxoByte = *paramUpdate.AdaPerUtxoByte
	}
	if paramUpdate.CostModels != nil {
		p.convertLegacyCostModels(paramUpdate.CostModels)
	}
	if paramUpdate.ExecutionCosts != nil {
		p.ExecutionCosts = *paramUpdate.ExecutionCosts
	}
	if paramUpdate.MaxTxExUnits != nil {
		p.MaxTxExUnits = *paramUpdate.MaxTxExUnits
	}
	if paramUpdate.MaxBlockExUnits != nil {
		p.MaxBlockExUnits = *paramUpdate.MaxBlockExUnits
	}
	if paramUpdate.MaxValueSize != nil {
		p.MaxValueSize = *paramUpdate.MaxValueSize
	}
	if paramUpdate.CollateralPercentage != nil {
		p.CollateralPercentage = *paramUpdate.CollateralPercentage
	}
	if paramUpdate.MaxCollateralInputs != nil {
		p.MaxCollateralInputs = *paramUpdate.MaxCollateralInputs
	}
}

func (p *AlonzoProtocolParameters) UpdateFromGenesis(genesis *AlonzoGenesis) error {
	if genesis == nil {
		return nil
	}

	// Common parameter updates
	p.AdaPerUtxoByte = genesis.LovelacePerUtxoWord / 8
	p.MaxValueSize = genesis.MaxValueSize
	p.CollateralPercentage = genesis.CollateralPercentage
	p.MaxCollateralInputs = genesis.MaxCollateralInputs
	p.MaxTxExUnits = common.ExUnits{
		Memory: uint64(genesis.MaxTxExUnits.Mem),
		Steps:  uint64(genesis.MaxTxExUnits.Steps),
	}
	p.MaxBlockExUnits = common.ExUnits{
		Memory: uint64(genesis.MaxBlockExUnits.Mem),
		Steps:  uint64(genesis.MaxBlockExUnits.Steps),
	}

	if genesis.ExecutionPrices.Mem != nil && genesis.ExecutionPrices.Steps != nil {
		p.ExecutionCosts = common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: genesis.ExecutionPrices.Mem.Rat},
			StepPrice: &cbor.Rat{Rat: genesis.ExecutionPrices.Steps.Rat},
		}
	}

	if genesis.CostModels != nil {
		p.CostModels = make(map[PlutusVersion]*CostModel)

		for lang, model := range genesis.CostModels {
			version, ok := toPlutusVersion(lang)
			if !ok {
				continue
			}

			params := make(map[string]int64)
			order := make([]string, 0, len(model))

			// Since model is now map[string]int, we don't need type assertions
			for name, val := range model {
				params[name] = int64(val) // Convert int to int64
				order = append(order, name)
			}

			// Sort keys alphabetically (maintains consistency with original behavior)
			sort.Strings(order)

			p.CostModels[version] = &CostModel{
				Version:    version,
				Parameters: params,
				Order:      order,
			}
		}
	}
	return nil
}

func (p *AlonzoProtocolParameters) convertLegacyCostModels(legacyModels map[uint][]int64) {
	p.CostModels = make(map[PlutusVersion]*CostModel)

	for langKey, values := range legacyModels {
		var version PlutusVersion
		switch langKey {
		case 0:
			version = PlutusV1
		case 1:
			version = PlutusV2
		case 2:
			version = PlutusV3
		default:
			continue
		}

		params := make(map[string]int64)
		order := make([]string, len(values))
		for i, val := range values {
			name := fmt.Sprintf("param%d", i)
			params[name] = val
			order[i] = name
		}

		p.CostModels[version] = &CostModel{
			Version:    version,
			Parameters: params,
			Order:      order,
		}
	}
}

func toPlutusVersion(key string) (PlutusVersion, bool) {
	switch strings.ToLower(key) {
	case "plutus:v1", "plutusv1":
		return PlutusV1, true
	case "plutus:v2", "plutusv2":
		return PlutusV2, true
	case "plutus:v3", "plutusv3":
		return PlutusV3, true
	default:
		return "", false
	}
}

func (p *AlonzoProtocolParameters) ToLegacyCostModels() map[uint][]int64 {
	legacyModels := make(map[uint][]int64)

	for version, model := range p.CostModels {
		var langKey uint
		switch version {
		case PlutusV1:
			langKey = 0
		case PlutusV2:
			langKey = 1
		case PlutusV3:
			langKey = 2
		default:
			continue
		}

		// Convert ordered parameters back to list format
		values := make([]int64, len(model.Order))
		for i, name := range model.Order {
			values[i] = model.Parameters[name]
		}
		legacyModels[langKey] = values
	}

	return legacyModels
}

type AlonzoProtocolParameterUpdate struct {
	cbor.DecodeStoreCbor
	MinFeeA              *uint                                     `cbor:"0,keyasint"`
	MinFeeB              *uint                                     `cbor:"1,keyasint"`
	MaxBlockBodySize     *uint                                     `cbor:"2,keyasint"`
	MaxTxSize            *uint                                     `cbor:"3,keyasint"`
	MaxBlockHeaderSize   *uint                                     `cbor:"4,keyasint"`
	KeyDeposit           *uint                                     `cbor:"5,keyasint"`
	PoolDeposit          *uint                                     `cbor:"6,keyasint"`
	MaxEpoch             *uint                                     `cbor:"7,keyasint"`
	NOpt                 *uint                                     `cbor:"8,keyasint"`
	A0                   *cbor.Rat                                 `cbor:"9,keyasint"`
	Rho                  *cbor.Rat                                 `cbor:"10,keyasint"`
	Tau                  *cbor.Rat                                 `cbor:"11,keyasint"`
	Decentralization     *cbor.Rat                                 `cbor:"12,keyasint"`
	ExtraEntropy         *common.Nonce                             `cbor:"13,keyasint"`
	ProtocolVersion      *common.ProtocolParametersProtocolVersion `cbor:"14,keyasint"`
	MinUtxoValue         *uint                                     `cbor:"15,keyasint"`
	MinPoolCost          *uint64                                   `cbor:"16,keyasint"`
	AdaPerUtxoByte       *uint64                                   `cbor:"17,keyasint"`
	CostModels           map[uint][]int64                          `cbor:"18,keyasint"`
	ExecutionCosts       *common.ExUnitPrice                       `cbor:"19,keyasint"`
	MaxTxExUnits         *common.ExUnits                           `cbor:"20,keyasint"`
	MaxBlockExUnits      *common.ExUnits                           `cbor:"21,keyasint"`
	MaxValueSize         *uint                                     `cbor:"22,keyasint"`
	CollateralPercentage *uint                                     `cbor:"23,keyasint"`
	MaxCollateralInputs  *uint                                     `cbor:"24,keyasint"`
}

func (AlonzoProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *AlonzoProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tAlonzoProtocolParameterUpdate AlonzoProtocolParameterUpdate
	var tmp tAlonzoProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = AlonzoProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (p *AlonzoProtocolParameters) Utxorpc() *cardano.PParams {
	if p == nil {
		return nil
	}

	// Helper function to safely convert rational numbers with bounds checking
	safeRatConvert := func(rat *cbor.Rat) *cardano.RationalNumber {
		if rat == nil || rat.Rat == nil {
			return nil
		}
		num := rat.Num().Int64()
		denom := rat.Denom().Int64()

		// Check bounds for int32 numerator and uint32 denominator
		if num < math.MinInt32 || num > math.MaxInt32 {
			return nil
		}
		if denom <= 0 || denom > math.MaxUint32 {
			return nil
		}

		return &cardano.RationalNumber{
			Numerator:   int32(num),
			Denominator: uint32(denom),
		}
	}

	// Convert all rational numbers with safety checks
	a0 := safeRatConvert(p.A0)
	rho := safeRatConvert(p.Rho)
	tau := safeRatConvert(p.Tau)
	memPrice := safeRatConvert(p.ExecutionCosts.MemPrice)
	stepPrice := safeRatConvert(p.ExecutionCosts.StepPrice)

	// Return nil if any conversion failed
	if a0 == nil || rho == nil || tau == nil || memPrice == nil || stepPrice == nil {
		return nil
	}

	// Convert cost models with proper version handling
	costModels := &cardano.CostModels{}
	if p.CostModels != nil {
		// Initialize all Plutus versions to empty models first
		costModels.PlutusV1 = &cardano.CostModel{Values: []int64{}}
		costModels.PlutusV2 = &cardano.CostModel{Values: []int64{}}
		costModels.PlutusV3 = &cardano.CostModel{Values: []int64{}}

		// Convert each version that exists in our parameters
		for version, model := range p.CostModels {
			var values []int64
			switch version {
			case PlutusV1:
				values = make([]int64, len(model.Order))
				for i, name := range model.Order {
					values[i] = model.Parameters[name]
				}
				costModels.PlutusV1.Values = values
			case PlutusV2:
				values = make([]int64, len(model.Order))
				for i, name := range model.Order {
					values[i] = model.Parameters[name]
				}
				costModels.PlutusV2.Values = values
			case PlutusV3:
				values = make([]int64, len(model.Order))
				for i, name := range model.Order {
					values[i] = model.Parameters[name]
				}
				costModels.PlutusV3.Values = values
			}
		}
	}

	return &cardano.PParams{
		CoinsPerUtxoByte:         p.AdaPerUtxoByte,
		MaxTxSize:                uint64(p.MaxTxSize),
		MinFeeCoefficient:        uint64(p.MinFeeA),
		MinFeeConstant:           uint64(p.MinFeeB),
		MaxBlockBodySize:         uint64(p.MaxBlockBodySize),
		MaxBlockHeaderSize:       uint64(p.MaxBlockHeaderSize),
		StakeKeyDeposit:          uint64(p.KeyDeposit),
		PoolDeposit:              uint64(p.PoolDeposit),
		PoolRetirementEpochBound: uint64(p.MaxEpoch),
		DesiredNumberOfPools:     uint64(p.NOpt),
		PoolInfluence:            a0,
		MonetaryExpansion:        rho,
		TreasuryExpansion:        tau,
		MinPoolCost:              p.MinPoolCost,
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: uint32(p.ProtocolMajor),
			Minor: uint32(p.ProtocolMinor),
		},
		MaxValueSize:         uint64(p.MaxValueSize),
		CollateralPercentage: uint64(p.CollateralPercentage),
		MaxCollateralInputs:  uint64(p.MaxCollateralInputs),
		CostModels:           costModels,
		Prices: &cardano.ExPrices{
			Memory: memPrice,
			Steps:  stepPrice,
		},
		MaxExecutionUnitsPerTransaction: &cardano.ExUnits{
			Memory: p.MaxTxExUnits.Memory,
			Steps:  p.MaxTxExUnits.Steps,
		},
		MaxExecutionUnitsPerBlock: &cardano.ExUnits{
			Memory: p.MaxBlockExUnits.Memory,
			Steps:  p.MaxBlockExUnits.Steps,
		},
	}
}
