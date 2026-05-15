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

	"github.com/stretchr/testify/require"
)

// TestVersionDataNtN11to12PeerSharingModes verifies that PeerSharing() reports
// "active" for both private (1) and public (2) modes, and "inactive" only for
// NoPeerSharing (0). V11/V12 advertised three modes (0/1/2) — V13 and later
// collapsed this to two (0/1).
func TestVersionDataNtN11to12PeerSharingModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     uint
		expected bool
	}{
		{"NoPeerSharing", PeerSharingModeV11NoPeerSharing, false},
		{"Private", PeerSharingModeV11PeerSharingPrivate, true},
		{"Public", PeerSharingModeV11PeerSharingPublic, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vd := VersionDataNtN11to12{CborPeerSharing: tt.mode}
			require.Equal(t, tt.expected, vd.PeerSharing(),
				"VersionDataNtN11to12.PeerSharing() with mode %d", tt.mode)
		})
	}
}

// TestVersionDataNtN13andUpPeerSharingModes verifies the V13+ mode mapping:
// 0 = NoPeerSharing (inactive), 1 = PeerSharingPublic (active).
func TestVersionDataNtN13andUpPeerSharingModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     uint
		expected bool
	}{
		{"NoPeerSharing", PeerSharingModeNoPeerSharing, false},
		{"Public", PeerSharingModePeerSharingPublic, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vd := VersionDataNtN13andUp{
				VersionDataNtN11to12: VersionDataNtN11to12{
					CborPeerSharing: tt.mode,
				},
			}
			require.Equal(t, tt.expected, vd.PeerSharing(),
				"VersionDataNtN13andUp.PeerSharing() with mode %d", tt.mode)
		})
	}
}
