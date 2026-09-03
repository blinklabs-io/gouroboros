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

package common

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPoolRegistrationRewardAccountNetworkId covers the network id recovered
// from a pool registration's reward account header byte, which the POOL rule's
// WrongNetworkPOOL predicate compares against the ledger's network.
func TestPoolRegistrationRewardAccountNetworkId(t *testing.T) {
	credential := bytes.Repeat([]byte{0x07}, Blake2b224Size)
	encode := func(t *testing.T, rewardAccount []byte) []byte {
		t.Helper()
		wire, err := cbor.Encode([]any{
			uint(CertificateTypePoolRegistration),
			NewBlake2b224(bytes.Repeat([]byte{0x01}, Blake2b224Size)),
			NewBlake2b256(bytes.Repeat([]byte{0x02}, Blake2b256Size)),
			uint64(1_000_000),
			uint64(340_000_000),
			NewGenesisRat(0, 1),
			rewardAccount,
			[]AddrKeyHash{},
			[]PoolRelay{},
			nil,
		})
		require.NoError(t, err)
		return wire
	}

	for _, networkId := range []byte{0, 1} {
		t.Run(
			"header network "+string(rune('0'+networkId)),
			func(t *testing.T) {
				rewardAccount := append(
					[]byte{0xe0 | networkId},
					credential...,
				)
				cert := &PoolRegistrationCertificate{}
				require.NoError(
					t,
					cert.UnmarshalCBOR(encode(t, rewardAccount)),
				)
				assert.Equal(
					t,
					AddrKeyHash(NewBlake2b224(credential)),
					cert.RewardAccount,
				)
				got, known := cert.RewardAccountNetworkId()
				require.True(t, known)
				assert.Equal(t, uint(networkId), got)
			},
		)
	}

	t.Run("script credential header keeps its network", func(t *testing.T) {
		// 0xf0 is the script-hash reward-address header. Only the low
		// nibble carries the network id.
		rewardAccount := append([]byte{0xf1}, credential...)
		cert := &PoolRegistrationCertificate{}
		require.NoError(t, cert.UnmarshalCBOR(encode(t, rewardAccount)))
		got, known := cert.RewardAccountNetworkId()
		require.True(t, known)
		assert.Equal(t, uint(1), got)
	})

	t.Run("legacy 28-byte encoding has no network id", func(t *testing.T) {
		cert := &PoolRegistrationCertificate{}
		require.NoError(t, cert.UnmarshalCBOR(encode(t, credential)))
		assert.Equal(
			t,
			AddrKeyHash(NewBlake2b224(credential)),
			cert.RewardAccount,
		)
		got, known := cert.RewardAccountNetworkId()
		assert.False(t, known)
		assert.Equal(t, uint(0), got)
	})

	t.Run("constructed certificate has no network id", func(t *testing.T) {
		cert := &PoolRegistrationCertificate{
			RewardAccount: NewBlake2b224(credential),
		}
		_, known := cert.RewardAccountNetworkId()
		assert.False(t, known)
	})

	t.Run("decoding preserves the wire bytes", func(t *testing.T) {
		rewardAccount := append([]byte{0xe1}, credential...)
		wire := encode(t, rewardAccount)
		cert := &PoolRegistrationCertificate{}
		require.NoError(t, cert.UnmarshalCBOR(wire))
		remarshaled, err := cert.MarshalCBOR()
		require.NoError(t, err)
		assert.Equal(t, wire, remarshaled)
	})
}

