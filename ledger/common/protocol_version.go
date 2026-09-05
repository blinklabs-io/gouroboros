// Copyright 2026 Blink Labs Software
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

package common

// Protocol version constants for Cardano hard forks.
// These correspond to the major protocol version numbers.
const (
	ProtocolVersionShelley   uint = 2
	ProtocolVersionAllegra   uint = 3
	ProtocolVersionMary      uint = 4
	ProtocolVersionAlonzo    uint = 6
	ProtocolVersionBabbage   uint = 8
	ProtocolVersionConway    uint = 9
	ProtocolVersionPlomin    uint = 10 // PV10 intra-era hard fork; enabled full governance (incl. treasury withdrawals)
	ProtocolVersionVanRossem uint = 11 // PV11 intra-era hard fork
	ProtocolVersionDijkstra  uint = 12 // PV12 Dijkstra hard fork
)

// IsProtocolVersionAtLeast checks if the given protocol version (major.minor)
// is at least the specified minimum major version.
func IsProtocolVersionAtLeast(major, minor, minMajor uint) bool {
	_ = minor // minor version not used in current comparison logic
	return major >= minMajor
}

// PoolAccountNetworkIdValidated reports whether the POOL rule validates the
// network id of a stake pool registration's reward account.
//
// Reference: eras/shelley/impl/src/Cardano/Ledger/Shelley/Era.hs,
// hardforkAlonzoValidatePoolAccountAddressNetID (pvMajor pv > natVersion @4),
// consumed by poolTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs.
//
// Note that this is major version 5, the first Alonzo protocol version, not
// ProtocolVersionAlonzo (6, the second one).
func PoolAccountNetworkIdValidated(major uint) bool {
	return major > 4
}

// DuplicateVrfKeysDisallowed reports whether the POOL rule rejects a stake pool
// registration whose VRF key hash is already registered by another pool.
//
// Reference: eras/shelley/impl/src/Cardano/Ledger/Shelley/Era.hs,
// hardforkConwayDisallowDuplicatedVRFKeys (pvMajor pv > natVersion @10),
// consumed by poolTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs.
func DuplicateVrfKeysDisallowed(major uint) bool {
	return major > 10
}

// PoolMetadataHashRestricted reports whether the POOL rule rejects a stake pool
// registration whose metadata hash is longer than 32 bytes.
//
// Reference: eras/shelley/impl/src/Cardano/Ledger/Shelley/SoftForks.hs,
// restrictPoolMetadataHash (pv > ProtVer (natVersion @4) 0), consumed by
// poolTransition in eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs.
//
// The reference compares the whole protocol version, so a 4.x version with a
// non-zero minor would also restrict. Only the major version reaches the POOL
// rule here, and no 4.x protocol version with a non-zero minor exists, so the
// comparison is on the major version alone.
func PoolMetadataHashRestricted(major uint) bool {
	return major > 4
}
