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
	"reflect"
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
	Pools map[string]common.PoolRegistrationCertificate `json:"pools"`
	Stake map[string]string                             `json:"stake"`
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
		vrfBytes := pool.VrfKeyHash.Bytes()
		rewardAccountBytes := pool.RewardAccount.Bytes()
		cborPools[cbor.NewByteString(poolIdBytes)] = []any{
			pool.Cost,
			pool.Margin,
			pool.Pledge,
			pool.Operator.Bytes(),
			[]any{
				[]byte{0},
				rewardAccountBytes,
			},
			convertAddrKeyHashesToBytes(pool.PoolOwners),
			convertPoolRelays(pool.Relays),
			vrfBytes,
			pool.PoolMetadata,
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

func convertAddrKeyHashesToBytes(hashes []common.AddrKeyHash) [][]byte {
	result := make([][]byte, len(hashes))
	for i, h := range hashes {
		result[i] = h.Bytes()
	}
	return result
}

func convertPoolRelays(relays []common.PoolRelay) []any {
	result := make([]any, len(relays))
	for i, relay := range relays {
		switch relay.Type {
		case 0: // SingleHostAddr
			var ipv4, ipv6 []byte
			var port uint32
			if relay.Ipv4 != nil {
				ipv4 = relay.Ipv4.To4()
			}
			if relay.Ipv6 != nil {
				ipv6 = relay.Ipv6.To16()
			}
			if relay.Port != nil {
				port = *relay.Port
			}
			result[i] = map[string]any{
				"single host addr": []any{
					ipv4,
					ipv6,
					port,
				},
			}
		case 1: // SingleHostName
			var hostname string
			var port uint32
			if relay.Hostname != nil {
				hostname = *relay.Hostname
			}
			if relay.Port != nil {
				port = *relay.Port
			}
			result[i] = map[string]any{
				"single host name": []any{
					hostname,
					port,
				},
			}
		case 2: // MultiHostName
			var hostname string
			if relay.Hostname != nil {
				hostname = *relay.Hostname
			}
			result[i] = map[string]any{
				"multi host name": hostname,
			}
		default:
			result[i] = nil
		}
	}
	return result
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

func (g *ShelleyGenesis) getNetworkId() (uint8, error) {
	switch g.NetworkId {
	case "Mainnet":
		return common.AddressNetworkMainnet, nil
	case "Testnet":
		return common.AddressNetworkTestnet, nil
	default:
		return 0, errors.New("unknown network ID")
	}
}

func (g *ShelleyGenesis) InitialPools() (map[string]common.PoolRegistrationCertificate, map[string][]common.Address, error) {
	pools := make(map[string]common.PoolRegistrationCertificate)
	poolStake := make(map[string][]common.Address)

	if reflect.DeepEqual(g.Staking, GenesisStaking{}) {
		return pools, poolStake, nil
	}

	networkId, err := g.getNetworkId()
	if err != nil {
		return nil, nil, err
	}

	// Process all stake addresses
	for stakeAddr, poolId := range g.Staking.Stake {
		stakeKey, err := hex.DecodeString(stakeAddr)
		if err != nil {
			return nil, nil, errors.New("failed to decode stake key")
		}

		addr, err := common.NewAddressFromParts(
			common.AddressTypeNoneScript, // Script stake address
			networkId,
			nil,
			stakeKey,
		)
		if err != nil {
			return nil, nil, errors.New("failed to create address")
		}

		poolStake[poolId] = append(poolStake[poolId], addr)
	}

	// Process all stake pools
	for poolId, pool := range g.Staking.Pools {
		operatorBytes, err := hex.DecodeString(poolId)
		if err != nil {
			return nil, nil, errors.New("failed to decode pool ID")
		}

		pools[poolId] = common.PoolRegistrationCertificate{
			Operator:      common.Blake2b224(operatorBytes),
			VrfKeyHash:    pool.VrfKeyHash,
			Pledge:        pool.Pledge,
			Cost:          pool.Cost,
			Margin:        pool.Margin,
			RewardAccount: pool.RewardAccount,
			PoolOwners:    pool.PoolOwners,
			Relays:        pool.Relays,
			PoolMetadata:  pool.PoolMetadata,
		}
	}

	return pools, poolStake, nil
}

func (g *ShelleyGenesis) PoolById(poolId string) (*common.PoolRegistrationCertificate, []common.Address, error) {
	if len(poolId) != 56 {
		return nil, nil, errors.New("invalid pool ID length")
	}

	pool, exists := g.Staking.Pools[poolId]
	if !exists {
		return nil, nil, errors.New("pool  not found")
	}

	networkId, err := g.getNetworkId()
	if err != nil {
		return nil, nil, err
	}

	var delegators []common.Address
	for stakeAddr, pId := range g.Staking.Stake {
		if pId == poolId {
			stakeKey, err := hex.DecodeString(stakeAddr)
			if err != nil {
				return nil, nil, errors.New("failed to decode stake key")
			}

			addr, err := common.NewAddressFromParts(
				common.AddressTypeNoneScript,
				networkId,
				nil,
				stakeKey,
			)
			if err != nil {
				return nil, nil, errors.New("failed to create address")
			}

			delegators = append(delegators, addr)
		}
	}

	operatorBytes, err := hex.DecodeString(poolId)
	if err != nil {
		return nil, nil, errors.New("failed to decode pool operator key")
	}

	return &common.PoolRegistrationCertificate{
		Operator:      common.Blake2b224(operatorBytes),
		VrfKeyHash:    pool.VrfKeyHash,
		Pledge:        pool.Pledge,
		Cost:          pool.Cost,
		Margin:        pool.Margin,
		RewardAccount: pool.RewardAccount,
		PoolOwners:    pool.PoolOwners,
		Relays:        pool.Relays,
		PoolMetadata:  pool.PoolMetadata,
	}, delegators, nil
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
