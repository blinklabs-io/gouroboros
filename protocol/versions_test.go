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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDMQVersionEncoding verifies the DMQ N2C version offset matches the
// upstream encoding: dmq-node uses bit 12 (= 0x1000) to mark DMQ N2C
// versions in the handshake, distinct from Cardano's NtC offset 0x8000
// (bit 15). NodeToClientV_1 is therefore wire-encoded as
// 1 | 0x1000 = 0x1001 = 4097.
func TestDMQVersionEncoding(t *testing.T) {
	require.Equal(t, uint16(0x1000), uint16(ProtocolVersionDMQNtCOffset))
	versions := GetProtocolVersionsDMQNtC()
	require.Len(t, versions, 1)
	assert.Equal(t, uint16(0x1001), versions[0])
	assert.NotEqual(
		t,
		uint16(ProtocolVersionNtCOffset+1),
		versions[0],
		"DMQ version must not collide with Cardano NtC version space",
	)
}

// TestGetProtocolVersionMapDMQNtC verifies the version map produced for a
// DMQ N2C handshake. The VersionData carries the supplied network magic
// and query flag, encoded with the same shape as Cardano's
// VersionDataNtC15andUp.
func TestGetProtocolVersionMapDMQNtC(t *testing.T) {
	const dmqDefaultMagic uint32 = 3_141_592 // dmq-node default
	m := GetProtocolVersionMapDMQNtC(dmqDefaultMagic, false)
	require.Len(t, m, 1, "DMQ currently advertises only V1")
	v, ok := m[0x1001]
	require.True(t, ok, "DMQ N2C v1 (0x1001) missing from version map")
	assert.Equal(t, dmqDefaultMagic, v.NetworkMagic())
	assert.False(t, v.Query())

	// Query flag passes through.
	mq := GetProtocolVersionMapDMQNtC(dmqDefaultMagic, true)
	vq, ok := mq[0x1001]
	require.True(t, ok, "DMQ N2C v1 (0x1001) missing from query-mode version map")
	assert.True(t, vq.Query(), "Query flag did not propagate from caller to VersionData")
}

// TestGetProtocolVersionDMQResolvable verifies that GetProtocolVersion can
// resolve DMQ N2C versions. Without this, handshake.Client.handleAcceptVersion
// and handshake.Server.handleProposeVersions would panic when calling
// NewVersionDataFromCborFunc on a zero-value ProtocolVersion returned for
// any DMQ version.
func TestGetProtocolVersionDMQResolvable(t *testing.T) {
	require.NotNil(
		t,
		GetProtocolVersion(0x1001).NewVersionDataFromCborFunc,
		"DMQ versions are not reachable via GetProtocolVersion",
	)
}

// TestGetProtocolVersionMapDMQNtCMithril verifies the well-known Mithril
// magics round-trip through the DMQ version map. These values are
// documented in CIP-0137 and used by Mithril's published deployments;
// breaking them would silently misroute DMQ traffic.
func TestGetProtocolVersionMapDMQNtCMithril(t *testing.T) {
	for _, tc := range []struct {
		name  string
		magic uint32
	}{
		{"mainnet", 2_912_307_721},
		{"preprod", 2_147_483_649},
		{"preview", 2_147_483_650},
		{"devnet", 2_147_483_690},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := GetProtocolVersionMapDMQNtC(tc.magic, false)
			v, ok := m[0x1001]
			require.True(t, ok, "DMQ N2C v1 (0x1001) missing from version map")
			assert.Equal(t, tc.magic, v.NetworkMagic())
		})
	}
}
