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

// The GetGenesisConfig reply has three wire layouts, chosen by the negotiated
// node-to-client protocol version. Up to version 20 the node emits a copy of an
// older ledger serialisation it keeps for the purpose; from version 21 it emits
// the ledger's current one, which a node normally sends without the extra
// config the ledger added to the genesis record; from version 24 it emits the
// record with the extra config. Consensus 4.0.0.0 to 4.2.0.1 (node 11.1.0
// pre-release, cardano-cli 11.2.1.0 and 11.2.2.0) sent the 16-field record
// at versions 21 to 23 as well, until 4.2.1.0 reverted that, so the decoder
// accepts both.
//
// The layouts differ in arity at both levels, so the reply identifies itself
// and the decoder does not need to be told which version was negotiated:
//
//	genesis record   15 fields -> 16 fields, gaining the extra config
//	protocol params  18 fields -> 17 fields, the protocol version having become
//	                              a nested pair rather than two adjacent fields
//
// The 15-field record occurs with both protocol parameter layouts, so the
// record arity alone no longer identifies the reply: the protocol parameters
// have to be read as well.
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

// GenesisConfigLayout selects the wire layout of a GetGenesisConfig reply.
type GenesisConfigLayout int

const (
	// GenesisConfigLayoutAuto derives the layout from the reply's contents:
	// the legacy layout when it carries no extra config, the current layout
	// otherwise. It is the zero value, so a reply built in Go encodes the way
	// it always has.
	GenesisConfigLayoutAuto GenesisConfigLayout = iota
	// GenesisConfigLayoutLegacy is the vendored serialisation a node sends up
	// to node-to-client protocol version 20: a 15-field genesis record with
	// 18-field protocol parameters.
	GenesisConfigLayoutLegacy
	// GenesisConfigLayoutNoExtraConfig is the serialisation a node sends from
	// node-to-client protocol version 21 to 23 from consensus 4.2.1.0 on: a
	// 15-field genesis record without the extra config, with the current
	// 17-field protocol parameters. Consensus 4.0.0.0 to 4.2.0.1 sent the
	// 16-field record at these versions, which decodes as
	// GenesisConfigLayoutCurrent.
	GenesisConfigLayoutNoExtraConfig
	// GenesisConfigLayoutCurrent is the serialisation a node sends from
	// node-to-client protocol version 24: a 16-field genesis record with the
	// extra config, with the current 17-field protocol parameters.
	GenesisConfigLayoutCurrent
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
	// alongside the version-21 encoding. It is nil when the reply used a layout
	// without that field.
	ExtraConfig cbor.RawMessage
	// Layout is the wire layout the reply uses. The zero value leaves the
	// choice to the contents of the reply, which is what a reply built in Go
	// wants; a reply that was decoded carries the layout it was decoded from,
	// so that re-encoding it reproduces the same bytes.
	Layout GenesisConfigLayout
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
		decoded.Layout = GenesisConfigLayoutCurrent
	} else if decoded.ProtocolParams.legacyLayout {
		decoded.Layout = GenesisConfigLayoutLegacy
	} else {
		// A 15-field record with the current protocol parameters: the layout
		// a node sends from node-to-client version 21 to 23 from consensus
		// 4.2.1.0 on.
		decoded.Layout = GenesisConfigLayoutNoExtraConfig
	}
	*g = decoded
	return nil
}

func (g GenesisConfigResult) MarshalCBOR() ([]byte, error) {
	layout := g.Layout
	switch layout {
	case GenesisConfigLayoutAuto, GenesisConfigLayoutLegacy,
		GenesisConfigLayoutNoExtraConfig, GenesisConfigLayoutCurrent:
	default:
		return nil, fmt.Errorf("genesis config: unknown layout %d", layout)
	}
	if layout == GenesisConfigLayoutAuto {
		if g.ExtraConfig != nil {
			layout = GenesisConfigLayoutCurrent
		} else {
			layout = GenesisConfigLayoutLegacy
		}
	}
	// The protocol parameters are nested inside this record, so their layout is
	// the record's to choose rather than their own.
	pparams := g.ProtocolParams
	pparams.legacyLayout = layout == GenesisConfigLayoutLegacy
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
	if layout == GenesisConfigLayoutCurrent {
		if g.ExtraConfig != nil {
			fields = append(fields, g.ExtraConfig)
		} else {
			// The ledger holds the extra config in a StrictMaybe, which it
			// encodes as a list: SNothing is the empty list.
			fields = append(fields, cbor.RawMessage{0x80})
		}
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

// genesisRational is a [numerator, denominator] pair. The reference codec
// writes it under CBOR tag 30 and rejects an untagged pair, so the current
// layout encodes the tag; decoding accepts both forms.
type genesisRational []int

func (r genesisRational) MarshalCBOR() ([]byte, error) {
	pair, err := cbor.Encode([]int(r))
	if err != nil {
		return nil, err
	}
	return append([]byte{0xd8, cbor.CborTagRational}, pair...), nil
}

func (r *genesisRational) UnmarshalCBOR(data []byte) error {
	if len(data) >= 2 && data[0] == 0xd8 && data[1] == cbor.CborTagRational {
		data = data[2:]
	}
	var pair []int
	if _, err := cbor.Decode(data, &pair); err != nil {
		return err
	}
	*r = pair
	return nil
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
	A0                    genesisRational
	Rho                   genesisRational
	Tau                   genesisRational
	DecentralizationParam genesisRational
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
