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

package bench

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBenchLedgerState(t *testing.T) {
	ls := BenchLedgerState()
	require.NotNil(t, ls)
	// Mainnet network ID is 1
	assert.Equal(t, uint(1), ls.NetworkId())
}

func TestBlockTypeFromEra(t *testing.T) {
	tests := []struct {
		era      string
		expected uint
		wantErr  bool
	}{
		{"byron", 1, false},
		{"shelley", 2, false},
		{"allegra", 3, false},
		{"mary", 4, false},
		{"alonzo", 5, false},
		{"babbage", 6, false},
		{"conway", 7, false},
		{"Conway", 7, false}, // case insensitive
		{"BYRON", 1, false},  // case insensitive
		{"unknown", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.era, func(t *testing.T) {
			blockType, err := BlockTypeFromEra(tc.era)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, blockType)
			}
		})
	}
}

func TestLoadBlockFixture(t *testing.T) {
	for _, era := range EraNames() {
		t.Run(era, func(t *testing.T) {
			fixture, err := LoadBlockFixture(era, "default")
			require.NoError(t, err)
			require.NotNil(t, fixture)

			assert.Equal(t, era, fixture.Era)
			assert.NotNil(t, fixture.Block)
			assert.NotEmpty(t, fixture.Cbor)
		})
	}
}

func TestLoadBlockFixture_UnknownEra(t *testing.T) {
	_, err := LoadBlockFixture("unknown", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown era")
}

func TestMustLoadBlockFixture_Panics(t *testing.T) {
	assert.Panics(t, func() {
		MustLoadBlockFixture("unknown", "default")
	})
}

func TestEraNames(t *testing.T) {
	eras := EraNames()
	assert.Len(t, eras, 7)
	assert.Contains(t, eras, "byron")
	assert.Contains(t, eras, "conway")
}

func TestPostByronEraNames(t *testing.T) {
	eras := PostByronEraNames()
	assert.Len(t, eras, 6)
	assert.NotContains(t, eras, "byron")
	assert.Contains(t, eras, "shelley")
	assert.Contains(t, eras, "conway")
}

func TestGetTestBlocks(t *testing.T) {
	blocks := GetTestBlocks()
	assert.Len(t, blocks, 7)

	// Verify all eras are present
	eraNames := make(map[string]bool)
	for _, block := range blocks {
		eraNames[block.Name] = true
	}

	assert.True(t, eraNames["Byron"])
	assert.True(t, eraNames["Shelley"])
	assert.True(t, eraNames["Allegra"])
	assert.True(t, eraNames["Mary"])
	assert.True(t, eraNames["Alonzo"])
	assert.True(t, eraNames["Babbage"])
	assert.True(t, eraNames["Conway"])
}
