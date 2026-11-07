// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package conway_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

func loadConwayDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	doc := map[string]any{}
	require.NoError(t, dec.Decode(&doc))
	return doc
}

// conwayWith decodes the mainnet fixture after setting the dotted path. A nil
// value deletes the key.
func conwayWith(t *testing.T, path string, value any) error {
	t.Helper()
	doc := loadConwayDoc(t, "testdata/mainnet-conway-genesis.json")
	parts := strings.Split(path, ".")
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		require.True(t, ok, "path element %q is not an object", part)
		cur = next
	}
	last := parts[len(parts)-1]
	if value == nil {
		delete(cur, last)
	} else {
		cur[last] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	_, err = conway.NewConwayGenesisFromReader(bytes.NewReader(data))
	return err
}

func TestConwayGenesisRealNetworkFilesAccepted(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"testdata/mainnet-conway-genesis.json",
		"testdata/preprod-conway-genesis.json",
		"testdata/preview-conway-genesis.json",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			_, err := conway.NewConwayGenesisFromFile(path)
			require.NoError(t, err)
		})
	}
}

func TestConwayGenesisRejectsEmptyObject(t *testing.T) {
	t.Parallel()
	_, err := conway.NewConwayGenesisFromReader(strings.NewReader(`{}`))
	require.ErrorContains(t, err, "missing required field")
}

func TestConwayGenesisUnknownKeyOnlyIsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, err := conway.NewConwayGenesisFromReader(
		strings.NewReader(`{"bogus": 1}`),
	)
	require.ErrorContains(t, err, "missing required field")
	require.NotContains(t, err.Error(), "unknown field")
}

func TestConwayGenesisRequiredFields(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"poolVotingThresholds",
		"poolVotingThresholds.motionNoConfidence",
		"poolVotingThresholds.committeeNormal",
		"poolVotingThresholds.committeeNoConfidence",
		"poolVotingThresholds.hardForkInitiation",
		"poolVotingThresholds.ppSecurityGroup",
		"dRepVotingThresholds",
		"dRepVotingThresholds.motionNoConfidence",
		"dRepVotingThresholds.committeeNormal",
		"dRepVotingThresholds.committeeNoConfidence",
		"dRepVotingThresholds.updateToConstitution",
		"dRepVotingThresholds.hardForkInitiation",
		"dRepVotingThresholds.ppNetworkGroup",
		"dRepVotingThresholds.ppEconomicGroup",
		"dRepVotingThresholds.ppTechnicalGroup",
		"dRepVotingThresholds.ppGovGroup",
		"dRepVotingThresholds.treasuryWithdrawal",
		"committeeMinSize",
		"committeeMaxTermLength",
		"govActionLifetime",
		"govActionDeposit",
		"dRepDeposit",
		"dRepActivity",
		"minFeeRefScriptCostPerByte",
		"plutusV3CostModel",
		"constitution",
		"constitution.anchor",
		"constitution.anchor.url",
		"constitution.anchor.dataHash",
		"committee",
		"committee.members",
		"committee.threshold",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			err := conwayWith(t, path, nil)
			require.ErrorContains(t, err, "missing required field")
			parts := strings.Split(path, ".")
			require.ErrorContains(t, err, parts[len(parts)-1])
		})
	}
}

func TestConwayGenesisOptionalFieldsAbsent(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"constitution.script",
		"delegs",
		"initialDReps",
		"extraConfig",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, conwayWith(t, path, nil))
		})
	}
}

func TestConwayGenesisIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"bogusTopLevel",
		"poolVotingThresholds.bogusNested",
		"dRepVotingThresholds.bogusNested",
		"constitution.bogusNested",
		"constitution.anchor.bogusDeep",
		"committee.bogusNested",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, conwayWith(t, path, json.Number("1")))
		})
	}
}

type conwayBoundCase struct {
	name  string
	value any
	valid bool
}

func runConwayBoundCases(
	t *testing.T,
	path string,
	cases []conwayBoundCase,
) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := conwayWith(t, path, tc.value)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorContains(t, err, path)
		})
	}
}

