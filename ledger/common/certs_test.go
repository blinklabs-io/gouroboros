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
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDrepString tests CIP-0129 bech32 encoding for DRep identifiers.
func TestDrepString(t *testing.T) {
	var zeroHash = make([]byte, 28)
	var sequentialHash = make([]byte, 28)
	for i := range sequentialHash {
		sequentialHash[i] = byte(i)
	}

	testCases := []struct {
		name string
		drep Drep
		want string
	}{
		{
			name: "CIP0129KeyHashZero",
			drep: Drep{
				Type:       DrepTypeAddrKeyHash,
				Credential: zeroHash,
			},
			want: "drep1ygqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq7vlc9n",
		},
		{
			name: "CIP0129ScriptHashZero",
			drep: Drep{
				Type:       DrepTypeScriptHash,
				Credential: zeroHash,
			},
			want: "drep1yvqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq770f95",
		},
		{
			name: "CIP0129KeyHashSequential",
			drep: Drep{
				Type:       DrepTypeAddrKeyHash,
				Credential: sequentialHash,
			},
			// Uses CIP-0129 header byte encoding (0x22 for key hash)
			want: "drep1ygqqzqsrqszsvpcgpy9qkrqdpc83qygjzv2p29shrqv35xc6zv3a4",
		},
		{
			name: "CIP0129Abstain",
			drep: Drep{
				Type: DrepTypeAbstain,
			},
			want: "drep_abstain",
		},
		{
			name: "CIP0129NoConfidence",
			drep: Drep{
				Type: DrepTypeNoConfidence,
			},
			want: "drep_no_confidence",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.drep.String()
			assert.Equal(t, tc.want, result)
		})
	}

	// Test unknown type returns descriptive string (doesn't panic)
	t.Run("UnknownType", func(t *testing.T) {
		drep := Drep{Type: 99}
		result := drep.String()
		assert.Equal(t, "drep_unknown_99", result)
	})

	// Test with wrong credential length (should still encode but produces non-standard output)
	// CIP-0129 expects 28-byte credentials for key/script hashes
	t.Run("ShortCredential", func(t *testing.T) {
		drep := Drep{
			Type:       DrepTypeAddrKeyHash,
			Credential: []byte{0x01, 0x02, 0x03}, // Only 3 bytes
		}
		// Should not panic, but produces non-standard bech32
		result := drep.String()
		assert.True(t, len(result) > 0)
		assert.True(t, strings.HasPrefix(result, "drep1"))
	})

	// Test nil credential for abstain/no-confidence (should work fine)
	t.Run("NilCredentialAbstain", func(t *testing.T) {
		drep := Drep{
			Type:       DrepTypeAbstain,
			Credential: nil,
		}
		assert.Equal(t, "drep_abstain", drep.String())
	})
}

func TestDrepUnmarshalJson(t *testing.T) {
	testDefs := []struct {
		json         string
		expectedDrep Drep
	}{
		{
			json: `"drep-keyHash-cec68dbf1507d74f92ec025cbce4122f10e7ed421c657924e9502a5e"`,
			expectedDrep: Drep{
				Type: DrepTypeAddrKeyHash,
				Credential: func() []byte {
					foo, _ := hex.DecodeString(
						`cec68dbf1507d74f92ec025cbce4122f10e7ed421c657924e9502a5e`,
					)
					return foo
				}(),
			},
		},
		{
			json: `"drep-scriptHash-83938146ce90d8b57ea5fde8734e3fc31fcc330c875d3b5a4c8d1830"`,
			expectedDrep: Drep{
				Type: DrepTypeScriptHash,
				Credential: func() []byte {
					foo, _ := hex.DecodeString(
						`83938146ce90d8b57ea5fde8734e3fc31fcc330c875d3b5a4c8d1830`,
					)
					return foo
				}(),
			},
		},
		{
			json: `"drep-alwaysAbstain"`,
			expectedDrep: Drep{
				Type: DrepTypeAbstain,
			},
		},
		{
			json: `"drep-alwaysNoConfidence"`,
			expectedDrep: Drep{
				Type: DrepTypeNoConfidence,
			},
		},
	}
	for _, testDef := range testDefs {
		var tmpDrep Drep
		if err := json.Unmarshal([]byte(testDef.json), &tmpDrep); err != nil {
			t.Errorf("unexpected error decoding JSON: %s", err)
			continue
		}
		if !reflect.DeepEqual(tmpDrep, testDef.expectedDrep) {
			t.Errorf(
				"did not get expected Drep value\n     got: %#v\n  wanted: %#v",
				tmpDrep,
				testDef.expectedDrep,
			)
		}
	}
}

