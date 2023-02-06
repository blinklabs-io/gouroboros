package ouroboros

// The NtC protocol versions have the 15th bit set in the handshake
const protocolVersionNtCFlag = 0x8000

// Most of these are enabled in all of the protocol versions that we support, but
// they are here for completeness
type protocolVersionNtC struct {
	EnableLocalQueryProtocol     bool
	EnableShelleyEra             bool
	EnableAllegraEra             bool
	EnableMaryEra                bool
	EnableAlonzoEra              bool
	EnableBabbageEra             bool
	EnableLocalTxMonitorProtocol bool
}

// Map of NtC protocol versions to protocol features
//
// We don't bother supporting NtC protocol versions before 9 (when Alonzo was enabled)
var protocolVersionMapNtC = map[uint16]protocolVersionNtC{
	9: protocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	// added GetChainBlockNo and GetChainPoint queries
	10: protocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	// added GetRewardInfoPools Block query
	11: protocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	12: protocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableLocalTxMonitorProtocol: true,
	},
	13: protocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
	// added GetPoolDistr, GetPoolState, @GetSnapshots
	14: protocolVersionNtC{
		EnableLocalQueryProtocol:     true,
		EnableShelleyEra:             true,
		EnableAllegraEra:             true,
		EnableMaryEra:                true,
		EnableAlonzoEra:              true,
		EnableBabbageEra:             true,
		EnableLocalTxMonitorProtocol: true,
	},
}

type protocolVersionNtN struct {
	// Most of these are enabled in all of the protocol versions that we support, but
	// they are here for completeness
	EnableShelleyEra        bool
	EnableKeepAliveProtocol bool
	EnableAllegraEra        bool
	EnableMaryEra           bool
	EnableAlonzoEra         bool
	EnableBabbageEra        bool
	EnableFullDuplex        bool
}

// Map of NtN protocol versions to protocol features
//
// We don't bother supporting NtN protocol versions before 7 (when Alonzo was enabled)
var protocolVersionMapNtN = map[uint16]protocolVersionNtN{
	7: protocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
	},
	8: protocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
	},
	9: protocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
		EnableBabbageEra:        true,
	},
	10: protocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
		EnableBabbageEra:        true,
		EnableFullDuplex:        true,
	},
}

// getProtocolVersionNtC returns a list of supported NtC protocol versions
func getProtocolVersionsNtC() []uint16 {
	versions := []uint16{}
	for key := range protocolVersionMapNtC {
		versions = append(versions, key+protocolVersionNtCFlag)
	}
	return versions
}

// getProtocolVersionNtC returns the protocol version config for the specified NtC protocol version
func getProtocolVersionNtC(version uint16) protocolVersionNtC {
	if version > protocolVersionNtCFlag {
		version = version - protocolVersionNtCFlag
	}
	return protocolVersionMapNtC[version]
}

// getProtocolVersionNtN returns a list of supported NtN protocol versions
func getProtocolVersionsNtN() []uint16 {
	versions := []uint16{}
	for key := range protocolVersionMapNtN {
		versions = append(versions, key)
	}
	return versions
}

// getProtocolVersionNtN returns the protocol version config for the specified NtN protocol version
func getProtocolVersionNtN(version uint16) protocolVersionNtN {
	return protocolVersionMapNtN[version]
}
