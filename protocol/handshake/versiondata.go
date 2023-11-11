package handshake

import "github.com/blinklabs-io/gouroboros/cbor"

type NtCVersionDataLegacy uint32

type NtCVersionData struct {
	cbor.StructAsArray
	NetworkMagic uint32
	Query        bool
}

type NtNVersionDataLegacy struct {
	cbor.StructAsArray
	NetworkMagic                       uint32
	InitiatorAndResponderDiffusionMode bool
}

type NtNVersionDataPeerSharingQuery struct {
	cbor.StructAsArray
	NetworkMagic                       uint32
	InitiatorAndResponderDiffusionMode bool
	PeerSharing                        uint
	Query                              bool
}
