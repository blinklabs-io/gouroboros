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
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// Constants for Plutus version mapping
const (
	PlutusV1Key uint = 0
	PlutusV2Key uint = 1
	PlutusV3Key uint = 2
)

// Expected parameter counts for validation
var plutusParamCounts = map[uint]int{
	PlutusV1Key: 166,
	PlutusV2Key: 175,
	PlutusV3Key: 187,
}

// PlutusCostModelParameterCount returns the reference parameter count for a
// known Plutus language.
func PlutusCostModelParameterCount(language uint) (int, bool) {
	count, ok := plutusParamCounts[language]
	return count, ok
}

// AlonzoProtocolParameters holds the Alonzo-era protocol parameters.
//
// AdaPerUtxoByte carries key 17, which in Alonzo is coinsPerUTxOWord: a price
// per 8-byte word of the UTxO entry size estimate, not per serialized byte.
// Babbage converts it to a genuine per-byte price by dividing by 8 at the era
// boundary, so the value held here is 8x the Babbage-era price for the same
// UTxO cost. MinUtxoValue is the flat Shelley parameter, which Alonzo replaced
// and no Alonzo rule reads.
//
// Reference: appCoinsPerUTxOWord in
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/PParams.hs.
type AlonzoProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA            uint
	MinFeeB            uint
	MaxBlockBodySize   uint
	MaxTxSize          uint
	MaxBlockHeaderSize uint
	KeyDeposit         uint
	PoolDeposit        uint
	MaxEpoch           uint
	NOpt               uint
	A0                 *cbor.Rat
	Rho                *cbor.Rat
	Tau                *cbor.Rat
	Decentralization   *cbor.Rat
	ExtraEntropy       common.Nonce
	ProtocolMajor      uint
	ProtocolMinor      uint
	// MinUtxoValue is the genesis-only Shelley parameter retained for
	// compatibility; Alonzo min-UTxO validation uses AdaPerUtxoByte instead.
	MinUtxoValue         uint
	MinPoolCost          uint64
	AdaPerUtxoByte       uint64
	CostModels           map[uint][]int64
	ExecutionCosts       common.ExUnitPrice
	MaxTxExUnits         common.ExUnits
	MaxBlockExUnits      common.ExUnits
	MaxValueSize         uint
	CollateralPercentage uint
	MaxCollateralInputs  uint
}

// KeyDepositAmount returns the key deposit as a *big.Int
func (p *AlonzoProtocolParameters) KeyDepositAmount() *big.Int {
	return new(big.Int).SetUint64(uint64(p.KeyDeposit))
}

// PoolDepositAmount returns the pool deposit as a *big.Int
func (p *AlonzoProtocolParameters) PoolDepositAmount() *big.Int {
	return new(big.Int).SetUint64(uint64(p.PoolDeposit))
}

// MinUtxoValueAmount returns the minimum UTxO value as a *big.Int
func (p *AlonzoProtocolParameters) MinUtxoValueAmount() *big.Int {
	return new(big.Int).SetUint64(uint64(p.MinUtxoValue))
}

// MinPoolCostAmount returns the minimum pool cost as a *big.Int
func (p *AlonzoProtocolParameters) MinPoolCostAmount() *big.Int {
	return new(big.Int).SetUint64(p.MinPoolCost)
}

