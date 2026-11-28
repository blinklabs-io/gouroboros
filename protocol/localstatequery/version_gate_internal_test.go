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

package localstatequery

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Each local-state-query feature turns on at the node-to-client version
// that introduced it, on the client and on the server alike. A protocol
// built without a negotiated version (0) reads as the newest version.
func TestFeatureGatesByVersion(t *testing.T) {
	type gates struct{ chainPoint, rewardInfo, immutableTip, peerSnapshot, peerKind bool }
	for _, test := range []struct {
		ntc  uint16
		want gates
	}{
		{9, gates{}},
		{10, gates{chainPoint: true}},
		{11, gates{chainPoint: true, rewardInfo: true}},
		{15, gates{chainPoint: true, rewardInfo: true}},
		{16, gates{chainPoint: true, rewardInfo: true, immutableTip: true}},
		{18, gates{chainPoint: true, rewardInfo: true, immutableTip: true}},
		{19, gates{true, true, true, true, false}},
		{22, gates{true, true, true, true, false}},
		{23, gates{true, true, true, true, true}},
	} {
		for _, version := range []uint16{test.ntc + protocol.ProtocolVersionNtCOffset} {
			c := NewClient(protocol.ProtocolOptions{Version: version}, nil)
			require.Equal(t, test.want, gates{
				c.enableGetChainPoint && c.enableGetChainBlockNo,
				c.enableGetRewardInfoPoolsBlock, c.enableAcquireImmutableTip,
				c.enableGetLedgerPeerSnapshot, c.enableLedgerPeerKind,
			}, "client, version %d", test.ntc)
			s := NewServer(protocol.ProtocolOptions{Version: version}, nil)
			want := test.want
			// The server accepts MsgAcquireImmutableTip at every version, as the
			// reference codec, which has only version 23, does.
			want.immutableTip = false
			require.Equal(t, want, gates{
				s.enableGetChainPoint && s.enableGetChainBlockNo,
				s.enableGetRewardInfoPoolsBlock, false,
				s.enableGetLedgerPeerSnapshot, s.enableLedgerPeerKind,
			}, "server, version %d", test.ntc)
		}
	}
	c := NewClient(protocol.ProtocolOptions{}, nil)
	require.True(t, c.enableGetChainPoint && c.enableGetRewardInfoPoolsBlock &&
		c.enableAcquireImmutableTip && c.enableGetLedgerPeerSnapshot &&
		c.enableLedgerPeerKind, "an unset version reads as the newest")
}
