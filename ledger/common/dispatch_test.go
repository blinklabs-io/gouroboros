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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/stretchr/testify/require"
)

func TestCommonDispatchersAcceptListLengthEncodings(t *testing.T) {
	credential := Credential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: Blake2b224{1, 2, 3},
	}

	t.Run("CertificateWrapper", func(t *testing.T) {
		canonical, err := cbor.Encode(&StakeRegistrationCertificate{
			CertType:        uint(CertificateTypeStakeRegistration),
			StakeCredential: credential,
		})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				var decoded CertificateWrapper
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.IsType(t, &StakeRegistrationCertificate{}, decoded.Certificate)
			})
		}
	})

	t.Run("Drep", func(t *testing.T) {
		canonical, err := cbor.Encode(Drep{Type: DrepTypeAbstain})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				var decoded Drep
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.Equal(t, DrepTypeAbstain, decoded.Type)
			})
		}
	})

	t.Run("PoolRelay", func(t *testing.T) {
		canonical, err := cbor.Encode([]any{1, uint32(3001), "relay.example"})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				var decoded PoolRelay
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.Equal(t, PoolRelayTypeSingleHostName, decoded.Type)
				require.NotNil(t, decoded.Hostname)
				require.Equal(t, "relay.example", *decoded.Hostname)
			})
		}
	})

	t.Run("Nonce", func(t *testing.T) {
		canonical, err := cbor.Encode([]any{NonceTypeNeutral})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				var decoded Nonce
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.Equal(t, uint(NonceTypeNeutral), decoded.Type)
			})
		}
	})

	t.Run("NativeScript", func(t *testing.T) {
		canonical, err := cbor.Encode(NativeScriptInvalidBefore{Type: 4, Slot: 5})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				var decoded NativeScript
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.IsType(t, &NativeScriptInvalidBefore{}, decoded.Item())
			})
		}
	})
}
