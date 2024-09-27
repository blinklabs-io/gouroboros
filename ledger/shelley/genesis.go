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

import "time"

type ShelleyGenesis struct {
	SystemStart        time.Time                    `json:"systemStart"`
	NetworkMagic       uint32                       `json:"networkMagic"`
	NetworkId          string                       `json:"networkid"`
	ActiveSlotsCoeff   float32                      `json:"activeSlotsCoeff"`
	SecurityParam      int                          `json:"securityParam"`
	EpochLength        int                          `json:"epochLength"`
	SlotsPerKESPeriod  int                          `json:"slotsPerKESPeriod"`
	MaxKESEvolutions   int                          `json:"maxKESEvolutions"`
	SlotLength         int                          `json:"slotLength"`
	UpdateQuorum       int                          `json:"updateQuorum"`
	MaxLovelaceSupply  uint64                       `json:"maxLovelaceSupply"`
	ProtocolParameters ShelleyGenesisProtocolParams `json:"protocolParams"`
	GenDelegs          map[string]map[string]any    `json:"genDelegs"`
	InitialFunds       map[string]any               `json:"initialFunds"`
	Staking            any                          `json:"staking"`
}

type ShelleyGenesisProtocolParams struct {
	MinFeeA            uint
	MinFeeB            uint
	MaxBlockBodySize   uint
	MaxTxSize          uint
	MaxBlockHeaderSize uint
	KeyDeposit         uint
	PoolDeposit        uint
	MaxEpoch           uint `json:"eMax"`
	NOpt               uint
	A0                 float32
	Rho                float32
	Tau                float32
	Decentralization   float32 `json:"decentralisationParam"`
	ExtraEntropy       map[string]string
	ProtocolVersion    struct {
		Major uint
		Minor uint
	}
	MinUtxoValue uint `json:"minUTxOValue"`
	MinPoolCost  uint
}