func TestPoolRegistrationCertificateLeiosKey(t *testing.T) {
	base := PoolRegistrationCertificate{
		CertType: uint(CertificateTypePoolRegistration),
		Operator: NewBlake2b224(
			bytes.Repeat([]byte{0x01}, Blake2b224Size),
		),
		VrfKeyHash: NewBlake2b256(
			bytes.Repeat([]byte{0x02}, Blake2b256Size),
		),
		Pledge: 1_000_000,
		Cost:   340_000_000,
		Margin: NewGenesisRat(1, 20),
		RewardAccount: NewBlake2b224(
			bytes.Repeat([]byte{0x03}, Blake2b224Size),
		),
		PoolOwners: []AddrKeyHash{
			NewBlake2b224(bytes.Repeat([]byte{0x04}, Blake2b224Size)),
		},
		Relays:       []PoolRelay{},
		PoolMetadata: nil,
	}

	t.Run("legacy certificate remains 10 fields", func(t *testing.T) {
		encoded, err := cbor.Encode(base)
		require.NoError(t, err)

		var fields []cbor.RawMessage
		_, err = cbor.Decode(encoded, &fields)
		require.NoError(t, err)
		require.Len(t, fields, 10)

		var decoded PoolRegistrationCertificate
		_, err = cbor.Decode(encoded, &decoded)
		require.NoError(t, err)
		assert.Nil(t, decoded.LeiosKey)
		assert.Equal(t, encoded, decoded.Cbor())
	})

	t.Run("Dijkstra certificate carries BLS key and proof", func(t *testing.T) {
		want := base
		want.LeiosKey = &LeiosKey{
			PublicKey: bytes.Repeat(
				[]byte{0x05},
				LeiosBlsPublicKeySize,
			),
			PossessionProof: bytes.Repeat(
				[]byte{0x06},
				LeiosBlsPossessionProofSize,
			),
		}
		encoded, err := cbor.Encode(want)
		require.NoError(t, err)

		var fields []cbor.RawMessage
		_, err = cbor.Decode(encoded, &fields)
		require.NoError(t, err)
		require.Len(t, fields, 11)

		var decoded PoolRegistrationCertificate
		_, err = cbor.Decode(encoded, &decoded)
		require.NoError(t, err)
		require.NotNil(t, decoded.LeiosKey)
		assert.Equal(t, want.LeiosKey, decoded.LeiosKey)
		assert.Equal(t, want.Pledge, decoded.Pledge)
		assert.Equal(t, encoded, decoded.Cbor())

		reencoded, err := cbor.Encode(decoded)
		require.NoError(t, err)
		assert.Equal(t, encoded, reencoded)
	})

	t.Run("Dijkstra certificate accepts explicit null key", func(t *testing.T) {
		encoded, err := cbor.Encode([]any{
			base.CertType,
			base.Operator,
			base.VrfKeyHash,
			nil,
			base.Pledge,
			base.Cost,
			base.Margin,
			base.RewardAccount,
			base.PoolOwners,
			base.Relays,
			base.PoolMetadata,
		})
		require.NoError(t, err)

		var decoded PoolRegistrationCertificate
		_, err = cbor.Decode(encoded, &decoded)
		require.NoError(t, err)
		assert.Nil(t, decoded.LeiosKey)
		assert.Equal(t, base.Pledge, decoded.Pledge)

		reencoded, err := cbor.Encode(decoded)
		require.NoError(t, err)
		assert.Equal(t, encoded, reencoded)
	})
}

func TestLeiosKeyRejectsInvalidLengths(t *testing.T) {
	tests := []struct {
		name  string
		key   []byte
		proof []byte
	}{
		{
			name:  "short public key",
			key:   make([]byte, LeiosBlsPublicKeySize-1),
			proof: make([]byte, LeiosBlsPossessionProofSize),
		},
		{
			name:  "short possession proof",
			key:   make([]byte, LeiosBlsPublicKeySize),
			proof: make([]byte, LeiosBlsPossessionProofSize-1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode([]any{test.key, test.proof})
			require.NoError(t, err)
			var decoded LeiosKey
			_, err = cbor.Decode(encoded, &decoded)
			require.Error(t, err)
		})
	}
}
