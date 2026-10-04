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
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestCredentialWireDiscriminant(t *testing.T) {
	hash := strings.Repeat("ab", common.Blake2b224Size)
	for _, tc := range []struct {
		name, wire string
		typ        uint
	}{
		{"key", "8200581c" + hash, 0},
		{"script", "8201581c" + hash, 1},
		{"wide_unsigned_key", "821800581c" + hash, 0},
		{"indefinite_script", "9f01581c" + hash + "ff", 1},
		{"wide_array_8", "980200581c" + hash, 0},
		{"wide_array_16", "99000201581c" + hash, 1},
		{"wide_array_32", "9a0000000200581c" + hash, 0},
		{"wide_array_64", "9b000000000000000201581c" + hash, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tc.wire)
			require.NoError(t, err)
			var cred common.Credential
			require.NoError(t, cred.UnmarshalCBOR(wire))
			require.Equal(t, tc.typ, cred.CredType)
			require.Equal(
				t,
				bytes.Repeat([]byte{0xab}, common.Blake2b224Size),
				cred.Credential.Bytes(),
			)
			require.Equal(t, wire, cred.Cbor())
			cred.SetCbor(nil)
			encoded, err := cbor.Encode(cred)
			require.NoError(t, err)
			expected, err := hex.DecodeString(
				"820" + string('0'+byte(tc.typ)) + "581c" + hash,
			)
			require.NoError(t, err)
			require.Equal(t, expected, encoded)
		})
	}
	for _, tc := range []struct{ name, wire string }{
		{"null_type", "82f6581c" + hash},
		{"undefined_type", "82f7581c" + hash},
		{"bignum_type", "82c24100581c" + hash},
		{"tagged_type", "82d86400581c" + hash},
		{"tagged_tuple", "d8648200581c" + hash},
		{"one_field", "8100"},
		{"one_indefinite_field", "9f00ff"},
		{"three_indefinite_fields", "9f00581c" + hash + "00ff"},
		{"missing_type", "9802"},
		{"truncated_header", "9b0000000000000002"},
		{"three_fields", "8300581c" + hash + "00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tc.wire)
			require.NoError(t, err)
			t.Run("direct", func(t *testing.T) {
				original := common.Credential{
					CredType: 1,
					Credential: common.NewBlake2b224(
						bytes.Repeat([]byte{0xcd}, common.Blake2b224Size),
					),
				}
				original.SetCbor([]byte{0x80})
				cred := original
				require.Error(t, cred.UnmarshalCBOR(wire))
				require.Equal(t, original, cred)
			})
			t.Run("MIR_map", func(t *testing.T) {
				var reward common.MoveInstantaneousRewardsCertificateReward
				_, err = cbor.Decode(
					append(append([]byte{0x82, 0x00, 0xa1}, wire...), 0x01),
					&reward,
				)
				require.Error(t, err)
			})
			t.Run("stake_certificate", func(t *testing.T) {
				var certificate common.StakeRegistrationCertificate
				_, err = cbor.Decode(
					append([]byte{0x82, 0x00}, wire...),
					&certificate,
				)
				require.Error(t, err)
			})
		})
	}
}
