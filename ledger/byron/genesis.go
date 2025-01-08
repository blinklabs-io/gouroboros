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

package byron

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strconv"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ByronGenesis struct {
	AvvmDistr        map[string]string
	BlockVersionData ByronGenesisBlockVersionData
	FtsSeed          string
	ProtocolConsts   ByronGenesisProtocolConsts
	StartTime        int
	BootStakeholders map[string]int
	HeavyDelegation  map[string]ByronGenesisHeavyDelegation
	NonAvvmBalances  map[string]string
	VssCerts         map[string]ByronGenesisVssCert
}

type ByronGenesisBlockVersionData struct {
	HeavyDelThd       int `json:",string"`
	MaxBlockSize      int `json:",string"`
	MaxHeaderSize     int `json:",string"`
	MaxProposalSize   int `json:",string"`
	MaxTxSize         int `json:",string"`
	MpcThd            int `json:",string"`
	ScriptVersion     int
	SlotDuration      int `json:",string"`
	SoftforkRule      ByronGenesisBlockVersionDataSoftforkRule
	TxFeePolicy       ByronGenesisBlockVersionDataTxFeePolicy
	UnlockStakeEpoch  uint64 `json:",string"`
	UpdateImplicit    int    `json:",string"`
	UpdateProposalThd int    `json:",string"`
	UpdateVoteThd     int    `json:",string"`
}

type ByronGenesisBlockVersionDataSoftforkRule struct {
	InitThd      int `json:",string"`
	MinThd       int `json:",string"`
	ThdDecrement int `json:",string"`
}

type ByronGenesisBlockVersionDataTxFeePolicy struct {
	Multiplier int `json:",string"`
	Summand    int `json:",string"`
}

type ByronGenesisProtocolConsts struct {
	K             int
	ProtocolMagic int
	VssMinTTL     int
	VssMaxTTL     int
}

type ByronGenesisHeavyDelegation struct {
	Cert       string
	DelegatePk string
	IssuerPk   string
	Omega      int
}

type ByronGenesisVssCert struct {
	ExpiryEpoch int
	Signature   string
	SigningKey  string
	VssKey      string
}

func (g *ByronGenesis) GenesisUtxos() ([]common.Utxo, error) {
	avvmUtxos, err := g.avvmUtxos()
	if err != nil {
		return nil, err
	}
	nonAvvmUtxos, err := g.nonAvvmUtxos()
	if err != nil {
		return nil, err
	}
	ret := slices.Concat(
		avvmUtxos,
		nonAvvmUtxos,
	)
	return ret, nil
}

func (g *ByronGenesis) avvmUtxos() ([]common.Utxo, error) {
	var ret []common.Utxo
	for pubkey, amount := range g.AvvmDistr {
		// Build address from redeem pubkey
		pubkeyBytes, err := base64.URLEncoding.DecodeString(pubkey)
		if err != nil {
			return nil, err
		}
		tmpAddr, err := common.NewByronAddressRedeem(
			pubkeyBytes,
			// XXX: do we need to specify the network ID?
			common.ByronAddressAttributes{},
		)
		if err != nil {
			return nil, err
		}
		tmpAmount, err := strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return nil, err
		}
		addrBytes, err := cbor.Encode(tmpAddr)
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			common.Utxo{
				Id: ByronTransactionInput{
					TxId:        common.Blake2b256Hash(addrBytes),
					OutputIndex: 0,
				},
				Output: ByronTransactionOutput{
					OutputAddress: tmpAddr,
					OutputAmount:  tmpAmount,
				},
			},
		)
	}
	return ret, nil
}

func (g *ByronGenesis) nonAvvmUtxos() ([]common.Utxo, error) {
	var ret []common.Utxo
	for address, amount := range g.NonAvvmBalances {
		tmpAddr, err := common.NewAddress(address)
		if err != nil {
			return nil, err
		}
		tmpAmount, err := strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return nil, err
		}
		addrBytes, err := cbor.Encode(tmpAddr)
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			common.Utxo{
				Id: ByronTransactionInput{
					TxId:        common.Blake2b256Hash(addrBytes),
					OutputIndex: 0,
				},
				Output: ByronTransactionOutput{
					OutputAddress: tmpAddr,
					OutputAmount:  tmpAmount,
				},
			},
		)
	}
	return ret, nil
}

func NewByronGenesisFromReader(r io.Reader) (ByronGenesis, error) {
	var ret ByronGenesis
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func NewByronGenesisFromFile(path string) (ByronGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return ByronGenesis{}, err
	}
	defer f.Close()
	return NewByronGenesisFromReader(f)
}
