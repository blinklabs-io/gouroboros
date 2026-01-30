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
	ProtocolVersionShelley    uint = 2
	ProtocolVersionAllegra    uint = 3
	ProtocolVersionMary       uint = 4
	ProtocolVersionAlonzo     uint = 6
	ProtocolVersionBabbage    uint = 8
	ProtocolVersionConway     uint = 9
	ProtocolVersionConwayPlus uint = 10
	ProtocolVersionVanRossem  uint = 11 // PV11 intra-era hard fork
)

// IsProtocolVersionAtLeast checks if the given protocol version (major.minor)
// is at least the specified minimum major version.
func IsProtocolVersionAtLeast(major, minor, minMajor uint) bool {
	_ = minor // minor version not used in current comparison logic
	return major >= minMajor
}
