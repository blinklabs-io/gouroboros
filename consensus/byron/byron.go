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

// Package byron provides Ouroboros BFT (Byzantine Fault Tolerant) consensus
// implementation for the Byron era.
//
// Byron uses a simpler consensus mechanism than Praos:
//   - Leader schedule is deterministic based on genesis delegates
//   - Uses Ed25519 signatures (not KES)
//   - No VRF for randomness
//   - Round-robin style slot leadership among delegates
package byron

import (
	"fmt"
	"math"
	"time"

	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
)

// ByronConfig contains Byron-specific consensus configuration.
// Parameters should be loaded from Byron genesis configuration.
type ByronConfig struct {
	ProtocolMagic    uint32
	SlotsPerEpoch    uint64
	SlotDuration     time.Duration
	SecurityParam    uint64
	NumGenesisKeys   int
	GenesisKeyHashes [][]byte // Hashes of genesis delegate keys
	TxFeePolicy      ByronTxFeePolicy
}

// ByronTxFeePolicy contains the transaction fee policy parameters.
// Byron uses a linear fee formula: fee = (summand + multiplier * txSize) / 10^9
// The values are stored as integers scaled by 10^9 for precision.
type ByronTxFeePolicy struct {
	// Summand is the base fee component (scaled by 10^9)
	Summand int64
	// Multiplier is the per-byte fee component (scaled by 10^9)
	Multiplier int64
}

// ByronFeeDivisor is the divisor used in the Byron fee calculation.
// Fee values in genesis are scaled by this factor (10^9).
// The summand represents base fee in lovelace * 10^9.
// The multiplier represents per-byte fee in lovelace * 10^9.
const ByronFeeDivisor = 1_000_000_000 // 10^9

// CalculateMinFee calculates the minimum required fee for a Byron transaction
// of the given size in bytes. Returns the fee in lovelace.
//
// The Byron fee formula is: fee = ceiling((summand + multiplier * txSize) / 10^9)
func (p *ByronTxFeePolicy) CalculateMinFee(txSizeBytes uint64) uint64 {
	if p.Summand == 0 && p.Multiplier == 0 {
		return 0 // No fee policy configured
	}

	// Reject negative policy fields - fee parameters should never be negative
	// This also avoids undefined behavior when negating math.MinInt64
	if p.Summand < 0 || p.Multiplier < 0 {
		return 0
	}

	// Pre-multiplication overflow check for p.Multiplier * int64(txSizeBytes)
	// Since we've rejected negative values above, p.Multiplier is non-negative
	var total int64
	if p.Multiplier != 0 {
		// #nosec G115 -- p.Multiplier is guaranteed non-negative (checked above)
		multiplierUint := uint64(p.Multiplier)
		// Check: txSizeBytes > MaxInt64 / multiplier would overflow
		if txSizeBytes > uint64(math.MaxInt64)/multiplierUint {
			// Overflow would occur - return maximum possible fee
			// This is a deterministic fallback for extreme edge cases
			total = math.MaxInt64
		} else {
			// Safe to compute: summand + multiplier * txSize
			// #nosec G115 -- overflow checked above
			total = p.Summand + p.Multiplier*int64(txSizeBytes)
		}
	} else {
		// Multiplier is 0, so just use summand
		total = p.Summand
	}

	// Ceiling division using unsigned arithmetic to avoid overflow when total == math.MaxInt64
	// Formula: (total + divisor - 1) / divisor, but computed with uint64 to prevent wraparound
	// total is guaranteed non-negative due to early return for negative policy fields
	// #nosec G115 -- total is guaranteed non-negative due to early return for negative policy fields
	fee := (uint64(total) + uint64(ByronFeeDivisor) - 1) / uint64(ByronFeeDivisor)

	return fee
}

// CalculateMinFee calculates the minimum required fee for a Byron transaction
// of the given size in bytes using this config's fee policy.
func (c *ByronConfig) CalculateMinFee(txSizeBytes uint64) uint64 {
	return c.TxFeePolicy.CalculateMinFee(txSizeBytes)
}

