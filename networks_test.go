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

package ouroboros_test

import (
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/stretchr/testify/require"
)

func TestDijkstraNetworkLookup(t *testing.T) {
	t.Parallel()

	byName, ok := ouroboros.NetworkByName("dijkstra")
	require.True(t, ok)
	require.Equal(t, uint8(0), byName.Id)
	require.Equal(t, uint32(6), byName.NetworkMagic)
	require.Equal(t, []ouroboros.NetworkBootstrapPeer{
		{Address: "dijkstra-node.play.dev.cardano.org", Port: 3001},
	}, byName.BootstrapPeers)

	byMagic, ok := ouroboros.NetworkByNetworkMagic(6)
	require.True(t, ok)
	require.Equal(t, byName, byMagic)

	_, ok = ouroboros.NetworkByName("unknown-network")
	require.False(t, ok)
}
