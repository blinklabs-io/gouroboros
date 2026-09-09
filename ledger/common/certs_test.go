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
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStakeDelegationCertificateUnmarshalCBORCredential(t *testing.T) {
	credential := Credential{
		CredType: CredentialTypeAddrKeyHash,
	}
	testCases := []struct {
		name       string
		credential *Credential
		wantErr    bool
	}{
		{
			name:       "valid credential",
			credential: &credential,
		},
		{
			name:    "null credential",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := cbor.Encode([]any{
				uint(CertificateTypeStakeDelegation),
				tc.credential,
				make([]byte, Blake2b224Size),
			})
			require.NoError(t, err)

			var certificate StakeDelegationCertificate
			_, err = cbor.Decode(encoded, &certificate)
			if tc.wantErr {
				require.ErrorContains(
					t,
					err,
					"stake delegation contains a nil credential",
				)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, certificate.StakeCredential)
		})
	}
}

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
	require.NoError(t, base.SetRewardAccountCredential(
		Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: CredentialHash(base.RewardAccount),
		},
		AddressNetworkTestnet,
	))

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
		rewardAccount, err := base.rewardAccountBytes()
		require.NoError(t, err)
		encoded, err := cbor.Encode([]any{
			base.CertType,
			base.Operator,
			base.VrfKeyHash,
			nil,
			base.Pledge,
			base.Cost,
			base.Margin,
			rewardAccount,
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

func TestPoolRegistrationCertificateRewardAccountDecode(t *testing.T) {
	credential := make([]byte, Blake2b224Size)
	for idx := range credential {
		credential[idx] = byte(idx + 1)
	}
	tests := []struct {
		name          string
		rewardAccount []byte
		wantErr       string
	}{
		{
			name:          "short",
			rewardAccount: bytes.Repeat([]byte{0x03}, Blake2b224Size-1),
			wantErr:       "invalid reward account length",
		},
		{
			name:          "legacy credential",
			rewardAccount: credential,
			wantErr:       "invalid reward account length",
		},
		{
			name: "reward address",
			rewardAccount: append(
				[]byte{0xe1},
				credential...,
			),
		},
		{
			name: "non-reward address",
			rewardAccount: append(
				[]byte{0x01},
				credential...,
			),
			wantErr: "invalid reward account address header",
		},
		{
			// 0xf1 is the script-credential reward-address header on
			// mainnet, the last of the four headers
			// headerIsAccountAddress admits.
			name: "script reward address",
			rewardAccount: append(
				[]byte{0xf1},
				credential...,
			),
		},
		// headerIsAccountAddress in Cardano.Ledger.Address requires
		// header .&. 0b11101110 == 0b11100000, so bits 3-1 must be clear
		// and only 0xe0, 0xe1, 0xf0 and 0xf1 are valid. Each header below
		// has the reward-address high nibble but a reserved bit set.
		{
			name: "reserved bit 1 set",
			rewardAccount: append(
				[]byte{0xe2},
				credential...,
			),
			wantErr: "invalid reward account address header",
		},
		{
			name: "reserved bit 2 set",
			rewardAccount: append(
				[]byte{0xe5},
				credential...,
			),
			wantErr: "invalid reward account address header",
		},
		{
			name: "reserved bit 3 set",
			rewardAccount: append(
				[]byte{0xe8},
				credential...,
			),
			wantErr: "invalid reward account address header",
		},
		{
			name:          "long",
			rewardAccount: bytes.Repeat([]byte{0x03}, Blake2b224Size+2),
			wantErr:       "invalid reward account length",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode([]any{
				uint(CertificateTypePoolRegistration),
				NewBlake2b224(bytes.Repeat([]byte{0x01}, Blake2b224Size)),
				NewBlake2b256(bytes.Repeat([]byte{0x02}, Blake2b256Size)),
				uint64(1_000_000),
				uint64(340_000_000),
				NewGenesisRat(1, 20),
				test.rewardAccount,
				[]AddrKeyHash{},
				[]PoolRelay{},
				nil,
			})
			require.NoError(t, err)

			var decoded PoolRegistrationCertificate
			_, err = cbor.Decode(encoded, &decoded)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, AddrKeyHash(credential), decoded.RewardAccount)
			provider, ok := any(&decoded).(interface {
				RewardAccountCredential() Credential
			})
			require.True(
				t,
				ok,
				"decoded pool registration must preserve reward credential type",
			)
			wantType := uint(CredentialTypeAddrKeyHash)
			if test.rewardAccount[0]&0x10 != 0 {
				wantType = uint(CredentialTypeScriptHash)
			}
			assert.Equal(
				t,
				wantType,
				provider.RewardAccountCredential().CredType,
			)
			assert.Equal(t, encoded, decoded.Cbor())
		})
	}
}

