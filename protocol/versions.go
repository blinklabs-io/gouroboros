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

package protocol

import "sort"

// The NtC protocol versions have the 15th bit set in the handshake
const ProtocolVersionNtCOffset = 0x8000

type NewVersionDataFromCborFunc func([]byte) (VersionData, error)

type ProtocolVersionMap map[uint16]VersionData

type ProtocolVersion struct {
	NewVersionDataFromCborFunc NewVersionDataFromCborFunc
	EnableShelleyEra           bool
	EnableAllegraEra           bool
	EnableMaryEra              bool
	EnableAlonzoEra            bool
	EnableBabbageEra           bool
	EnableConwayEra            bool
	// NtC only
	EnableLocalQueryProtocol     bool
	EnableLocalTxMonitorProtocol bool
	// NtN only
	EnableKeepAliveProtocol   bool
	EnableFullDuplex          bool
	EnablePeerSharingProtocol bool
	PeerSharingUseV11         bool
}

var protocolVersions = map[uint16]ProtocolVersion{
	// NtC protocol versions
	//
	// We don't bother supporting NtC protocol versions before 9 (when Alonzo was enabled)

	(9 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	// added GetChainBlockNo and GetChainPoint queries
	(10 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	// added GetRewardInfoPools Block query
	(11 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	(12 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc:   NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},
	(13 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc:   NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added GetPoolDistr, GetPoolState, @GetSnapshots
	(14 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc:   NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added query param to handshake
	(15 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc:   NewVersionDataNtC15andUpFromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	(16 + ProtocolVersionNtCOffset): ProtocolVersion{
		NewVersionDataFromCborFunc:   NewVersionDataNtC15andUpFromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableConwayEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},

	// NtN versions
	//
	// We don't bother supporting NtN protocol versions before 7 (when Alonzo was enabled)

	7: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	8: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	9: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
	},
	10: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableFullDuplex:           true,
	},
	11: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN11to12FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableFullDuplex:           true,
		EnablePeerSharingProtocol:  true,
		PeerSharingUseV11:          true,
	},
	12: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN11to12FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableConwayEra:            true,
		EnableFullDuplex:           true,
		EnablePeerSharingProtocol:  true,
		PeerSharingUseV11:          true,
	},
	13: ProtocolVersion{
		NewVersionDataFromCborFunc: NewVersionDataNtN13andUpFromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableConwayEra:            true,
		EnableFullDuplex:           true,
		EnablePeerSharingProtocol:  true,
	},
}

// GetProtocolVersionMap returns a data structure suitable for use with the protocol handshake
func GetProtocolVersionMap(
	protocolMode ProtocolMode,
	networkMagic uint32,
	diffusionMode bool,
	peerSharing bool,
	queryMode bool,
) ProtocolVersionMap {
	ret := ProtocolVersionMap{}
	for version := range protocolVersions {
		if protocolMode == ProtocolModeNodeToClient {
			if version >= ProtocolVersionNtCOffset {
				if version >= (15 + ProtocolVersionNtCOffset) {
					ret[version] = VersionDataNtC15andUp{
						CborNetworkMagic: networkMagic,
						CborQuery:        queryMode,
					}
				} else {
					ret[version] = VersionDataNtC9to14(networkMagic)
				}
			}
		} else {
			if version < ProtocolVersionNtCOffset {
				if version >= 13 {
					var tmpPeerSharing uint = PeerSharingModeNoPeerSharing
					if peerSharing {
						tmpPeerSharing = PeerSharingModePeerSharingPublic
					}
					ret[version] = VersionDataNtN13andUp{
						VersionDataNtN11to12{
							CborNetworkMagic:                       networkMagic,
							CborInitiatorAndResponderDiffusionMode: diffusionMode,
							CborPeerSharing:                        tmpPeerSharing,
							CborQuery:                              queryMode,
						},
					}
				} else if version >= 11 {
					var tmpPeerSharing uint = PeerSharingModeV11NoPeerSharing
					if peerSharing {
						tmpPeerSharing = PeerSharingModeV11PeerSharingPublic
					}
					ret[version] = VersionDataNtN11to12{
						CborNetworkMagic:                       networkMagic,
						CborInitiatorAndResponderDiffusionMode: diffusionMode,
						CborPeerSharing:                        tmpPeerSharing,
						CborQuery:                              queryMode,
					}
				} else {
					ret[version] = VersionDataNtN7to10{
						CborNetworkMagic:                       networkMagic,
						CborInitiatorAndResponderDiffusionMode: diffusionMode,
					}

				}
			}
		}
	}
	return ret
}

// GetProtocolVersionsNtC returns a list of supported NtC protocol versions
func GetProtocolVersionsNtC() []uint16 {
	versions := []uint16{}
	for key := range protocolVersions {
		if key >= ProtocolVersionNtCOffset {
			versions = append(versions, key)
		}
	}

	// sort asending - iterating over map is not deterministic
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	return versions
}

// GetProtocolVersionsNtN returns a list of supported NtN protocol versions
func GetProtocolVersionsNtN() []uint16 {
	versions := []uint16{}
	for key := range protocolVersions {
		if key < ProtocolVersionNtCOffset {
			versions = append(versions, key)
		}
	}

	// sort asending - iterating over map is not deterministic
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	return versions
}

// GetProtocolVersion returns the protocol version config for the specified protocol version
func GetProtocolVersion(version uint16) ProtocolVersion {
	return protocolVersions[version]
}