func TestConwayGenesisThresholdsAreUnitIntervals(t *testing.T) {
	t.Parallel()
	paths := []string{
		"poolVotingThresholds.motionNoConfidence",
		"poolVotingThresholds.committeeNormal",
		"poolVotingThresholds.committeeNoConfidence",
		"poolVotingThresholds.hardForkInitiation",
		"poolVotingThresholds.ppSecurityGroup",
		"dRepVotingThresholds.motionNoConfidence",
		"dRepVotingThresholds.committeeNormal",
		"dRepVotingThresholds.committeeNoConfidence",
		"dRepVotingThresholds.updateToConstitution",
		"dRepVotingThresholds.hardForkInitiation",
		"dRepVotingThresholds.ppNetworkGroup",
		"dRepVotingThresholds.ppEconomicGroup",
		"dRepVotingThresholds.ppTechnicalGroup",
		"dRepVotingThresholds.ppGovGroup",
		"dRepVotingThresholds.treasuryWithdrawal",
		"committee.threshold",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			runConwayBoundCases(t, path, []conwayBoundCase{
				{"zero", json.Number("0"), true},
				{"one", json.Number("1"), true},
				{"half", json.Number("0.5"), true},
				{
					"half object",
					map[string]any{
						"numerator":   json.Number("1"),
						"denominator": json.Number("2"),
					},
					true,
				},
				{"five", json.Number("5"), false},
				{"seven", json.Number("7"), false},
				{"above one", json.Number("1.01"), false},
				{"negative", json.Number("-0.1"), false},
			})
		})
	}
}

func TestConwayGenesisMinFeeRefScriptCostPerByte(t *testing.T) {
	t.Parallel()
	runConwayBoundCases(
		t,
		"minFeeRefScriptCostPerByte",
		[]conwayBoundCase{
			{"negative", json.Number("-3"), false},
			{"zero", json.Number("0"), true},
			{"fifteen", json.Number("15"), true},
			{"fractional", json.Number("15.5"), true},
			{"word64 max", json.Number("18446744073709551615"), true},
			{"above word64 max", json.Number("18446744073709551616"), false},
		},
	)
}

func TestConwayGenesisIntegerBounds(t *testing.T) {
	t.Parallel()
	word32 := []conwayBoundCase{
		{"zero", json.Number("0"), true},
		{"word32 max", json.Number("4294967295"), true},
		{"above word32 max", json.Number("4294967296"), false},
		{"negative", json.Number("-1"), false},
	}
	word16 := []conwayBoundCase{
		{"zero", json.Number("0"), true},
		{"word16 max", json.Number("65535"), true},
		{"above word16 max", json.Number("65536"), false},
		{"negative", json.Number("-1"), false},
	}
	word64 := []conwayBoundCase{
		{"zero", json.Number("0"), true},
		{"word64 max", json.Number("18446744073709551615"), true},
		{"above word64 max", json.Number("18446744073709551616"), false},
		{"negative", json.Number("-1"), false},
	}
	for path, cases := range map[string][]conwayBoundCase{
		"committeeMinSize":       word16,
		"committeeMaxTermLength": word32,
		"govActionLifetime":      word32,
		"dRepActivity":           word32,
		"govActionDeposit":       word64,
		"dRepDeposit":            word64,
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			runConwayBoundCases(t, path, cases)
		})
	}
}

func TestConwayGenesisCommitteeMemberExpiryIsNonNegative(t *testing.T) {
	t.Parallel()
	doc := loadConwayDoc(t, "testdata/mainnet-conway-genesis.json")
	members, ok := doc["committee"].(map[string]any)["members"].(map[string]any)
	require.True(t, ok)
	var member string
	for key := range members {
		member = key
		break
	}
	require.NotEmpty(t, member)
	for name, tc := range map[string]struct {
		value json.Number
		valid bool
	}{
		"zero":     {"0", true},
		"typical":  {"580", true},
		"negative": {"-1", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := conwayWith(t, "committee.members."+member, tc.value)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "committee.members")
		})
	}
}