func TestPoolRelayCBORRoundTrip(t *testing.T) {
	port := uint32(3001)
	ipv4 := net.IPv4(10, 0, 0, 1).To4()
	ipv6 := net.ParseIP("2001:db8::1")
	hostname := "relay.example"
	tests := []struct {
		name string
		raw  string
		want PoolRelay
	}{
		{
			name: "single host address",
			raw:  "8400190bb9440a000001f6",
			want: PoolRelay{
				Type: PoolRelayTypeSingleHostAddress,
				Port: &port,
				Ipv4: &ipv4,
			},
		},
		{
			name: "single host name",
			raw:  "8301190bb96d72656c61792e6578616d706c65",
			want: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Port:     &port,
				Hostname: &hostname,
			},
		},
		{
			name: "multi host name",
			raw:  "82026d72656c61792e6578616d706c65",
			want: PoolRelay{
				Type:     PoolRelayTypeMultiHostName,
				Hostname: &hostname,
			},
		},
		{
			name: "single host address with ipv6",
			raw:  "8400190bb9f65020010db8000000000000000000000001",
			want: PoolRelay{
				Type: PoolRelayTypeSingleHostAddress,
				Port: &port,
				Ipv6: &ipv6,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := hex.DecodeString(test.raw)
			require.NoError(t, err)

			var decoded PoolRelay
			_, err = cbor.Decode(raw, &decoded)
			require.NoError(t, err)
			assert.Equal(t, test.want, decoded)

			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err)
			assert.Equal(t, raw, reencoded)
		})
	}
}

func TestPoolRelayCBORMarshalFreshValues(t *testing.T) {
	port := uint32(3001)
	ipv4 := net.ParseIP("10.0.0.1")
	hostname := "relay.example"
	tests := []struct {
		name  string
		relay PoolRelay
		want  string
	}{
		{
			name: "single host address",
			relay: PoolRelay{
				Type: PoolRelayTypeSingleHostAddress,
				Port: &port,
				Ipv4: &ipv4,
			},
			want: "8400190bb9440a000001f6",
		},
		{
			name: "single host name",
			relay: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Port:     &port,
				Hostname: &hostname,
			},
			want: "8301190bb96d72656c61792e6578616d706c65",
		},
		{
			name: "multi host name",
			relay: PoolRelay{
				Type:     PoolRelayTypeMultiHostName,
				Hostname: &hostname,
			},
			want: "82026d72656c61792e6578616d706c65",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode(test.relay)
			require.NoError(t, err)
			assert.Equal(t, test.want, hex.EncodeToString(encoded))

			encoded, err = cbor.Encode(&test.relay)
			require.NoError(t, err)
			assert.Equal(t, test.want, hex.EncodeToString(encoded))
		})
	}
}

