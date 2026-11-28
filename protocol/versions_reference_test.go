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

package protocol

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// wireFields returns the number of fields the version data for version has
// on the wire, as GetProtocolVersionMap encodes it.
func wireFields(t *testing.T, data VersionData) int {
	t.Helper()
	encoded, err := cbor.Encode(&data)
	require.NoError(t, err)
	var fields []any
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	return len(fields)
}

// The node-to-node versions of the released reference (cardano-diffusion
// 1.3.0.1, NodeToNode/Version.hs): 14, 15 and 16, each with four data
// fields [magic, initiatorOnly, peerSharing 0|1, query], and the Peras
// field from 16. Every version from 14 runs keep-alive, full duplex and
// peer sharing with the two-mode encoding.
func TestNodeToNodeVersionsMatchReference(t *testing.T) {
	const magic = 764824073
	versions := GetProtocolVersionsNtN()
	for _, test := range []struct {
		version uint16
		fields  int
	}{
		{14, 4},
		{15, 4},
		{16, 5},
	} {
		require.Contains(t, versions, test.version)
		info := GetProtocolVersion(test.version)
		require.True(t, info.EnableKeepAliveProtocol, "version %d", test.version)
		require.True(t, info.EnableFullDuplex, "version %d", test.version)
		require.True(t, info.EnablePeerSharingProtocol, "version %d", test.version)
		require.False(t, info.PeerSharingUseV11, "version %d", test.version)
		for _, peerSharing := range []bool{false, true} {
			table := GetProtocolVersionMapWithPerasSupport(
				ProtocolModeNodeToNode, magic,
				DiffusionModeInitiatorOnly, peerSharing, false, true,
			)
			data := table[test.version]
			require.Equal(t, test.fields, wireFields(t, data), "version %d", test.version)
			encoded, err := cbor.Encode(&data)
			require.NoError(t, err)
			var fields []any
			_, err = cbor.Decode(encoded, &fields)
			require.NoError(t, err)
			require.NotNil(t, fields)
			// peerSharing is 0 (disabled) or 1 (enabled), nothing else.
			want := uint64(0)
			if peerSharing {
				want = 1
			}
			require.Equal(t, want, fields[2], "version %d", test.version)
			// The Peras field is carried, and only carried, from 16.
			_, carriesPeras := data.(interface{ PerasSupported() bool })
			require.Equal(t, test.fields == 5, carriesPeras)
			decoded, err := info.NewVersionDataFromCborFunc(encoded)
			require.NoError(t, err)
			require.Equal(t, peerSharing, decoded.PeerSharing())
		}
	}
	// Without the Peras option the version 16 field is sent as false.
	data := GetProtocolVersionMap(
		ProtocolModeNodeToNode, magic, DiffusionModeInitiatorOnly, true, false,
	)[16]
	peras, ok := data.(interface{ PerasSupported() bool })
	require.True(t, ok)
	require.False(t, peras.PerasSupported())
}

// The node-to-client version of the reference (NodeToClient/Version.hs) is
// 23 with bit 15 set, and its data is [magic, query]. dmq-node's
// node-to-client version is 1 with bit 12 set and the same data; its
// node-to-node versions are 1 and 2 with four fields
// (DMQ/NodeToNode/Version.hs).
func TestOtherVersionTablesMatchReference(t *testing.T) {
	const magic = 3141592
	require.Contains(t, GetProtocolVersionsNtC(), uint16(23+ProtocolVersionNtCOffset))
	for _, query := range []bool{false, true} {
		ntc := GetProtocolVersionMap(ProtocolModeNodeToClient, magic, false, false, query)
		data := ntc[23+ProtocolVersionNtCOffset]
		require.NotNil(t, data)
		require.Equal(t, 2, wireFields(t, data))
		require.Equal(t, query, data.Query())

		require.Equal(t, []uint16{1 + ProtocolVersionDMQNtCOffset}, GetProtocolVersionsDMQNtC())
		dmqNtC := GetProtocolVersionMapDMQNtC(magic, query)[1+ProtocolVersionDMQNtCOffset]
		require.NotNil(t, dmqNtC)
		require.Equal(t, 2, wireFields(t, dmqNtC))
		require.Equal(t, query, dmqNtC.Query())

		require.Equal(t, []uint16{1, 2}, GetProtocolVersionsDMQNtN())
		for version, data := range GetProtocolVersionMapDMQNtN(
			magic, DiffusionModeInitiatorAndResponder, true, query,
		) {
			require.Equal(t, 4, wireFields(t, data), "version %d", version)
			require.Equal(t, query, data.Query())
			require.True(t, data.PeerSharing())
		}
	}
}
