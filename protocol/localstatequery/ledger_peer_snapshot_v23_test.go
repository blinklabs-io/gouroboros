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
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const relayExampleHex = "72656c61792e6578616d706c65" // "relay.example"

// TestLedgerPeerSnapshotV2Golden decodes the version 23 big-ledger-peer
// snapshot as encodeLedgerPeerSnapshot writes it and re-encodes it to the
// same bytes.
func TestLedgerPeerSnapshotV2Golden(t *testing.T) {
	hash := strings.Repeat("aa", 32)
	wire := spacedHex(t, "82 02 83"+
		" 83 01 19 3039 58 20 "+hash+
		" 1a 2d964a09"+
		" 9f 83 82 01 0a 82 01 0a 9f 82 03 4d "+relayExampleHex+" ff ff")
	var snap LedgerPeerSnapshotResult
	_, err := cbor.Decode(wire, &snap)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), snap.Version)
	assert.Equal(t, WithOriginSlot{HasSlot: true, Slot: 12345}, snap.Slot)
	assert.Equal(t, hash, hex.EncodeToString(snap.BlockHash))
	assert.Equal(t, uint32(764824073), snap.NetworkMagic)
	require.Len(t, snap.Pools, 1)
	pool := snap.Pools[0]
	assert.Zero(t, pool.AccumulatedStake.Cmp(big.NewRat(1, 10)))
	assert.Zero(t, pool.Detail.PoolStake.Cmp(big.NewRat(1, 10)))
	require.Len(t, pool.Detail.Relays, 1)
	assert.Equal(t, RelayKindSRV, pool.Detail.Relays[0].Kind)
	assert.Equal(t, "relay.example", *pool.Detail.Relays[0].Domain)
	data, err := cbor.Encode(snap)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(wire), hex.EncodeToString(data))
}

// TestLedgerPeerSnapshotV3Golden covers the all-ledger-peer snapshot taken at
// origin, whose pool entries carry no accumulated stake.
func TestLedgerPeerSnapshotV3Golden(t *testing.T) {
	wire := spacedHex(t, "82 03 83 81 00 01"+
		" 9f 82 82 01 02 9f 83 00 19 0bb9 4d "+relayExampleHex+" ff ff")
	var snap LedgerPeerSnapshotResult
	_, err := cbor.Decode(wire, &snap)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), snap.Version)
	assert.False(t, snap.Slot.HasSlot)
	assert.Nil(t, snap.BlockHash)
	assert.Equal(t, uint32(1), snap.NetworkMagic)
	require.Len(t, snap.Pools, 1)
	pool := snap.Pools[0]
	assert.Nil(t, pool.AccumulatedStake)
	assert.Zero(t, pool.Detail.PoolStake.Cmp(big.NewRat(1, 2)))
	require.Len(t, pool.Detail.Relays, 1)
	relay := pool.Detail.Relays[0]
	assert.Equal(t, RelayKindDomain, relay.Kind)
	assert.Equal(t, uint16(3001), *relay.Port)
	data, err := cbor.Encode(snap)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(wire), hex.EncodeToString(data))
}

func TestLedgerPeerSnapshotV23Malformed(t *testing.T) {
	for name, wire := range map[string]string{
		// A big-ledger-peer entry with only two fields.
		"v2_short_pool": "82 02 83 81 00 01 9f 82 82 01 02 9f 82 03 41 61 ff ff",
		// An all-ledger-peer entry with an accumulated stake.
		"v3_long_pool": "82 03 83 81 00 01" +
			" 9f 83 82 01 02 82 01 02 9f 82 03 41 61 ff ff",
		// Origin carries no slot or hash.
		"origin_with_hash": "82 03 83 83 00 01 41 aa 01 80",
		// The magic is the second of three fields, not the first of two.
		"v2_two_fields": "82 02 82 81 00 80",
		// A block hash is 32 bytes.
		"v2_short_hash": "82 02 83 83 01 19 3039 58 1f " +
			strings.Repeat("aa", 31) + " 1a 2d964a09" +
			" 9f 83 82 01 0a 82 01 0a 9f 82 03 4d " + relayExampleHex + " ff ff",
		// Relays are NonEmpty.
		"empty_relays": "82 03 83 81 00 01 9f 82 82 01 02 80 ff",
	} {
		var snap LedgerPeerSnapshotResult
		_, err := cbor.Decode(spacedHex(t, wire), &snap)
		assert.Error(t, err, name)
	}
	_, err := cbor.Encode(LedgerPeerSnapshotResult{
		Version:   3,
		BlockHash: []byte{1},
	})
	assert.Error(t, err, "a block hash at origin cannot be encoded")
}