// AdaPerUtxoByteAmount returns protocol parameter key 17 as a *big.Int. In
// Alonzo that parameter is coinsPerUTxOWord; see AdaPerUtxoByte.
func (p *AlonzoProtocolParameters) AdaPerUtxoByteAmount() *big.Int {
	return new(big.Int).SetUint64(p.AdaPerUtxoByte)
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
	if paramUpdate.MinPoolCost != nil {
		p.MinPoolCost = *paramUpdate.MinPoolCost
	}
	if paramUpdate.AdaPerUtxoByte != nil {
		p.AdaPerUtxoByte = *paramUpdate.AdaPerUtxoByte
	}
	if paramUpdate.CostModels != nil {
		if p.CostModels == nil {
			p.CostModels = make(map[uint][]int64)
		}
		maps.Copy(p.CostModels, paramUpdate.CostModels)
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

func (p *AlonzoProtocolParameters) UpdateFromGenesis(
	genesis *AlonzoGenesis,
) error {
	if genesis == nil {
		return nil
	}

	// Common parameter updates
	// Alonzo stores lovelacePerUTxOWord verbatim; the division to a
	// per-byte price belongs at the Babbage era boundary, not here.
	p.AdaPerUtxoByte = genesis.LovelacePerUtxoWord
	p.MaxValueSize = genesis.MaxValueSize
	p.CollateralPercentage = genesis.CollateralPercentage
	p.MaxCollateralInputs = genesis.MaxCollateralInputs
	p.MaxTxExUnits = common.ExUnits{
		Memory: int64(genesis.MaxTxExUnits.Mem),   // nolint:gosec
		Steps:  int64(genesis.MaxTxExUnits.Steps), // nolint:gosec
	}
	p.MaxBlockExUnits = common.ExUnits{
		Memory: int64(genesis.MaxBlockExUnits.Mem),   // nolint:gosec
		Steps:  int64(genesis.MaxBlockExUnits.Steps), // nolint:gosec
	}

	if genesis.ExecutionPrices.Mem != nil &&
		genesis.ExecutionPrices.Steps != nil {
		p.ExecutionCosts = common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: genesis.ExecutionPrices.Mem.Rat},
			StepPrice: &cbor.Rat{Rat: genesis.ExecutionPrices.Steps.Rat},
		}
	}

	if genesis.CostModels != nil {
		p.CostModels = make(map[uint][]int64)
		for versionStr, model := range genesis.CostModels {
			key, ok := plutusVersionToKey(versionStr)
			if !ok {
				continue
			}
			expectedCount, ok := plutusParamCounts[key]
			if !ok {
				continue
			}
			if len(model) < expectedCount {
				return fmt.Errorf(
					"insufficient param count for %s: %d",
					versionStr,
					len(model),
				)
			}
			p.CostModels[key] = model
		}
	}
	return nil
}

// Helper to convert Plutus version string to key
func plutusVersionToKey(version string) (uint, bool) {
	switch version {
	case "PlutusV1":
		return PlutusV1Key, true
	case "PlutusV2":
		return PlutusV2Key, true
	case "PlutusV3":
		return PlutusV3Key, true
	default:
		return 0, false
	}
}

// AlonzoProtocolParameterUpdate holds an Alonzo-era protocol parameter update.
//
// MinUtxoValue carries no CBOR key and is never populated from the wire: the
// Alonzo CDDL removed protocol_param_update key 15 and UnmarshalCBOR rejects
// an update carrying it. The field is retained so that existing Go callers
// still compile, and Update does not apply it.
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
	MinUtxoValue         *uint                                     `cbor:"-"`
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

func (u AlonzoProtocolParameterUpdate) ProtocolParameterUpdateCostModels() map[uint][]int64 {
	return u.CostModels
}

func (u AlonzoProtocolParameterUpdate) ProtocolParameterVersionUpdate() *common.ProtocolParametersProtocolVersion {
	return u.ProtocolVersion
}

func (u AlonzoProtocolParameterUpdate) ValidateProtocolParameterUpdateVersion(
	currentVersion common.ProtocolParametersProtocolVersion,
) error {
	return common.ValidateClassicCostModelUpdate(
		u.CostModels,
		currentVersion.Major,
		map[uint]int{PlutusV1Key: plutusParamCounts[PlutusV1Key]},
	)
}

// removedMinUtxoValueKey is protocol_param_update key 15 (minUTxOValue). The
// Alonzo CDDL dropped it and the reference decoder routes any key it does not
// recognize to Invalid, failing the decode rather than ignoring the entry, so
// an update carrying key 15 must be rejected outright.
//
// Reference: updateField in
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/PParams.hs, whose final clause is
// `updateField k = field (\_x up -> up) (Invalid k)`.
const removedMinUtxoValueKey = 15

