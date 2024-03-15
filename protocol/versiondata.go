// Copyright 2023 Blink Labs, LLC.
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

import "github.com/blinklabs-io/gouroboros/cbor"

// Diffusion modes
const (
	DiffusionModeInitiatorOnly         = true
	DiffusionModeInitiatorAndResponder = false
)

// Peer sharing modes
const (
	PeerSharingModeNoPeerSharing         = 0
	PeerSharingModePeerSharingPublic     = 1
	PeerSharingModeV11NoPeerSharing      = 0
	PeerSharingModeV11PeerSharingPrivate = 1
	PeerSharingModeV11PeerSharingPublic  = 2
)

// Query modes
const (
	QueryModeDisabled = false
	QueryModeEnabled  = true
)

type VersionData interface {
	NetworkMagic() uint32
	//Query() bool
	// NtN only
	DiffusionMode() bool
	PeerSharing() bool
}

type VersionDataNtC9to14 uint32

func NewVersionDataNtC9to14FromCbor(cborData []byte) (VersionData, error) {
	var v VersionDataNtC9to14
	_, err := cbor.Decode(cborData, &v)
	return v, err
}

func (v VersionDataNtC9to14) NetworkMagic() uint32 {
	return uint32(v)
}

func (v VersionDataNtC9to14) DiffusionMode() bool {
	return DiffusionModeInitiatorOnly
}

func (v VersionDataNtC9to14) PeerSharing() bool {
	return false
}

type VersionDataNtC15andUp struct {
	cbor.StructAsArray
	CborNetworkMagic uint32
	CborQuery        bool
}

func NewVersionDataNtC15andUpFromCbor(cborData []byte) (VersionData, error) {
	var v VersionDataNtC15andUp
	_, err := cbor.Decode(cborData, &v)
	return v, err
}

func (v VersionDataNtC15andUp) NetworkMagic() uint32 {
	return v.CborNetworkMagic
}

func (v VersionDataNtC15andUp) DiffusionMode() bool {
	return DiffusionModeInitiatorOnly
}

func (v VersionDataNtC15andUp) PeerSharing() bool {
	return false
}

type VersionDataNtN7to10 struct {
	cbor.StructAsArray
	CborNetworkMagic                       uint32
	CborInitiatorAndResponderDiffusionMode bool
}

func NewVersionDataNtN7to10FromCbor(cborData []byte) (VersionData, error) {
	var v VersionDataNtN7to10
	_, err := cbor.Decode(cborData, &v)
	return v, err
}

func (v VersionDataNtN7to10) NetworkMagic() uint32 {
	return v.CborNetworkMagic
}

func (v VersionDataNtN7to10) DiffusionMode() bool {
	return v.CborInitiatorAndResponderDiffusionMode
}

func (v VersionDataNtN7to10) PeerSharing() bool {
	return false
}

type VersionDataNtN11to12 struct {
	cbor.StructAsArray
	CborNetworkMagic                       uint32
	CborInitiatorAndResponderDiffusionMode bool
	CborPeerSharing                        uint
	CborQuery                              bool
}

func NewVersionDataNtN11to12FromCbor(cborData []byte) (VersionData, error) {
	var v VersionDataNtN11to12
	_, err := cbor.Decode(cborData, &v)
	return v, err
}

func (v VersionDataNtN11to12) NetworkMagic() uint32 {
	return v.CborNetworkMagic
}

func (v VersionDataNtN11to12) DiffusionMode() bool {
	return v.CborInitiatorAndResponderDiffusionMode
}

func (v VersionDataNtN11to12) PeerSharing() bool {
	return v.CborPeerSharing >= PeerSharingModeV11PeerSharingPublic
}

// NOTE: the format stays the same, but the values for PeerSharing change
type VersionDataNtN13andUp struct {
	VersionDataNtN11to12
}

func NewVersionDataNtN13andUpFromCbor(cborData []byte) (VersionData, error) {
	var v VersionDataNtN13andUp
	_, err := cbor.Decode(cborData, &v)
	return v, err
}

func (v VersionDataNtN13andUp) PeerSharing() bool {
	return v.CborPeerSharing >= PeerSharingModePeerSharingPublic
}