func TestPoolRelayCBORMarshalRejectsMissingHostname(t *testing.T) {
	tests := []struct {
		name    string
		relay   PoolRelay
		wantErr string
	}{
		{
			name: "single host name",
			relay: PoolRelay{
				Type: PoolRelayTypeSingleHostName,
			},
			wantErr: "single-host-name relay requires hostname",
		},
		{
			name: "multi host name",
			relay: PoolRelay{
				Type: PoolRelayTypeMultiHostName,
			},
			wantErr: "multi-host-name relay requires hostname",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := cbor.Encode(test.relay)
			require.ErrorContains(t, err, test.wantErr)

			_, err = cbor.Encode(&test.relay)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestPoolRelayCBORBounds(t *testing.T) {
	maxPort := uint32(65535)
	overPort := uint32(65536)
	ipv4 := net.IPv4(10, 0, 0, 1).To4()
	hostname := "relay.example"
	maxHostname := strings.Repeat("a", 128)
	overHostname := strings.Repeat("a", 129)
	tests := []struct {
		name    string
		relay   PoolRelay
		raw     []any
		wantErr string
	}{
		{
			name: "address max port",
			relay: PoolRelay{
				Type: PoolRelayTypeSingleHostAddress,
				Port: &maxPort,
				Ipv4: &ipv4,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostAddress),
				maxPort,
				[]byte(ipv4),
				nil,
			},
		},
		{
			name: "address over max port",
			relay: PoolRelay{
				Type: PoolRelayTypeSingleHostAddress,
				Port: &overPort,
				Ipv4: &ipv4,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostAddress),
				overPort,
				[]byte(ipv4),
				nil,
			},
			wantErr: "pool relay port must not exceed 65535",
		},
		{
			name: "single host name max port",
			relay: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Port:     &maxPort,
				Hostname: &hostname,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostName),
				maxPort,
				hostname,
			},
		},
		{
			name: "single host name over max port",
			relay: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Port:     &overPort,
				Hostname: &hostname,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostName),
				overPort,
				hostname,
			},
			wantErr: "pool relay port must not exceed 65535",
		},
		{
			name: "single host name max length",
			relay: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Hostname: &maxHostname,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostName),
				nil,
				maxHostname,
			},
		},
		{
			name: "single host name over max length",
			relay: PoolRelay{
				Type:     PoolRelayTypeSingleHostName,
				Hostname: &overHostname,
			},
			raw: []any{
				uint(PoolRelayTypeSingleHostName),
				nil,
				overHostname,
			},
			wantErr: "pool relay hostname must not exceed 128 bytes",
		},
		{
			name: "multi host name max length",
			relay: PoolRelay{
				Type:     PoolRelayTypeMultiHostName,
				Hostname: &maxHostname,
			},
			raw: []any{
				uint(PoolRelayTypeMultiHostName),
				maxHostname,
			},
		},
		{
			name: "multi host name over max length",
			relay: PoolRelay{
				Type:     PoolRelayTypeMultiHostName,
				Hostname: &overHostname,
			},
			raw: []any{
				uint(PoolRelayTypeMultiHostName),
				overHostname,
			},
			wantErr: "pool relay hostname must not exceed 128 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("marshal", func(t *testing.T) {
				_, err := cbor.Encode(test.relay)
				if test.wantErr == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, test.wantErr)
				}
			})

			t.Run("unmarshal", func(t *testing.T) {
				raw, err := cbor.Encode(test.raw)
				require.NoError(t, err)
				var decoded PoolRelay
				_, err = cbor.Decode(raw, &decoded)
				if test.wantErr == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, test.wantErr)
				}
			})
		})
	}
}

func TestPoolRegistrationCertificateRejectsShortOperatorKey(t *testing.T) {
	var cert PoolRegistrationCertificate
	err := json.Unmarshal([]byte(`{"publicKey":"01"}`), &cert)
	require.Error(t, err)
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

			encoded, err = json.Marshal(struct {
				PublicKey       []byte `json:"publicKey"`
				PossessionProof []byte `json:"possessionProof"`
			}{
				PublicKey:       test.key,
				PossessionProof: test.proof,
			})
			require.NoError(t, err)
			err = json.Unmarshal(encoded, &decoded)
			require.Error(t, err)
		})
	}
}

func TestDrepCBORRoundTrip(t *testing.T) {
	testCases := []Drep{
		{Type: DrepTypeAddrKeyHash, Credential: make([]byte, 28)},
		{Type: DrepTypeScriptHash, Credential: bytes.Repeat([]byte{0xaa}, 28)},
		{Type: DrepTypeAbstain},
		{Type: DrepTypeNoConfidence},
	}
	for _, expected := range testCases {
		encoded, err := cbor.Encode(expected)
		require.NoError(t, err)
		var actual Drep
		_, err = cbor.Decode(encoded, &actual)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}
