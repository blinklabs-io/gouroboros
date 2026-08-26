// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package consensus

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func networkConfigJSON(
	securityParam uint64,
	activeSlotCoeff string,
	slotLength string,
	epochLength uint64,
	slotsPerKESPeriod uint64,
	maxKESEvolutions uint64,
) string {
	return fmt.Sprintf(`{
		"securityParam": %d,
		"activeSlotsCoeff": %s,
		"slotLength": %s,
		"epochLength": %d,
		"slotsPerKESPeriod": %d,
		"maxKESEvolutions": %d
	}`,
		securityParam,
		activeSlotCoeff,
		slotLength,
		epochLength,
		slotsPerKESPeriod,
		maxKESEvolutions,
	)
}

func TestNewNetworkConfigFromReaderRejectsInvalidConfig(t *testing.T) {
	const validRat = `{"numerator": 1, "denominator": 1}`
	tests := []struct {
		name        string
		config      string
		expectError string
	}{
		{
			name:        "all zero",
			config:      `{}`,
			expectError: "security parameter",
		},
		{
			name: "active slot coefficient zero denominator",
			config: networkConfigJSON(
				1,
				`{"numerator": 1, "denominator": 0}`,
				validRat,
				1, 1, 1,
			),
			expectError: "denominator cannot be zero",
		},
		{
			name: "slot length zero denominator",
			config: networkConfigJSON(
				1,
				validRat,
				`{"numerator": 1, "denominator": 0}`,
				1, 1, 1,
			),
			expectError: "denominator cannot be zero",
		},
		{
			name: "missing active slot coefficient",
			config: `{
				"securityParam": 1,
				"slotLength": {"numerator": 1, "denominator": 1},
				"epochLength": 1,
				"slotsPerKESPeriod": 1,
				"maxKESEvolutions": 1
			}`,
			expectError: "active slot coefficient",
		},
		{
			name: "zero active slot coefficient",
			config: networkConfigJSON(
				1,
				`{"numerator": 0, "denominator": 1}`,
				validRat,
				1, 1, 1,
			),
			expectError: "active slot coefficient",
		},
		{
			name: "negative active slot coefficient",
			config: networkConfigJSON(
				1,
				`{"numerator": -1, "denominator": 20}`,
				validRat,
				1, 1, 1,
			),
			expectError: "active slot coefficient",
		},
		{
			name: "active slot coefficient above one",
			config: networkConfigJSON(
				1,
				`{"numerator": 2, "denominator": 1}`,
				validRat,
				1, 1, 1,
			),
			expectError: "active slot coefficient",
		},
		{
			name: "zero slot length",
			config: networkConfigJSON(
				1,
				validRat,
				`{"numerator": 0, "denominator": 1}`,
				1, 1, 1,
			),
			expectError: "slot length",
		},
		{
			name: "negative slot length",
			config: networkConfigJSON(
				1,
				validRat,
				`{"numerator": -1, "denominator": 1}`,
				1, 1, 1,
			),
			expectError: "slot length",
		},
		{
			name:        "zero epoch length",
			config:      networkConfigJSON(1, validRat, validRat, 0, 1, 1),
			expectError: "epoch length",
		},
		{
			name:        "zero slots per KES period",
			config:      networkConfigJSON(1, validRat, validRat, 1, 0, 1),
			expectError: "slots per KES period",
		},
		{
			name:        "zero maximum KES evolutions",
			config:      networkConfigJSON(1, validRat, validRat, 1, 1, 0),
			expectError: "maximum KES evolutions",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				_, err = NewNetworkConfigFromReader(
					strings.NewReader(test.config),
				)
			}, "invalid genesis config must return an error, not panic")
			require.ErrorContains(t, err, test.expectError)
		})
	}
}

func TestNewNetworkConfigFromReaderAcceptsBoundaryConfig(t *testing.T) {
	config, err := NewNetworkConfigFromReader(strings.NewReader(`{
		"securityParam": 1,
		"activeSlotsCoeff": {"numerator": 1, "denominator": 1},
		"slotLength": {"numerator": 1, "denominator": 1000000000},
		"epochLength": 1,
		"slotsPerKESPeriod": 1,
		"maxKESEvolutions": 1
	}`))
	require.NoError(t, err)
	require.Equal(t, int64(1), config.ActiveSlotCoeff.Num().Int64())
	require.Equal(t, int64(1), config.ActiveSlotCoeff.Denom().Int64())
	require.Equal(t, int64(1), config.SlotLength.Num().Int64())
	require.Equal(t, int64(1_000_000_000), config.SlotLength.Denom().Int64())
}
