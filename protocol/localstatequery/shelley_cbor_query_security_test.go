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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wrappedSnapshotQuery(depth int, withPeerKind bool) QueryWrapper {
	var query any = &ShelleyGetLedgerPeerSnapshotQuery{
		Type:         QueryTypeShelleyGetLedgerPeerSnapshot,
		PeerKind:     LedgerPeerKindBig,
		WithPeerKind: withPeerKind,
	}
	for range depth {
		query = &ShelleyCborQuery{
			Type:  QueryTypeShelleyCbor,
			Query: query,
		}
	}
	return QueryWrapper{Query: &BlockQuery{Query: &ShelleyQuery{
		Era:   6,
		Query: query,
	}}}
}

func TestServerRejectsWrappedSnapshotBeforeVersion19(t *testing.T) {
	called := false
	server := newServerWithVersion(ntc(18), func(
		_ CallbackContext,
		_ QueryWrapper,
	) (any, error) {
		called = true
		return nil, nil
	})
	err := server.handleQuery(NewMsgQuery(wrappedSnapshotQuery(2, false).Query))
	require.Error(t, err)
	assert.False(t, called)
}

func TestServerRejectsWrappedSnapshotRequestForm(t *testing.T) {
	called := false
	server := newServerWithVersion(ntc(23), func(
		_ CallbackContext,
		_ QueryWrapper,
	) (any, error) {
		called = true
		return nil, nil
	})
	err := server.handleQuery(NewMsgQuery(wrappedSnapshotQuery(2, false).Query))
	require.ErrorIs(t, err, ErrQueryUnsupportedVersion)
	assert.False(t, called)
}

func TestServerRejectsWrappedSnapshotResultVersion(t *testing.T) {
	server := newServerWithVersion(ntc(23), func(
		_ CallbackContext,
		_ QueryWrapper,
	) (any, error) {
		return LedgerPeerSnapshotResult{Version: 1}, nil
	})
	err := server.handleQuery(NewMsgQuery(wrappedSnapshotQuery(2, true).Query))
	require.ErrorIs(t, err, ErrLedgerPeerSnapshotUnsupportedWireVersion)
}

func TestWrappedSnapshotUsesSRVFiltering(t *testing.T) {
	query := ledgerPeerSnapshotQuery(wrappedSnapshotQuery(2, false))
	require.NotNil(t, query)
	snapshot := srvSnapshotFixture()
	filtered := withoutSRVRelays(snapshot, ntc(21))
	result, ok := filtered.(LedgerPeerSnapshotResult)
	require.True(t, ok)
	require.Len(t, result.Pools, 1)
	require.Len(t, result.Pools[0].Detail.Relays, 1)
	assert.Equal(t, RelayKindIPv4, result.Pools[0].Detail.Relays[0].Kind)
}

func TestShelleyCborQueryWrapperDepth(t *testing.T) {
	encode := func(depth int) []byte {
		t.Helper()
		var query any = []any{QueryTypeShelleyGetLedgerPeerSnapshot}
		for range depth {
			query = []any{QueryTypeShelleyCbor, query}
		}
		data, err := cbor.Encode([]any{
			MessageTypeQuery,
			[]any{QueryTypeBlock, []any{
				QueryTypeShelley,
				[]any{uint(6), query},
			}},
		})
		require.NoError(t, err)
		return data
	}

	_, err := NewMsgFromCbor(
		MessageTypeQuery,
		encode(maxShelleyCborQueryDepth),
	)
	require.NoError(t, err)
	_, err = NewMsgFromCbor(
		MessageTypeQuery,
		encode(maxShelleyCborQueryDepth+1),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper depth exceeds maximum 32")
}
