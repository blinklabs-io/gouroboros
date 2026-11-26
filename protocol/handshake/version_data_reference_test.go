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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// refFlags is node-to-node version data reduced to the fields the reference
// acceptableVersion combines.
type refFlags struct {
	initiatorOnly bool
	peerSharing   uint
	query         bool
	peras         bool
}

// refAgreeNtN is the reference acceptableVersion for Cardano node-to-node
// data (cardano-diffusion NodeToNode/Version.hs): the diffusion mode is the
// smaller (initiator-only wins), peer sharing Disabled <> _ = Disabled,
// query local || remote, Peras support the smaller.
func refAgreeNtN(l, r refFlags) refFlags {
	return refFlags{
		initiatorOnly: l.initiatorOnly || r.initiatorOnly,
		peerSharing:   min(l.peerSharing, r.peerSharing),
		query:         l.query || r.query,
		peras:         l.peras && r.peras,
	}
}

// everyNtNFlags lists every decodable combination of the four fields.
func everyNtNFlags() []refFlags {
	var all []refFlags
	for _, init := range []bool{false, true} {
		for _, ps := range []uint{0, 1} {
			for _, query := range []bool{false, true} {
				for _, peras := range []bool{false, true} {
					all = append(all, refFlags{init, ps, query, peras})
				}
			}
		}
	}
	return all
}

// agreedVersionData combines every pair of decodable node-to-node data as
// the reference does, for the four-field data of versions 14 and 15 and the
// five-field data of version 16, and node-to-client query as
// local || remote. The DMQ rule does not apply to Cardano versions: an
// initiator-only agreement keeps the peer sharing both sides offer.
func TestAgreedVersionDataMatchesReference(t *testing.T) {
	const magic = 42
	ntn13 := func(f refFlags) protocol.VersionDataNtN13andUp {
		return protocol.VersionDataNtN13andUp{
			VersionDataNtN11to12: protocol.VersionDataNtN11to12{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: f.initiatorOnly,
				CborPeerSharing:                        f.peerSharing,
				CborQuery:                              f.query,
			},
		}
	}
	ntn16 := func(f refFlags) protocol.VersionDataNtN16andUp {
		return protocol.VersionDataNtN16andUp{
			CborNetworkMagic:                       magic,
			CborInitiatorAndResponderDiffusionMode: f.initiatorOnly,
			CborPeerSharing:                        f.peerSharing,
			CborQuery:                              f.query,
			CborPerasSupport:                       f.peras,
		}
	}
	for _, l := range everyNtNFlags() {
		for _, r := range everyNtNFlags() {
			want := refAgreeNtN(l, r)
			name := fmt.Sprintf("%+v/%+v", l, r)
			require.Equal(t, ntn16(want), agreedVersionData(16, ntn16(l), ntn16(r)), name)
			want.peras, l.peras, r.peras = false, false, false
			require.Equal(t, ntn13(want), agreedVersionData(14, ntn13(l), ntn13(r)), name)
		}
	}
	for _, l := range []bool{false, true} {
		for _, r := range []bool{false, true} {
			got := agreedVersionData(
				protocol.ProtocolVersionNtCOffset+16,
				protocol.VersionDataNtC15andUp{CborNetworkMagic: magic, CborQuery: l},
				protocol.VersionDataNtC15andUp{CborNetworkMagic: magic, CborQuery: r},
			)
			require.Equal(t, l || r, got.Query())
			require.Equal(t, uint32(magic), got.NetworkMagic())
		}
	}
}
