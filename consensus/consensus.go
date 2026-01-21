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

// Package consensus provides Cardano consensus primitives for Ouroboros Praos.
// It supports leader election, block construction, and chain selection.
package consensus

import (
	"encoding/json"
	"io"
	"math/big"
	"os"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// VRFSigner produces VRF proofs for leader election
type VRFSigner interface {
	// Prove generates a VRF proof for the given input
	Prove(input []byte) (proof []byte, output []byte, err error)
	// PublicKey returns the VRF verification key
	PublicKey() []byte
}

// KESSigner signs with key-evolving signatures
type KESSigner interface {
	// Sign produces a KES signature for the given message at current period
	Sign(message []byte) (signature []byte, err error)
	// PublicKey returns the current KES verification key
	PublicKey() []byte
	// Period returns the current KES period
	Period() uint64
}

// ConsensusState provides consensus-relevant state
type ConsensusState interface {
	// PoolStake returns (pool stake, total stake) for leader eligibility
	PoolStake(poolId []byte) (uint64, uint64, error)
	// PoolVRFKeyHash returns the registered VRF key hash for a pool
	PoolVRFKeyHash(poolId []byte) ([]byte, error)
	// EpochNonce returns the nonce for the given epoch
	EpochNonce(epoch uint64) ([]byte, error)
	// ProtocolParameters returns consensus-relevant protocol params
	ProtocolParameters() ProtocolParameters
}

// ProtocolParameters contains consensus-relevant parameters
type ProtocolParameters struct {
	SecurityParam     uint64   // k - max rollback depth
	ActiveSlotCoeff   *big.Rat // f - active slot coefficient
	SlotLength        uint64   // milliseconds per slot
	EpochLength       uint64   // slots per epoch
	SlotsPerKESPeriod uint64   // slots per KES period
	MaxKESEvolutions  uint64   // maximum KES key evolutions
}

// NetworkConfig contains network-specific consensus configuration.
// Load from Shelley genesis JSON using NewNetworkConfigFromReader or NewNetworkConfigFromFile.
type NetworkConfig struct {
	Name              string            `json:"-"` // Not in genesis, set by caller
	SecurityParam     uint64            `json:"securityParam"`
	ActiveSlotCoeff   common.GenesisRat `json:"activeSlotsCoeff"`
	SlotLength        common.GenesisRat `json:"slotLength"`
	EpochLength       uint64            `json:"epochLength"`
	SlotsPerKESPeriod uint64            `json:"slotsPerKESPeriod"`
	MaxKESEvolutions  uint64            `json:"maxKESEvolutions"`
}

// ActiveSlotCoeffRat returns the active slot coefficient as a *big.Rat.
func (c *NetworkConfig) ActiveSlotCoeffRat() *big.Rat {
	return c.ActiveSlotCoeff.Rat
}

// SlotDuration returns the slot length as a time.Duration.
func (c *NetworkConfig) SlotDuration() time.Duration {
	if c.SlotLength.Rat == nil {
		return 0
	}
	// SlotLength is in seconds as a rational
	f, _ := c.SlotLength.Float64()
	return time.Duration(f * float64(time.Second))
}

// NewNetworkConfigFromReader creates a NetworkConfig from a Shelley genesis JSON reader.
func NewNetworkConfigFromReader(r io.Reader) (NetworkConfig, error) {
	var ret NetworkConfig
	dec := json.NewDecoder(r)
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	return ret, nil
}

// NewNetworkConfigFromFile creates a NetworkConfig from a Shelley genesis JSON file.
func NewNetworkConfigFromFile(path string) (NetworkConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return NetworkConfig{}, err
	}
	defer f.Close()
	return NewNetworkConfigFromReader(f)
}

// ConsensusHeader represents the consensus-relevant portion of a block header
type ConsensusHeader interface {
	// Slot returns the slot number
	Slot() uint64
	// BlockNumber returns the block height
	BlockNumber() uint64
	// PrevHash returns the previous block hash
	PrevHash() []byte
	// IssuerVKey returns the block issuer's verification key (cold key)
	IssuerVKey() []byte
	// VRFVKey returns the VRF verification key
	VRFVKey() []byte
	// VRFProof returns the VRF proof (for leader eligibility)
	VRFProof() []byte
	// VRFOutput returns the VRF output (certified random value)
	VRFOutput() []byte
	// KESSignature returns the KES signature over the header
	KESSignature() []byte
	// KESPeriod returns the KES period used for signing
	KESPeriod() uint64
	// Era returns the era identifier
	Era() uint8
}

// ChainTip represents the tip of a chain for selection purposes
type ChainTip interface {
	// Slot returns tip slot
	Slot() uint64
	// BlockNumber returns tip block height
	BlockNumber() uint64
	// VRFOutput returns the VRF output of the tip block
	VRFOutput() []byte
	// Density returns block density for the chain (blocks / slots) - for deep fork comparison
	Density(fromSlot uint64) float64
}

// ChainSelector determines the preferred chain
type ChainSelector interface {
	// Compare returns positive if a is preferred over b, negative if b preferred, 0 if equal
	Compare(a, b ChainTip) int
	// Preferred returns the preferred chain from a set of candidates
	Preferred(candidates []ChainTip) ChainTip
}
