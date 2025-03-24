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

package shelley

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ShelleyGenesis struct {
	cbor.StructAsArray
	SystemStart        time.Time                    `json:"systemStart"`
	NetworkMagic       uint32                       `json:"networkMagic"`
	NetworkId          string                       `json:"networkid"`
	ActiveSlotsCoeff   common.GenesisRat            `json:"activeSlotsCoeff"`
	SecurityParam      int                          `json:"securityParam"`
	EpochLength        int                          `json:"epochLength"`
	SlotsPerKESPeriod  int                          `json:"slotsPerKESPeriod"`
	MaxKESEvolutions   int                          `json:"maxKESEvolutions"`
	SlotLength         int                          `json:"slotLength"`
	UpdateQuorum       int                          `json:"updateQuorum"`
	MaxLovelaceSupply  uint64                       `json:"maxLovelaceSupply"`
	ProtocolParameters ShelleyGenesisProtocolParams `json:"protocolParams"`
	GenDelegs          map[string]map[string]string `json:"genDelegs"`
	InitialFunds       map[string]any               `json:"initialFunds"`
	Staking            any                          `json:"staking"`
}

func (g ShelleyGenesis) MarshalCBOR() ([]byte, error) {
	genDelegs := map[cbor.ByteString][]cbor.ByteString{}
	for k, v := range g.GenDelegs {
		keyBytes, err := hex.DecodeString(k)
		if err != nil {
			return nil, err
		}
		vrfBytes, err := hex.DecodeString(v["vrf"])
		if err != nil {
			return nil, err
		}
		delegateBytes, err := hex.DecodeString(v["delegate"])
		if err != nil {
			return nil, err
		}
		genDelegs[cbor.NewByteString(keyBytes)] = []cbor.ByteString{
			cbor.NewByteString(delegateBytes),
			cbor.NewByteString(vrfBytes),
		}
	}
	staking := []any{}
	if g.Staking == nil {
		staking = []any{
			map[any]any{},
			map[any]any{},
		}
	}
	tmpData := []any{
		[]any{
			g.SystemStart.Year(),
			g.SystemStart.YearDay(),
			g.SystemStart.Nanosecond() * 1000,
		},
		g.NetworkMagic,
		map[string]int{"Testnet": 0, "Mainnet": 1}[g.NetworkId],
		[]any{
			g.ActiveSlotsCoeff.Num().Int64(),
			g.ActiveSlotsCoeff.Denom().Int64(),
		},
		g.SecurityParam,
		g.EpochLength,
		g.SlotsPerKESPeriod,
		g.MaxKESEvolutions,
		g.SlotLength * 1_000_000,
		g.UpdateQuorum,
		g.MaxLovelaceSupply,
		g.ProtocolParameters,
		genDelegs,
		g.InitialFunds,
		staking,
	}
	return cbor.Encode(tmpData)
}

type ShelleyGenesisProtocolParams struct {
	cbor.StructAsArray
	MinFeeA            uint               `json:"minFeeA"`
	MinFeeB            uint               `json:"minFeeB"`
	MaxBlockBodySize   uint               `json:"maxBlockBodySize"`
	MaxTxSize          uint               `json:"maxTxSize"`
	MaxBlockHeaderSize uint               `json:"maxBlockHeaderSize"`
	KeyDeposit         uint               `json:"keyDeposit"`
	PoolDeposit        uint               `json:"poolDeposit"`
	MaxEpoch           uint               `json:"eMax"`
	NOpt               uint               `json:"nOpt"`
	A0                 *common.GenesisRat `json:"a0"`
	Rho                *common.GenesisRat `json:"rho"`
	Tau                *common.GenesisRat `json:"tau"`
	Decentralization   *common.GenesisRat `json:"decentralisationParam"`
	ExtraEntropy       common.Nonce       `json:"extraEntropy"`
	ProtocolVersion    struct {
		Major uint `json:"major"`
		Minor uint `json:"minor"`
	} `json:"protocolVersion"`
	MinUtxoValue uint `json:"minUTxOValue"`
	MinPoolCost  uint `json:"minPoolCost"`
}

func (p ShelleyGenesisProtocolParams) MarshalCBOR() ([]byte, error) {
	tmpData := []any{
		p.MinFeeA,
		p.MinFeeB,
		p.MaxBlockBodySize,
		p.MaxTxSize,
		p.MaxBlockHeaderSize,
		p.KeyDeposit,
		p.PoolDeposit,
		p.MaxEpoch,
		p.NOpt,
		cbor.Rat{
			Rat: p.A0.Rat,
		},
		cbor.Rat{
			Rat: p.Rho.Rat,
		},
		cbor.Rat{
			Rat: p.Tau.Rat,
		},
		cbor.Rat{
			Rat: p.Decentralization.Rat,
		},
		p.ExtraEntropy,
		p.ProtocolVersion.Major,
		p.ProtocolVersion.Minor,
		p.MinUtxoValue,
		p.MinPoolCost,
	}
	return cbor.Encode(tmpData)
}

func NewShelleyGenesisFromReader(r io.Reader) (ShelleyGenesis, error) {
	var ret ShelleyGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewShelleyGenesisFromFile(path string) (ShelleyGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return ShelleyGenesis{}, err
	}
	defer f.Close()
	return NewShelleyGenesisFromReader(f)
}