// TestPoolMetadataHashLengthIsFixed proves a pool registration whose metadata
// hash is not exactly 32 bytes fails to decode. The Shelley POOL rule's
// PoolMedataHashTooBig predicate is unreachable for that reason, so
// shelley.UtxoValidatePoolCertificates does not reimplement it.
func TestPoolMetadataHashLengthIsFixed(t *testing.T) {
	encode := func(t *testing.T, metadata any) []byte {
		t.Helper()
		wire, err := cbor.Encode([]any{
			uint(CertificateTypePoolRegistration),
			NewBlake2b224(bytes.Repeat([]byte{0x01}, Blake2b224Size)),
			NewBlake2b256(bytes.Repeat([]byte{0x02}, Blake2b256Size)),
			uint64(1_000_000),
			uint64(340_000_000),
			NewGenesisRat(0, 1),
			append(
				[]byte{0xe1},
				bytes.Repeat([]byte{0x07}, Blake2b224Size)...,
			),
			[]AddrKeyHash{},
			[]PoolRelay{},
			metadata,
		})
		require.NoError(t, err)
		return wire
	}

	t.Run("32-byte metadata hash decodes", func(t *testing.T) {
		wire := encode(t, []any{
			"https://example.com/pool.json",
			bytes.Repeat([]byte{0x05}, Blake2b256Size),
		})
		cert := &PoolRegistrationCertificate{}
		require.NoError(t, cert.UnmarshalCBOR(wire))
		require.NotNil(t, cert.PoolMetadata)
		assert.Equal(
			t,
			PoolMetadataHash(
				NewBlake2b256(bytes.Repeat([]byte{0x05}, Blake2b256Size)),
			),
			cert.PoolMetadata.Hash,
		)
	})

	wrongSizes := map[string]int{
		"one byte short": Blake2b256Size - 1,
		"one byte long":  Blake2b256Size + 1,
	}
	for name, size := range wrongSizes {
		t.Run(name+" fails to decode", func(t *testing.T) {
			wire := encode(t, []any{
				"https://example.com/pool.json",
				bytes.Repeat([]byte{0x05}, size),
			})
			cert := &PoolRegistrationCertificate{}
			err := cert.UnmarshalCBOR(wire)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blake2b-256 hash")
		})
	}
}

// TestPoolRegistrationSetCborInvalidatesNetworkId pins the cache invalidation
// on the field WrongNetworkPOOL reads. SetCbor(nil) followed by field mutation
// is a supported pattern for re-encoding a decoded certificate, and the decoded
// reward-account header must not outlive the bytes it came from.
func TestPoolRegistrationSetCborPreservesNetworkId(t *testing.T) {
	credential := bytes.Repeat([]byte{0x07}, Blake2b224Size)
	wire, err := cbor.Encode([]any{
		uint(CertificateTypePoolRegistration),
		NewBlake2b224(bytes.Repeat([]byte{0x01}, Blake2b224Size)),
		NewBlake2b256(bytes.Repeat([]byte{0x02}, Blake2b256Size)),
		uint64(1_000_000),
		uint64(340_000_000),
		NewGenesisRat(0, 1),
		append([]byte{0xe1}, credential...),
		[]AddrKeyHash{},
		[]PoolRelay{},
		nil,
	})
	require.NoError(t, err)

	cert := &PoolRegistrationCertificate{}
	require.NoError(t, cert.UnmarshalCBOR(wire))

	// Decoding must keep the header metadata: UnmarshalCBOR caches the wire
	// bytes through the embedded DecodeStoreCbor rather than this override.
	networkId, known := cert.RewardAccountNetworkId()
	require.True(t, known)
	assert.Equal(t, uint(AddressNetworkMainnet), networkId)

	// Clearing the cached bytes must preserve it.
	cert.SetCbor(nil)
	networkId, known = cert.RewardAccountNetworkId()
	assert.True(t, known)
	assert.Equal(t, uint(AddressNetworkMainnet), networkId)

	// Replacing the cached bytes must drop it too.
	cert2 := &PoolRegistrationCertificate{}
	require.NoError(t, cert2.UnmarshalCBOR(wire))
	cert2.SetCbor([]byte{0x01})
	networkId, known = cert2.RewardAccountNetworkId()
	assert.True(t, known)
	assert.Equal(t, uint(AddressNetworkMainnet), networkId)
}
