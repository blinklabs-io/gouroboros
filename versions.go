// Copyright 2023 Blink Labs Software
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

package ouroboros

// The NtC protocol versions have the 15th bit set in the handshake
const protocolVersionNtCFlag = 0x8000

// Most of these are enabled in all of the protocol versions that we support, but
// they are here for completeness
type ProtocolVersionNtC struct {
	EnableLocalQueryProtocol     bool
	EnableShelleyEra             bool
	EnableAllegraEra             bool
	EnableMaryEra                bool
	EnableAlonzoEra              bool
	EnableBabbageEra             bool
	EnableConwayEra              bool
	EnableLocalTxMonitorProtocol bool
}

// Map of NtC protocol versions to protocol features
//
// We don't bother supporting NtC protocol versions before 9 (when Alonzo was enabled)
var protocolVersionMapNtC = map[uint16]ProtocolVersionNtC{
	9: ProtocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	// added GetChainBlockNo and GetChainPoint queries
	10: ProtocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	// added GetRewardInfoPools Block query
	11: ProtocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	12: ProtocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},
	13: ProtocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added GetPoolDistr, GetPoolState, @GetSnapshots
	14: ProtocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added query param to handshake
	15: ProtocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	16: ProtocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableConwayEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},
}

type ProtocolVersionNtN struct {
	// Most of these are enabled in all of the protocol versions that we support, but
	// they are here for completeness
	EnableShelleyEra          bool
	EnableKeepAliveProtocol   bool
	EnableAllegraEra          bool
	EnableMaryEra             bool
	EnableAlonzoEra           bool
	EnableBabbageEra          bool
	EnableConwayEra           bool
	EnableFullDuplex          bool
	EnablePeerSharingProtocol bool
}

// Map of NtN protocol versions to protocol features
//
// We don't bother supporting NtN protocol versions before 7 (when Alonzo was enabled)
var protocolVersionMapNtN = map[uint16]ProtocolVersionNtN{
	7: ProtocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
	},
	8: ProtocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
	},
	9: ProtocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
		EnableBabbageEra:        true,
	},
	10: ProtocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
		EnableBabbageEra:        true,
		EnableFullDuplex:        true,
	},
	11: ProtocolVersionNtN{
		EnableShelleyEra:          true,
		EnableKeepAliveProtocol:   true,
		EnableAllegraEra:          true,
		EnableMaryEra:             true,
		EnableAlonzoEra:           true,
		EnableBabbageEra:          true,
		EnableFullDuplex:          true,
		EnablePeerSharingProtocol: true,
	},
	12: ProtocolVersionNtN{
		EnableShelleyEra:          true,
		EnableKeepAliveProtocol:   true,
		EnableAllegraEra:          true,
		EnableMaryEra:             true,
		EnableAlonzoEra:           true,
		EnableBabbageEra:          true,
		EnableConwayEra:           true,
		EnableFullDuplex:          true,
		EnablePeerSharingProtocol: true,
	},
}

// GetProtocolVersionNtC returns a list of supported NtC protocol versions
func GetProtocolVersionsNtC() []uint16 {
	versions := []uint16{}
	for key := range protocolVersionMapNtC {
		versions = append(versions, key+protocolVersionNtCFlag)
	}
	return versions
}

// GetProtocolVersionNtC returns the protocol version config for the specified NtC protocol version
func GetProtocolVersionNtC(version uint16) ProtocolVersionNtC {
	if version > protocolVersionNtCFlag {
		version = version - protocolVersionNtCFlag
	}
	return protocolVersionMapNtC[version]
}

// GetProtocolVersionNtN returns a list of supported NtN protocol versions
func GetProtocolVersionsNtN() []uint16 {
	versions := []uint16{}
	for key := range protocolVersionMapNtN {
		versions = append(versions, key)
	}
	return versions
}

// GetProtocolVersionNtN returns the protocol version config for the specified NtN protocol version
func GetProtocolVersionNtN(version uint16) ProtocolVersionNtN {
	return protocolVersionMapNtN[version]
}