func (u *AlonzoProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	if err := common.ValidateProtocolParameterUpdateDomains(
		cborData,
		common.ProtocolParameterUpdateEraAlonzo,
	); err != nil {
		return err
	}
	var rawKeys map[uint64]cbor.RawMessage
	if _, err := cbor.Decode(cborData, &rawKeys); err != nil {
		return err
	}
	if _, ok := rawKeys[removedMinUtxoValueKey]; ok {
		return fmt.Errorf(
			"alonzo protocol parameter update contains key %d"+
				" (minUTxOValue), removed in the Alonzo era",
			removedMinUtxoValueKey,
		)
	}
	type tAlonzoProtocolParameterUpdate AlonzoProtocolParameterUpdate
	var tmp tAlonzoProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = AlonzoProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

// ratOutOfRange reports whether r's numerator or denominator cannot be
// represented in utxorpc.RationalNumber's int32/uint32 fields. Compares the
// underlying *big.Int values directly against the bounds, not via Int64()
// first: Int64() is undefined for a value that does not fit in int64 at all
// (silently wraps rather than erroring, per math/big's own documentation),
// so a value far outside range (e.g. 2^64+1) could pass an
// Int64()-based comparison completely undetected (blinklabs-io/gouroboros#2296,
// porting the fix from conway.ratOutOfRange).
// r itself must be non-nil; every caller already guards that separately
// (e.g. "p.A0 == nil ||", short-circuiting before this runs).
func ratOutOfRange(r *big.Rat) bool {
	return r.Num().Cmp(big.NewInt(math.MinInt32)) < 0 ||
		r.Num().Cmp(big.NewInt(math.MaxInt32)) > 0 ||
		r.Denom().Sign() < 0 ||
		r.Denom().Cmp(new(big.Int).SetUint64(math.MaxUint32)) > 0
}

func (p *AlonzoProtocolParameters) Utxorpc() (*utxorpc.PParams, error) {
	// sanity check
	//
	// Checks the embedded *big.Rat for nil separately from the *cbor.Rat
	// pointer itself: a non-nil *cbor.Rat with a nil embedded Rat must be
	// rejected as invalid rather than reaching ratOutOfRange's Num()/Denom()
	// calls, which require their receiver to be non-nil
	// (blinklabs-io/gouroboros#2296, porting the fix from conway.Utxorpc).
	if p.A0 == nil || p.A0.Rat == nil || ratOutOfRange(p.A0.Rat) {
		return nil, errors.New("invalid A0 rational number values")
	}
	if p.Rho == nil || p.Rho.Rat == nil || ratOutOfRange(p.Rho.Rat) {
		return nil, errors.New("invalid Rho rational number values")
	}
	if p.Tau == nil || p.Tau.Rat == nil || ratOutOfRange(p.Tau.Rat) {
		return nil, errors.New("invalid Tau rational number values")
	}
	if p.ExecutionCosts.MemPrice == nil ||
		p.ExecutionCosts.MemPrice.Rat == nil ||
		ratOutOfRange(p.ExecutionCosts.MemPrice.Rat) {
		return nil, errors.New("invalid memory price rational number values")
	}
	if p.ExecutionCosts.StepPrice == nil ||
		p.ExecutionCosts.StepPrice.Rat == nil ||
		ratOutOfRange(p.ExecutionCosts.StepPrice.Rat) {
		return nil, errors.New("invalid step price rational number values")
	}
	if p.MaxTxExUnits.Memory < 0 || p.MaxTxExUnits.Steps < 0 ||
		p.MaxBlockExUnits.Memory < 0 || p.MaxBlockExUnits.Steps < 0 {
		return nil, errors.New("invalid execution unit values")
	}
	// #nosec G115
	return &utxorpc.PParams{
		CoinsPerUtxoByte:         common.ToUtxorpcBigInt(p.AdaPerUtxoByte),
		MaxTxSize:                uint64(p.MaxTxSize),
		MinFeeCoefficient:        common.ToUtxorpcBigInt(uint64(p.MinFeeA)),
		MinFeeConstant:           common.ToUtxorpcBigInt(uint64(p.MinFeeB)),
		MaxBlockBodySize:         uint64(p.MaxBlockBodySize),
		MaxBlockHeaderSize:       uint64(p.MaxBlockHeaderSize),
		StakeKeyDeposit:          common.ToUtxorpcBigInt(uint64(p.KeyDeposit)),
		PoolDeposit:              common.ToUtxorpcBigInt(uint64(p.PoolDeposit)),
		MinPoolCost:              common.ToUtxorpcBigInt(p.MinPoolCost),
		PoolRetirementEpochBound: uint64(p.MaxEpoch),
		DesiredNumberOfPools:     uint64(p.NOpt),
		PoolInfluence: &utxorpc.RationalNumber{
			Numerator:   int32(p.A0.Num().Int64()),
			Denominator: uint32(p.A0.Denom().Int64()),
		},
		MonetaryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(p.Rho.Num().Int64()),
			Denominator: uint32(p.Rho.Denom().Int64()),
		},
		TreasuryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(p.Tau.Num().Int64()),
			Denominator: uint32(p.Tau.Denom().Int64()),
		},
		ProtocolVersion: &utxorpc.ProtocolVersion{
			Major: uint32(p.ProtocolMajor),
			Minor: uint32(p.ProtocolMinor),
		},
		MaxValueSize:         uint64(p.MaxValueSize),
		CollateralPercentage: uint64(p.CollateralPercentage),
		MaxCollateralInputs:  uint64(p.MaxCollateralInputs),
		CostModels: common.ConvertToUtxorpcCardanoCostModels(
			p.CostModels,
		),
		Prices: &utxorpc.ExPrices{
			Memory: &utxorpc.RationalNumber{
				Numerator:   int32(p.ExecutionCosts.MemPrice.Num().Int64()),
				Denominator: uint32(p.ExecutionCosts.MemPrice.Denom().Int64()),
			},
			Steps: &utxorpc.RationalNumber{
				Numerator:   int32(p.ExecutionCosts.StepPrice.Num().Int64()),
				Denominator: uint32(p.ExecutionCosts.StepPrice.Denom().Int64()),
			},
		},
		MaxExecutionUnitsPerTransaction: &utxorpc.ExUnits{
			Memory: uint64(p.MaxTxExUnits.Memory),
			Steps:  uint64(p.MaxTxExUnits.Steps),
		},
		MaxExecutionUnitsPerBlock: &utxorpc.ExUnits{
			Memory: uint64(p.MaxBlockExUnits.Memory),
			Steps:  uint64(p.MaxBlockExUnits.Steps),
		},
	}, nil
}

