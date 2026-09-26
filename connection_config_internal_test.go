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

package ouroboros

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/stretchr/testify/require"
)

func TestNewConnectionSynchronizesByronSlotsPerEpoch(t *testing.T) {
	tests := []struct {
		name    string
		options []ConnectionOptionFunc
	}{
		{
			name: "BlockFetch config",
			options: []ConnectionOptionFunc{
				WithBlockFetchConfig(blockfetch.Config{
					ByronSlotsPerEpoch: 600,
				}),
			},
		},
		{
			name: "ChainSync config",
			options: []ConnectionOptionFunc{
				WithChainSyncConfig(chainsync.Config{
					ByronSlotsPerEpoch: 600,
				}),
			},
		},
		{
			name: "BlockFetch config with a default ChainSync config",
			options: []ConnectionOptionFunc{
				WithBlockFetchConfig(blockfetch.Config{
					ByronSlotsPerEpoch: 600,
				}),
				WithChainSyncConfig(chainsync.Config{}),
			},
		},
		{
			name: "ChainSync config with a default BlockFetch config",
			options: []ConnectionOptionFunc{
				WithBlockFetchConfig(blockfetch.Config{}),
				WithChainSyncConfig(chainsync.Config{
					ByronSlotsPerEpoch: 600,
				}),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, err := NewConnection(test.options...)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			require.NotNil(t, conn.blockFetchConfig)
			require.NotNil(t, conn.chainSyncConfig)
			require.Equal(t, uint64(600), conn.blockFetchConfig.ByronSlotsPerEpoch)
			require.Equal(t, uint64(600), conn.chainSyncConfig.ByronSlotsPerEpoch)
		})
	}
}

func TestNewConnectionRejectsConflictingByronSlotsPerEpoch(t *testing.T) {
	_, err := NewConnection(
		WithBlockFetchConfig(blockfetch.Config{ByronSlotsPerEpoch: 600}),
		WithChainSyncConfig(chainsync.Config{ByronSlotsPerEpoch: 1200}),
	)
	require.ErrorContains(t, err, "conflicting Byron slots per epoch")
}
