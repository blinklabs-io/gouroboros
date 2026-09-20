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

package common_test

import (
	"encoding/hex"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// singleHostAddrCBOR builds a single_host_addr relay with the supplied raw
// ipv4 and ipv6 byte strings, passing nil through as the CDDL nil alternative.
func singleHostAddrCBOR(t *testing.T, ipv4, ipv6 []byte) []byte {
	t.Helper()
	var v4 any
	if ipv4 != nil {
		v4 = ipv4
	}
	var v6 any
	if ipv6 != nil {
		v6 = ipv6
	}
	raw, err := cbor.Encode(
		[]any{uint(common.PoolRelayTypeSingleHostAddress), uint32(3001), v4, v6},
	)
	require.NoError(t, err)
	return raw
}

func TestPoolRelayIpv4DecodeWidth(t *testing.T) {
	tests := []struct {
		name   string
		ipv4   []byte
		reject bool
	}{
		{name: "empty", ipv4: []byte{}, reject: true},
		{name: "three bytes", ipv4: []byte{10, 0, 0}, reject: true},
		{name: "five bytes", ipv4: []byte{10, 0, 0, 1, 1}, reject: true},
		{
			name:   "ipv6 width in ipv4 slot",
			ipv4:   make([]byte, 16),
			reject: true,
		},
		{name: "four bytes", ipv4: []byte{10, 0, 0, 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := singleHostAddrCBOR(t, test.ipv4, nil)
			var decoded common.PoolRelay
			_, err := cbor.Decode(raw, &decoded)
			if test.reject {
				require.ErrorIs(t, err, common.ErrPoolRelayAddressWidth)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, decoded.Ipv4)
			assert.Equal(t, net.IP(test.ipv4), *decoded.Ipv4)
			assert.Nil(t, decoded.Ipv6)
		})
	}
}

func TestPoolRelayIpv6DecodeWidth(t *testing.T) {
	tests := []struct {
		name   string
		ipv6   []byte
		reject bool
	}{
		{name: "empty", ipv6: []byte{}, reject: true},
		{
			name:   "ipv4 width in ipv6 slot",
			ipv6:   []byte{10, 0, 0, 1},
			reject: true,
		},
		{name: "fifteen bytes", ipv6: make([]byte, 15), reject: true},
		{name: "seventeen bytes", ipv6: make([]byte, 17), reject: true},
		{name: "sixteen bytes", ipv6: make([]byte, 16)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := singleHostAddrCBOR(t, nil, test.ipv6)
			var decoded common.PoolRelay
			_, err := cbor.Decode(raw, &decoded)
			if test.reject {
				require.ErrorIs(t, err, common.ErrPoolRelayAddressWidth)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, decoded.Ipv6)
			assert.Equal(t, net.IP(test.ipv6), *decoded.Ipv6)
			assert.Nil(t, decoded.Ipv4)
		})
	}
}

// TestPoolRelayAddressWidthRejectedInCertificate proves the bound reaches the
// pool_registration_cert decode path rather than only a bare relay.
func TestPoolRelayAddressWidthRejectedInCertificate(t *testing.T) {
	operator := make([]byte, common.Blake2b224Size)
	vrf := make([]byte, common.Blake2b256Size)
	rewardAccount := append(
		[]byte{0xe1},
		make([]byte, common.Blake2b224Size)...)
	margin, err := hex.DecodeString("d81e820101")
	require.NoError(t, err)

	certFor := func(t *testing.T, relay []byte) []byte {
		t.Helper()
		raw, err := cbor.Encode(
			[]any{
				uint(3),
				operator,
				vrf,
				uint64(0),
				uint64(0),
				cbor.RawMessage(margin),
				rewardAccount,
				[]any{},
				[]any{cbor.RawMessage(relay)},
				nil,
			},
		)
		require.NoError(t, err)
		return raw
	}

	t.Run("ipv4", func(t *testing.T) {
		var cert common.PoolRegistrationCertificate
		_, err := cbor.Decode(
			certFor(t, singleHostAddrCBOR(t, []byte{10, 0, 0, 1, 1}, nil)),
			&cert,
		)
		require.ErrorIs(t, err, common.ErrPoolRelayAddressWidth)
	})
	t.Run("ipv6", func(t *testing.T) {
		var cert common.PoolRegistrationCertificate
		_, err := cbor.Decode(
			certFor(t, singleHostAddrCBOR(t, nil, []byte{10, 0, 0, 1})),
			&cert,
		)
		require.ErrorIs(t, err, common.ErrPoolRelayAddressWidth)
	})
	t.Run("accepted widths", func(t *testing.T) {
		var cert common.PoolRegistrationCertificate
		_, err := cbor.Decode(
			certFor(
				t,
				singleHostAddrCBOR(t, []byte{10, 0, 0, 1}, make([]byte, 16)),
			),
			&cert,
		)
		require.NoError(t, err)
		require.Len(t, cert.Relays, 1)
		require.NotNil(t, cert.Relays[0].Ipv4)
		require.NotNil(t, cert.Relays[0].Ipv6)
		assert.Equal(t, net.IP{10, 0, 0, 1}, *cert.Relays[0].Ipv4)
		assert.Len(t, *cert.Relays[0].Ipv6, 16)
	})
}

// TestPoolRelayAddressWidthPreservesFixtures pins the on-chain relay encodings
// already covered elsewhere in this package, so the bound cannot narrow what
// the chain accepts.
func TestPoolRelayAddressWidthPreservesFixtures(t *testing.T) {
	fixtures := []string{
		// single_host_addr with a 4-byte ipv4
		"8400190bb9440a000001f6",
		// single_host_addr with a 16-byte ipv6
		"8400190bb9f65020010db8000000000000000000000001",
		// single_host_name
		"8301190bb96d72656c61792e6578616d706c65",
		// multi_host_name
		"82026d72656c61792e6578616d706c65",
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			raw, err := hex.DecodeString(fixture)
			require.NoError(t, err)
			var decoded common.PoolRelay
			_, err = cbor.Decode(raw, &decoded)
			require.NoError(t, err)
			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err)
			assert.Equal(t, raw, reencoded)
		})
	}
}

// TestPoolRelayHostnameRequiredAtDecode covers the dns_name slot, which both
// single_host_name and multi_host_name take unconditionally.
func TestPoolRelayHostnameRequiredAtDecode(t *testing.T) {
	hostname := "relay.example"
	tests := []struct {
		name   string
		raw    []any
		reject bool
	}{
		{
			name:   "single host name null",
			raw:    []any{uint(common.PoolRelayTypeSingleHostName), uint32(3001), nil},
			reject: true,
		},
		{
			name: "single host name present",
			raw:  []any{uint(common.PoolRelayTypeSingleHostName), uint32(3001), hostname},
		},
		{
			name:   "multi host name null",
			raw:    []any{uint(common.PoolRelayTypeMultiHostName), nil},
			reject: true,
		},
		{
			name: "multi host name present",
			raw:  []any{uint(common.PoolRelayTypeMultiHostName), hostname},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := cbor.Encode(test.raw)
			require.NoError(t, err)
			var decoded common.PoolRelay
			_, err = cbor.Decode(raw, &decoded)
			if test.reject {
				require.ErrorIs(t, err, common.ErrPoolRelayMissingHostname)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, decoded.Hostname)
			assert.Equal(t, hostname, *decoded.Hostname)
		})
	}
}
