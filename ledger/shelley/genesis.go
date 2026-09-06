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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	// ExtraConfig carries the Musashi genesis injection block. Its values are
	// applied by the bootstrap helpers, but are not part of the Shelley genesis
	// CBOR encoding.
	ExtraConfig *ShelleyGenesisExtraConfig `json:"extraConfig,omitempty" cbor:"-"`
}

type GenesisStaking struct {
	Pools map[string]common.PoolRegistrationCertificate `json:"pools"`
	Stake map[string]string                             `json:"stake"`
}

// ShelleyGenesisExtraConfig contains the bootstrap values injected by the
// Musashi Leios prototype genesis.
type ShelleyGenesisExtraConfig struct {
	InitialFunds     shelleyExtraFunds `json:"initialFunds"`
	StakeCredentials shelleyExtraCreds `json:"stakeCredentials"`
	StakePools       shelleyExtraPools `json:"stakePools"`
}

type shelleyExtraFunds struct {
	Data map[string]uint64 `json:"data"`
}

type shelleyExtraCreds struct {
	Data map[string]string `json:"data"`
}

type shelleyExtraPools struct {
	Data map[string]ShelleyGenesisExtraPool `json:"data"`
}

// ShelleyGenesisExtraPool is an injected initial pool registration.
type ShelleyGenesisExtraPool struct {
	Vrf            string                      `json:"vrf"`
	Pledge         uint64                      `json:"pledge"`
	Cost           uint64                      `json:"cost"`
	Margin         json.RawMessage             `json:"margin"`
	LeiosKey       json.RawMessage             `json:"leiosKey"`
	Metadata       json.RawMessage             `json:"metadata"`
	Owners         json.RawMessage             `json:"owners"`
	Relays         json.RawMessage             `json:"relays"`
	PoolId         string                      `json:"poolId"`
	AccountAddress shelleyExtraPoolAccountAddr `json:"accountAddress"`
	Unknown        map[string]json.RawMessage  `json:"-"`
}

// UnmarshalJSON retains unmodelled pool properties. This lets the strict
// top-level genesis decoder continue to accept future Musashi pool fields.
func (p *ShelleyGenesisExtraPool) UnmarshalJSON(data []byte) error {
	type extraPool ShelleyGenesisExtraPool
	var known extraPool
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&known); err != nil {
		return err
	}
	*p = ShelleyGenesisExtraPool(known)

	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, key := range []string{
		"vrf", "pledge", "cost", "margin", "leiosKey", "metadata",
		"owners", "relays", "poolId", "accountAddress",
	} {
		delete(all, key)
	}
	if len(all) > 0 {
		p.Unknown = all
	}
	return nil
}

type shelleyExtraPoolAccountAddr struct {
	Credential shelleyExtraPoolCredential `json:"credential"`
	Network    string                     `json:"network"`
}

type shelleyExtraPoolCredential struct {
	KeyHash    string `json:"keyHash"`
	ScriptHash string `json:"scriptHash"`
}

func (g *ShelleyGenesis) effectiveInitialFunds() map[string]uint64 {
	if g.ExtraConfig == nil || len(g.ExtraConfig.InitialFunds.Data) == 0 {
		return g.InitialFunds
	}
	out := make(
		map[string]uint64,
		len(g.InitialFunds)+len(g.ExtraConfig.InitialFunds.Data),
	)
	for key, amount := range g.InitialFunds {
		out[key] = amount
	}
	for key, amount := range g.ExtraConfig.InitialFunds.Data {
		out[key] = amount
	}
	return out
}

func (g *ShelleyGenesis) effectiveStake() map[string]string {
	if g.ExtraConfig == nil || len(g.ExtraConfig.StakeCredentials.Data) == 0 {
		return g.Staking.Stake
	}
	out := make(
		map[string]string,
		len(g.Staking.Stake)+len(g.ExtraConfig.StakeCredentials.Data),
	)
	for credential, poolID := range g.Staking.Stake {
		out[credential] = poolID
	}
	for credential, poolID := range g.ExtraConfig.StakeCredentials.Data {
		out[credential] = poolID
	}
	return out
}