// TestGetLedgerPeerSnapshotV23Gate checks that only version 23 enables the
// peer-kind request, and so LedgerPeerKindAll.
func TestGetLedgerPeerSnapshotV23Gate(t *testing.T) {
	c := newClientWithVersion(ntc(22))
	assert.False(t, c.enableLedgerPeerKind)
	_, err := c.GetLedgerPeerSnapshot(LedgerPeerKindAll)
	require.ErrorIs(t, err, ErrLedgerPeerKindUnsupportedVersion)
	c = newClientWithVersion(ntc(23))
	assert.True(t, c.enableLedgerPeerKind)
}

// TestServerLedgerPeerSnapshotFormGate rejects the request form the
// negotiated version does not use: the rc node fails [34] at version 23,
// and versions before 23 have no peer kind.
func TestServerLedgerPeerSnapshotFormGate(t *testing.T) {
	for _, tc := range []struct {
		name         string
		version      uint16
		withPeerKind bool
	}{
		{"bare_on_v23", 23, false},
		{"peer_kind_on_v22", 22, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			s := newServerWithVersion(
				ntc(tc.version),
				func(_ CallbackContext, _ QueryWrapper) (any, error) {
					called = true
					return nil, nil
				},
			)
			msg := NewMsgQuery(&BlockQuery{
				Query: &ShelleyQuery{
					Era: 6,
					Query: &ShelleyGetLedgerPeerSnapshotQuery{
						Type:         QueryTypeShelleyGetLedgerPeerSnapshot,
						PeerKind:     LedgerPeerKindBig,
						WithPeerKind: tc.withPeerKind,
					},
				},
			})
			err := s.handleQuery(msg)
			require.True(t, errors.Is(err, ErrQueryUnsupportedVersion), err)
			assert.False(t, called)
		})
	}
}

// TestServerRejectsSnapshotVersionForRequestForm checks the server refuses to
// encode a snapshot whose wire version the request form does not take, which
// the client could not decode.
func TestServerRejectsSnapshotVersionForRequestForm(t *testing.T) {
	cases := []struct {
		name     string
		version  uint16
		peerKind LedgerPeerKind
		withKind bool
		result   any
	}{
		{
			"v1_for_big_peer_kind", 23, LedgerPeerKindBig, true,
			[]any{LedgerPeerSnapshotResult{Version: 1}},
		},
		{
			"unset_for_all_peer_kind", 23, LedgerPeerKindAll, true,
			LedgerPeerSnapshotResult{},
		},
		{
			"v3_for_big_peer_kind", 23, LedgerPeerKindBig, true,
			&LedgerPeerSnapshotResult{Version: 3},
		},
		{
			"v2_for_bare_form", 22, LedgerPeerKindBig, false,
			[]any{LedgerPeerSnapshotResult{Version: 2}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newServerWithVersion(
				ntc(tc.version),
				func(_ CallbackContext, _ QueryWrapper) (any, error) {
					return tc.result, nil
				},
			)
			msg := NewMsgQuery(&BlockQuery{
				Query: &ShelleyQuery{
					Era: 6,
					Query: &ShelleyGetLedgerPeerSnapshotQuery{
						Type:         QueryTypeShelleyGetLedgerPeerSnapshot,
						PeerKind:     tc.peerKind,
						WithPeerKind: tc.withKind,
					},
				},
			})
			// An unstarted server blocks sending a reply, so a reply that
			// gets past the check shows as a timeout.
			errCh := make(chan error, 1)
			go func() { errCh <- s.handleQuery(msg) }()
			select {
			case err := <-errCh:
				require.ErrorIs(
					t,
					err,
					ErrLedgerPeerSnapshotUnsupportedWireVersion,
				)
			case <-time.After(5 * time.Second):
				t.Fatal("server tried to send the mismatched snapshot")
			}
		})
	}
}

// TestCheckLedgerPeerSnapshotResultAcceptsMatchingVersion covers the versions
// each request form takes.
func TestCheckLedgerPeerSnapshotResultAcceptsMatchingVersion(t *testing.T) {
	cases := []struct {
		query  ShelleyGetLedgerPeerSnapshotQuery
		result any
	}{
		{
			ShelleyGetLedgerPeerSnapshotQuery{PeerKind: LedgerPeerKindBig},
			[]any{LedgerPeerSnapshotResult{}},
		},
		{
			ShelleyGetLedgerPeerSnapshotQuery{PeerKind: LedgerPeerKindBig},
			LedgerPeerSnapshotResult{Version: 1},
		},
		{
			ShelleyGetLedgerPeerSnapshotQuery{
				PeerKind: LedgerPeerKindBig, WithPeerKind: true,
			},
			[]any{&LedgerPeerSnapshotResult{Version: 2}},
		},
		{
			ShelleyGetLedgerPeerSnapshotQuery{
				PeerKind: LedgerPeerKindAll, WithPeerKind: true,
			},
			[]any{LedgerPeerSnapshotResult{Version: 3}},
		},
	}
	for _, tc := range cases {
		assert.NoError(t, checkLedgerPeerSnapshotResult(&tc.query, tc.result))
	}
}
