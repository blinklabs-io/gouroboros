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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// The GetGenesisConfig reply has two wire layouts, chosen by the negotiated
// node-to-client protocol version. Up to version 20 the node emits a copy of an
// older ledger serialisation it keeps for the purpose; from version 21 it emits
// the ledger's current one.
//
// The layouts differ in arity at both levels, so the reply identifies itself
// and the decoder does not need to be told which version was negotiated:
//
//	genesis record   15 fields -> 16 fields, gaining the extra config
//	protocol params  18 fields -> 17 fields, the protocol version having become
//	                              a nested pair rather than two adjacent fields
//
// Reading one layout as the other is not a near miss. The protocol parameters
// sit in the middle of the genesis record, so a wrong guess shifts every
// following field: the genesis delegates are read out of the protocol
// parameters, and the decode either fails on a type mismatch or, worse,
// succeeds with values that belong to other fields.
const (
	genesisConfigFieldsLegacy  = 15
	genesisConfigFieldsCurrent = 16

	genesisPParamsFieldsLegacy  = 18
	genesisPParamsFieldsCurrent = 17
)

type GenesisConfigResult struct {
	Start             SystemStartResult
	NetworkMagic      int
	NetworkId         uint8
	ActiveSlotsCoeff  []any
	SecurityParam     int
	EpochLength       int
	SlotsPerKESPeriod int
	MaxKESEvolutions  int
	SlotLength        int
	UpdateQuorum      int
	MaxLovelaceSupply int64
	ProtocolParams    GenesisConfigResultProtocolParameters
	// GenDelegs, InitialFunds and Staking are held as raw CBOR: they are maps
	// with bytestring keys, which we can't parse yet.
	GenDelegs    cbor.RawMessage
	InitialFunds cbor.RawMessage
	Staking      cbor.RawMessage
	// ExtraConfig carries the optional injection configuration the ledger added
	// alongside the version-21 encoding. It is nil when the reply used the
	// legacy layout, which has no such field, and it doubles as the record's
	// layout marker when re-encoding.
	ExtraConfig cbor.RawMessage
}

func (g *GenesisConfigResult) UnmarshalCBOR(data []byte) error {
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(data, &fields); err != nil {
		return err
	}
	if len(fields) != genesisConfigFieldsLegacy &&
		len(fields) != genesisConfigFieldsCurrent {
		return fmt.Errorf(
			"genesis config: expected %d or %d fields, got %d",
			genesisConfigFieldsLegacy,
			genesisConfigFieldsCurrent,
			len(fields),
		)
	}
	decoded := GenesisConfigResult{}
	targets := []any{
		&decoded.Start,
		&decoded.NetworkMagic,
		&decoded.NetworkId,
		&decoded.ActiveSlotsCoeff,
		&decoded.SecurityParam,
		&decoded.EpochLength,
		&decoded.SlotsPerKESPeriod,
		&decoded.MaxKESEvolutions,
		&decoded.SlotLength,
		&decoded.UpdateQuorum,
		&decoded.MaxLovelaceSupply,
		&decoded.ProtocolParams,
	}
	for idx, target := range targets {
		if _, err := cbor.Decode(fields[idx], target); err != nil {
			return fmt.Errorf("genesis config field %d: %w", idx+1, err)
		}
	}
	decoded.GenDelegs = fields[len(targets)]
	decoded.InitialFunds = fields[len(targets)+1]
	decoded.Staking = fields[len(targets)+2]
	if len(fields) == genesisConfigFieldsCurrent {
		decoded.ExtraConfig = fields[genesisConfigFieldsCurrent-1]
	}
	*g = decoded
	return nil
}