func (g *ShelleyGenesis) effectivePools() (map[string]common.PoolRegistrationCertificate, error) {
	if g.ExtraConfig == nil || len(g.ExtraConfig.StakePools.Data) == 0 {
		return g.Staking.Pools, nil
	}
	out := make(
		map[string]common.PoolRegistrationCertificate,
		len(g.Staking.Pools)+len(g.ExtraConfig.StakePools.Data),
	)
	for poolID, pool := range g.Staking.Pools {
		out[poolID] = pool
	}
	for poolID, extraPool := range g.ExtraConfig.StakePools.Data {
		operator, err := hex.DecodeString(poolID)
		if err != nil {
			return nil, err
		}
		if len(operator) != common.Blake2b224Size {
			return nil, errors.New("invalid extraConfig pool operator length")
		}
		vrf, err := hex.DecodeString(extraPool.Vrf)
		if err != nil {
			return nil, err
		}
		if len(vrf) != common.Blake2b256Size {
			return nil, errors.New("invalid extraConfig pool vrf length")
		}

		credential := extraPool.AccountAddress.Credential
		if credential.ScriptHash != "" {
			return nil, errors.New(
				"extraConfig pool reward account script credentials are not supported",
			)
		}
		if credential.KeyHash == "" {
			return nil, errors.New(
				"extraConfig pool reward account key hash is required",
			)
		}
		reward, err := hex.DecodeString(credential.KeyHash)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid extraConfig pool reward account key hash: %w",
				err,
			)
		}
		if len(reward) != common.Blake2b224Size {
			return nil, errors.New("invalid extraConfig pool reward account length")
		}
		rewardAccount := common.Blake2b224(reward)

		var leiosKey *common.LeiosKey
		if err := decodeExtraPoolField(
			extraPool.LeiosKey,
			"leiosKey",
			&leiosKey,
		); err != nil {
			return nil, err
		}

		var metadata *common.PoolMetadata
		if err := decodeExtraPoolField(
			extraPool.Metadata,
			"metadata",
			&metadata,
		); err != nil {
			return nil, err
		}

		var owners []common.AddrKeyHash
		if err := decodeExtraPoolField(
			extraPool.Owners,
			"owners",
			&owners,
		); err != nil {
			return nil, err
		}

		var relays []common.PoolRelay
		if err := decodeExtraPoolField(
			extraPool.Relays,
			"relays",
			&relays,
		); err != nil {
			return nil, err
		}
		for idx := range relays {
			if err := validateExtraPoolRelay(relays[idx]); err != nil {
				return nil, fmt.Errorf(
					"invalid extraConfig pool relays[%d]: %w",
					idx,
					err,
				)
			}
		}

		margin := common.NewGenesisRat(0, 1)
		if marginJSON := bytes.TrimSpace(extraPool.Margin); len(marginJSON) > 0 &&
			!bytes.Equal(marginJSON, []byte("null")) {
			margin, err = common.ParsePoolMarginJSON(extraPool.Margin)
			if err != nil {
				return nil, err
			}
		}
		out[poolID] = common.PoolRegistrationCertificate{
			Operator:      common.Blake2b224(operator),
			VrfKeyHash:    common.NewBlake2b256(vrf),
			LeiosKey:      leiosKey,
			Pledge:        extraPool.Pledge,
			Cost:          extraPool.Cost,
			Margin:        margin,
			RewardAccount: rewardAccount,
			PoolOwners:    owners,
			Relays:        relays,
			PoolMetadata:  metadata,
		}
	}
	return out, nil
}

