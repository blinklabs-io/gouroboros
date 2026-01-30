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

	"github.com/stretchr/testify/assert"
)

func TestProtocolVersionConstants(t *testing.T) {
	// Verify known hard fork protocol versions
	assert.Equal(t, uint(2), ProtocolVersionShelley)
	assert.Equal(t, uint(3), ProtocolVersionAllegra)
	assert.Equal(t, uint(4), ProtocolVersionMary)
	assert.Equal(t, uint(6), ProtocolVersionAlonzo)
	assert.Equal(t, uint(8), ProtocolVersionBabbage)
	assert.Equal(t, uint(9), ProtocolVersionConway)
	assert.Equal(t, uint(10), ProtocolVersionConwayPlus)
	assert.Equal(t, uint(11), ProtocolVersionVanRossem)
}

func TestIsProtocolVersionAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		major    uint
		minor    uint
		target   uint
		expected bool
	}{
		{"PV11 meets PV11", 11, 0, 11, true},
		{"PV12 meets PV11", 12, 0, 11, true},
		{"PV10 fails PV11", 10, 0, 11, false},
		{"PV9 meets PV9", 9, 0, 9, true},
		{"PV8 fails PV9", 8, 0, 9, false},
		{"minor version ignored", 10, 5, 11, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProtocolVersionAtLeast(tt.major, tt.minor, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}