// SlotToEpoch converts a slot number to an epoch number.
func (c *ByronConfig) SlotToEpoch(slot uint64) uint64 {
	if c.SlotsPerEpoch == 0 {
		return 0
	}
	return slot / c.SlotsPerEpoch
}

// EpochFirstSlot returns the first slot of an epoch.
func (c *ByronConfig) EpochFirstSlot(epoch uint64) uint64 {
	return epoch * c.SlotsPerEpoch
}

// IsEpochBoundarySlot returns true if the slot is at an epoch boundary.
func (c *ByronConfig) IsEpochBoundarySlot(slot uint64) bool {
	if c.SlotsPerEpoch == 0 {
		return false
	}
	return slot%c.SlotsPerEpoch == 0
}

// SlotLeader returns the index and key hash of the expected slot leader for a given slot.
// Byron uses OBFT round-robin assignment: leader_index = slot % numDelegates.
// Returns the delegate index and key hash, or (-1, nil) if no delegates are configured.
func (c *ByronConfig) SlotLeader(slot uint64) (int, []byte) {
	if len(c.GenesisKeyHashes) == 0 {
		return -1, nil
	}
	// #nosec G115 -- len() is always non-negative and fits in int
	index := int(slot % uint64(len(c.GenesisKeyHashes)))
	return index, c.GenesisKeyHashes[index]
}

// NewByronConfigFromGenesis creates a ByronConfig from a ByronGenesis.
// This extracts and computes all necessary consensus parameters including:
//   - Protocol magic
//   - Slots per epoch (computed from security parameter K: 10 * K)
//   - Slot duration
//   - Security parameter (K)
//   - Genesis delegate key hashes (sorted for OBFT slot leader assignment)
func NewByronConfigFromGenesis(genesis *ledgerbyron.ByronGenesis) (ByronConfig, error) {
	// Validate security parameter K
	if genesis.ProtocolConsts.K < 0 {
		return ByronConfig{}, fmt.Errorf("invalid security parameter K: %d (must be non-negative)", genesis.ProtocolConsts.K)
	}

	// Validate protocol magic fits in uint32
	if genesis.ProtocolConsts.ProtocolMagic < 0 || genesis.ProtocolConsts.ProtocolMagic > math.MaxUint32 {
		return ByronConfig{}, fmt.Errorf("invalid protocol magic: %d (must be 0 to %d)", genesis.ProtocolConsts.ProtocolMagic, math.MaxUint32)
	}

	// Extract genesis delegate key hashes
	keyHashes, err := genesis.GenesisDelegateKeyHashes()
	if err != nil {
		return ByronConfig{}, err
	}

	// Convert to [][]byte for storage
	keyHashBytes := make([][]byte, len(keyHashes))
	for i, h := range keyHashes {
		keyHashBytes[i] = h.Bytes()
	}

	// Byron slots per epoch = 10 * K (security parameter)
	// This is the standard Byron formula
	// #nosec G115 -- K is validated to be non-negative above
	k := uint64(genesis.ProtocolConsts.K)
	slotsPerEpoch := 10 * k

	// Slot duration is in milliseconds in the genesis file
	slotDuration := time.Duration(genesis.BlockVersionData.SlotDuration) * time.Millisecond

	// Extract fee policy
	feePolicy := ByronTxFeePolicy{
		Summand:    genesis.BlockVersionData.TxFeePolicy.Summand,
		Multiplier: genesis.BlockVersionData.TxFeePolicy.Multiplier,
	}

	return ByronConfig{
		// #nosec G115 -- ProtocolMagic is validated to fit in uint32 above
		ProtocolMagic:    uint32(genesis.ProtocolConsts.ProtocolMagic),
		SlotsPerEpoch:    slotsPerEpoch,
		SlotDuration:     slotDuration,
		SecurityParam:    k,
		NumGenesisKeys:   len(keyHashes),
		GenesisKeyHashes: keyHashBytes,
		TxFeePolicy:      feePolicy,
	}, nil
}