func (g GenesisConfigResult) MarshalCBOR() ([]byte, error) {
	// The protocol parameters are nested inside this record, so their layout is
	// the record's to choose rather than their own.
	pparams := g.ProtocolParams
	pparams.legacyLayout = g.ExtraConfig == nil
	fields := []any{
		g.Start,
		g.NetworkMagic,
		g.NetworkId,
		g.ActiveSlotsCoeff,
		g.SecurityParam,
		g.EpochLength,
		g.SlotsPerKESPeriod,
		g.MaxKESEvolutions,
		g.SlotLength,
		g.UpdateQuorum,
		g.MaxLovelaceSupply,
		pparams,
		g.GenDelegs,
		g.InitialFunds,
		g.Staking,
	}
	if g.ExtraConfig != nil {
		fields = append(fields, g.ExtraConfig)
	}
	return cbor.Encode(fields)
}

type GenesisConfigResultProtocolParameters struct {
	MinFeeA               int
	MinFeeB               int
	MaxBlockBodySize      int
	MaxTxSize             int
	MaxBlockHeaderSize    int
	KeyDeposit            int
	PoolDeposit           int
	EMax                  int
	NOpt                  int
	A0                    []int
	Rho                   []int
	Tau                   []int
	DecentralizationParam []int
	ExtraEntropy          any
	ProtocolVersionMajor  int
	ProtocolVersionMinor  int
	MinUTxOValue          int
	MinPoolCost           int
	// legacyLayout records which of the two encodings this value was decoded
	// from, so that re-encoding reproduces it. A value built in Go rather than
	// decoded encodes as the current layout.
	legacyLayout bool
}

// genesisPParamsLegacy is the layout sent up to node-to-client protocol version
// 20, with the protocol version split across two adjacent fields.
type genesisPParamsLegacy struct {
	cbor.StructAsArray
	MinFeeA               int
	MinFeeB               int
	MaxBlockBodySize      int
	MaxTxSize             int
	MaxBlockHeaderSize    int
	KeyDeposit            int
	PoolDeposit           int
	EMax                  int
	NOpt                  int
	A0                    []int
	Rho                   []int
	Tau                   []int
	DecentralizationParam []int
	ExtraEntropy          any
	ProtocolVersionMajor  int
	ProtocolVersionMinor  int
	MinUTxOValue          int
	MinPoolCost           int
}

// genesisProtocolVersion is the nested pair the current layout uses in place of
// the two adjacent fields.
type genesisProtocolVersion struct {
	cbor.StructAsArray
	Major int
	Minor int
}

// genesisPParamsCurrent is the layout sent from node-to-client protocol version
// 21 onwards.
type genesisPParamsCurrent struct {
	cbor.StructAsArray
	MinFeeA               int
	MinFeeB               int
	MaxBlockBodySize      int
	MaxTxSize             int
	MaxBlockHeaderSize    int
	KeyDeposit            int
	PoolDeposit           int
	EMax                  int
	NOpt                  int
	A0                    []int
	Rho                   []int
	Tau                   []int
	DecentralizationParam []int
	ExtraEntropy          any
	ProtocolVersion       genesisProtocolVersion
	MinUTxOValue          int
	MinPoolCost           int
}

