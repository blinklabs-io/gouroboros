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

package shelley

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
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
	SlotLength         common.GenesisRat            `json:"slotLength"`
	UpdateQuorum       int                          `json:"updateQuorum"`
	MaxLovelaceSupply  uint64                       `json:"maxLovelaceSupply"`
	ProtocolParameters ShelleyGenesisProtocolParams `json:"protocolParams"`
	GenDelegs          map[string]map[string]string `json:"genDelegs"`
	InitialFunds       map[string]uint64            `json:"initialFunds"`
	Staking            GenesisStaking               `json:"staking"`
}

type GenesisStaking struct {
	Pools map[string]GenesisPool `json:"pools"`
	Stake map[string]string      `json:"stake"`
}

type GenesisPool struct {
	Cost          int64          `json:"cost"`
	Margin        float64        `json:"margin"`
	Metadata      interface{}    `json:"metadata"`
	Owners        []string       `json:"owners"`
	Pledge        int64          `json:"pledge"`
	PublicKey     string         `json:"publicKey"`
	Relays        []GenesisRelay `json:"relays"`
	RewardAccount GenesisReward  `json:"rewardAccount"`
	Vrf           string         `json:"vrf"`
}

type GenesisRelay struct {
	SingleHostName *SingleHostName `json:"single host name,omitempty"`
}

type SingleHostName struct {
	DNSName string `json:"dnsName"`
	Port    int    `json:"port"`
}

type GenesisReward struct {
	Credential struct {
		KeyHash string `json:"key hash"`
	} `json:"credential"`
	Network string `json:"network"`
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

	// Convert pools to CBOR format
	cborPools := make(map[cbor.ByteString]any)
	for poolId, pool := range g.Staking.Pools {
		poolIdBytes, err := hex.DecodeString(poolId)
		if err != nil {
			return nil, err
		}
		vrfBytes, err := hex.DecodeString(pool.Vrf)
		if err != nil {
			return nil, err
		}
		rewardAccountBytes, err := hex.DecodeString(pool.RewardAccount.Credential.KeyHash)
		if err != nil {
			return nil, err
		}
		cborPools[cbor.NewByteString(poolIdBytes)] = []any{
			pool.Cost,
			pool.Margin,
			pool.Pledge,
			pool.PublicKey,
			[]any{
				[]byte{0},
				rewardAccountBytes,
			},
			pool.Owners,
			pool.Relays,
			vrfBytes,
			pool.Metadata,
		}
	}

	// Convert stake to CBOR format
	cborStake := make(map[cbor.ByteString]cbor.ByteString)
	for stakeAddr, poolId := range g.Staking.Stake {
		stakeAddrBytes, err := hex.DecodeString(stakeAddr)
		if err != nil {
			return nil, err
		}
		poolIdBytes, err := hex.DecodeString(poolId)
		if err != nil {
			return nil, err
		}
		cborStake[cbor.NewByteString(stakeAddrBytes)] = cbor.NewByteString(poolIdBytes)
	}

	slotLengthMs := &big.Rat{}
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
		slotLengthMs.Mul(g.SlotLength.Rat, big.NewRat(1_000_000, 1)),
		g.UpdateQuorum,
		g.MaxLovelaceSupply,
		g.ProtocolParameters,
		genDelegs,
		g.InitialFunds,
		[]any{
			cborPools,
			cborStake,
		},
	}
	return cbor.Encode(tmpData)
}

func (g *ShelleyGenesis) GenesisUtxos() ([]common.Utxo, error) {
	ret := []common.Utxo{}
	for address, amount := range g.InitialFunds {
		addrBytes, err := hex.DecodeString(address)
		if err != nil {
			return nil, err
		}
		tmpAddr, err := common.NewAddressFromBytes(addrBytes)
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			common.Utxo{
				Id: ShelleyTransactionInput{
					TxId:        common.Blake2b256Hash(addrBytes),
					OutputIndex: 0,
				},
				Output: ShelleyTransactionOutput{
					OutputAddress: tmpAddr,
					OutputAmount:  amount,
				},
			},
		)
	}
	return ret, nil
}

// GetInitialPools returns all initial stake pools with their delegators
func (g *ShelleyGenesis) GetInitialPools() (map[string]GenesisPool, map[string][]string, error) {
	poolStake := make(map[string][]string)
	for stakeAddr, poolId := range g.Staking.Stake {
		poolStake[poolId] = append(poolStake[poolId], stakeAddr)
	}
	return g.Staking.Pools, poolStake, nil
}

// GetPoolById returns a specific pool by its ID along with its delegators
func (g *ShelleyGenesis) GetPoolById(poolId string) (*GenesisPool, []string, error) {
	pool, exists := g.Staking.Pools[poolId]
	if !exists {
		return nil, nil, errors.New("pool not found")
	}

	var delegators []string
	for stakeAddr, pId := range g.Staking.Stake {
		if pId == poolId {
			delegators = append(delegators, stakeAddr)
		}
	}

	return &pool, delegators, nil
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
