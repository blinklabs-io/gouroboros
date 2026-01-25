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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ByronGenesisFtsSeed struct {
	Value    string
	IsObject bool
}
type ByronGenesis struct {
	AvvmDistr        map[string]string                      `json:"avvmDistr"`
	BlockVersionData ByronGenesisBlockVersionData           `json:"blockVersionData"`
	FtsSeed          ByronGenesisFtsSeed                    `json:"ftsSeed"`
	ProtocolConsts   ByronGenesisProtocolConsts             `json:"protocolConsts"`
	StartTime        int                                    `json:"startTime"`
	BootStakeholders map[string]int                         `json:"bootStakeholders"`
	HeavyDelegation  map[string]ByronGenesisHeavyDelegation `json:"heavyDelegation"`
	NonAvvmBalances  map[string]string                      `json:"nonAvvmBalances"`
	VssCerts         map[string]ByronGenesisVssCert         `json:"vssCerts"`
}

type ByronGenesisBlockVersionData struct {
	HeavyDelThd       int                                      `json:"heavyDelThd,string"`
	MaxBlockSize      int                                      `json:"maxBlockSize,string"`
	MaxHeaderSize     int                                      `json:"maxHeaderSize,string"`
	MaxProposalSize   int                                      `json:"maxProposalSize,string"`
	MaxTxSize         int                                      `json:"maxTxSize,string"`
	MpcThd            int                                      `json:"mpcThd,string"`
	ScriptVersion     int                                      `json:"scriptVersion"`
	SlotDuration      int                                      `json:"slotDuration,string"`
	SoftforkRule      ByronGenesisBlockVersionDataSoftforkRule `json:"softforkRule"`
	TxFeePolicy       ByronGenesisBlockVersionDataTxFeePolicy  `json:"txFeePolicy"`
	UnlockStakeEpoch  uint64                                   `json:"unlockStakeEpoch,string"`
	UpdateImplicit    int                                      `json:"updateImplicit,string"`
	UpdateProposalThd int                                      `json:"updateProposalThd,string"`
	UpdateVoteThd     int                                      `json:"updateVoteThd,string"`
}

type ByronGenesisBlockVersionDataSoftforkRule struct {
	InitThd      int `json:"initThd,string"`
	MinThd       int `json:"minThd,string"`
	ThdDecrement int `json:"thdDecrement,string"`
}

type ByronGenesisBlockVersionDataTxFeePolicy struct {
	Multiplier int64 `json:"multiplier,string"`
	Summand    int64 `json:"summand,string"`
}

type ByronGenesisProtocolConsts struct {
	K             int `json:"k"`
	ProtocolMagic int `json:"protocolMagic"`
	VssMinTTL     int `json:"vssMinTtl"`
	VssMaxTTL     int `json:"vssMaxTtl"`
}

type ByronGenesisHeavyDelegation struct {
	Cert       string `json:"cert"`
	DelegatePk string `json:"delegatePk"`
	IssuerPk   string `json:"issuerPk"`
	Omega      int    `json:"omega"`
}

type ByronGenesisVssCert struct {
	ExpiryEpoch int    `json:"expiryEpoch"`
	Signature   string `json:"signature"`
	SigningKey  string `json:"signingKey"`
	VssKey      string `json:"vssKey"`
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
	ret := []common.Utxo{}
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
		addrBytes, err := tmpAddr.Bytes()
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
	ret := []common.Utxo{}
	for address, amount := range g.NonAvvmBalances {
		tmpAddr, err := common.NewAddress(address)
		if err != nil {
			return nil, err
		}
		tmpAmount, err := strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return nil, err
		}
		addrBytes, err := tmpAddr.Bytes()
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

// UnmarshalJSON accepts: "string", {}, or null
// Tries each expected shape and accepts the first that parses cleanly
func (f *ByronGenesisFtsSeed) UnmarshalJSON(b []byte) error {
	// Try string
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		f.Value = s
		f.IsObject = false
		return nil
	}

	// Try empty object
	var m map[string]any
	if err := json.Unmarshal(b, &m); err == nil {
		if len(m) == 0 {
			f.Value = ""
			f.IsObject = true
			return nil
		}
		return errors.New("ftsSeed: non-empty object not supported")
	}

	// Try null
	var v any
	if err := json.Unmarshal(b, &v); err == nil && v == nil {
		f.Value = ""
		f.IsObject = false
		return nil
	}

	return errors.New("ftsSeed: expected string, empty object, or null")
}

func (f ByronGenesisFtsSeed) MarshalJSON() ([]byte, error) {
	if f.IsObject {
		// serialize as empty object
		return []byte(`{}`), nil
	}
	if f.Value == "" {
		// serialize as null
		return []byte(`null`), nil
	}
	return json.Marshal(f.Value)
}

// GenesisDelegateKeyHashes returns the Blake2b-224 hashes of the genesis delegate
// keys, sorted for deterministic OBFT slot leader assignment.
//
// The key hashes are extracted directly from the HeavyDelegation map keys,
// which are hex-encoded Blake2b-224 hashes (same format as BootStakeholders).
// The ordering is by the hex-encoded key hash (lexicographic), which matches
// how Byron determines the slot leader rotation order.
func (g *ByronGenesis) GenesisDelegateKeyHashes() ([]common.Blake2b224, error) {
	if len(g.HeavyDelegation) == 0 {
		return nil, nil
	}

	// Collect the map keys (which are hex-encoded Blake2b224 hashes)
	hexKeys := make([]string, 0, len(g.HeavyDelegation))
	for keyHex := range g.HeavyDelegation {
		hexKeys = append(hexKeys, keyHex)
	}

	// Sort by hex representation for deterministic ordering
	slices.Sort(hexKeys)

	// Parse each hex key into a Blake2b224
	result := make([]common.Blake2b224, len(hexKeys))
	for i, keyHex := range hexKeys {
		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid hex key in HeavyDelegation: %s: %w", keyHex, err)
		}
		if len(keyBytes) != common.Blake2b224Size {
			return nil, fmt.Errorf("invalid key hash length in HeavyDelegation: expected %d bytes, got %d for key %s",
				common.Blake2b224Size, len(keyBytes), keyHex)
		}
		result[i] = common.NewBlake2b224(keyBytes)
	}

	return result, nil
}
