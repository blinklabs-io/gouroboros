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

import "slices"

// The NtC protocol versions have the 15th bit set in the handshake
const ProtocolVersionNtCOffset = 0x8000

// The DMQ N2C protocol versions have the 12th bit set in the handshake.
// Note that this is a different bit than Cardano's NtC offset (15th bit);
// dmq-node uses the 12th bit deliberately to keep the DMQ namespace
// distinct from the Cardano N2N/N2C version space (see
// dmq-node/src/DMQ/NodeToClient/Version.hs).
const ProtocolVersionDMQNtCOffset = 0x1000

// DMQ node-to-node protocol versions from CIP-0137.
const (
	ProtocolVersionDMQNtN1 uint16 = 1
	ProtocolVersionDMQNtN2 uint16 = 2
)

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
	EnableDijkstraEra          bool
	// Deprecated: Leios is delivered within the Dijkstra era.
	EnableLeiosEra bool
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

	(9 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	// added GetChainBlockNo and GetChainPoint queries
	(10 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	// added GetRewardInfoPools Block query
	(11 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc: NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:   true,
		EnableShelleyEra:           true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	(12 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc:   NewVersionDataNtC9to14FromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},
	(13 + ProtocolVersionNtCOffset): {
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
	(14 + ProtocolVersionNtCOffset): {
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
	(15 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc:   NewVersionDataNtC15andUpFromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added @ImmutableTip@ to @LocalStateQuery@, enabled Conway, and @GetStakeDelegDeposits@.
	(16 + ProtocolVersionNtCOffset): {
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
	// added @GetProposals@ and @GetRatifyState@ queries
	(17 + ProtocolVersionNtCOffset): {
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
	// added @GetFuturePParams@ query
	(18 + ProtocolVersionNtCOffset): {
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
	// added @GetLedgerPeerSnapshot@
	(19 + ProtocolVersionNtCOffset): {
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
	// Added additional Conway governance queries and Dijkstra-era support.
	(20 + ProtocolVersionNtCOffset): {
		NewVersionDataFromCborFunc:   NewVersionDataNtC15andUpFromCbor,
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableConwayEra:              true,
		EnableDijkstraEra:            true,
		EnableLeiosEra:               true,
		EnableLocalTxMonitorProtocol: true,
	},

	//
	// We don't bother supporting NtN protocol versions before 7 (when Alonzo was enabled)

	7: {
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	8: {
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
	},
	9: {
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
	},
	10: {
		NewVersionDataFromCborFunc: NewVersionDataNtN7to10FromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableFullDuplex:           true,
	},
	11: {
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
	12: {
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
	13: {
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
	// Enables Chang+1 HF
	14: {
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
	// Enables Dijkstra era
	15: {
		NewVersionDataFromCborFunc: NewVersionDataNtN13andUpFromCbor,
		EnableShelleyEra:           true,
		EnableKeepAliveProtocol:    true,
		EnableAllegraEra:           true,
		EnableMaryEra:              true,
		EnableAlonzoEra:            true,
		EnableBabbageEra:           true,
		EnableConwayEra:            true,
		EnableDijkstraEra:          true,
		EnableLeiosEra:             true,
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

// dmqProtocolVersionsNtC maps supported DMQ N2C protocol versions to their
// version metadata. dmq-node currently advertises only NodeToClientV_1
// (encoded with the 12th bit set: 1 | 0x1000 = 0x1001).
//
// VersionData is structurally [uint32 networkMagic, bool query] — identical
// to Cardano's VersionDataNtC15andUp — so the same CBOR decoder is reused.
var dmqProtocolVersionsNtC = map[uint16]ProtocolVersion{
	(1 + ProtocolVersionDMQNtCOffset): {
		NewVersionDataFromCborFunc: NewVersionDataNtC15andUpFromCbor,
	},
}

// dmqProtocolVersionsNtN maps supported DMQ N2N protocol versions to their
// version metadata. CIP-0137 defines NodeToNodeV_1 and NodeToNodeV_2.
var dmqProtocolVersionsNtN = map[uint16]ProtocolVersion{
	ProtocolVersionDMQNtN1: {
		NewVersionDataFromCborFunc: NewVersionDataNtN13andUpFromCbor,
	},
	ProtocolVersionDMQNtN2: {
		NewVersionDataFromCborFunc: NewVersionDataNtN13andUpFromCbor,
	},
}

// GetProtocolVersionMapDMQNtC returns a ProtocolVersionMap suitable for
// handshaking against a DMQ node's node-to-client listener (CIP-0137).
// The networkMagic argument is the DMQ-specific topic magic (e.g. Mithril
// mainnet = 2912307721, dmq-node default = 3141592), distinct from any
// Cardano network magic.
func GetProtocolVersionMapDMQNtC(
	networkMagic uint32,
	queryMode bool,
) ProtocolVersionMap {
	ret := ProtocolVersionMap{}
	for version := range dmqProtocolVersionsNtC {
		ret[version] = VersionDataNtC15andUp{
			CborNetworkMagic: networkMagic,
			CborQuery:        queryMode,
		}
	}
	return ret
}

// GetProtocolVersionMapDMQNtN returns a ProtocolVersionMap suitable for
// handshaking against a DMQ node-to-node listener (CIP-0137).
func GetProtocolVersionMapDMQNtN(
	networkMagic uint32,
	diffusionMode bool,
	peerSharing bool,
	queryMode bool,
) ProtocolVersionMap {
	ret := ProtocolVersionMap{}
	for version := range dmqProtocolVersionsNtN {
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
	}
	return ret
}

// GetProtocolVersionsDMQNtC returns a list of supported DMQ N2C protocol
// versions in ascending order.
func GetProtocolVersionsDMQNtC() []uint16 {
	versions := make([]uint16, 0, len(dmqProtocolVersionsNtC))
	for key := range dmqProtocolVersionsNtC {
		versions = append(versions, key)
	}
	slices.Sort(versions)
	return versions
}

// GetProtocolVersionsDMQNtN returns a list of supported DMQ N2N protocol
// versions in ascending order.
func GetProtocolVersionsDMQNtN() []uint16 {
	versions := make([]uint16, 0, len(dmqProtocolVersionsNtN))
	for key := range dmqProtocolVersionsNtN {
		versions = append(versions, key)
	}
	slices.Sort(versions)
	return versions
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
	slices.Sort(versions)

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
	slices.Sort(versions)

	return versions
}

// GetProtocolVersion returns the protocol version config for the specified protocol version.
// It searches Cardano NtN/NtC plus DMQ N2C/N2N maps. The key spaces are
// disjoint for supported Cardano versions: DMQ N2C uses the 0x1000 offset,
// DMQ N2N uses 1/2, Cardano NtC uses 0x8000, and supported Cardano NtN uses 7-15.
func GetProtocolVersion(version uint16) ProtocolVersion {
	if v, ok := protocolVersions[version]; ok {
		return v
	}
	if v, ok := dmqProtocolVersionsNtC[version]; ok {
		return v
	}
	return dmqProtocolVersionsNtN[version]
}
