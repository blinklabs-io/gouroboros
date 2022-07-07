package ouroboros

const (
	// The NtC protocol versions have the 15th bit set in the handshake
	PROTOCOL_VERSION_NTC_FLAG = 0x8000
)

type ProtocolVersionNtC struct {
	// Most of these are enabled in all of the protocol versions that we support, but
	// they are here for completeness
	EnableLocalQueryProtocol     bool
	EnableShelleyEra             bool
	EnableAllegraEra             bool
	EnableMaryEra                bool
	EnableAlonzoEra              bool
	EnableBabbageEra             bool
	EnableLocalTxMonitorProtocol bool
}

// We don't bother supporting NtC protocol versions before 9 (when Alonzo was enabled)
var ProtocolVersionMapNtC = map[uint16]ProtocolVersionNtC{
	9: ProtocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
	10: ProtocolVersionNtC{
		EnableLocalQueryProtocol: true,
		EnableShelleyEra:         true,
		EnableAllegraEra:         true,
		EnableMaryEra:            true,
		EnableAlonzoEra:          true,
	},
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
}

type ProtocolVersionNtN struct {
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

// We don't bother supporting NtN protocol versions before 7 (when Alonzo was enabled)
var ProtocolVersionMapNtN = map[uint16]ProtocolVersionNtN{
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
		EnableFullDuplex:        true,
	},
	9: ProtocolVersionNtN{
		EnableShelleyEra:        true,
		EnableKeepAliveProtocol: true,
		EnableAllegraEra:        true,
		EnableMaryEra:           true,
		EnableAlonzoEra:         true,
		EnableBabbageEra:        true,
		EnableFullDuplex:        true,
	},
}

func GetProtocolVersionsNtC() []uint16 {
	versions := []uint16{}
	for key := range ProtocolVersionMapNtC {
		versions = append(versions, key+PROTOCOL_VERSION_NTC_FLAG)
	}
	return versions
}

func GetProtocolVersionNtC(version uint16) ProtocolVersionNtC {
	if version > PROTOCOL_VERSION_NTC_FLAG {
		version = version - PROTOCOL_VERSION_NTC_FLAG
	}
	return ProtocolVersionMapNtC[version]
}

func GetProtocolVersionsNtN() []uint16 {
	versions := []uint16{}
	for key := range ProtocolVersionMapNtN {
		versions = append(versions, key)
	}
	return versions
}

func GetProtocolVersionNtN(version uint16) ProtocolVersionNtN {
	return ProtocolVersionMapNtN[version]
}
