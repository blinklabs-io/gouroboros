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
	"github.com/stretchr/testify/require"
)

// TestPoolOwnerSetTagWithLeiosKey covers the 11-field pool registration,
// whose owner set is field 8 because the Leios key is field 3: one tag 258
// there decodes, and any other tag is refused as on the 10-field form.
func TestPoolOwnerSetTagWithLeiosKey(t *testing.T) {
	cert := PoolRegistrationCertificate{
		CertType:   uint(CertificateTypePoolRegistration),
		Operator:   NewBlake2b224(bytes.Repeat([]byte{0x01}, Blake2b224Size)),
		VrfKeyHash: NewBlake2b256(bytes.Repeat([]byte{0x02}, Blake2b256Size)),
		LeiosKey: &LeiosKey{
			PublicKey:       bytes.Repeat([]byte{0x05}, LeiosBlsPublicKeySize),
			PossessionProof: bytes.Repeat([]byte{0x06}, LeiosBlsPossessionProofSize),
		},
		Pledge: 1_000_000,
		Cost:   340_000_000,
		Margin: NewGenesisRat(1, 20),
		PoolOwners: []AddrKeyHash{
			NewBlake2b224(bytes.Repeat([]byte{0x04}, Blake2b224Size)),
		},
		Relays: []PoolRelay{},
	}
	require.NoError(t, cert.SetRewardAccountCredential(
		Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: CredentialHash(cert.Operator),
		},
		AddressNetworkTestnet,
	))
	encoded, err := cbor.Encode(cert)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.DecodeExact(encoded, &fields)
	require.NoError(t, err)
	require.Len(t, fields, 11)
	owners := []byte(fields[8])
	require.True(t, bytes.HasPrefix(owners, []byte{0xd9, 0x01, 0x02}))
	untagged := owners[3:]
	for _, tc := range []struct {
		name string
		head []byte
		ok   bool
	}{
		{"untagged", nil, true},
		{"tag 258", []byte{0xd9, 0x01, 0x02}, true},
		{"tag 259", []byte{0xd9, 0x01, 0x03}, false},
		{"tag 30", []byte{0xd8, 0x1e}, false},
		{"tag 258 twice", []byte{0xd9, 0x01, 0x02, 0xd9, 0x01, 0x02}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := append([]cbor.RawMessage(nil), fields...)
			changed[8] = append(append([]byte{}, tc.head...), untagged...)
			data, err := cbor.Encode(changed)
			require.NoError(t, err)
			var decoded PoolRegistrationCertificate
			_, err = cbor.Decode(data, &decoded)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, cert.PoolOwners, decoded.PoolOwners)
				return
			}
			require.Error(t, err, "%x", data)
		})
	}
}
