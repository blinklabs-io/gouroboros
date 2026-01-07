// Copyright 2025 Blink Labs Software
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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