func decodeExtraPoolField(
	raw json.RawMessage,
	name string,
	dest any,
) error {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf(
			"invalid extraConfig pool %s: %w",
			name,
			err,
		)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf(
			"invalid extraConfig pool %s: trailing JSON value",
			name,
		)
	}
	return nil
}

func validateExtraPoolRelay(relay common.PoolRelay) error {
	switch relay.Type {
	case common.PoolRelayTypeSingleHostAddress:
		if relay.Hostname != nil {
			return errors.New("single-host-address relay cannot have hostname")
		}
	case common.PoolRelayTypeSingleHostName:
		if relay.Hostname == nil || *relay.Hostname == "" {
			return errors.New("single-host-name relay requires hostname")
		}
		if relay.Ipv4 != nil || relay.Ipv6 != nil {
			return errors.New("single-host-name relay cannot have IP addresses")
		}
	case common.PoolRelayTypeMultiHostName:
		if relay.Hostname == nil || *relay.Hostname == "" {
			return errors.New("multi-host-name relay requires hostname")
		}
		if relay.Port != nil || relay.Ipv4 != nil || relay.Ipv6 != nil {
			return errors.New(
				"multi-host-name relay cannot have port or IP addresses",
			)
		}
	default:
		return fmt.Errorf("unsupported relay type %d", relay.Type)
	}
	return nil
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
		cborStake[cbor.NewByteString(stakeAddrBytes)] = cbor.NewByteString(
			poolIdBytes,
		)
	}

	networkId, err := g.getNetworkId()
	if err != nil {
		return nil, err
	}

	slotLengthMs := &big.Rat{}
	tmpData := []any{
		[]any{
			g.SystemStart.Year(),
			g.SystemStart.YearDay(),
			g.SystemStart.Nanosecond() * 1000,
		},
		g.NetworkMagic,
		networkId,
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
	for address, amount := range g.effectiveInitialFunds() {
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

	effectiveStake := g.effectiveStake()
	effectivePools, err := g.effectivePools()
	if err != nil {
		return nil, nil, err
	}
	if len(effectiveStake) == 0 && len(effectivePools) == 0 {
		return pools, poolStake, nil
	}

	networkId, err := g.getNetworkId()
	if err != nil {
		return nil, nil, err
	}

	// Process all stake addresses
	for stakeAddr, poolId := range effectiveStake {
		stakeKey, err := hex.DecodeString(stakeAddr)
		if err != nil {
			return nil, nil, errors.New("failed to decode stake key")
		}

		addr, err := common.NewAddressFromParts(
			common.AddressTypeNoneKey,
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
	for poolId, pool := range effectivePools {
		operatorBytes, err := hex.DecodeString(poolId)
		if err != nil {
			return nil, nil, errors.New("failed to decode pool ID")
		}

		pools[poolId] = common.PoolRegistrationCertificate{
			Operator:      common.Blake2b224(operatorBytes),
			VrfKeyHash:    pool.VrfKeyHash,
			LeiosKey:      pool.LeiosKey,
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

func (g *ShelleyGenesis) PoolById(
	poolId string,
) (*common.PoolRegistrationCertificate, []common.Address, error) {
	if len(poolId) != 56 {
		return nil, nil, errors.New("invalid pool ID length")
	}

	effectivePools, err := g.effectivePools()
	if err != nil {
		return nil, nil, err
	}
	pool, exists := effectivePools[poolId]
	if !exists {
		return nil, nil, errors.New("pool  not found")
	}

	networkId, err := g.getNetworkId()
	if err != nil {
		return nil, nil, err
	}

	var delegators []common.Address
	for stakeAddr, pId := range g.effectiveStake() {
		if pId == poolId {
			stakeKey, err := hex.DecodeString(stakeAddr)
			if err != nil {
				return nil, nil, errors.New("failed to decode stake key")
			}

			addr, err := common.NewAddressFromParts(
				common.AddressTypeNoneKey,
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
		LeiosKey:      pool.LeiosKey,
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
