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
	"time"
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