func (p *GenesisConfigResultProtocolParameters) UnmarshalCBOR(
	data []byte,
) error {
	listLen, err := cbor.ListLength(data)
	if err != nil {
		return err
	}
	switch listLen {
	case genesisPParamsFieldsLegacy:
		var tmp genesisPParamsLegacy
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		*p = GenesisConfigResultProtocolParameters{
			MinFeeA:               tmp.MinFeeA,
			MinFeeB:               tmp.MinFeeB,
			MaxBlockBodySize:      tmp.MaxBlockBodySize,
			MaxTxSize:             tmp.MaxTxSize,
			MaxBlockHeaderSize:    tmp.MaxBlockHeaderSize,
			KeyDeposit:            tmp.KeyDeposit,
			PoolDeposit:           tmp.PoolDeposit,
			EMax:                  tmp.EMax,
			NOpt:                  tmp.NOpt,
			A0:                    tmp.A0,
			Rho:                   tmp.Rho,
			Tau:                   tmp.Tau,
			DecentralizationParam: tmp.DecentralizationParam,
			ExtraEntropy:          tmp.ExtraEntropy,
			ProtocolVersionMajor:  tmp.ProtocolVersionMajor,
			ProtocolVersionMinor:  tmp.ProtocolVersionMinor,
			MinUTxOValue:          tmp.MinUTxOValue,
			MinPoolCost:           tmp.MinPoolCost,
			legacyLayout:          true,
		}
	case genesisPParamsFieldsCurrent:
		var tmp genesisPParamsCurrent
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		*p = GenesisConfigResultProtocolParameters{
			MinFeeA:               tmp.MinFeeA,
			MinFeeB:               tmp.MinFeeB,
			MaxBlockBodySize:      tmp.MaxBlockBodySize,
			MaxTxSize:             tmp.MaxTxSize,
			MaxBlockHeaderSize:    tmp.MaxBlockHeaderSize,
			KeyDeposit:            tmp.KeyDeposit,
			PoolDeposit:           tmp.PoolDeposit,
			EMax:                  tmp.EMax,
			NOpt:                  tmp.NOpt,
			A0:                    tmp.A0,
			Rho:                   tmp.Rho,
			Tau:                   tmp.Tau,
			DecentralizationParam: tmp.DecentralizationParam,
			ExtraEntropy:          tmp.ExtraEntropy,
			ProtocolVersionMajor:  tmp.ProtocolVersion.Major,
			ProtocolVersionMinor:  tmp.ProtocolVersion.Minor,
			MinUTxOValue:          tmp.MinUTxOValue,
			MinPoolCost:           tmp.MinPoolCost,
		}
	default:
		return fmt.Errorf(
			"genesis protocol parameters: expected %d or %d fields, got %d",
			genesisPParamsFieldsLegacy,
			genesisPParamsFieldsCurrent,
			listLen,
		)
	}
	return nil
}

func (p GenesisConfigResultProtocolParameters) MarshalCBOR() ([]byte, error) {
	if p.legacyLayout {
		return cbor.Encode(
			genesisPParamsLegacy{
				MinFeeA:               p.MinFeeA,
				MinFeeB:               p.MinFeeB,
				MaxBlockBodySize:      p.MaxBlockBodySize,
				MaxTxSize:             p.MaxTxSize,
				MaxBlockHeaderSize:    p.MaxBlockHeaderSize,
				KeyDeposit:            p.KeyDeposit,
				PoolDeposit:           p.PoolDeposit,
				EMax:                  p.EMax,
				NOpt:                  p.NOpt,
				A0:                    p.A0,
				Rho:                   p.Rho,
				Tau:                   p.Tau,
				DecentralizationParam: p.DecentralizationParam,
				ExtraEntropy:          p.ExtraEntropy,
				ProtocolVersionMajor:  p.ProtocolVersionMajor,
				ProtocolVersionMinor:  p.ProtocolVersionMinor,
				MinUTxOValue:          p.MinUTxOValue,
				MinPoolCost:           p.MinPoolCost,
			},
		)
	}
	return cbor.Encode(
		genesisPParamsCurrent{
			MinFeeA:               p.MinFeeA,
			MinFeeB:               p.MinFeeB,
			MaxBlockBodySize:      p.MaxBlockBodySize,
			MaxTxSize:             p.MaxTxSize,
			MaxBlockHeaderSize:    p.MaxBlockHeaderSize,
			KeyDeposit:            p.KeyDeposit,
			PoolDeposit:           p.PoolDeposit,
			EMax:                  p.EMax,
			NOpt:                  p.NOpt,
			A0:                    p.A0,
			Rho:                   p.Rho,
			Tau:                   p.Tau,
			DecentralizationParam: p.DecentralizationParam,
			ExtraEntropy:          p.ExtraEntropy,
			ProtocolVersion: genesisProtocolVersion{
				Major: p.ProtocolVersionMajor,
				Minor: p.ProtocolVersionMinor,
			},
			MinUTxOValue: p.MinUTxOValue,
			MinPoolCost:  p.MinPoolCost,
		},
	)
}