func UpgradePParams(
	prevPParams mary.MaryProtocolParameters,
) AlonzoProtocolParameters {
	return AlonzoProtocolParameters{
		MinFeeA:            prevPParams.MinFeeA,
		MinFeeB:            prevPParams.MinFeeB,
		MaxBlockBodySize:   prevPParams.MaxBlockBodySize,
		MaxTxSize:          prevPParams.MaxTxSize,
		MaxBlockHeaderSize: prevPParams.MaxBlockHeaderSize,
		KeyDeposit:         prevPParams.KeyDeposit,
		PoolDeposit:        prevPParams.PoolDeposit,
		MaxEpoch:           prevPParams.MaxEpoch,
		NOpt:               prevPParams.NOpt,
		A0:                 prevPParams.A0,
		Rho:                prevPParams.Rho,
		Tau:                prevPParams.Tau,
		Decentralization:   prevPParams.Decentralization,
		ExtraEntropy:       prevPParams.ExtraEntropy,
		ProtocolMajor:      prevPParams.ProtocolMajor,
		ProtocolMinor:      prevPParams.ProtocolMinor,
		MinUtxoValue:       prevPParams.MinUtxoValue,
		MinPoolCost:        prevPParams.MinPoolCost,
	}
}

// ProtocolMajorVersion returns the active major protocol version.
func (p *AlonzoProtocolParameters) ProtocolMajorVersion() uint {
	return p.ProtocolMajor
}

func (p *AlonzoProtocolParameters) ProtocolParametersProtocolVersion() common.ProtocolParametersProtocolVersion {
	return common.ProtocolParametersProtocolVersion{Major: p.ProtocolMajor, Minor: p.ProtocolMinor}
}

// MinPoolCostValue returns the minPoolCost protocol parameter.
func (p *AlonzoProtocolParameters) MinPoolCostValue() uint64 {
	return p.MinPoolCost
}

// PoolRetirementMaxEpoch returns the eMax protocol parameter.
func (p *AlonzoProtocolParameters) PoolRetirementMaxEpoch() uint64 {
	return uint64(p.MaxEpoch)
}
