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

package handshake

import (
	"slices"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// agreedVersionData returns the version data both sides agree on, the
// reference acceptableVersion local remote: the server sends it back in
// MsgAcceptVersion, the client agrees its own data with the data in that
// message and ends with the result, and a client in a simultaneous open
// ends with it. The
// network magic is the local one, already checked equal; the diffusion mode
// is initiator-only if either side is; peer sharing and Peras support hold
// only if both sides offer them; query is set if either side asks. Every
// rule is commutative and idempotent, so a peer that combines the agreed
// data with its own again reaches the same result. DMQ node-to-node
// versions share the Cardano data type but not its rule: dmq-node also
// disables peer sharing when the agreed mode is initiator-only, so the
// version decides. Data of a type it does not know, or of mismatched types,
// is returned as local. A table entry held as a pointer agrees as the value
// it points to.
func agreedVersionData(
	version uint16,
	local, remote protocol.VersionData,
) protocol.VersionData {
	local, remote = versionDataValue(local), versionDataValue(remote)
	switch l := local.(type) {
	case protocol.VersionDataNtC15andUp:
		if r, ok := remote.(protocol.VersionDataNtC15andUp); ok {
			l.CborQuery = l.CborQuery || r.CborQuery
			return l
		}
	case protocol.VersionDataNtN7to10:
		if r, ok := remote.(protocol.VersionDataNtN7to10); ok {
			l.CborInitiatorAndResponderDiffusionMode = l.CborInitiatorAndResponderDiffusionMode ||
				r.CborInitiatorAndResponderDiffusionMode
			return l
		}
	case protocol.VersionDataNtN11to12:
		if r, ok := remote.(protocol.VersionDataNtN11to12); ok {
			return agreedVersionDataNtN11(l, r)
		}
	case protocol.VersionDataNtN13andUp:
		if r, ok := remote.(protocol.VersionDataNtN13andUp); ok {
			l.VersionDataNtN11to12 = agreedVersionDataNtN11(
				l.VersionDataNtN11to12,
				r.VersionDataNtN11to12,
			)
			if slices.Contains(protocol.GetProtocolVersionsDMQNtN(), version) &&
				l.CborInitiatorAndResponderDiffusionMode == protocol.DiffusionModeInitiatorOnly {
				l.CborPeerSharing = protocol.PeerSharingModeNoPeerSharing
			}
			return l
		}
	case protocol.VersionDataNtN16andUp:
		if r, ok := remote.(protocol.VersionDataNtN16andUp); ok {
			l.CborInitiatorAndResponderDiffusionMode = l.CborInitiatorAndResponderDiffusionMode ||
				r.CborInitiatorAndResponderDiffusionMode
			l.CborPeerSharing = min(l.CborPeerSharing, r.CborPeerSharing)
			l.CborQuery = l.CborQuery || r.CborQuery
			l.CborPerasSupport = l.CborPerasSupport && r.CborPerasSupport
			return l
		}
	}
	return local
}

// agreedVersionDataNtN11 combines node-to-node data from version 11 on. The
// peer sharing modes are ordered from none to most permissive, so the
// smaller one is the mode both sides offer.
func agreedVersionDataNtN11(
	l, r protocol.VersionDataNtN11to12,
) protocol.VersionDataNtN11to12 {
	l.CborInitiatorAndResponderDiffusionMode = l.CborInitiatorAndResponderDiffusionMode ||
		r.CborInitiatorAndResponderDiffusionMode
	l.CborPeerSharing = min(l.CborPeerSharing, r.CborPeerSharing)
	l.CborQuery = l.CborQuery || r.CborQuery
	return l
}

// versionDataValue returns a copy of the value a version data pointer
// points to, and any other data as it is. The version data methods have
// value receivers, so a version table may hold either form; the copy keeps
// the agreement from changing the caller's entry.
func versionDataValue(v protocol.VersionData) protocol.VersionData {
	switch d := v.(type) {
	case *protocol.VersionDataNtC9to14:
		if d != nil {
			return *d
		}
	case *protocol.VersionDataNtC15andUp:
		if d != nil {
			return *d
		}
	case *protocol.VersionDataNtN7to10:
		if d != nil {
			return *d
		}
	case *protocol.VersionDataNtN11to12:
		if d != nil {
			return *d
		}
	case *protocol.VersionDataNtN13andUp:
		if d != nil {
			return *d
		}
	case *protocol.VersionDataNtN16andUp:
		if d != nil {
			return *d
		}
	}
	return v
}
